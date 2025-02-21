// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package s3

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statefile"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

func TestRemoteClient_impl(t *testing.T) {
	var _ remote.Client = new(RemoteClient)
	var _ remote.ClientLocker = new(RemoteClient)
}

func TestRemoteClient(t *testing.T) {
	testACC(t)
	bucketName := fmt.Sprintf("%s-%x", testBucketPrefix, time.Now().Unix())
	keyName := "testState"

	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"bucket":  bucketName,
		"key":     keyName,
		"encrypt": true,
	})).(*Backend)

	ctx := context.TODO()
	createS3Bucket(ctx, t, b.s3Client, bucketName, b.awsConfig.Region)
	defer deleteS3Bucket(ctx, t, b.s3Client, bucketName)

	state, err := b.StateMgr(backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}

	remote.TestClient(t, state.(*remote.State).Client)
}

func TestRemoteClientLocks(t *testing.T) {
	testACC(t)
	bucketName := fmt.Sprintf("%s-%x", testBucketPrefix, time.Now().Unix())
	keyName := "testState"

	b1 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"bucket":         bucketName,
		"key":            keyName,
		"encrypt":        true,
		"dynamodb_table": bucketName,
	})).(*Backend)

	b2 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"bucket":         bucketName,
		"key":            keyName,
		"encrypt":        true,
		"dynamodb_table": bucketName,
	})).(*Backend)

	ctx := context.TODO()
	createS3Bucket(ctx, t, b1.s3Client, bucketName, b1.awsConfig.Region)
	defer deleteS3Bucket(ctx, t, b1.s3Client, bucketName)
	createDynamoDBTable(ctx, t, b1.dynClient, bucketName)
	defer deleteDynamoDBTable(ctx, t, b1.dynClient, bucketName)

	s1, err := b1.StateMgr(backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}

	s2, err := b2.StateMgr(backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}

	remote.TestRemoteLocks(t, s1.(*remote.State).Client, s2.(*remote.State).Client)
}

func TestRemoteS3ClientLocks(t *testing.T) {
	testACC(t)
	bucketName := fmt.Sprintf("%s-%x", testBucketPrefix, time.Now().Unix())
	keyName := "testState"

	b1, _ := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"bucket":       bucketName,
		"key":          keyName,
		"encrypt":      true,
		"use_lockfile": true,
	})).(*Backend)

	b2, _ := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"bucket":       bucketName,
		"key":          keyName,
		"encrypt":      true,
		"use_lockfile": true,
	})).(*Backend)

	ctx := context.TODO()
	createS3Bucket(ctx, t, b1.s3Client, bucketName, b1.awsConfig.Region)
	defer deleteS3Bucket(ctx, t, b1.s3Client, bucketName)

	s1, err := b1.StateMgr(backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}

	s2, err := b2.StateMgr(backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}

	//nolint:errcheck // don't need to check the error from type assertion
	remote.TestRemoteLocks(t, s1.(*remote.State).Client, s2.(*remote.State).Client)
}

func TestRemoteS3AndDynamoDBClientLocks(t *testing.T) {
	testACC(t)
	bucketName := fmt.Sprintf("%s-%x", testBucketPrefix, time.Now().Unix())
	keyName := "testState"

	b1, _ := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"bucket":         bucketName,
		"key":            keyName,
		"dynamodb_table": bucketName,
		"encrypt":        true,
	})).(*Backend)

	b2, _ := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"bucket":         bucketName,
		"key":            keyName,
		"dynamodb_table": bucketName,
		"encrypt":        true,
		"use_lockfile":   true,
	})).(*Backend)

	ctx := context.TODO()
	createS3Bucket(ctx, t, b1.s3Client, bucketName, b1.awsConfig.Region)
	defer deleteS3Bucket(ctx, t, b1.s3Client, bucketName)
	createDynamoDBTable(ctx, t, b1.dynClient, bucketName)
	defer deleteDynamoDBTable(ctx, t, b1.dynClient, bucketName)

	s1, err := b1.StateMgr(backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}

	s2, err := b2.StateMgr(backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("dynamo lock goes first and s3+dynamo locks second", func(t *testing.T) {
		//nolint:errcheck // don't need to check the error from type assertion
		remote.TestRemoteLocks(t, s1.(*remote.State).Client, s2.(*remote.State).Client)
	})

	t.Run("s3+dynamo lock goes first and dynamo locks second", func(t *testing.T) {
		//nolint:errcheck // don't need to check the error from type assertion
		remote.TestRemoteLocks(t, s2.(*remote.State).Client, s1.(*remote.State).Client)
	})
}

