package oci

import (
	"context"
	"io"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	orasErrcode "oras.land/oras-go/v2/registry/remote/errcode"
)

func TestRemoteClient_LockContentionAndUnlockMismatch(t *testing.T) {
	ctx := context.Background()
	fake := newFakeOCIRepo()
	repo := &ociRepositoryClient{inner: fake}

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
	fake := newFakeOCIRepo()
	repo := &ociRepositoryClient{inner: fake}

	// Tag-safe workspace
	c1 := newRemoteClient(repo, "dev")
	if err := c1.Put(ctx, []byte("state-dev")); err != nil {
		t.Fatalf("put dev: %v", err)
	}

	// Tag-unsafe workspace (space)
	c2 := newRemoteClient(repo, "my workspace")
	if err := c2.Put(ctx, []byte("state-unsafe")); err != nil {
		t.Fatalf("put unsafe: %v", err)
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
	inner *fakeOCIRepo
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
	fake := newFakeOCIRepo()
	repo := &ociRepositoryClient{inner: deleteUnsupportedRepo{inner: fake}}

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
