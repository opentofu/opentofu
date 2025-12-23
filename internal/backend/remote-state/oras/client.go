package oras

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/errdef"
	orasRegistry "oras.land/oras-go/v2/registry"
	orasErrcode "oras.land/oras-go/v2/registry/remote/errcode"
)

const (
	mediaTypeStateLayer     = "application/vnd.opentofu.statefile.v1"
	mediaTypeStateLayerGzip = "application/vnd.opentofu.statefile.v1+gzip"
	artifactTypeState       = "application/vnd.opentofu.state.v1"
	artifactTypeLock        = "application/vnd.opentofu.lock.v1"

	annotationWorkspace = "org.opentofu.workspace"
	annotationLockID    = "org.opentofu.lock.id"
	annotationLockInfo  = "org.opentofu.lock.info"
)

// Tag naming scheme:
//
//   - State is stored at "state-<workspaceTag>".
//   - Lock is stored at "locked-<workspaceTag>".
//   - On registries that don't support manifest deletion (GHCR returns 405),
//     unlock retags to "unlocked-<workspaceTag>" instead.
//
// workspaceTag equals the workspace name if it's a valid OCI tag,
// otherwise we use "ws-<hash>" and store the name in annotations.
const (
	stateTagPrefix           = "state-"
	lockTagPrefix            = "locked-"
	unlockedTagPrefix        = "unlocked-"
	stateVersionTagSeparator = "-v"
)

type RemoteClient struct {
	repo             *orasRepositoryClient
	workspaceName    string
	stateTag         string
	lockTag          string
	unlockedTag      string
	retryConfig      RetryConfig
	stateCompression string
	lockTTL          time.Duration
	now              func() time.Time

	versioningEnabled     bool
	versioningMaxVersions int
}

var _ remote.Client = (*RemoteClient)(nil)
var _ remote.ClientLocker = (*RemoteClient)(nil)

func newRemoteClient(repo *orasRepositoryClient, workspaceName string) *RemoteClient {
	wsTag := workspaceTagFor(workspaceName)
	return &RemoteClient{
		repo:                  repo,
		workspaceName:         workspaceName,
		stateTag:              stateTagPrefix + wsTag,
		lockTag:               lockTagPrefix + wsTag,
		unlockedTag:           unlockedTagPrefix + wsTag,
		retryConfig:           DefaultRetryConfig(),
		stateCompression:      "none",
		lockTTL:               0,
		now:                   time.Now,
		versioningEnabled:     false,
		versioningMaxVersions: 0,
	}
}

func (c *RemoteClient) Get(ctx context.Context) (*remote.Payload, error) {
	return withRetry(ctx, c.retryConfig, func(ctx context.Context) (*remote.Payload, error) {
		return c.get(ctx)
	})
}