func TestRemoteS3AndDynamoDBClientLocksWithNoDBInstance(t *testing.T) {
	testACC(t)
	bucketName := fmt.Sprintf("%s-%x", testBucketPrefix, time.Now().Unix())
	keyName := "testState"

	b1, _ := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"bucket":         bucketName,
		"key":            keyName,
		"dynamodb_table": bucketName,
		"encrypt":        true,
		"use_lockfile":   true,
	})).(*Backend)

	ctx := context.TODO()
	createS3Bucket(ctx, t, b1.s3Client, bucketName, b1.awsConfig.Region)
	defer deleteS3Bucket(ctx, t, b1.s3Client, bucketName)

	s1, err := b1.StateMgr(backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}

	infoA := statemgr.NewLockInfo()
	infoA.Operation = "test"
	infoA.Who = "clientA"

	if _, err := s1.Lock(infoA); err == nil {
		t.Fatal("unexpected successful lock: ", err)
	}

	expected := 0
	if actual := numberOfObjectsInBucket(t, ctx, b1.s3Client, bucketName); actual != expected {
		t.Fatalf("expected to have %d objects but got %d", expected, actual)
	}
}

// verify that we can unlock a state with an existing lock
func TestForceUnlock(t *testing.T) {
	testACC(t)
	bucketName := fmt.Sprintf("%s-force-%x", testBucketPrefix, time.Now().Unix())
	keyName := "testState"

	b1 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"bucket":         bucketName,
		"key":            keyName,
		"encrypt":        true,
		"dynamodb_table": bucketName,
	})).(*Backend)

	b2 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"bucket":         bucketName,
		"key":            keyName,
		"encrypt":        true,
		"dynamodb_table": bucketName,
	})).(*Backend)

	ctx := context.TODO()
	createS3Bucket(ctx, t, b1.s3Client, bucketName, b1.awsConfig.Region)
	defer deleteS3Bucket(ctx, t, b1.s3Client, bucketName)
	createDynamoDBTable(ctx, t, b1.dynClient, bucketName)
	defer deleteDynamoDBTable(ctx, t, b1.dynClient, bucketName)

	// first test with default
	s1, err := b1.StateMgr(backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}

	info := statemgr.NewLockInfo()
	info.Operation = "test"
	info.Who = "clientA"

	lockID, err := s1.Lock(info)
	if err != nil {
		t.Fatal("unable to get initial lock:", err)
	}

	// s1 is now locked, get the same state through s2 and unlock it
	s2, err := b2.StateMgr(backend.DefaultStateName)
	if err != nil {
		t.Fatal("failed to get default state to force unlock:", err)
	}

	if err := s2.Unlock(lockID); err != nil {
		t.Fatal("failed to force-unlock default state")
	}

	// now try the same thing with a named state
	// first test with default
	s1, err = b1.StateMgr("test")
	if err != nil {
		t.Fatal(err)
	}

	info = statemgr.NewLockInfo()
	info.Operation = "test"
	info.Who = "clientA"

	lockID, err = s1.Lock(info)
	if err != nil {
		t.Fatal("unable to get initial lock:", err)
	}

	// s1 is now locked, get the same state through s2 and unlock it
	s2, err = b2.StateMgr("test")
	if err != nil {
		t.Fatal("failed to get named state to force unlock:", err)
	}

	if err = s2.Unlock(lockID); err != nil {
		t.Fatal("failed to force-unlock named state")
	}

	// No State lock information found for the new workspace. The client should throw the appropriate error message.
	secondWorkspace := "new-workspace"
	s2, err = b2.StateMgr(secondWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	err = s2.Unlock(lockID)
	if err == nil {
		t.Fatal("expected an error to occur:", err)
	}
	expectedErrorMsg := fmt.Errorf("failed to retrieve lock info: no lock info found for: \"%s/env:/%s/%s\" within the DynamoDB table: %s", bucketName, secondWorkspace, keyName, bucketName)
	if err.Error() != expectedErrorMsg.Error() {
		t.Errorf("Unlock() error = %v, want: %v", err, expectedErrorMsg)
	}
}

