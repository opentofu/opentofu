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
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	awsbase "github.com/hashicorp/aws-sdk-go-base/v2"
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

// TestS3ChecksumsHeaders is testing the compatibility with aws-sdk when it comes to the defaults baked inside the sdk
// related to checksums.
// This test was introduced during upgrading the version of the sdk including a breaking change that could
// impact the usage of the 3rd party s3 providers.
// More details in https://github.com/opentofu/opentofu/issues/2570.
func TestS3ChecksumsHeaders(t *testing.T) {
	// Configured the aws config the same way it is done for the backend to ensure a similar setup as the actual main logic.
	_, awsCfg, _ := awsbase.GetAwsConfig(context.Background(), &awsbase.Config{Region: "us-east-1", AccessKey: "test", SecretKey: "key"})
	httpCl := &mockHttpClient{resp: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}}
	s3Cl := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.HTTPClient = httpCl
	})

	tests := []struct {
		name         string
		action       func(cl *RemoteClient) error
		skipChecksum bool

		wantHeaders        map[string]string
		wantMissingHeaders []string
	}{
		{
			// When configured to calculate checksums, the aws-sdk should include the corresponding x-amz-checksum-* header for the selected
			// checksum algorithm
			name:         "s3.Put with included checksum",
			skipChecksum: false,
			action: func(cl *RemoteClient) error {
				return cl.Put([]byte("test"))
			},
			wantMissingHeaders: []string{
				"X-Amz-Checksum-Mode",
			},
			wantHeaders: map[string]string{
				"X-Amz-Checksum-Sha256":        "",
				"X-Amz-Sdk-Checksum-Algorithm": "SHA256",
				"X-Amz-Content-Sha256":         "UNSIGNED-PAYLOAD", // same with github.com/aws/aws-sdk-go-v2/aws/signer/internal/v4/const.go#UnsignedPayload
			},
		},
		{
			// When configured to skip computing checksums, the aws-sdk should not include any x-amz-checksum-* header in the request
			name:         "s3.Put with skipped checksum",
			skipChecksum: true,
			action: func(cl *RemoteClient) error {
				return cl.Put([]byte("test"))
			},
			wantMissingHeaders: []string{
				"X-Amz-Checksum-Mode",
				"X-Amz-Checksum-Sha256",
				"X-Amz-Sdk-Checksum-Algorithm",
			},
			wantHeaders: map[string]string{
				"X-Amz-Content-Sha256": "",
			},
		},
		{
			// When configured to calculate checksums, the aws-sdk should include the corresponding x-amz-checksum-* header for the selected
			// checksum algorithm
			name:         "s3.HeadObject and s3.GetObject with included checksum",
			skipChecksum: false,
			action: func(cl *RemoteClient) error {
				_, err := cl.Get()
				return err
			},
			wantMissingHeaders: []string{
				"X-Amz-Checksum-Sha256",
				"X-Amz-Sdk-Checksum-Algorithm",
			},
			wantHeaders: map[string]string{
				"X-Amz-Checksum-Mode":  string(types.ChecksumModeEnabled),
				"X-Amz-Content-Sha256": "",
			},
		},
		{
			// When configured to skip computing checksums, the aws-sdk should not include any x-amz-checksum-* header in the request
			name:         "s3.HeadObject and s3.GetObject with skipped checksum",
			skipChecksum: true,
			action: func(cl *RemoteClient) error {
				_, err := cl.Get()
				return err
			},
			wantMissingHeaders: []string{
				"X-Amz-Checksum-Mode",
				"X-Amz-Checksum-Sha256",
				"X-Amz-Sdk-Checksum-Algorithm",
			},
			wantHeaders: map[string]string{
				"X-Amz-Content-Sha256": "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := RemoteClient{
				s3Client:       s3Cl,
				bucketName:     "test-bucket",
				path:           "state-file",
				skipS3Checksum: tt.skipChecksum,
			}
			err := tt.action(&rc)
			if err != nil {
				t.Fatalf("expected to have no error but got one: %s", err)
			}
			if httpCl.receivedReq == nil {
				t.Fatal("request didn't reach the mock http client")
			}
			for wantHeader, wantHeaderVal := range tt.wantHeaders {
				got := httpCl.receivedReq.Header.Get(wantHeader)
				// for some headers, we cannot check the value since it's generated, so ensure that the header is present but do not check the value
				if wantHeaderVal == "" && got == "" {
					t.Errorf("missing header value for the %q header", wantHeader)
				} else if wantHeaderVal != "" && got != wantHeaderVal {
					t.Errorf("wrong header value for the %q header. expected %q; got: %q", wantHeader, wantHeaderVal, got)
				}
			}
			for _, wantHeader := range tt.wantMissingHeaders {
				got := httpCl.receivedReq.Header.Get(wantHeader)
				if got != "" {
					t.Errorf("expected missing %q header from the request. got: %q", wantHeader, got)
				}
			}
		})
	}
}

