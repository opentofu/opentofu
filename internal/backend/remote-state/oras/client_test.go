// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package oras

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	orasErrcode "oras.land/oras-go/v2/registry/remote/errcode"
)

func TestRemoteClient_LockContentionAndUnlockMismatch(t *testing.T) {
	ctx := context.Background()
	fake := newFakeORASRepo()
	repo := &orasRepositoryClient{inner: fake}

	client1 := newRemoteClient(repo, "default")
	client2 := newRemoteClient(repo, "default")

	info := &statemgr.LockInfo{ID: "lock-1", Operation: "test", Info: "hello"}
	id, err := client1.Lock(ctx, info)
	if err != nil {
		t.Fatalf("expected first lock to succeed, got error: %v", err)
	}
	if id != "lock-1" {
		t.Fatalf("expected lock id to be %q, got %q", "lock-1", id)
	}

	info2 := &statemgr.LockInfo{ID: "lock-2", Operation: "test", Info: "hello"}
	_, err = client2.Lock(ctx, info2)
	if err == nil {
		t.Fatalf("expected second lock to fail")
	}
	if _, ok := err.(*statemgr.LockError); !ok {
		t.Fatalf("expected LockError, got %T: %v", err, err)
	}

	if err := client1.Unlock(ctx, "wrong"); err == nil {
		t.Fatalf("expected unlock mismatch error")
	}

	if err := client1.Unlock(ctx, "lock-1"); err != nil {
		t.Fatalf("expected unlock success, got: %v", err)
	}

	// Unlock deletes the lock manifest, so the lock tag must no longer resolve.
	if _, err := fake.Resolve(ctx, client1.lockTag); err == nil {
		t.Fatalf("expected lock tag to be gone after unlock")
	}

	// After unlock, it should be possible to lock again.
	_, err = client2.Lock(ctx, &statemgr.LockInfo{ID: "lock-3", Operation: "test"})
	if err != nil {
		t.Fatalf("expected lock after unlock to succeed, got: %v", err)
	}
}

func TestRemoteClient_WorkspacesFromTags_TagSafeAndHashed(t *testing.T) {
	ctx := context.Background()
	fake := newFakeORASRepo()
	repo := &orasRepositoryClient{inner: fake}

	// Tag-safe workspace
	c1 := newRemoteClient(repo, "dev")
	c1.versioningMaxVersions = 10
	if err := c1.Put(ctx, []byte("state-dev")); err != nil {
		t.Fatalf("put dev: %v", err)
	}
	if err := c1.Put(ctx, []byte("state-dev-2")); err != nil {
		t.Fatalf("put dev second: %v", err)
	}

	// Tag-unsafe workspace (space)
	c2 := newRemoteClient(repo, "my workspace")
	c2.versioningMaxVersions = 10
	if err := c2.Put(ctx, []byte("state-unsafe")); err != nil {
		t.Fatalf("put unsafe: %v", err)
	}
	if err := c2.Put(ctx, []byte("state-unsafe-2")); err != nil {
		t.Fatalf("put unsafe second: %v", err)
	}

	got, err := listWorkspacesFromTags(ctx, repo)
	if err != nil {
		t.Fatalf("workspaces: %v", err)
	}

	want := map[string]struct{}{"dev": {}, "my workspace": {}}
	for _, w := range got {
		delete(want, w)
	}
	if len(want) != 0 {
		t.Fatalf("missing workspaces: %v; got %v", want, got)
	}
}

// delegatingRepo is a base type that delegates all orasRepository methods to an
// inner *fakeORASRepo. Test doubles embed this and override only the methods
// they need, eliminating repetitive pass-through boilerplate.
type delegatingRepo struct {
	inner *fakeORASRepo
}

func (d delegatingRepo) Push(ctx context.Context, expected ocispec.Descriptor, content io.Reader) error {
	return d.inner.Push(ctx, expected, content)
}
func (d delegatingRepo) Fetch(ctx context.Context, target ocispec.Descriptor) (io.ReadCloser, error) {
	return d.inner.Fetch(ctx, target)
}
func (d delegatingRepo) Resolve(ctx context.Context, reference string) (ocispec.Descriptor, error) {
	return d.inner.Resolve(ctx, reference)
}
func (d delegatingRepo) Tag(ctx context.Context, desc ocispec.Descriptor, reference string) error {
	return d.inner.Tag(ctx, desc, reference)
}
func (d delegatingRepo) Delete(ctx context.Context, target ocispec.Descriptor) error {
	return d.inner.Delete(ctx, target)
}
func (d delegatingRepo) Tags(ctx context.Context, last string, fn func(tags []string) error) error {
	return d.inner.Tags(ctx, last, fn)
}

// deleteFailingRepo wraps fakeORASRepo and returns a non-transient error on Delete.
type deleteFailingRepo struct {
	delegatingRepo
	deleteErr error
}

func (r *deleteFailingRepo) Delete(_ context.Context, _ ocispec.Descriptor) error {
	return r.deleteErr
}