// verify that we can unlock a state with an existing lock
func TestForceUnlockS3Only(t *testing.T) {
	testACC(t)
	bucketName := fmt.Sprintf("%s-force-s3-%x", testBucketPrefix, time.Now().Unix())
	keyName := "testState"

	b1, _ := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"bucket":       bucketName,
		"key":          keyName,
		"encrypt":      true,
		"use_lockfile": true,
	})).(*Backend)

	b2, _ := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"bucket":       bucketName,
		"key":          keyName,
		"encrypt":      true,
		"use_lockfile": true,
	})).(*Backend)

	ctx := context.TODO()
	createS3Bucket(ctx, t, b1.s3Client, bucketName, b1.awsConfig.Region)
	defer deleteS3Bucket(ctx, t, b1.s3Client, bucketName)

	// first test with default
	s1, err := b1.StateMgr(backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}

	info := statemgr.NewLockInfo()
	info.Operation = "test"
	info.Who = "clientA"

	lockID, err := s1.Lock(info)
	if err != nil {
		t.Fatal("unable to get initial lock:", err)
	}

	// s1 is now locked, get the same state through s2 and unlock it
	s2, err := b2.StateMgr(backend.DefaultStateName)
	if err != nil {
		t.Fatal("failed to get default state to force unlock:", err)
	}

	if err = s2.Unlock(lockID); err != nil {
		t.Fatal("failed to force-unlock default state")
	}

	// now try the same thing with a named state
	// first test with default
	s1, err = b1.StateMgr("test")
	if err != nil {
		t.Fatal(err)
	}

	info = statemgr.NewLockInfo()
	info.Operation = "test"
	info.Who = "clientA"

	lockID, err = s1.Lock(info)
	if err != nil {
		t.Fatal("unable to get initial lock:", err)
	}

	// s1 is now locked, get the same state through s2 and unlock it
	s2, err = b2.StateMgr("test")
	if err != nil {
		t.Fatal("failed to get named state to force unlock:", err)
	}

	if err = s2.Unlock(lockID); err != nil {
		t.Fatal("failed to force-unlock named state")
	}

	// No State lock information found for the new workspace. The client should throw the appropriate error message.
	secondWorkspace := "new-workspace"
	s2, err = b2.StateMgr(secondWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	err = s2.Unlock(lockID)
	if err == nil {
		t.Fatal("expected an error to occur:", err)
	}
	expectedErrorMsg := fmt.Errorf("failed to retrieve s3 lock info: operation error S3: GetObject, https response error StatusCode: 404")
	if !strings.HasPrefix(err.Error(), expectedErrorMsg.Error()) {
		t.Errorf("Unlock()\nactual = %v\nexpected = %v", err, expectedErrorMsg)
	}
}