// TestS3LockingWritingHeaders is double-checking that the configuration for writing the lock object is the same
// with the state writing configuration
func TestS3LockingWritingHeaders(t *testing.T) {
	ignoredValues := []string{"Amz-Sdk-Invocation-Id", "Authorization", "X-Amz-Checksum-Sha256"}
	// Configured the aws config the same way it is done for the backend to ensure a similar setup as the actual main logic.
	_, awsCfg, _ := awsbase.GetAwsConfig(context.Background(), &awsbase.Config{Region: "us-east-1", AccessKey: "test", SecretKey: "key"})
	httpCl := &mockHttpClient{resp: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}}
	s3Cl := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.HTTPClient = httpCl
	})
	rc := RemoteClient{
		s3Client:    s3Cl,
		bucketName:  "test-bucket",
		path:        "state-file",
		useLockfile: true,
	}
	var (
		stateWritingReq, lockWritingReq *http.Request
	)
	// get the request from state writing
	{
		err := rc.Put([]byte("test"))
		if err != nil {
			t.Fatalf("expected to have no error writing the state object but got one: %s", err)
		}
		if httpCl.receivedReq == nil {
			t.Fatal("request didn't reach the mock http client")
		}
		stateWritingReq = httpCl.receivedReq
	}
	// get the request from lock object writing
	{
		err := rc.s3Lock(&statemgr.LockInfo{Info: "test"})
		if err != nil {
			t.Fatalf("expected to have no error writing the lock object but got one: %s", err)
		}
		if httpCl.receivedReq == nil {
			t.Fatal("request didn't reach the mock http client")
		}
		lockWritingReq = httpCl.receivedReq
	}

	// compare headers
	for k, v := range stateWritingReq.Header {
		got := lockWritingReq.Header.Values(k)
		if len(got) == 0 {
			t.Errorf("found a header that is missing from the request for locking: %s", k)
			continue
		}
		// do not compare header values that are meant to be different since are generated but ensure that there are values in those
		if slices.Contains(ignoredValues, k) {
			if len(got[0]) == 0 {
				t.Errorf("header %q from lock request is having an empty value: %#v", k, got)
			}
			continue
		}
		if !slices.Equal(got, v) {
			t.Errorf("found a header %q in lock request that is having a different value from the state one\nin-state-req: %#v\nin-lock-req: %#v", k, v, got)
		}
	}
}

// mockHttpClient is used to test the interaction of the s3 backend with the aws-sdk.
// This is meant to be configured with a response that will be returned to the aws-sdk.
// The receivedReq is going to contain the last request received by it.
type mockHttpClient struct {
	receivedReq *http.Request
	resp        *http.Response
}

func (m *mockHttpClient) Do(r *http.Request) (*http.Response, error) {
	m.receivedReq = r
	return m.resp, nil
}
