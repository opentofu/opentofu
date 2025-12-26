package oras

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
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
	c1.versioningEnabled = true
	c1.versioningMaxVersions = 10
	if err := c1.Put(ctx, []byte("state-dev")); err != nil {
		t.Fatalf("put dev: %v", err)
	}
	if err := c1.Put(ctx, []byte("state-dev-2")); err != nil {
		t.Fatalf("put dev second: %v", err)
	}

	// Tag-unsafe workspace (space)
	c2 := newRemoteClient(repo, "my workspace")
	c2.versioningEnabled = true
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

func TestRemoteClient_Put_VersioningTagsAndRetention(t *testing.T) {
	ctx := context.Background()
	fake := newFakeORASRepo()
	repo := &orasRepositoryClient{inner: fake}

	c := newRemoteClient(repo, "default")
	c.versioningEnabled = true
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
		t.Fatalf("expected v1 to be deleted due to retention")
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
	inner *fakeORASRepo
}

func (r deleteUnsupportedRepo) Push(ctx context.Context, expected ocispec.Descriptor, content io.Reader) error {
	return r.inner.Push(ctx, expected, content)
}
func (r deleteUnsupportedRepo) Fetch(ctx context.Context, target ocispec.Descriptor) (io.ReadCloser, error) {
	return r.inner.Fetch(ctx, target)
}
func (r deleteUnsupportedRepo) Resolve(ctx context.Context, reference string) (ocispec.Descriptor, error) {
	return r.inner.Resolve(ctx, reference)
}
func (r deleteUnsupportedRepo) Tag(ctx context.Context, desc ocispec.Descriptor, reference string) error {
	return r.inner.Tag(ctx, desc, reference)
}
func (r deleteUnsupportedRepo) Delete(ctx context.Context, target ocispec.Descriptor) error {
	_ = ctx
	_ = target
	return &orasErrcode.ErrorResponse{StatusCode: 405}
}
func (r deleteUnsupportedRepo) Exists(ctx context.Context, target ocispec.Descriptor) (bool, error) {
	return r.inner.Exists(ctx, target)
}
func (r deleteUnsupportedRepo) Tags(ctx context.Context, last string, fn func(tags []string) error) error {
	return r.inner.Tags(ctx, last, fn)
}

func TestRemoteClient_UnlockFallbackWhenDeleteUnsupported(t *testing.T) {
	ctx := context.Background()
	fake := newFakeORASRepo()
	repo := &orasRepositoryClient{inner: deleteUnsupportedRepo{inner: fake}}

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
		{name: "timeout", err: errors.New("connection timeout occurred"), expected: true},
		{name: "EOF", err: errors.New("unexpected EOF"), expected: true},
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

	client1 := newRemoteClient(repo, "default")
	client2 := newRemoteClient(repo, "default")
	client2.lockTTL = time.Hour
	client2.now = func() time.Time { return time.Unix(10_000, 0).UTC() }

	staleCreated := time.Unix(1_000, 0).UTC()
	_, err := client1.Lock(ctx, &statemgr.LockInfo{ID: "lock-stale", Operation: "test", Created: staleCreated})
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
	repo := &orasRepositoryClient{inner: deleteUnsupportedRepo{inner: fake}}

	client1 := newRemoteClient(repo, "default")
	client2 := newRemoteClient(repo, "default")
	client2.lockTTL = time.Hour
	client2.now = func() time.Time { return time.Unix(10_000, 0).UTC() }

	staleCreated := time.Unix(1_000, 0).UTC()
	_, err := client1.Lock(ctx, &statemgr.LockInfo{ID: "lock-stale", Operation: "test", Created: staleCreated})
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

	m, err := c.fetchManifest(ctx, c.stateTag)
	if err != nil {
		t.Fatalf("fetch manifest: %v", err)
	}
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