// verify that we can unlock a state with an existing lock
func TestForceUnlockS3AndDynamo(t *testing.T) {
	testACC(t)
	bucketName := fmt.Sprintf("%s-force-s3-dynamo-%x", testBucketPrefix, time.Now().Unix())
	keyName := "testState"

	b1, _ := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"bucket":         bucketName,
		"key":            keyName,
		"encrypt":        true,
		"use_lockfile":   true,
		"dynamodb_table": bucketName,
	})).(*Backend)

	b2, _ := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"bucket":         bucketName,
		"key":            keyName,
		"encrypt":        true,
		"use_lockfile":   true,
		"dynamodb_table": bucketName,
	})).(*Backend)

	ctx := context.TODO()
	createS3Bucket(ctx, t, b1.s3Client, bucketName, b1.awsConfig.Region)
	defer deleteS3Bucket(ctx, t, b1.s3Client, bucketName)
	createDynamoDBTable(ctx, t, b1.dynClient, bucketName)
	defer deleteDynamoDBTable(ctx, t, b1.dynClient, bucketName)

	// first test with default
	s1, err := b1.StateMgr(backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}

	info := statemgr.NewLockInfo()
	info.Operation = "test"
	info.Who = "clientA"

	lockID, err := s1.Lock(info)
	if err != nil {
		t.Fatal("unable to get initial lock:", err)
	}

	// s1 is now locked, get the same state through s2 and unlock it
	s2, err := b2.StateMgr(backend.DefaultStateName)
	if err != nil {
		t.Fatal("failed to get default state to force unlock:", err)
	}

	if err = s2.Unlock(lockID); err != nil {
		t.Fatal("failed to force-unlock default state")
	}

	// now try the same thing with a named state
	// first test with default
	s1, err = b1.StateMgr("test")
	if err != nil {
		t.Fatal(err)
	}

	info = statemgr.NewLockInfo()
	info.Operation = "test"
	info.Who = "clientA"

	lockID, err = s1.Lock(info)
	if err != nil {
		t.Fatal("unable to get initial lock:", err)
	}

	// s1 is now locked, get the same state through s2 and unlock it
	s2, err = b2.StateMgr("test")
	if err != nil {
		t.Fatal("failed to get named state to force unlock:", err)
	}

	if err = s2.Unlock(lockID); err != nil {
		t.Fatal("failed to force-unlock named state")
	}

	// No State lock information found for the new workspace. The client should throw the appropriate error message.
	secondWorkspace := "new-workspace"
	s2, err = b2.StateMgr(secondWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	err = s2.Unlock(lockID)
	if err == nil {
		t.Fatal("expected an error to occur:", err)
	}
	expectedErrorMsg := []error{
		fmt.Errorf("failed to retrieve s3 lock info: operation error S3: GetObject, https response error StatusCode: 404"),
		fmt.Errorf("failed to retrieve lock info: no lock info found for: \"%s/env:/%s/%s\" within the DynamoDB table: %s", bucketName, secondWorkspace, keyName, bucketName),
	}
	for _, expectedErr := range expectedErrorMsg {
		if !strings.Contains(err.Error(), expectedErr.Error()) {
			t.Errorf("Unlock() should contain expected.\nactual = %v\nexpected = %v", err, expectedErr)
		}
	}
}