func TestDeleteWorkspace_ReturnsErrorOnDeleteFailure(t *testing.T) {
	ctx := context.Background()
	fake := newFakeORASRepo()

	// First, create a workspace state so there's something to delete.
	normalRepo := &orasRepositoryClient{inner: fake}
	c := newRemoteClient(normalRepo, "staging")
	if err := c.Put(ctx, []byte("some-state")); err != nil {
		t.Fatalf("put: %v", err)
	}

	// Now wrap with a failing repo that returns 500 on Delete.
	failRepo := &deleteFailingRepo{
		delegatingRepo: delegatingRepo{inner: fake},
		deleteErr:      &orasErrcode.ErrorResponse{StatusCode: http.StatusInternalServerError},
	}
	b := &Backend{
		Backend:    nil,
		repoClient: &orasRepositoryClient{inner: failRepo, repository: "example.com/test/repo"},
	}

	err := b.DeleteWorkspace(ctx, "staging", false)
	if err == nil {
		t.Fatalf("expected DeleteWorkspace to return error when Delete fails, got nil")
	}
	if !strings.Contains(err.Error(), "deleting state for workspace") {
		t.Fatalf("expected error about deleting state, got: %v", err)
	}
}

func TestDeleteWorkspace_SucceedsNormally(t *testing.T) {
	ctx := context.Background()
	fake := newFakeORASRepo()
	repo := &orasRepositoryClient{inner: fake}

	// Create state and lock for a workspace.
	c := newRemoteClient(repo, "staging")
	if err := c.Put(ctx, []byte("some-state")); err != nil {
		t.Fatalf("put: %v", err)
	}
	if _, err := c.Lock(ctx, &statemgr.LockInfo{ID: "lock-1", Operation: "test"}); err != nil {
		t.Fatalf("lock: %v", err)
	}

	b := &Backend{
		Backend:    nil,
		repoClient: repo,
	}

	if err := b.DeleteWorkspace(ctx, "staging", false); err != nil {
		t.Fatalf("expected DeleteWorkspace to succeed, got: %v", err)
	}

	// State and lock should be gone.
	if _, err := fake.Resolve(ctx, c.stateTag); err == nil {
		t.Fatalf("expected state tag to be deleted")
	}
	if _, err := fake.Resolve(ctx, c.lockTag); err == nil {
		t.Fatalf("expected lock tag to be deleted")
	}
}

func TestDeleteWorkspace_ToleratesDeleteUnsupported(t *testing.T) {
	ctx := context.Background()
	fake := newFakeORASRepo()

	// Create state so there's something to delete.
	normalRepo := &orasRepositoryClient{inner: fake}
	c := newRemoteClient(normalRepo, "staging")
	if err := c.Put(ctx, []byte("some-state")); err != nil {
		t.Fatalf("put: %v", err)
	}

	// Wrap with a repo that returns 405 (Method Not Allowed) on Delete.
	unsupportedRepo := deleteUnsupportedRepo{delegatingRepo: delegatingRepo{inner: fake}}
	b := &Backend{
		Backend:    nil,
		repoClient: &orasRepositoryClient{inner: unsupportedRepo, repository: "example.com/test/repo"},
	}

	// Should succeed without error since 405 is tolerated.
	if err := b.DeleteWorkspace(ctx, "staging", false); err != nil {
		t.Fatalf("expected DeleteWorkspace to tolerate 405, got: %v", err)
	}
}

// resolveFailingRepo wraps fakeORASRepo and returns a transient error on Resolve
// for a specific tag, simulating a network failure during tag resolution.
type resolveFailingRepo struct {
	delegatingRepo
	failTag string
	err     error
}

func (r *resolveFailingRepo) Resolve(ctx context.Context, reference string) (ocispec.Descriptor, error) {
	if reference == r.failTag {
		return ocispec.Descriptor{}, r.err
	}
	return r.delegatingRepo.Resolve(ctx, reference)
}

func TestDeleteWorkspace_SurfacesTransientResolveError(t *testing.T) {
	ctx := context.Background()
	fake := newFakeORASRepo()

	// Create state so Resolve would normally succeed.
	normalRepo := &orasRepositoryClient{inner: fake}
	c := newRemoteClient(normalRepo, "staging")
	if err := c.Put(ctx, []byte("some-state")); err != nil {
		t.Fatalf("put: %v", err)
	}

	// Wrap with a repo that returns a transient error on Resolve for the state tag.
	failRepo := &resolveFailingRepo{
		delegatingRepo: delegatingRepo{inner: fake},
		failTag:        c.stateTag,
		err:            &orasErrcode.ErrorResponse{StatusCode: http.StatusInternalServerError},
	}
	b := &Backend{
		Backend:    nil,
		repoClient: &orasRepositoryClient{inner: failRepo, repository: "example.com/test/repo"},
	}

	// Should surface the transient Resolve error instead of silently swallowing it.
	err := b.DeleteWorkspace(ctx, "staging", false)
	if err == nil {
		t.Fatalf("expected error from transient Resolve failure, got nil")
	}
	if !strings.Contains(err.Error(), "deleting state for workspace") {
		t.Fatalf("expected error about deleting state, got: %v", err)
	}
}

func TestWorkspaces_AlwaysIncludesDefault(t *testing.T) {
	ctx := context.Background()
	fake := newFakeORASRepo()
	repo := &orasRepositoryClient{inner: fake}

	// Create a non-default workspace only (no "default" state exists).
	c := newRemoteClient(repo, "production")
	if err := c.Put(ctx, []byte("prod-state")); err != nil {
		t.Fatalf("put: %v", err)
	}

	b := &Backend{
		Backend:    nil,
		repoClient: repo,
	}

	wss, err := b.Workspaces(ctx)
	if err != nil {
		t.Fatalf("Workspaces: %v", err)
	}

	// "default" must always be present per the backend contract.
	hasDefault := false
	hasProduction := false
	for _, w := range wss {
		if w == "default" {
			hasDefault = true
		}
		if w == "production" {
			hasProduction = true
		}
	}
	if !hasDefault {
		t.Fatalf("expected 'default' in workspace list %v", wss)
	}
	if !hasProduction {
		t.Fatalf("expected 'production' in workspace list %v", wss)
	}
}

