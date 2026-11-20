// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package oci

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/objectstorage"

	"github.com/opentofu/opentofu/internal/states/statemgr"
)

// These tests model OCI's conditional object operations; they do not establish
// live service behavior, IAM correctness, or credentials accepted by OCI.
type mockOCIError struct {
	status int
	code   string
}

func (e mockOCIError) Error() string           { return "sensitive-service-message TOKEN PRIVATE_KEY" }
func (e mockOCIError) GetHTTPStatusCode() int  { return e.status }
func (e mockOCIError) GetCode() string         { return e.code }
func (e mockOCIError) GetMessage() string      { return e.Error() }
func (e mockOCIError) GetOpcRequestID() string { return "sensitive-request-id" }

type mutationFault struct {
	err       error
	committed bool
	replace   bool
}

type mockObjectStore struct {
	mu             sync.Mutex
	objects        map[string]storedObject
	serial         int
	gets           int
	puts           []objectstorage.PutObjectRequest
	deletes        []objectstorage.DeleteObjectRequest
	getErrors      []error
	putFaults      []mutationFault
	deleteFaults   []mutationFault
	missingPutETag bool
	badChecksum    bool
	beforeDelete   func(*mockObjectStore, string)
	afterPut       func(*mockObjectStore, string)
}

func newMockObjectStore() *mockObjectStore {
	return &mockObjectStore{objects: make(map[string]storedObject)}
}

func testRemoteClient(store objectStorageClient) *remoteClient {
	c := newRemoteClient(store, "namespace", "bucket", "terraform.tfstate", true, "")
	c.retryWait = func(context.Context, int) error { return nil }
	return c
}

func (s *mockObjectStore) set(name string, data []byte, token string) {
	s.serial++
	s.objects[name] = storedObject{exists: true, etag: fmt.Sprintf("etag-%d", s.serial), data: bytes.Clone(data), token: token}
}

func (s *mockObjectStore) GetObject(ctx context.Context, req objectstorage.GetObjectRequest) (objectstorage.GetObjectResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gets++
	if err := ctx.Err(); err != nil {
		return objectstorage.GetObjectResponse{}, err
	}
	if len(s.getErrors) > 0 {
		err := s.getErrors[0]
		s.getErrors = s.getErrors[1:]
		if err != nil {
			return objectstorage.GetObjectResponse{}, err
		}
	}
	obj, ok := s.objects[*req.ObjectName]
	if !ok {
		return objectstorage.GetObjectResponse{}, mockOCIError{404, "ObjectNotFound"}
	}
	digest := md5.Sum(obj.data)
	checksum := base64.StdEncoding.EncodeToString(digest[:])
	if s.badChecksum {
		checksum = "wrong"
	}
	return objectstorage.GetObjectResponse{
		Content: io.NopCloser(bytes.NewReader(bytes.Clone(obj.data))), ETag: common.String(obj.etag),
		ContentMd5: common.String(checksum), OpcMeta: map[string]string{writeTokenMetadata: obj.token},
	}, nil
}