func (c *RemoteClient) get(ctx context.Context) (*remote.Payload, error) {
	m, err := c.fetchManifest(ctx, c.stateTag)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(m.Layers) == 0 {
		return nil, nil
	}

	layer := m.Layers[0]
	rc, err := c.repo.inner.Fetch(ctx, layer)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	var r io.Reader = rc
	if layer.MediaType == mediaTypeStateLayerGzip {
		gz, err := gzip.NewReader(rc)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		r = gz
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	md5sum := md5.Sum(data)
	return &remote.Payload{MD5: md5sum[:], Data: data}, nil
}

func (c *RemoteClient) Put(ctx context.Context, state []byte) error {
	return withRetryNoResult(ctx, c.retryConfig, func(ctx context.Context) error {
		return c.put(ctx, state)
	})
}

func (c *RemoteClient) put(ctx context.Context, state []byte) error {
	stateToPush := state
	layerMediaType := mediaTypeStateLayer

	if c.stateCompression == "gzip" {
		var buf bytes.Buffer
		gz, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
		if err != nil {
			return err
		}
		if _, err := gz.Write(state); err != nil {
			_ = gz.Close()
			return err
		}
		if err := gz.Close(); err != nil {
			return err
		}
		stateToPush = buf.Bytes()
		layerMediaType = mediaTypeStateLayerGzip
	}

	layerDesc, err := oras.PushBytes(ctx, c.repo.inner, layerMediaType, stateToPush)
	if err != nil {
		return err
	}

	manifestDesc, err := oras.PackManifest(ctx, c.repo.inner, oras.PackManifestVersion1_1, artifactTypeState, oras.PackManifestOptions{
		Layers: []ocispec.Descriptor{layerDesc},
		ManifestAnnotations: map[string]string{
			annotationWorkspace: c.workspaceName,
		},
	})
	if err != nil {
		return err
	}

	if err := c.repo.inner.Tag(ctx, manifestDesc, c.stateTag); err != nil {
		return err
	}

	if !c.versioningEnabled {
		return nil
	}

	nextVersion, existing, err := c.nextStateVersion(ctx)
	if err != nil {
		return err
	}

	newVersionTag := c.versionTagFor(nextVersion)
	if err := c.repo.inner.Tag(ctx, manifestDesc, newVersionTag); err != nil {
		return err
	}

	if c.versioningMaxVersions > 0 {
		existing = append(existing, nextVersion)
		if err := c.enforceVersionRetention(ctx, manifestDesc, existing); err != nil {
			return err
		}
	}

	return nil
}

func (c *RemoteClient) versionTagFor(version int) string {
	return fmt.Sprintf("%s%s%d", c.stateTag, stateVersionTagSeparator, version)
}

func (c *RemoteClient) nextStateVersion(ctx context.Context) (next int, existing []int, err error) {
	var tags []string
	if err := c.repo.inner.Tags(ctx, "", func(page []string) error {
		tags = append(tags, page...)
		return nil
	}); err != nil {
		return 0, nil, err
	}

	max := 0
	for _, t := range tags {
		v, ok := parseStateVersionTag(t, c.stateTag)
		if !ok {
			continue
		}
		existing = append(existing, v)
		if v > max {
			max = v
		}
	}

	return max + 1, existing, nil
}

func (c *RemoteClient) enforceVersionRetention(ctx context.Context, current ocispec.Descriptor, versions []int) error {
	if c.versioningMaxVersions <= 0 {
		return nil
	}

	sort.Ints(versions)
	toDeleteCount := len(versions) - c.versioningMaxVersions
	if toDeleteCount <= 0 {
		return nil
	}

	for _, v := range versions[:toDeleteCount] {
		tag := c.versionTagFor(v)
		desc, err := c.repo.inner.Resolve(ctx, tag)
		if err != nil {
			if isNotFound(err) {
				continue
			}
			return err
		}
		if desc.Digest == current.Digest {
			continue
		}
		if err := c.repo.inner.Delete(ctx, desc); err != nil {
			if isNotFound(err) || isDeleteUnsupported(err) {
				continue
			}
			return err
		}
	}

	return nil
}

func (c *RemoteClient) Delete(ctx context.Context) error {
	return withRetryNoResult(ctx, c.retryConfig, func(ctx context.Context) error {
		return c.delete(ctx)
	})
}

func (c *RemoteClient) delete(ctx context.Context) error {
	desc, err := c.repo.inner.Resolve(ctx, c.stateTag)
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return err
	}
	return c.repo.inner.Delete(ctx, desc)
}

func (c *RemoteClient) Lock(ctx context.Context, info *statemgr.LockInfo) (string, error) {
	// Lock operations use retry internally for network calls,
	// but lock contention errors are not retried (they're not transient)
	return c.lock(ctx, info)
}

func (c *RemoteClient) lock(ctx context.Context, info *statemgr.LockInfo) (string, error) {
	if info == nil {
		return "", fmt.Errorf("lock info is required")
	}

	// Check for existing lock (with retry for transient network errors)
	existingDesc, err := withRetry(ctx, c.retryConfig, func(ctx context.Context) (ocispec.Descriptor, error) {
		return c.repo.inner.Resolve(ctx, c.lockTag)
	})
	if err == nil {
		existing, err := c.getLockInfo(ctx)
		if err != nil {
			return "", &statemgr.LockError{InconsistentRead: true, Err: err}
		}
		if existing != nil && existing.ID != "" {
			if c.isLockStale(existing) {
				if err := c.clearLock(ctx, existingDesc); err != nil {
					return "", err
				}
			} else {
				return "", &statemgr.LockError{Info: existing, Err: fmt.Errorf("state is locked")}
			}
		}
	} else if !isNotFound(err) {
		return "", err
	}

	info.Path = c.stateTag
	infoBytes, err := json.Marshal(info)
	if err != nil {
		return "", err
	}

	manifestDesc, err := oras.PackManifest(ctx, c.repo.inner, oras.PackManifestVersion1_1, artifactTypeLock, oras.PackManifestOptions{
		ManifestAnnotations: map[string]string{
			annotationWorkspace: c.workspaceName,
			annotationLockID:    info.ID,
			annotationLockInfo:  string(infoBytes),
		},
	})
	if err != nil {
		return "", err
	}

	// Tag with retry for transient network errors
	err = withRetryNoResult(ctx, c.retryConfig, func(ctx context.Context) error {
		return c.repo.inner.Tag(ctx, manifestDesc, c.lockTag)
	})
	if err != nil {
		if _, resolveErr := c.repo.inner.Resolve(ctx, c.lockTag); resolveErr == nil {
			existing, _ := c.getLockInfo(ctx)
			return "", &statemgr.LockError{Info: existing, Err: fmt.Errorf("state is locked")}
		}
		return "", err
	}

	return info.ID, nil
}