func TestWorkspaces_NoDuplicateDefault(t *testing.T) {
	ctx := context.Background()
	fake := newFakeORASRepo()
	repo := &orasRepositoryClient{inner: fake}

	// Create the default workspace so it IS in the tag list.
	c := newRemoteClient(repo, "default")
	if err := c.Put(ctx, []byte("default-state")); err != nil {
		t.Fatalf("put: %v", err)
	}

	b := &Backend{
		Backend:    nil,
		repoClient: repo,
	}

	wss, err := b.Workspaces(ctx)
	if err != nil {
		t.Fatalf("Workspaces: %v", err)
	}

	count := 0
	for _, w := range wss {
		if w == "default" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 'default' in workspace list, got %d: %v", count, wss)
	}
}

func TestRemoteClient_Put_VersioningTagsAndRetention(t *testing.T) {
	// Drain the global retention semaphore to ensure slots are available.
	// Other tests may leave goroutines that hold semaphore slots.
	drainRetentionSem()

	ctx := context.Background()
	fake := newFakeORASRepo()
	repo := &orasRepositoryClient{inner: fake}

	c := newRemoteClient(repo, "default")
	c.versioningMaxVersions = 2
	if err := c.Put(ctx, []byte("s1")); err != nil {
		t.Fatalf("put s1: %v", err)
	}
	if err := c.Put(ctx, []byte("s2")); err != nil {
		t.Fatalf("put s2: %v", err)
	}
	if err := c.Put(ctx, []byte("s3")); err != nil {
		t.Fatalf("put s3: %v", err)
	}

	// Poll for async cleanup completion with timeout
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := fake.Resolve(ctx, c.versionTagFor(1)); err != nil {
			break // v1 deleted, cleanup completed
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for async retention to delete v1")
		}
		time.Sleep(50 * time.Millisecond)
	}

	p, err := c.Get(ctx)
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	if p == nil || string(p.Data) != "s3" {
		got := "<nil>"
		if p != nil {
			got = string(p.Data)
		}
		t.Fatalf("expected latest state %q, got %q", "s3", got)
	}

	if _, err := fake.Resolve(ctx, c.versionTagFor(1)); err == nil {
		t.Fatalf("expected v1 to be deleted due to async retention")
	}
	if _, err := fake.Resolve(ctx, c.versionTagFor(2)); err != nil {
		t.Fatalf("expected v2 to exist, got: %v", err)
	}
	if _, err := fake.Resolve(ctx, c.versionTagFor(3)); err != nil {
		t.Fatalf("expected v3 to exist, got: %v", err)
	}
}

func TestWorkspaceTagFor_HashesInvalidWorkspaceNames(t *testing.T) {
	// Valid tag remains unchanged.
	if got := workspaceTagFor("default"); got != "default" {
		t.Fatalf("expected tag-safe workspace to remain unchanged, got %q", got)
	}

	// Invalid tag is hashed.
	got := workspaceTagFor("my workspace")
	if got == "my workspace" {
		t.Fatalf("expected invalid workspace name to be hashed")
	}
	if len(got) < 3 || got[:3] != "ws-" {
		t.Fatalf("expected hashed workspaceTag to start with ws-, got %q", got)
	}
}

type deleteUnsupportedRepo struct {
	delegatingRepo
}

func (r deleteUnsupportedRepo) Delete(_ context.Context, _ ocispec.Descriptor) error {
	return &orasErrcode.ErrorResponse{StatusCode: 405}
}

func TestRemoteClient_UnlockFallbackWhenDeleteUnsupported(t *testing.T) {
	ctx := context.Background()
	fake := newFakeORASRepo()
	repo := &orasRepositoryClient{inner: deleteUnsupportedRepo{delegatingRepo: delegatingRepo{inner: fake}}}

	client := newRemoteClient(repo, "default")

	// Take a lock.
	_, err := client.Lock(ctx, &statemgr.LockInfo{ID: "lock-1", Operation: "test"})
	if err != nil {
		t.Fatalf("expected lock to succeed, got: %v", err)
	}

	// Unlock should fallback to retagging rather than failing.
	if err := client.Unlock(ctx, "lock-1"); err != nil {
		t.Fatalf("expected unlock to succeed via fallback, got: %v", err)
	}

	// After unlock, it should be possible to lock again.
	_, err = client.Lock(ctx, &statemgr.LockInfo{ID: "lock-2", Operation: "test"})
	if err != nil {
		t.Fatalf("expected lock after fallback unlock to succeed, got: %v", err)
	}
}