// verify the way it's handled the situation when the lock is in S3 but not in DynamoDB
func TestForceUnlockS3WithAndDynamoWithout(t *testing.T) {
	testACC(t)
	bucketName := fmt.Sprintf("%s-force-s3-dynamo-%x", testBucketPrefix, time.Now().Unix())
	keyName := "testState"

	b1, _ := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"bucket":         bucketName,
		"key":            keyName,
		"encrypt":        true,
		"use_lockfile":   true,
		"dynamodb_table": bucketName,
	})).(*Backend)

	ctx := context.TODO()
	createS3Bucket(ctx, t, b1.s3Client, bucketName, b1.awsConfig.Region)
	defer deleteS3Bucket(ctx, t, b1.s3Client, bucketName)
	createDynamoDBTable(ctx, t, b1.dynClient, bucketName)
	defer deleteDynamoDBTable(ctx, t, b1.dynClient, bucketName)

	// first create both locks: s3 and dynamo
	s1, err := b1.StateMgr(backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}

	info := statemgr.NewLockInfo()
	info.Operation = "test"
	info.Who = "clientA"

	lockID, err := s1.Lock(info)
	if err != nil {
		t.Fatal("unable to get initial lock:", err)
	}

	// Remove the dynamo lock to simulate that the lock in s3 was acquired, dynamo failed but s3 release failed in the end.
	// Therefore, the user is left in the situation with s3 lock existing and dynamo missing.
	deleteDynamoEntry(ctx, t, b1.dynClient, bucketName, info.Path)
	err = s1.Unlock(lockID)
	if err == nil {
		t.Fatal("expected to get an error but got nil")
	}
	expectedErrMsg := fmt.Sprintf("s3 lock released but dynamoDB failed: failed to retrieve lock info: no lock info found for: %q within the DynamoDB table: %s", info.Path, bucketName)
	if err.Error() != expectedErrMsg {
		t.Fatalf("unexpected error message.\nexpected: %s\nactual:%s", expectedErrMsg, err.Error())
	}

	// Now, unlocking should fail with error on both locks
	err = s1.Unlock(lockID)
	if err == nil {
		t.Fatal("expected to get an error but got nil")
	}
	expectedErrMsgs := []string{
		fmt.Sprintf("failed to retrieve lock info: no lock info found for: %q within the DynamoDB table: %s", info.Path, bucketName),
		"failed to retrieve s3 lock info: operation error S3: GetObject, https response error StatusCode: 404",
	}
	for _, expectedErrorMsg := range expectedErrMsgs {
		if !strings.Contains(err.Error(), expectedErrorMsg) {
			t.Fatalf("returned error does not contain the expected content.\nexpected: %s\nactual:%s", expectedErrorMsg, err.Error())
		}
	}
}

func TestRemoteClient_clientMD5(t *testing.T) {
	testACC(t)

	bucketName := fmt.Sprintf("%s-%x", testBucketPrefix, time.Now().Unix())
	keyName := "testState"

	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"bucket":         bucketName,
		"key":            keyName,
		"dynamodb_table": bucketName,
	})).(*Backend)

	ctx := context.TODO()
	createS3Bucket(ctx, t, b.s3Client, bucketName, b.awsConfig.Region)
	defer deleteS3Bucket(ctx, t, b.s3Client, bucketName)
	createDynamoDBTable(ctx, t, b.dynClient, bucketName)
	defer deleteDynamoDBTable(ctx, t, b.dynClient, bucketName)

	s, err := b.StateMgr(backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}
	client := s.(*remote.State).Client.(*RemoteClient)

	sum := md5.Sum([]byte("test"))

	if err := client.putMD5(ctx, sum[:]); err != nil {
		t.Fatal(err)
	}

	getSum, err := client.getMD5(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(getSum, sum[:]) {
		t.Fatalf("getMD5 returned the wrong checksum: expected %x, got %x", sum[:], getSum)
	}

	if err := client.deleteMD5(ctx); err != nil {
		t.Fatal(err)
	}

	if getSum, err := client.getMD5(ctx); err == nil {
		t.Fatalf("expected getMD5 error, got none. checksum: %x", getSum)
	}
}