func (c *RemoteClient) isLockStale(info *statemgr.LockInfo) bool {
	if c.lockTTL <= 0 || info == nil || info.Created.IsZero() {
		return false
	}
	now := time.Now
	if c.now != nil {
		now = c.now
	}
	age := now().UTC().Sub(info.Created)
	if age < 0 {
		return false
	}
	return age > c.lockTTL
}

func (c *RemoteClient) clearLock(ctx context.Context, desc ocispec.Descriptor) error {
	// Delete with retry for transient network errors
	err := withRetryNoResult(ctx, c.retryConfig, func(ctx context.Context) error {
		return c.repo.inner.Delete(ctx, desc)
	})
	if err == nil || isNotFound(err) {
		return nil
	}
	if !isDeleteUnsupported(err) {
		return err
	}

	// GHCR fallback: retag to unlocked manifest
	return c.retagToUnlocked(ctx)
}

func (c *RemoteClient) Unlock(ctx context.Context, id string) error {
	// Unlock operations use retry internally for network calls,
	// but lock ID mismatch errors are not retried (they're not transient)
	return c.unlock(ctx, id)
}

func (c *RemoteClient) unlock(ctx context.Context, id string) error {
	// Resolve with retry for transient network errors
	desc, err := withRetry(ctx, c.retryConfig, func(ctx context.Context) (ocispec.Descriptor, error) {
		return c.repo.inner.Resolve(ctx, c.lockTag)
	})
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return err
	}

	existing, err := c.getLockInfo(ctx)
	if err != nil {
		return err
	}
	if existing == nil || existing.ID == "" {
		return nil
	}
	if id != "" && existing.ID != id {
		return fmt.Errorf("lock ID mismatch: held by %q", existing.ID)
	}

	// Delete with retry for transient network errors
	err = withRetryNoResult(ctx, c.retryConfig, func(ctx context.Context) error {
		return c.repo.inner.Delete(ctx, desc)
	})
	if err == nil {
		return nil
	}
	if !isDeleteUnsupported(err) {
		return err
	}

	// GHCR fallback: retag to unlocked manifest
	return c.retagToUnlocked(ctx)
}

func (c *RemoteClient) retagToUnlocked(ctx context.Context) error {
	// Resolve with retry
	desc, err := withRetry(ctx, c.retryConfig, func(ctx context.Context) (ocispec.Descriptor, error) {
		return c.repo.inner.Resolve(ctx, c.unlockedTag)
	})
	if isNotFound(err) {
		desc, err = oras.PackManifest(ctx, c.repo.inner, oras.PackManifestVersion1_1, artifactTypeLock, oras.PackManifestOptions{})
		if err != nil {
			return err
		}
		// Tag with retry
		if err := withRetryNoResult(ctx, c.retryConfig, func(ctx context.Context) error {
			return c.repo.inner.Tag(ctx, desc, c.unlockedTag)
		}); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	// Final tag with retry
	return withRetryNoResult(ctx, c.retryConfig, func(ctx context.Context) error {
		return c.repo.inner.Tag(ctx, desc, c.lockTag)
	})
}

func (c *RemoteClient) getLockInfo(ctx context.Context) (*statemgr.LockInfo, error) {
	m, err := c.fetchManifest(ctx, c.lockTag)
	if err != nil {
		return nil, err
	}

	if raw, ok := m.Annotations[annotationLockInfo]; ok && raw != "" {
		var info statemgr.LockInfo
		if err := json.Unmarshal([]byte(raw), &info); err == nil {
			return &info, nil
		}
	}

	id := m.Annotations[annotationLockID]
	if id == "" {
		return &statemgr.LockInfo{}, nil
	}
	return &statemgr.LockInfo{ID: id, Path: c.stateTag}, nil
}

type manifest struct {
	Annotations map[string]string    `json:"annotations"`
	Layers      []ocispec.Descriptor `json:"layers"`
}

func (c *RemoteClient) fetchManifest(ctx context.Context, reference string) (*manifest, error) {
	return withRetry(ctx, c.retryConfig, func(ctx context.Context) (*manifest, error) {
		return c.fetchManifestInternal(ctx, reference)
	})
}

func (c *RemoteClient) fetchManifestInternal(ctx context.Context, reference string) (*manifest, error) {
	desc, err := c.repo.inner.Resolve(ctx, reference)
	if err != nil {
		return nil, err
	}
	rc, err := c.repo.inner.Fetch(ctx, desc)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("decoding manifest %q: %w", reference, err)
	}
	if m.Annotations == nil {
		m.Annotations = map[string]string{}
	}
	return &m, nil
}