func (s *mockObjectStore) PutObject(ctx context.Context, req objectstorage.PutObjectRequest) (objectstorage.PutObjectResponse, error) {
	data, err := io.ReadAll(req.PutObjectBody)
	if err != nil {
		return objectstorage.PutObjectResponse{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.puts = append(s.puts, req)
	if err := ctx.Err(); err != nil {
		return objectstorage.PutObjectResponse{}, err
	}
	if req.RequestMetadata.RetryPolicy == nil || req.RequestMetadata.RetryPolicy.MaximumNumberAttempts != 1 {
		return objectstorage.PutObjectResponse{}, errors.New("SDK retries must be disabled")
	}
	name := *req.ObjectName
	current := s.objects[name]
	if req.IfMatch == nil && (req.IfNoneMatch == nil || *req.IfNoneMatch != "*") {
		return objectstorage.PutObjectResponse{}, errors.New("unconditional write rejected by test")
	}
	if req.IfMatch != nil && (!current.exists || current.etag != *req.IfMatch) || req.IfNoneMatch != nil && current.exists {
		return objectstorage.PutObjectResponse{}, mockOCIError{412, "PreconditionFailed"}
	}
	digest := md5.Sum(data)
	if req.ContentMD5 == nil || *req.ContentMD5 != base64.StdEncoding.EncodeToString(digest[:]) || req.ContentLength == nil || *req.ContentLength != int64(len(data)) {
		return objectstorage.PutObjectResponse{}, errors.New("invalid content integrity headers")
	}
	var fault mutationFault
	if len(s.putFaults) > 0 {
		fault, s.putFaults = s.putFaults[0], s.putFaults[1:]
	}
	if fault.err != nil && !fault.committed {
		return objectstorage.PutObjectResponse{}, fault.err
	}
	s.set(name, data, req.OpcMeta[writeTokenMetadata])
	etag := s.objects[name].etag
	if fault.replace {
		s.set(name, []byte("replacement state"), "other-writer")
	}
	if s.afterPut != nil {
		s.afterPut(s, name)
	}
	if fault.err != nil {
		return objectstorage.PutObjectResponse{}, fault.err
	}
	if s.missingPutETag {
		return objectstorage.PutObjectResponse{}, nil
	}
	return objectstorage.PutObjectResponse{ETag: common.String(etag)}, nil
}

func (s *mockObjectStore) DeleteObject(ctx context.Context, req objectstorage.DeleteObjectRequest) (objectstorage.DeleteObjectResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deletes = append(s.deletes, req)
	if err := ctx.Err(); err != nil {
		return objectstorage.DeleteObjectResponse{}, err
	}
	name := *req.ObjectName
	if s.beforeDelete != nil {
		s.beforeDelete(s, name)
	}
	obj := s.objects[name]
	if !obj.exists {
		return objectstorage.DeleteObjectResponse{}, mockOCIError{404, "ObjectNotFound"}
	}
	if req.IfMatch == nil || obj.etag != *req.IfMatch {
		return objectstorage.DeleteObjectResponse{}, mockOCIError{412, "PreconditionFailed"}
	}
	var fault mutationFault
	if len(s.deleteFaults) > 0 {
		fault, s.deleteFaults = s.deleteFaults[0], s.deleteFaults[1:]
	}
	if fault.err != nil && !fault.committed {
		return objectstorage.DeleteObjectResponse{}, fault.err
	}
	delete(s.objects, name)
	if fault.replace {
		s.set(name, []byte("replacement state"), "other-writer")
	}
	return objectstorage.DeleteObjectResponse{}, fault.err
}

func (s *mockObjectStore) ListObjects(context.Context, objectstorage.ListObjectsRequest) (objectstorage.ListObjectsResponse, error) {
	return objectstorage.ListObjectsResponse{}, nil
}

func TestRemoteClientStateLifecycle(t *testing.T) {
	ctx := context.Background()
	store := newMockObjectStore()
	c := testRemoteClient(store)
	if p, err := c.Get(ctx); err != nil || p != nil {
		t.Fatalf("missing state: %v %v", p, err)
	}
	data := []byte(`{"version":4,"serial":1,"sensitive":"retain exactly"}`)
	if err := c.Put(ctx, data); err != nil {
		t.Fatal(err)
	}
	p, err := c.Get(ctx)
	digest := md5.Sum(data)
	if err != nil || !bytes.Equal(p.Data, data) || !bytes.Equal(p.MD5, digest[:]) {
		t.Fatalf("payload mismatch: %v %v", p, err)
	}
	if err := c.Put(ctx, []byte("updated")); err != nil {
		t.Fatal(err)
	}
	if store.puts[0].IfNoneMatch == nil || store.puts[1].IfMatch == nil {
		t.Fatal("state writes lacked creation/update preconditions")
	}
	if err := c.Delete(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(ctx); err != nil {
		t.Fatal(err)
	}
	if p, err := c.Get(ctx); err != nil || p != nil {
		t.Fatalf("deleted state: %v %v", p, err)
	}
	if len(store.deletes) != 1 || store.deletes[0].IfMatch == nil {
		t.Fatal("delete must be conditional and missing delete a no-op")
	}
}

func TestRemoteClientConcurrentStateCreation(t *testing.T) {
	store := newMockObjectStore()
	clients := []*remoteClient{testRemoteClient(store), testRemoteClient(store)}
	for _, c := range clients {
		if _, err := c.Get(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for i, c := range clients {
		go func(i int, c *remoteClient) {
			<-start
			results <- c.Put(context.Background(), []byte(fmt.Sprintf("writer-%d", i)))
		}(i, c)
	}
	close(start)
	first, second := <-results, <-results
	if (first == nil) == (second == nil) {
		t.Fatalf("expected exactly one winning writer: %v, %v", first, second)
	}
	if first != nil && !errors.Is(first, errObjectChanged) || second != nil && !errors.Is(second, errObjectChanged) {
		t.Fatalf("unexpected race error: %v, %v", first, second)
	}
}

func TestRemoteClientStaleStateCannotOverwriteOrDelete(t *testing.T) {
	store := newMockObjectStore()
	store.set("terraform.tfstate", []byte("initial"), "")
	a, b := testRemoteClient(store), testRemoteClient(store)
	for _, c := range []*remoteClient{a, b} {
		if _, err := c.Get(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.Put(context.Background(), []byte("new")); err != nil {
		t.Fatal(err)
	}
	if err := b.Put(context.Background(), []byte("stale")); !errors.Is(err, errObjectChanged) {
		t.Fatalf("stale write: %v", err)
	}
	if err := b.Delete(context.Background()); !errors.Is(err, errObjectChanged) {
		t.Fatalf("stale delete: %v", err)
	}
	if string(store.objects[b.stateFilePath].data) != "new" {
		t.Fatal("a stale client modified state")
	}
	if _, err := b.Get(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := b.Put(context.Background(), []byte("refreshed")); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteClientAbsentBaselineDoesNotDeleteNewState(t *testing.T) {
	store := newMockObjectStore()
	c := testRemoteClient(store)
	if _, err := c.Get(context.Background()); err != nil {
		t.Fatal(err)
	}
	store.set(c.stateFilePath, []byte("another writer"), "")
	if err := c.Delete(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.deletes) != 0 || !store.objects[c.stateFilePath].exists {
		t.Fatal("deleted a state created since absent baseline")
	}
}

func TestRemoteClientPutReconcilesAmbiguousOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name        string
		fault       mutationFault
		missingETag bool
		wantPuts    int
	}{
		{"retry-before-commit", mutationFault{err: mockOCIError{503, "Unavailable"}}, false, 2},
		{"response-lost-after-commit", mutationFault{err: io.EOF, committed: true}, false, 1},
		{"missing-response-etag", mutationFault{}, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newMockObjectStore()
			store.putFaults = []mutationFault{tc.fault}
			store.missingPutETag = tc.missingETag
			c := testRemoteClient(store)
			if err := c.Put(context.Background(), []byte("state")); err != nil {
				t.Fatal(err)
			}
			if len(store.puts) != tc.wantPuts || c.stateETag == "" {
				t.Fatalf("puts %d, ETag %q", len(store.puts), c.stateETag)
			}
		})
	}
}

func TestRemoteClientAmbiguousPutNeverOverwritesReplacement(t *testing.T) {
	store := newMockObjectStore()
	store.putFaults = []mutationFault{{err: io.EOF, committed: true, replace: true}}
	c := testRemoteClient(store)
	if err := c.Put(context.Background(), []byte("state")); !errors.Is(err, errObjectChanged) {
		t.Fatalf("want conflict, got %v", err)
	}
	if len(store.puts) != 1 || string(store.objects[c.stateFilePath].data) != "replacement state" {
		t.Fatal("reconciliation overwrote another writer")
	}
}

func TestRemoteClientUnconfirmedPutIsActionableAndSanitized(t *testing.T) {
	store := newMockObjectStore()
	store.putFaults = []mutationFault{{err: io.EOF, committed: true}}
	store.getErrors = []error{nil, mockOCIError{401, "NotAuthenticated"}}
	c := testRemoteClient(store)
	err := c.Put(context.Background(), []byte("state"))
	if err == nil || !strings.Contains(err.Error(), "outcome could not be confirmed") || strings.Contains(err.Error(), "TOKEN") {
		t.Fatalf("unconfirmed outcome: %v", err)
	}
	if len(store.puts) != 1 {
		t.Fatal("unconfirmed write retried unsafely")
	}
}

func TestRemoteClientDeleteReconciliation(t *testing.T) {
	for _, tc := range []struct {
		name        string
		fault       mutationFault
		wantDeletes int
		conflict    bool
	}{
		{"retry-before-commit", mutationFault{err: mockOCIError{503, "Unavailable"}}, 2, false},
		{"lost-response-after-commit", mutationFault{err: io.EOF, committed: true}, 1, false},
		{"replacement-after-commit", mutationFault{err: io.EOF, committed: true, replace: true}, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newMockObjectStore()
			store.set("terraform.tfstate", []byte("initial"), "")
			store.deleteFaults = []mutationFault{tc.fault}
			c := testRemoteClient(store)
			err := c.Delete(context.Background())
			if tc.conflict && !errors.Is(err, errObjectChanged) || !tc.conflict && err != nil {
				t.Fatalf("delete outcome: %v", err)
			}
			if len(store.deletes) != tc.wantDeletes {
				t.Fatalf("deletes: %d", len(store.deletes))
			}
			if tc.conflict && !store.objects[c.stateFilePath].exists {
				t.Fatal("replacement was deleted")
			}
		})
	}
}

func TestRemoteClientReadRetriesAndCredentialErrors(t *testing.T) {
	for _, tc := range []struct {
		name      string
		failures  []error
		wantCalls int
		wantError string
	}{
		{"transient", []error{mockOCIError{503, "Unavailable"}, io.ErrUnexpectedEOF}, 3, ""},
		{"expired", []error{mockOCIError{401, "NotAuthenticated"}}, 1, "refresh expired security tokens"},
		{"denied", []error{mockOCIError{403, "NotAuthorized"}}, 1, "IAM"},
		{"bucket-missing", []error{mockOCIError{404, "BucketNotFound"}}, 1, "bucket"},
		{"ambiguous-404", []error{mockOCIError{404, "NotAuthorizedOrNotFound"}}, 1, "IAM"},
		{"bounded", []error{mockOCIError{500, "Internal"}, mockOCIError{500, "Internal"}, mockOCIError{500, "Internal"}, mockOCIError{500, "Internal"}}, 4, "HTTP 500"},
		{"local-error", []error{errors.New("token=PRIVATE_KEY")}, 1, "credentials"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newMockObjectStore()
			store.set("terraform.tfstate", []byte("state"), "")
			store.getErrors = tc.failures
			p, err := testRemoteClient(store).Get(context.Background())
			if tc.wantError == "" {
				if err != nil || p == nil {
					t.Fatalf("payload %v error %v", p, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("want %q: %v", tc.wantError, err)
			}
			if err != nil && (strings.Contains(err.Error(), "TOKEN") || strings.Contains(err.Error(), "PRIVATE_KEY") || strings.Contains(err.Error(), "sensitive")) {
				t.Fatalf("secret leaked: %v", err)
			}
			if store.gets != tc.wantCalls {
				t.Fatalf("calls %d, want %d", store.gets, tc.wantCalls)
			}
		})
	}
}

func TestRemoteClientRejectsCorruptPayload(t *testing.T) {
	store := newMockObjectStore()
	store.set("terraform.tfstate", []byte("state"), "")
	store.badChecksum = true
	if _, err := testRemoteClient(store).Get(context.Background()); err == nil {
		t.Fatal("checksum mismatch accepted")
	}
}

func TestRemoteClientLockLifecycleAndConflict(t *testing.T) {
	store := newMockObjectStore()
	a, b := testRemoteClient(store), testRemoteClient(store)
	info := &statemgr.LockInfo{ID: "holder", Operation: "apply"}
	id, err := a.Lock(context.Background(), info)
	if err != nil || id != "holder" {
		t.Fatalf("lock: %s %v", id, err)
	}
	if _, err := b.Lock(context.Background(), &statemgr.LockInfo{ID: "contender"}); err == nil {
		t.Fatal("second lock succeeded")
	} else {
		var le *statemgr.LockError
		if !errors.As(err, &le) || le.Info == nil || le.Info.ID != id || !le.Retriable() {
			t.Fatalf("conflict missing owner: %v", err)
		}
	}
	if err := b.Unlock(context.Background(), "wrong"); err == nil {
		t.Fatal("wrong ID released lock")
	}
	if len(store.deletes) != 0 {
		t.Fatal("wrong ID attempted deletion")
	}
	if err := a.Put(context.Background(), []byte("state")); err != nil {
		t.Fatal(err)
	}
	if err := a.Unlock(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Lock(context.Background(), &statemgr.LockInfo{ID: "contender"}); err != nil {
		t.Fatal(err)
	}
	if store.deletes[0].IfMatch == nil {
		t.Fatal("unlock lacked ETag")
	}
}

func TestRemoteClientLockRecoversLostResponse(t *testing.T) {
	store := newMockObjectStore()
	store.putFaults = []mutationFault{{err: io.EOF, committed: true}}
	c := testRemoteClient(store)
	id, err := c.Lock(context.Background(), &statemgr.LockInfo{ID: "holder"})
	if err != nil || id != "holder" || len(store.puts) != 1 {
		t.Fatalf("lock outcome %s %v; puts %d", id, err, len(store.puts))
	}
}

func TestRemoteClientStolenLockFencesState(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(fmt.Sprint(replace), func(t *testing.T) {
			store := newMockObjectStore()
			c := testRemoteClient(store)
			store.set(c.stateFilePath, []byte("original"), "")
			if _, err := c.Get(context.Background()); err != nil {
				t.Fatal(err)
			}
			if _, err := c.Lock(context.Background(), &statemgr.LockInfo{ID: "holder"}); err != nil {
				t.Fatal(err)
			}
			if replace {
				store.set(c.lockFilePath, (&statemgr.LockInfo{ID: "replacement"}).Marshal(), "")
			} else {
				delete(store.objects, c.lockFilePath)
			}
			if err := c.Put(context.Background(), []byte("stale")); err == nil {
				t.Fatal("stolen lock permitted write")
			}
			if err := c.Delete(context.Background()); err == nil {
				t.Fatal("stolen lock permitted deletion")
			}
			if string(store.objects[c.stateFilePath].data) != "original" || len(store.deletes) != 0 {
				t.Fatal("stolen holder modified state")
			}
		})
	}
}

func TestRemoteClientUnlockCannotDeleteReplacement(t *testing.T) {
	store := newMockObjectStore()
	c := testRemoteClient(store)
	if _, err := c.Lock(context.Background(), &statemgr.LockInfo{ID: "same-id"}); err != nil {
		t.Fatal(err)
	}
	store.set(c.lockFilePath, (&statemgr.LockInfo{ID: "same-id"}).Marshal(), "")
	if err := c.Unlock(context.Background(), "same-id"); err == nil || len(store.deletes) != 0 {
		t.Fatalf("replaced ETag accepted: %v", err)
	}

	force := testRemoteClient(store)
	store.beforeDelete = func(s *mockObjectStore, name string) {
		s.set(name, (&statemgr.LockInfo{ID: "new-holder"}).Marshal(), "")
		s.beforeDelete = nil
	}
	if err := force.Unlock(context.Background(), "same-id"); err == nil {
		t.Fatal("unlock raced replacement accepted")
	}
	var owner statemgr.LockInfo
	if err := json.Unmarshal(store.objects[c.lockFilePath].data, &owner); err != nil || owner.ID != "new-holder" {
		t.Fatal("replacement removed")
	}
	if err := force.Unlock(context.Background(), "new-holder"); err != nil {
		t.Fatalf("valid force unlock failed: %v", err)
	}
}

func TestRemoteClientMalformedLockFailsClosed(t *testing.T) {
	store := newMockObjectStore()
	c := testRemoteClient(store)
	store.set(c.lockFilePath, []byte("sensitive TOKEN"), "")
	err := c.Unlock(context.Background(), "holder")
	if err == nil || strings.Contains(err.Error(), "TOKEN") || len(store.deletes) != 0 {
		t.Fatalf("invalid lock handling: %v", err)
	}
}

func TestRemoteClientCanceledContext(t *testing.T) {
	store := newMockObjectStore()
	c := testRemoteClient(store)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Put(ctx, []byte("state")); err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("cancellation: %v", err)
	}
	if store.gets != 0 || len(store.puts) != 0 {
		t.Fatal("canceled operation sent a request")
	}
}

// This checks real OCI SDK serialization/signing against a local HTTP service.
// It remains a mocked transport test, not live OCI validation.
func TestRemoteClientOfficialSDKHTTPTransport(t *testing.T) {
	store := newMockObjectStore()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/n/namespace/b/bucket/o/")
		if !strings.HasPrefix(r.URL.Path, "/n/namespace/b/bucket/o/") || !strings.HasPrefix(r.Header.Get("Authorization"), "Signature ") {
			t.Errorf("unexpected SDK request path/signature: %s", r.URL.Path)
			w.WriteHeader(400)
			return
		}
		if r.Method == http.MethodPut {
			resp, err := store.PutObject(r.Context(), objectstorage.PutObjectRequest{
				ObjectName: common.String(name), PutObjectBody: r.Body, ContentLength: common.Int64(r.ContentLength), ContentMD5: common.String(r.Header.Get("Content-MD5")),
				IfMatch: optionalString(r.Header.Get("If-Match")), IfNoneMatch: optionalString(r.Header.Get("If-None-Match")),
				OpcMeta: map[string]string{writeTokenMetadata: r.Header.Get("opc-meta-" + writeTokenMetadata)}, RequestMetadata: common.RequestMetadata{RetryPolicy: func() *common.RetryPolicy { p := common.NoRetryPolicy(); return &p }()},
			})
			if err != nil {
				writeMockOCIError(w, err)
				return
			}
			w.Header().Set("ETag", *resp.ETag)
			w.WriteHeader(200)
			return
		}
		if r.Method == http.MethodGet {
			resp, err := store.GetObject(r.Context(), objectstorage.GetObjectRequest{ObjectName: common.String(name)})
			if err != nil {
				writeMockOCIError(w, err)
				return
			}
			defer resp.Content.Close()
			w.Header().Set("ETag", *resp.ETag)
			w.Header().Set("Content-MD5", *resp.ContentMd5)
			w.Header().Set("opc-meta-"+writeTokenMetadata, resp.OpcMeta[writeTokenMetadata])
			io.Copy(w, resp.Content)
			return
		}
		if r.Method == http.MethodDelete {
			_, err := store.DeleteObject(r.Context(), objectstorage.DeleteObjectRequest{ObjectName: common.String(name), IfMatch: optionalString(r.Header.Get("If-Match"))})
			if err != nil {
				writeMockOCIError(w, err)
				return
			}
			w.WriteHeader(204)
			return
		}
		w.WriteHeader(405)
	})
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	provider := common.NewRawConfigurationProvider("tenancy", "user", "us-ashburn-1", "fingerprint", keyPEM, nil)
	sdk, err := objectstorage.NewObjectStorageClientWithConfigurationProvider(provider)
	if err != nil {
		t.Fatal(err)
	}
	sdk.Host = "https://mock-oci.invalid"
	sdk.HTTPClient = &http.Client{Transport: mockSDKTransport{handler}}
	c := testRemoteClient(&sdk)
	if p, err := c.Get(context.Background()); err != nil || p != nil {
		t.Fatalf("SDK missing response: %v %v", p, err)
	}
	id, err := c.Lock(context.Background(), &statemgr.LockInfo{ID: "transport-holder"})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Put(context.Background(), []byte("transport state")); err != nil {
		t.Fatal(err)
	}
	if p, err := c.Get(context.Background()); err != nil || p == nil || string(p.Data) != "transport state" {
		t.Fatalf("SDK payload: %v %v", p, err)
	}
	if err := c.Delete(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Unlock(context.Background(), id); err != nil {
		t.Fatal(err)
	}
}

// PutObject currently uses the SDK's custom-signer call path rather than its
// refreshable wrapper. Exercise that official wrapper explicitly with the
// actual backend upload body, so rewinding remains safe if the SDK selects it.
func TestRemoteClientUploadBodySupportsSDKRefreshableIdentityRetry(t *testing.T) {
	store := newMockObjectStore()
	data := []byte(`{"sensitive":"STATE_SECRET"}`)
	if err := testRemoteClient(store).Put(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	request := store.puts[0]
	body, binary := request.BinaryRequestBody()
	if !binary || body == nil || !body.Seekable() {
		t.Fatal("backend upload body lost seeking required for credential refresh retries")
	}
	if _, err := body.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	httpRequest, err := request.HTTPRequest(http.MethodPut, "/n/{namespaceName}/b/{bucketName}/o/{objectName}", body, nil)
	if err != nil {
		t.Fatal("SDK could not construct backend upload request")
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	provider := &mockRefreshableKeyProvider{key: key}
	var bodies [][]byte
	var lengths []int64
	var logs bytes.Buffer
	oldOutput := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(oldOutput)
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		contents, err := io.ReadAll(req.Body)
		if err != nil {
			t.Error("could not read retried request")
		}
		bodies = append(bodies, contents)
		lengths = append(lengths, req.ContentLength)
		if len(bodies) == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"code":"NotAuthenticated","message":"TOKEN_SECRET"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	client := common.BaseClient{
		Host: "https://mock-oci.invalid", UserAgent: "OpenTofu-test",
		Signer: common.DefaultRequestSigner(provider), HTTPClient: &http.Client{Transport: mockSDKTransport{handler}},
	}
	response, err := client.CallWithServiceAndOperationName(t.Context(), &httpRequest, "objectStorage", "PutObject")
	if err != nil || response == nil || response.StatusCode != http.StatusOK {
		t.Fatal("refreshable identity SDK retry failed")
	}
	defer response.Body.Close()
	if len(bodies) != 2 || !bytes.Equal(bodies[0], data) || !bytes.Equal(bodies[1], data) || lengths[0] != int64(len(data)) || lengths[1] != int64(len(data)) || provider.signatures != 2 {
		t.Fatal("SDK identity retry changed the body, length, or did not sign again")
	}
	if strings.Contains(logs.String(), "STATE_SECRET") || strings.Contains(logs.String(), "TOKEN_SECRET") || strings.Contains(logs.String(), "SIGNING_TOKEN_SECRET") {
		t.Fatal("SDK logging exposed sensitive body or credential material")
	}
}

type mockRefreshableKeyProvider struct {
	key        *rsa.PrivateKey
	signatures int
}

func (p *mockRefreshableKeyProvider) PrivateRSAKey() (*rsa.PrivateKey, error) { return p.key, nil }
func (p *mockRefreshableKeyProvider) KeyID() (string, error) {
	p.signatures++
	return fmt.Sprintf("ST$SIGNING_TOKEN_SECRET-%d", p.signatures), nil
}
func (*mockRefreshableKeyProvider) Refreshable() bool { return true }

// An in-memory RoundTripper exercises the SDK's actual HTTP implementation
// while remaining usable in environments that prohibit listening sockets.
type mockSDKTransport struct{ handler http.Handler }

func (m mockSDKTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response := httptest.NewRecorder()
	m.handler.ServeHTTP(response, request)
	result := response.Result()
	result.Request = request
	return result, nil
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return common.String(value)
}

func writeMockOCIError(w http.ResponseWriter, err error) {
	var serviceErr common.ServiceError
	if !errors.As(err, &serviceErr) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"code": "InternalError", "message": "test transport failure"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(serviceErr.GetHTTPStatusCode())
	json.NewEncoder(w).Encode(map[string]string{"code": serviceErr.GetCode(), "message": serviceErr.GetMessage()})
}