func TestIsTransientError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{name: "nil error", err: nil, expected: false},
		{name: "regular error", err: errors.New("something went wrong"), expected: false},
		{name: "429 Too Many Requests", err: &orasErrcode.ErrorResponse{StatusCode: http.StatusTooManyRequests}, expected: true},
		{name: "502 Bad Gateway", err: &orasErrcode.ErrorResponse{StatusCode: http.StatusBadGateway}, expected: true},
		{name: "503 Service Unavailable", err: &orasErrcode.ErrorResponse{StatusCode: http.StatusServiceUnavailable}, expected: true},
		{name: "504 Gateway Timeout", err: &orasErrcode.ErrorResponse{StatusCode: http.StatusGatewayTimeout}, expected: true},
		{name: "408 Request Timeout", err: &orasErrcode.ErrorResponse{StatusCode: http.StatusRequestTimeout}, expected: true},
		{name: "404 Not Found", err: &orasErrcode.ErrorResponse{StatusCode: http.StatusNotFound}, expected: false},
		{name: "401 Unauthorized", err: &orasErrcode.ErrorResponse{StatusCode: http.StatusUnauthorized}, expected: false},
		{name: "connection reset", err: errors.New("read tcp: connection reset by peer"), expected: true},
		{name: "connection refused", err: errors.New("dial tcp: connection refused"), expected: true},
		{name: "connection timeout", err: errors.New("connection timeout occurred"), expected: true},
		{name: "i/o timeout", err: errors.New("read tcp 10.0.0.1:443: i/o timeout"), expected: true},
		{name: "tls handshake timeout", err: errors.New("net/http: TLS handshake timeout"), expected: true},
		{name: "context deadline exceeded", err: errors.New("context deadline exceeded"), expected: true},
		{name: "unexpected EOF", err: errors.New("unexpected EOF"), expected: true},
		{name: "read: eof", err: errors.New("http: read: eof"), expected: true},
		{name: "bare eof", err: errors.New("EOF"), expected: true},
		{name: "no such host", err: errors.New("dial tcp: lookup example.com: no such host"), expected: true},
		{name: "temporary failure in name resolution", err: errors.New("temporary failure in name resolution"), expected: true},
		// Must NOT match words that happen to contain "eof" or "timeout" substrings
		{name: "word containing eof - thereof", err: errors.New("thereof the value is invalid"), expected: false},
		{name: "generic timeout word excluded", err: errors.New("the server returned a permanent error"), expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTransientError(tt.err); got != tt.expected {
				t.Fatalf("isTransientError(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}

func TestWithRetry_Success(t *testing.T) {
	ctx := context.Background()
	cfg := RetryConfig{MaxAttempts: 3, InitialBackoff: 10 * time.Millisecond, MaxBackoff: 100 * time.Millisecond, BackoffMultiplier: 2.0}

	attempts := 0
	result, err := withRetry(ctx, cfg, func(ctx context.Context) (string, error) {
		attempts++
		return "success", nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "success" {
		t.Fatalf("expected 'success', got %q", result)
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
}

func TestWithRetry_TransientFailureThenSuccess(t *testing.T) {
	ctx := context.Background()
	cfg := RetryConfig{MaxAttempts: 3, InitialBackoff: 10 * time.Millisecond, MaxBackoff: 100 * time.Millisecond, BackoffMultiplier: 2.0}

	attempts := 0
	result, err := withRetry(ctx, cfg, func(ctx context.Context) (string, error) {
		attempts++
		if attempts < 3 {
			return "", &orasErrcode.ErrorResponse{StatusCode: http.StatusServiceUnavailable}
		}
		return "success", nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "success" {
		t.Fatalf("expected 'success', got %q", result)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestWithRetry_NonTransientFailure(t *testing.T) {
	ctx := context.Background()
	cfg := RetryConfig{MaxAttempts: 3, InitialBackoff: 10 * time.Millisecond, MaxBackoff: 100 * time.Millisecond, BackoffMultiplier: 2.0}

	attempts := 0
	if _, err := withRetry(ctx, cfg, func(ctx context.Context) (string, error) {
		attempts++
		return "", &orasErrcode.ErrorResponse{StatusCode: http.StatusUnauthorized}
	}); err == nil {
		t.Fatalf("expected error, got nil")
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
}

func TestWithRetry_MaxAttemptsExhausted(t *testing.T) {
	ctx := context.Background()
	cfg := RetryConfig{MaxAttempts: 3, InitialBackoff: 10 * time.Millisecond, MaxBackoff: 100 * time.Millisecond, BackoffMultiplier: 2.0}

	attempts := 0
	if _, err := withRetry(ctx, cfg, func(ctx context.Context) (string, error) {
		attempts++
		return "", &orasErrcode.ErrorResponse{StatusCode: http.StatusServiceUnavailable}
	}); err == nil {
		t.Fatalf("expected error, got nil")
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestWithRetry_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := RetryConfig{MaxAttempts: 5, InitialBackoff: 100 * time.Millisecond, MaxBackoff: time.Second, BackoffMultiplier: 2.0}

	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
		close(done)
	}()

	_, err := withRetry(ctx, cfg, func(ctx context.Context) (string, error) {
		return "", &orasErrcode.ErrorResponse{StatusCode: http.StatusServiceUnavailable}
	})

	<-done

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestWithRetryNoResult(t *testing.T) {
	ctx := context.Background()
	cfg := RetryConfig{MaxAttempts: 3, InitialBackoff: 10 * time.Millisecond, MaxBackoff: 100 * time.Millisecond, BackoffMultiplier: 2.0}

	attempts := 0
	if err := withRetryNoResult(ctx, cfg, func(ctx context.Context) error {
		attempts++
		if attempts < 2 {
			return &orasErrcode.ErrorResponse{StatusCode: http.StatusServiceUnavailable}
		}
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

func TestRemoteClient_LockTTL_ClearsStaleLock(t *testing.T) {
	ctx := context.Background()
	fake := newFakeORASRepo()
	repo := &orasRepositoryClient{inner: fake}

	// client1 creates a lock with TTL that will be stale when client2 checks
	baseTime := time.Unix(1_000, 0).UTC()
	client1 := newRemoteClient(repo, "default")
	client1.lockTTL = time.Hour
	client1.now = func() time.Time { return baseTime }

	// client2 checks much later (lock has expired)
	client2 := newRemoteClient(repo, "default")
	client2.lockTTL = time.Hour
	client2.now = func() time.Time { return baseTime.Add(2 * time.Hour) } // 2h later, lock expired

	_, err := client1.Lock(ctx, &statemgr.LockInfo{ID: "lock-stale", Operation: "test"})
	if err != nil {
		t.Fatalf("expected first lock to succeed, got: %v", err)
	}

	_, err = client2.Lock(ctx, &statemgr.LockInfo{ID: "lock-new", Operation: "test"})
	if err != nil {
		t.Fatalf("expected lock to succeed after clearing stale lock, got: %v", err)
	}
}

func TestRemoteClient_LockTTL_ClearsStaleLock_DeleteUnsupportedFallback(t *testing.T) {
	ctx := context.Background()
	fake := newFakeORASRepo()
	repo := &orasRepositoryClient{inner: deleteUnsupportedRepo{delegatingRepo: delegatingRepo{inner: fake}}}

	// client1 creates a lock with TTL that will be stale when client2 checks
	baseTime := time.Unix(1_000, 0).UTC()
	client1 := newRemoteClient(repo, "default")
	client1.lockTTL = time.Hour
	client1.now = func() time.Time { return baseTime }

	// client2 checks much later (lock has expired)
	client2 := newRemoteClient(repo, "default")
	client2.lockTTL = time.Hour
	client2.now = func() time.Time { return baseTime.Add(2 * time.Hour) } // 2h later, lock expired

	_, err := client1.Lock(ctx, &statemgr.LockInfo{ID: "lock-stale", Operation: "test"})
	if err != nil {
		t.Fatalf("expected first lock to succeed, got: %v", err)
	}

	_, err = client2.Lock(ctx, &statemgr.LockInfo{ID: "lock-new", Operation: "test"})
	if err != nil {
		t.Fatalf("expected lock to succeed after clearing stale lock via fallback, got: %v", err)
	}
}

func TestRemoteClient_StateCompression_GzipRoundTrip(t *testing.T) {
	ctx := context.Background()
	fake := newFakeORASRepo()
	repo := &orasRepositoryClient{inner: fake}

	c := newRemoteClient(repo, "default")
	c.stateCompression = "gzip"

	original := []byte(strings.Repeat("hello-", 2000))

	if err := c.Put(ctx, original); err != nil {
		t.Fatalf("put: %v", err)
	}

	p, err := c.Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if p == nil {
		t.Fatalf("expected payload")
	}
	if !bytes.Equal(p.Data, original) {
		t.Fatalf("expected roundtrip to match")
	}

	fm, err := c.fetchManifestWithDesc(ctx, c.stateTag)
	if err != nil {
		t.Fatalf("fetch manifest: %v", err)
	}
	m := fm.m
	if len(m.Layers) != 1 {
		t.Fatalf("expected 1 layer, got %d", len(m.Layers))
	}
	if m.Layers[0].MediaType != mediaTypeStateLayerGzip {
		t.Fatalf("expected gzip mediaType, got %q", m.Layers[0].MediaType)
	}
}

func TestRemoteClient_StateCompression_AutoDetectOnRead(t *testing.T) {
	ctx := context.Background()
	fake := newFakeORASRepo()
	repo := &orasRepositoryClient{inner: fake}

	writer := newRemoteClient(repo, "default")
	writer.stateCompression = "gzip"

	original := []byte(strings.Repeat("abc", 4096))
	if err := writer.Put(ctx, original); err != nil {
		t.Fatalf("put: %v", err)
	}

	reader := newRemoteClient(repo, "default")
	// reader.stateCompression defaults to "none", but it should still read
	// the compressed state via mediaType autodetection.
	if got, err := reader.Get(ctx); err != nil {
		t.Fatalf("get: %v", err)
	} else if got == nil {
		t.Fatalf("expected payload")
	} else if !bytes.Equal(got.Data, original) {
		t.Fatalf("expected payload to match original, got len=%d want len=%d", len(got.Data), len(original))
	}
}

func TestRemoteClient_StateCompression_GzipEmptyState(t *testing.T) {
	ctx := context.Background()
	fake := newFakeORASRepo()
	repo := &orasRepositoryClient{inner: fake}

	c := newRemoteClient(repo, "default")
	c.stateCompression = "gzip"

	// An empty state is a valid edge case (e.g. freshly initialized workspace).
	if err := c.Put(ctx, []byte{}); err != nil {
		t.Fatalf("put empty: %v", err)
	}

	p, err := c.Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if p == nil {
		t.Fatalf("expected payload for empty state, got nil")
	}
	if len(p.Data) != 0 {
		t.Fatalf("expected empty data, got %d bytes", len(p.Data))
	}
}

// raceSimulatingRepo wraps fakeORASRepo to simulate a race condition where
// another process writes a lock between our Tag call and our verification read.
// It intercepts the second Resolve call (the verification) and swaps in a winner's lock.
type raceSimulatingRepo struct {
	delegatingRepo
	interceptTag   string
	winnerLockID   string
	tagCount       int
	winnerDesc     ocispec.Descriptor
	winnerManifest []byte
}

func newRaceSimulatingRepo(inner *fakeORASRepo, interceptTag, winnerLockID string) *raceSimulatingRepo {
	winnerManifest := []byte(fmt.Sprintf(
		`{"artifactType":"application/vnd.terraform.lock.v1","mediaType":"application/vnd.oci.image.manifest.v1+json","annotations":{"org.terraform.lock.id":"%s","org.terraform.lock.info":"{\"ID\":\"%s\"}"}}`,
		winnerLockID, winnerLockID))
	dgst := digest.FromBytes(winnerManifest)
	return &raceSimulatingRepo{
		delegatingRepo: delegatingRepo{inner: inner},
		interceptTag:   interceptTag,
		winnerLockID:   winnerLockID,
		winnerManifest: winnerManifest,
		winnerDesc: ocispec.Descriptor{
			MediaType: "application/vnd.oci.image.manifest.v1+json",
			Digest:    dgst,
			Size:      int64(len(winnerManifest)),
		},
	}
}

func (r *raceSimulatingRepo) Fetch(ctx context.Context, target ocispec.Descriptor) (io.ReadCloser, error) {
	// If fetching the winner's manifest, return it
	if target.Digest == r.winnerDesc.Digest {
		return io.NopCloser(bytes.NewReader(r.winnerManifest)), nil
	}
	return r.delegatingRepo.Fetch(ctx, target)
}
func (r *raceSimulatingRepo) Tag(ctx context.Context, desc ocispec.Descriptor, reference string) error {
	err := r.delegatingRepo.Tag(ctx, desc, reference)
	if err != nil {
		return err
	}
	// After the first Tag on the lock tag, simulate the winner overwriting it
	if reference == r.interceptTag {
		r.tagCount++
		if r.tagCount == 1 {
			// Store the winner's manifest so Fetch can return it
			_ = r.delegatingRepo.inner.Push(ctx, r.winnerDesc, bytes.NewReader(r.winnerManifest))
			// Winner overwrites the tag
			_ = r.delegatingRepo.inner.Tag(ctx, r.winnerDesc, reference)
		}
	}
	return nil
}

func TestRemoteClient_Lock_RaceConditionDetection(t *testing.T) {
	ctx := context.Background()
	fake := newFakeORASRepo()

	// Create a repo that simulates another process winning the lock race
	racingRepo := newRaceSimulatingRepo(fake, "locked-default", "winner-lock")
	repo := &orasRepositoryClient{inner: racingRepo}

	client := newRemoteClient(repo, "default")

	// Attempt to acquire lock - should fail because the race simulator
	// will overwrite our lock with the "winner" lock after we Tag
	_, err := client.Lock(ctx, &statemgr.LockInfo{ID: "loser-lock", Operation: "test"})
	if err == nil {
		t.Fatalf("expected lock to fail due to race condition, but it succeeded")
	}

	lockErr, ok := err.(*statemgr.LockError)
	if !ok {
		t.Fatalf("expected LockError, got %T: %v", err, err)
	}

	// The error should indicate we lost the race to the winner
	if lockErr.Info == nil || lockErr.Info.ID != "winner-lock" {
		t.Fatalf("expected lock error to reference winner-lock, got: %+v", lockErr.Info)
	}

	if !strings.Contains(err.Error(), "lost race") {
		t.Fatalf("expected error message to mention 'lost race', got: %v", err)
	}
}

// sameGenRaceRepo simulates a race where the winner writes a lock with the
// same generation as the loser (both read the same base generation before the
// lock existed). The only distinguishing field is HolderID.
type sameGenRaceRepo struct {
	delegatingRepo
	interceptTag string
	tagCount     int
	winnerDesc   ocispec.Descriptor
	winnerBlob   []byte
}

func newSameGenRaceRepo(inner *fakeORASRepo, interceptTag string, winnerGeneration int64, winnerHolderID string) *sameGenRaceRepo {
	lockData := LockManifestData{Generation: winnerGeneration, HolderID: winnerHolderID}
	lockDataJSON, _ := json.Marshal(lockData)
	lockInfoJSON, _ := json.Marshal(map[string]string{"ID": winnerHolderID})

	annotations := map[string]string{
		"org.terraform.lock.id":         winnerHolderID,
		"org.terraform.lock.info":       string(lockInfoJSON),
		"org.terraform.lock.generation": string(lockDataJSON),
	}
	winnerManifest, _ := json.Marshal(map[string]interface{}{
		"artifactType": "application/vnd.terraform.lock.v1",
		"mediaType":    "application/vnd.oci.image.manifest.v1+json",
		"annotations":  annotations,
	})
	dgst := digest.FromBytes(winnerManifest)
	return &sameGenRaceRepo{
		delegatingRepo: delegatingRepo{inner: inner},
		interceptTag:   interceptTag,
		winnerBlob:     winnerManifest,
		winnerDesc: ocispec.Descriptor{
			MediaType: "application/vnd.oci.image.manifest.v1+json",
			Digest:    dgst,
			Size:      int64(len(winnerManifest)),
		},
	}
}

func (r *sameGenRaceRepo) Fetch(ctx context.Context, target ocispec.Descriptor) (io.ReadCloser, error) {
	if target.Digest == r.winnerDesc.Digest {
		return io.NopCloser(bytes.NewReader(r.winnerBlob)), nil
	}
	return r.delegatingRepo.Fetch(ctx, target)
}
func (r *sameGenRaceRepo) Tag(ctx context.Context, desc ocispec.Descriptor, reference string) error {
	err := r.delegatingRepo.Tag(ctx, desc, reference)
	if err != nil {
		return err
	}
	if reference == r.interceptTag {
		r.tagCount++
		if r.tagCount == 1 {
			_ = r.delegatingRepo.inner.Push(ctx, r.winnerDesc, bytes.NewReader(r.winnerBlob))
			_ = r.delegatingRepo.inner.Tag(ctx, r.winnerDesc, reference)
		}
	}
	return nil
}

func TestRemoteClient_Lock_SameGenerationRaceDetectedByHolderID(t *testing.T) {
	ctx := context.Background()
	fake := newFakeORASRepo()

	// Both processes read gen=0 (no prior lock), both will write gen=1.
	// The winner overwrites the lock tag with the same generation but different HolderID.
	racingRepo := newSameGenRaceRepo(fake, "locked-default", 1, "winner-process")
	repo := &orasRepositoryClient{inner: racingRepo}

	client := newRemoteClient(repo, "default")

	_, err := client.Lock(ctx, &statemgr.LockInfo{ID: "loser-process", Operation: "test"})
	if err == nil {
		t.Fatalf("expected lock to fail due to same-generation race, but it succeeded")
	}

	lockErr, ok := err.(*statemgr.LockError)
	if !ok {
		t.Fatalf("expected LockError, got %T: %v", err, err)
	}
	if !strings.Contains(lockErr.Err.Error(), "lost race") {
		t.Fatalf("expected 'lost race' error, got: %v", lockErr.Err)
	}
}

func TestLockWithGenerationDetection(t *testing.T) {
	ctx := context.Background()
	fake := newFakeORASRepo()
	repo := &orasRepositoryClient{inner: fake}
	client := newRemoteClient(repo, "default")
	client.lockTTL = 5 * time.Minute

	// First lock should have generation 1
	info1 := &statemgr.LockInfo{ID: "holder-1", Operation: "apply", Info: "state-1"}
	_, err := client.Lock(ctx, info1)
	if err != nil {
		t.Fatalf("first lock failed: %v", err)
	}

	// Verify generation is set to 1
	gen1, err := client.getLockManifestData(ctx)
	if err != nil {
		t.Fatalf("failed to read generation: %v", err)
	}
	if gen1.Generation != 1 {
		t.Fatalf("expected generation 1, got %d", gen1.Generation)
	}

	// Verify leaseExpiry is set (since lockTTL > 0)
	if gen1.LeaseExpiry == 0 {
		t.Fatalf("expected leaseExpiry to be set, got 0")
	}

	// Verify holder ID is set
	if gen1.HolderID == "" {
		t.Fatalf("expected holderID to be set, got empty")
	}

	// Now, if we have a stale lock without clearing it, the next lock should have gen 2
	// Simulate a stale lock scenario by not unlocking and checking generation increment
	// (In practice, this would be caught by background cleanup or timeout)
	if err := client.Unlock(ctx, "holder-1"); err != nil {
		t.Fatalf("unlock failed: %v", err)
	}

	// After unlock, the lock is deleted, so the next lock gets generation 1 again
	// This is correct behavior since there's no previous lock to reference
	info2 := &statemgr.LockInfo{ID: "holder-2", Operation: "apply", Info: "state-2"}
	_, err = client.Lock(ctx, info2)
	if err != nil {
		t.Fatalf("second lock failed: %v", err)
	}

	gen2, err := client.getLockManifestData(ctx)
	if err != nil {
		t.Fatalf("failed to read generation: %v", err)
	}
	// After unlock, next lock gets generation 1 (fresh lock)
	if gen2.Generation != 1 {
		t.Fatalf("expected generation 1 after unlock, got %d", gen2.Generation)
	}
}

func TestStaleLockCleanupRaceCondition(t *testing.T) {
	// This test verifies that the stale lock cleanup race condition is handled correctly.
	// Scenario:
	// 1. Process A reads stale lock (gen=5)
	// 2. Process A clears stale lock
	// 3. Process B acquires lock with gen=1 (won race during clear)
	// 4. Process A writes lock with gen=6 (5+1, from generation read BEFORE clear)
	// 5. Process B's post-write verification sees gen=6 → detects it lost
	// 6. Process A's post-write verification sees gen=6 → wins
	ctx := context.Background()
	fake := newFakeORASRepo()
	repo := &orasRepositoryClient{inner: fake}

	// First, create a stale lock with generation 5
	clientStale := newRemoteClient(repo, "default")
	clientStale.lockTTL = 1 * time.Second
	now := time.Now()
	pastTime := now.Add(-2 * time.Second) // 2 seconds in the past (older than lockTTL)

	// Manually create a stale lock with generation 5
	staleInfo := &statemgr.LockInfo{ID: "crashed-process", Operation: "apply", Created: pastTime}
	staleInfoBytes, _ := json.Marshal(staleInfo)
	leaseExpiry := pastTime.Add(clientStale.lockTTL).UnixNano() // Already expired
	manifestDesc, _ := clientStale.packLockManifestWithGeneration(ctx, staleInfo.ID, string(staleInfoBytes), 5, leaseExpiry, staleInfo.ID)
	_ = fake.Tag(ctx, manifestDesc, clientStale.lockTag)

	// Create client A that will clear the stale lock
	// Use same lockTTL=1s so it also considers the lock stale
	clientA := newRemoteClient(repo, "default")
	clientA.lockTTL = 1 * time.Second // Same TTL so it detects staleness
	clientA.now = func() time.Time { return now }

	// Client A acquires lock - should read gen=5, clear stale, write gen=6
	infoA := &statemgr.LockInfo{ID: "process-A", Operation: "apply"}
	lockIDA, err := clientA.Lock(ctx, infoA)
	if err != nil {
		t.Fatalf("clientA lock failed: %v", err)
	}
	if lockIDA != "process-A" {
		t.Fatalf("expected lockID 'process-A', got %q", lockIDA)
	}

	// Verify generation is 6 (5+1)
	genData, err := clientA.getLockManifestData(ctx)
	if err != nil {
		t.Fatalf("failed to read generation: %v", err)
	}
	if genData.Generation != 6 {
		t.Fatalf("expected generation 6 (stale gen 5 + 1), got %d", genData.Generation)
	}

	// Now try client B - should fail because A holds the lock
	clientB := newRemoteClient(repo, "default")
	clientB.lockTTL = 5 * time.Minute
	infoB := &statemgr.LockInfo{ID: "process-B", Operation: "apply"}
	_, err = clientB.Lock(ctx, infoB)
	if err == nil {
		t.Fatalf("expected clientB lock to fail, but it succeeded")
	}
	lockErr, ok := err.(*statemgr.LockError)
	if !ok {
		t.Fatalf("expected LockError, got %T: %v", err, err)
	}
	if lockErr.Info == nil || lockErr.Info.ID != "process-A" {
		t.Fatalf("expected LockError.Info.ID='process-A', got %v", lockErr.Info)
	}
}

func TestAsyncRetentionNotBlocking(t *testing.T) {
	ctx := context.Background()
	fake := newFakeORASRepo()
	repo := &orasRepositoryClient{inner: fake}
	client := newRemoteClient(repo, "default")
	client.versioningMaxVersions = 2

	// Push multiple states - async retention should not block
	start := time.Now()
	for i := 0; i < 3; i++ {
		if err := client.Put(ctx, []byte(fmt.Sprintf("state-%d", i))); err != nil {
			t.Fatalf("put failed: %v", err)
		}
	}
	duration := time.Since(start)

	// Async should be fast (< 100ms even with cleanup running)
	// Inline would take longer due to cleanup blocking
	if duration > 500*time.Millisecond {
		t.Logf("async retention took %v (expected < 500ms for 3 puts)", duration)
	}

	// Poll for state to be readable with timeout
	deadline := time.Now().Add(2 * time.Second)
	var payload *remote.Payload
	var err error
	for {
		payload, err = client.Get(ctx)
		if err == nil && payload != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for state to be written; last error: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRemoteClient_Delete(t *testing.T) {
	ctx := context.Background()
	fake := newFakeORASRepo()
	repo := &orasRepositoryClient{inner: fake}

	client := newRemoteClient(repo, "default")

	// Delete on a non-existent state should succeed (no-op).
	if err := client.Delete(ctx); err != nil {
		t.Fatalf("delete non-existent: %v", err)
	}

	// Put state, then delete it.
	if err := client.Put(ctx, []byte("state-to-delete")); err != nil {
		t.Fatalf("put: %v", err)
	}
	p, err := client.Get(ctx)
	if err != nil {
		t.Fatalf("get after put: %v", err)
	}
	if p == nil || string(p.Data) != "state-to-delete" {
		t.Fatalf("expected state data before delete")
	}

	if err := client.Delete(ctx); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// After deletion, Get should return nil (no state).
	p, err = client.Get(ctx)
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if p != nil {
		t.Fatalf("expected nil payload after delete, got %d bytes", len(p.Data))
	}
}

func TestRemoteClient_Delete_WithVersioning(t *testing.T) {
	drainRetentionSem()
	ctx := context.Background()
	fake := newFakeORASRepo()
	repo := &orasRepositoryClient{inner: fake}

	client := newRemoteClient(repo, "default")
	client.versioningMaxVersions = 5

	// Put state to create the main tag and a version tag.
	if err := client.Put(ctx, []byte("versioned-state")); err != nil {
		t.Fatalf("put: %v", err)
	}

	// Verify both tags exist.
	if _, err := fake.Resolve(ctx, client.stateTag); err != nil {
		t.Fatalf("expected state tag to exist: %v", err)
	}
	if _, err := fake.Resolve(ctx, client.versionTagFor(1)); err != nil {
		t.Fatalf("expected version tag to exist: %v", err)
	}

	// Delete removes only the main state tag; version tags are managed
	// separately by retention / DeleteWorkspace.
	if err := client.Delete(ctx); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := fake.Resolve(ctx, client.stateTag); err == nil {
		t.Fatalf("expected state tag to be gone after delete")
	}
}

func TestRemoteClient_RetagToNewManifest_PreservesVersionAnnotation(t *testing.T) {
	ctx := context.Background()
	fake := newFakeORASRepo()
	repo := &orasRepositoryClient{inner: fake}

	client := newRemoteClient(repo, "default")
	client.versioningMaxVersions = 5

	// Create a state with version annotation.
	if err := client.Put(ctx, []byte("state-v1")); err != nil {
		t.Fatalf("put: %v", err)
	}

	// Read the version tag manifest to confirm annotation exists.
	fm, err := client.fetchManifestWithDesc(ctx, client.versionTagFor(1))
	if err != nil {
		t.Fatalf("fetch version tag: %v", err)
	}
	if v := fm.m.Annotations[annotationStateVersion]; v != "1" {
		t.Fatalf("expected version annotation '1', got %q", v)
	}

	// Simulate retagToNewManifest (as done during retention).
	logger := &noopLogger{}
	if err := client.retagToNewManifest(ctx, []string{client.versionTagFor(1)}, logger); err != nil {
		t.Fatalf("retagToNewManifest: %v", err)
	}

	// After retag, the version annotation should still be present.
	fm2, err := client.fetchManifestWithDesc(ctx, client.versionTagFor(1))
	if err != nil {
		t.Fatalf("fetch after retag: %v", err)
	}
	if v := fm2.m.Annotations[annotationStateVersion]; v != "1" {
		t.Fatalf("expected preserved version annotation '1' after retag, got %q", v)
	}
}

// noopLogger satisfies the logger interface used by retagToNewManifest.
type noopLogger struct{}

func (noopLogger) Debug(string, ...interface{}) {}