// Workspace tag helpers

func workspaceTagFor(workspace string) string {
	ref := orasRegistry.Reference{Reference: workspace}
	if err := ref.ValidateReferenceAsTag(); err == nil {
		return workspace
	}
	h := sha256.Sum256([]byte(workspace))
	return "ws-" + hex.EncodeToString(h[:8])
}

func listWorkspacesFromTags(ctx context.Context, repo *orasRepositoryClient) ([]string, error) {
	var tags []string
	if err := repo.inner.Tags(ctx, "", func(page []string) error {
		tags = append(tags, page...)
		return nil
	}); err != nil {
		return nil, err
	}

	tagSet := make(map[string]struct{}, len(tags))
	for _, t := range tags {
		tagSet[t] = struct{}{}
	}

	seen := map[string]bool{}
	var out []string
	for _, tag := range tags {
		if !strings.HasPrefix(tag, stateTagPrefix) {
			continue
		}
		if base, _, ok := splitStateVersionTag(tag); ok {
			if _, ok := tagSet[base]; ok {
				continue
			}
		}
		name, err := workspaceNameFromTag(ctx, repo, tag)
		if err != nil {
			return nil, err
		}
		if name != "" && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}

func parseStateVersionTag(tag string, stateTag string) (int, bool) {
	prefix := stateTag + stateVersionTagSeparator
	if !strings.HasPrefix(tag, prefix) {
		return 0, false
	}
	s := strings.TrimPrefix(tag, prefix)
	if s == "" {
		return 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	v := 0
	for i := 0; i < len(s); i++ {
		v = v*10 + int(s[i]-'0')
	}
	if v <= 0 {
		return 0, false
	}
	return v, true
}

func splitStateVersionTag(tag string) (base string, version int, ok bool) {
	idx := strings.LastIndex(tag, stateVersionTagSeparator)
	if idx < 0 {
		return "", 0, false
	}
	base = tag[:idx]
	if base == "" {
		return "", 0, false
	}
	s := tag[idx+len(stateVersionTagSeparator):]
	if s == "" {
		return "", 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return "", 0, false
		}
	}
	v := 0
	for i := 0; i < len(s); i++ {
		v = v*10 + int(s[i]-'0')
	}
	if v <= 0 {
		return "", 0, false
	}
	return base, v, true
}

func workspaceNameFromTag(ctx context.Context, repo *orasRepositoryClient, stateTag string) (string, error) {
	wsTag := strings.TrimPrefix(stateTag, stateTagPrefix)
	if !strings.HasPrefix(wsTag, "ws-") {
		return wsTag, nil
	}
	// Hash fallback - need to read annotation
	desc, err := repo.inner.Resolve(ctx, stateTag)
	if err != nil {
		return "", err
	}
	rc, err := repo.inner.Fetch(ctx, desc)
	if err != nil {
		return "", err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}

	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return wsTag, nil
	}
	if name := m.Annotations[annotationWorkspace]; name != "" {
		return name, nil
	}
	return wsTag, nil
}

// Error helpers

func isNotFound(err error) bool {
	if errors.Is(err, errdef.ErrNotFound) {
		return true
	}
	var resp *orasErrcode.ErrorResponse
	if errors.As(err, &resp) {
		return resp.StatusCode == 404
	}
	return false
}

func isDeleteUnsupported(err error) bool {
	var resp *orasErrcode.ErrorResponse
	if errors.As(err, &resp) {
		return resp.StatusCode == 405
	}
	return false
}
