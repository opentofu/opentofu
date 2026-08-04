// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package s3

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	awsbase "github.com/hashicorp/aws-sdk-go-base/v2"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

// routingHttpClient answers per HTTP method so a single test can make the
// conditional PUT fail while the follow-up GET still serves a lock object. The
// shared mockHttpClient returns one fixed response and cannot express that.
type routingHttpClient struct {
	byMethod map[string]*http.Response
	byTarget map[string]*http.Response
	seen     []string
}

func (m *routingHttpClient) Do(r *http.Request) (*http.Response, error) {
	target := r.Header.Get("X-Amz-Target")
	if target != "" {
		m.seen = append(m.seen, target)
		if resp, ok := m.byTarget[target]; ok {
			return resp, nil
		}
	}

	m.seen = append(m.seen, r.Method)
	if resp, ok := m.byMethod[r.Method]; ok {
		return resp, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}, nil
}

func newLockingClient(t *testing.T, httpCl *routingHttpClient) *RemoteClient {
	t.Helper()
	_, awsCfg, _ := awsbase.GetAwsConfig(context.Background(), &awsbase.Config{Region: "us-east-1", AccessKey: "test", SecretKey: "key"})
	s3Cl := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.HTTPClient = httpCl
	})
	return &RemoteClient{
		s3Client:    s3Cl,
		bucketName:  "test-bucket",
		path:        "state-file",
		useLockfile: true,
	}
}

func newDynamoLockingClient(t *testing.T, httpCl *routingHttpClient) *RemoteClient {
	t.Helper()
	_, awsCfg, _ := awsbase.GetAwsConfig(context.Background(), &awsbase.Config{Region: "us-east-1", AccessKey: "test", SecretKey: "key"})
	dynCl := dynamodb.NewFromConfig(awsCfg, func(options *dynamodb.Options) {
		options.HTTPClient = httpCl
	})
	return &RemoteClient{
		dynClient:  dynCl,
		bucketName: "test-bucket",
		path:       "state-file",
		ddbTable:   "test-locks",
	}
}

// A conditional PUT is not idempotent under retry: when the first attempt lands
// but its response is lost, the retry 412s against the object that attempt
// created. Acquisition must recognise its own lock rather than report it as a
// foreign holder, because a LockError leaves Lock() with no ID to release and
// the orphan then blocks every later operation on the state.
func TestS3Lock_412AgainstOwnLockIsAcquired(t *testing.T) {
	info := &statemgr.LockInfo{ID: "11111111-1111-1111-1111-111111111111", Info: "test"}

	httpCl := &routingHttpClient{byMethod: map[string]*http.Response{
		http.MethodPut: {
			StatusCode: http.StatusPreconditionFailed,
			Body:       io.NopCloser(strings.NewReader(`<Error><Code>PreconditionFailed</Code></Error>`)),
		},
		http.MethodGet: {
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(string(info.Marshal()))),
		},
	}}

	if err := newLockingClient(t, httpCl).Lock(t.Context(), info); err != nil {
		t.Fatalf("expected the acquisition to succeed against its own lock, got: %s", err)
	}
	if !strings.Contains(strings.Join(httpCl.seen, ","), "GET") {
		t.Errorf("expected the lock to be read back after the failed PUT, saw %v", httpCl.seen)
	}
}

// DynamoDB uses the same ownership predicate as S3 but reaches it through a
// different conditional-write and read-back protocol. This guards against one
// call site losing the recovery behavior while the other remains covered.
func TestDynamoDBLock_ConditionalFailureAgainstOwnLockIsAcquired(t *testing.T) {
	info := &statemgr.LockInfo{ID: "11111111-1111-1111-1111-111111111111", Info: "test"}
	getBody, err := json.Marshal(map[string]any{
		"Item": map[string]any{
			"LockID": map[string]string{"S": "test-bucket/state-file"},
			"Info":   map[string]string{"S": string(info.Marshal())},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	httpCl := &routingHttpClient{byTarget: map[string]*http.Response{
		"DynamoDB_20120810.PutItem": {
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": {"application/x-amz-json-1.0"}, "X-Amzn-Errortype": {"ConditionalCheckFailedException"}},
			Body:       io.NopCloser(strings.NewReader(`{"__type":"com.amazonaws.dynamodb.v20120810#ConditionalCheckFailedException","message":"The conditional request failed"}`)),
		},
		"DynamoDB_20120810.GetItem": {
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/x-amz-json-1.0"}},
			Body:       io.NopCloser(strings.NewReader(string(getBody))),
		},
	}}

	if err := newDynamoLockingClient(t, httpCl).Lock(t.Context(), info); err != nil {
		t.Fatalf("expected the acquisition to succeed against its own lock, got: %s", err)
	}
	if !strings.Contains(strings.Join(httpCl.seen, ","), "DynamoDB_20120810.GetItem") {
		t.Errorf("expected the lock to be read back after the failed PutItem, saw %v", httpCl.seen)
	}
}

// Negative control: a lock belonging to someone else must still fail, or the fix
// above would silently let two operations write the same state.
func TestS3Lock_412AgainstForeignLockStillFails(t *testing.T) {
	held := &statemgr.LockInfo{ID: "22222222-2222-2222-2222-222222222222", Who: "someone@elsewhere"}
	mine := &statemgr.LockInfo{ID: "11111111-1111-1111-1111-111111111111", Info: "test"}

	httpCl := &routingHttpClient{byMethod: map[string]*http.Response{
		http.MethodPut: {
			StatusCode: http.StatusPreconditionFailed,
			Body:       io.NopCloser(strings.NewReader(`<Error><Code>PreconditionFailed</Code></Error>`)),
		},
		http.MethodGet: {
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(string(held.Marshal()))),
		},
	}}

	err := newLockingClient(t, httpCl).Lock(t.Context(), mine)
	if err == nil {
		t.Fatal("expected a foreign lock to block acquisition")
	}
	lockErr, ok := err.(*statemgr.LockError)
	if !ok {
		t.Fatalf("expected a *statemgr.LockError, got %T: %s", err, err)
	}
	if lockErr.Info == nil || lockErr.Info.ID != held.ID {
		t.Errorf("expected the reported holder to be the foreign lock %q, got %#v", held.ID, lockErr.Info)
	}
}

// An unreadable lock object must not be mistaken for our own: lockInfo is nil on
// a read failure, and a nil-deref or a false ownership claim would both be worse
// than reporting the original error.
func TestS3Lock_412WithUnreadableLockStillFails(t *testing.T) {
	mine := &statemgr.LockInfo{ID: "11111111-1111-1111-1111-111111111111"}

	httpCl := &routingHttpClient{byMethod: map[string]*http.Response{
		http.MethodPut: {
			StatusCode: http.StatusPreconditionFailed,
			Body:       io.NopCloser(strings.NewReader(`<Error><Code>PreconditionFailed</Code></Error>`)),
		},
		http.MethodGet: {
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{not json`)),
		},
	}}

	if err := newLockingClient(t, httpCl).Lock(t.Context(), mine); err == nil {
		t.Fatal("expected an unreadable lock object to fail the acquisition")
	}
}