// verify that a client won't return a state with an incorrect checksum.
func TestRemoteClient_stateChecksum(t *testing.T) {
	testACC(t)

	bucketName := fmt.Sprintf("%s-%x", testBucketPrefix, time.Now().Unix())
	keyName := "testState"

	b1 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"bucket":         bucketName,
		"key":            keyName,
		"dynamodb_table": bucketName,
	})).(*Backend)

	ctx := context.TODO()
	createS3Bucket(ctx, t, b1.s3Client, bucketName, b1.awsConfig.Region)
	defer deleteS3Bucket(ctx, t, b1.s3Client, bucketName)
	createDynamoDBTable(ctx, t, b1.dynClient, bucketName)
	defer deleteDynamoDBTable(ctx, t, b1.dynClient, bucketName)

	s1, err := b1.StateMgr(backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}
	client1 := s1.(*remote.State).Client

	// create an old and new state version to persist
	s := statemgr.TestFullInitialState()
	sf := &statefile.File{State: s}
	var oldState bytes.Buffer
	if err := statefile.Write(sf, &oldState, encryption.StateEncryptionDisabled()); err != nil {
		t.Fatal(err)
	}
	sf.Serial++
	var newState bytes.Buffer
	if err := statefile.Write(sf, &newState, encryption.StateEncryptionDisabled()); err != nil {
		t.Fatal(err)
	}

	// Use b2 without a dynamodb_table to bypass the lock table to write the state directly.
	// client2 will write the "incorrect" state, simulating s3 eventually consistency delays
	b2 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"bucket": bucketName,
		"key":    keyName,
	})).(*Backend)
	s2, err := b2.StateMgr(backend.DefaultStateName)
	if err != nil {
		t.Fatal(err)
	}
	client2 := s2.(*remote.State).Client

	// write the new state through client2 so that there is no checksum yet
	if err := client2.Put(newState.Bytes()); err != nil {
		t.Fatal(err)
	}

	// verify that we can pull a state without a checksum
	if _, err := client1.Get(); err != nil {
		t.Fatal(err)
	}

	// write the new state back with its checksum
	if err := client1.Put(newState.Bytes()); err != nil {
		t.Fatal(err)
	}

	// put an empty state in place to check for panics during get
	if err := client2.Put([]byte{}); err != nil {
		t.Fatal(err)
	}

	// remove the timeouts so we can fail immediately
	origTimeout := consistencyRetryTimeout
	origInterval := consistencyRetryPollInterval
	defer func() {
		consistencyRetryTimeout = origTimeout
		consistencyRetryPollInterval = origInterval
	}()
	consistencyRetryTimeout = 0
	consistencyRetryPollInterval = 0

	// fetching an empty state through client1 should now error out due to a
	// mismatched checksum.
	if _, err := client1.Get(); !strings.HasPrefix(err.Error(), errBadChecksumFmt[:80]) {
		t.Fatalf("expected state checksum error: got %s", err)
	}

	// put the old state in place of the new, without updating the checksum
	if err := client2.Put(oldState.Bytes()); err != nil {
		t.Fatal(err)
	}

	// fetching the wrong state through client1 should now error out due to a
	// mismatched checksum.
	if _, err := client1.Get(); !strings.HasPrefix(err.Error(), errBadChecksumFmt[:80]) {
		t.Fatalf("expected state checksum error: got %s", err)
	}

	// update the state with the correct one after we Get again
	testChecksumHook = func() {
		if err := client2.Put(newState.Bytes()); err != nil {
			t.Fatal(err)
		}
		testChecksumHook = nil
	}

	consistencyRetryTimeout = origTimeout

	// this final Get will fail to fail the checksum verification, the above
	// callback will update the state with the correct version, and Get should
	// retry automatically.
	if _, err := client1.Get(); err != nil {
		t.Fatal(err)
	}
}

// Tests the IsLockingEnabled method for the S3 remote client.
// It checks if locking is enabled based on the ddbTable field.
func TestRemoteClient_IsLockingEnabled(t *testing.T) {
	tests := []struct {
		name        string
		ddbTable    string
		useLockfile bool
		wantResult  bool
	}{
		{
			name:       "Locking enabled when ddbTable is set",
			ddbTable:   "my-lock-table",
			wantResult: true,
		},
		{
			name:       "Locking disabled when ddbTable is empty",
			ddbTable:   "",
			wantResult: false,
		},
		{
			name:        "Locking disabled when ddbTable is empty and useLockfile disabled",
			ddbTable:    "",
			useLockfile: false,
			wantResult:  false,
		},
		{
			name:        "Locking enabled when ddbTable is set or useLockfile enabled",
			ddbTable:    "my-lock-table",
			useLockfile: true,
			wantResult:  true,
		},
		{
			name:        "Locking enabled when ddbTable is empty and useLockfile enabled",
			ddbTable:    "",
			useLockfile: true,
			wantResult:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &RemoteClient{
				ddbTable:    tt.ddbTable,
				useLockfile: tt.useLockfile,
			}

			gotResult := client.IsLockingEnabled()
			if gotResult != tt.wantResult {
				t.Errorf("IsLockingEnabled() = %v; want %v", gotResult, tt.wantResult)
			}
		})
	}
}
