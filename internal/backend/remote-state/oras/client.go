// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

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
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/errdef"
	orasRegistry "oras.land/oras-go/v2/registry"
	orasErrcode "oras.land/oras-go/v2/registry/remote/errcode"
)

const (
	mediaTypeStateLayer     = "application/vnd.terraform.statefile.v1"
	mediaTypeStateLayerGzip = "application/vnd.terraform.statefile.v1+gzip"
	artifactTypeState       = "application/vnd.terraform.state.v1"
	artifactTypeLock        = "application/vnd.terraform.lock.v1"

	annotationWorkspace = "org.terraform.workspace"
	annotationUpdatedAt = "org.terraform.state.updated_at"
	annotationLockID    = "org.terraform.lock.id"
	annotationLockInfo  = "org.terraform.lock.info"
	annotationLockGen   = "org.terraform.lock.generation"
)

// Tag naming scheme:
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

// RetryConfig defines retry behavior for operations against the registry.
type RetryConfig struct {
	MaxAttempts       int
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	BackoffMultiplier float64
}

// DefaultRetryConfig returns a RetryConfig with sensible defaults:
// 3 attempts, 1s initial backoff, 30s max backoff, 2x multiplier.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:       3,
		InitialBackoff:    time.Second,
		MaxBackoff:        30 * time.Second,
		BackoffMultiplier: 2.0,
	}
}

func withRetry[T any](ctx context.Context, cfg RetryConfig, operation func(ctx context.Context) (T, error)) (T, error) {
	var zero T
	var lastErr error

	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 1
	}

	backoff := cfg.InitialBackoff
	if backoff <= 0 {
		backoff = time.Second
	}

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		result, err := operation(ctx)
		if err == nil {
			return result, nil
		}

		lastErr = err

		if ctx.Err() != nil {
			return zero, ctx.Err()
		}
		if !isTransientError(err) {
			return zero, err
		}
		if attempt == cfg.MaxAttempts {
			break
		}

		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(backoff):
		}

		backoff = time.Duration(float64(backoff) * cfg.BackoffMultiplier)
		if cfg.MaxBackoff > 0 && backoff > cfg.MaxBackoff {
			backoff = cfg.MaxBackoff
		}
	}

	return zero, lastErr
}

func withRetryNoResult(ctx context.Context, cfg RetryConfig, operation func(ctx context.Context) error) error {
	_, err := withRetry(ctx, cfg, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, operation(ctx)
	})
	return err
}

func isTransientError(err error) bool {
	if err == nil {
		return false
	}

	var errResp *orasErrcode.ErrorResponse
	if errors.As(err, &errResp) {
		switch errResp.StatusCode {
		case http.StatusTooManyRequests,
			http.StatusRequestTimeout,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout:
			return true
		}
		return false
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}

	// Fallback to string matching to catch transient errors after wrapping.
	// Use specific substrings to avoid false positives (e.g. "eof" matching "thereof").
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "connection reset"):
		return true
	case strings.Contains(msg, "connection refused"):
		return true
	case strings.Contains(msg, "i/o timeout"),
		strings.Contains(msg, "connection timeout"),
		strings.Contains(msg, "tls handshake timeout"),
		strings.Contains(msg, "context deadline exceeded"):
		return true
	case strings.Contains(msg, "temporary failure in name resolution"):
		return true
	case strings.Contains(msg, "no such host"):
		return true
	case strings.Contains(msg, "unexpected eof"),
		strings.Contains(msg, "read: eof"),
		msg == "eof":
		return true
	default:
		return false
	}
}

// LockManifestData holds metadata about a lock that helps detect stale locks
// and prevent concurrent lock holder scenarios. It's stored as JSON in the
// lock manifest's annotations.
type LockManifestData struct {
	Generation  int64  `json:"generation"`
	LeaseExpiry int64  `json:"lease_expiry,omitempty"`
	HolderID    string `json:"holder_id,omitempty"`
}

// defaultRetentionSem is the default semaphore used to limit concurrent async
// retention goroutines. It prevents goroutine accumulation when Put() is called
// rapidly. Tests may inject a custom semaphore via the retentionSem field.
var defaultRetentionSem = make(chan struct{}, 3)

// RemoteClient implements remote.Client and remote.ClientLocker for OCI
// registries using the ORAS library. It stores state as OCI artifacts and
// uses manifest tags for workspace locking.
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

	// versioningMaxVersions controls state versioning:
	// - 0: versioning disabled (no version tags created)
	// - >0: versioning enabled with retention limit
	versioningMaxVersions int

	// retentionSem limits concurrent async retention goroutines.
	// If nil, defaultRetentionSem is used.
	retentionSem chan struct{}
}

type digestGroup struct {
	desc ocispec.Descriptor
	tags []string
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
		versioningMaxVersions: 0,
	}
}

func (c *RemoteClient) packStateManifest(ctx context.Context, layers []ocispec.Descriptor) (ocispec.Descriptor, error) {
	return oras.PackManifest(ctx, c.repo.inner, oras.PackManifestVersion1_1, artifactTypeState, oras.PackManifestOptions{
		Layers: layers,
		ManifestAnnotations: map[string]string{
			annotationWorkspace: c.workspaceName,
			annotationUpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		},
	})
}

func (c *RemoteClient) packLockManifest(ctx context.Context, id, infoJSON string) (ocispec.Descriptor, error) {
	return oras.PackManifest(ctx, c.repo.inner, oras.PackManifestVersion1_1, artifactTypeLock, oras.PackManifestOptions{
		ManifestAnnotations: map[string]string{
			annotationWorkspace: c.workspaceName,
			annotationLockID:    id,
			annotationLockInfo:  infoJSON,
		},
	})
}

func (c *RemoteClient) packLockManifestWithGeneration(ctx context.Context, id, infoJSON string, generation int64, leaseExpiry int64, holderID string) (ocispec.Descriptor, error) {
	lockData := LockManifestData{
		Generation:  generation,
		LeaseExpiry: leaseExpiry,
		HolderID:    holderID,
	}
	lockDataJSON, err := json.Marshal(lockData)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to marshal lock metadata: %w", err)
	}

	return oras.PackManifest(ctx, c.repo.inner, oras.PackManifestVersion1_1, artifactTypeLock, oras.PackManifestOptions{
		ManifestAnnotations: map[string]string{
			annotationWorkspace: c.workspaceName,
			annotationLockID:    id,
			annotationLockInfo:  infoJSON,
			annotationLockGen:   string(lockDataJSON),
		},
	})
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
	if m.ArtifactType != "" && m.ArtifactType != artifactTypeState {
		return nil, fmt.Errorf("unexpected state manifest artifactType %q for %q", m.ArtifactType, c.stateTag)
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
	switch layer.MediaType {
	case mediaTypeStateLayer:
		// no-op
	case mediaTypeStateLayerGzip:
		gz, err := gzip.NewReader(rc)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		r = gz
	default:
		return nil, fmt.Errorf("unsupported state layer media type %q", layer.MediaType)
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	// MD5 is required by remote.Payload for ETag-like change detection, not for security.
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
		compressed, err := compressGzip(state)
		if err != nil {
			return fmt.Errorf("compressing state: %w", err)
		}
		stateToPush = compressed
		layerMediaType = mediaTypeStateLayerGzip
	}

	layerDesc, err := oras.PushBytes(ctx, c.repo.inner, layerMediaType, stateToPush)
	if err != nil {
		return err
	}

	manifestDesc, err := c.packStateManifest(ctx, []ocispec.Descriptor{layerDesc})
	if err != nil {
		return err
	}

	if err := c.repo.inner.Tag(ctx, manifestDesc, c.stateTag); err != nil {
		return err
	}

	// Versioning: max_versions > 0 enables versioning with that retention limit
	if c.versioningMaxVersions <= 0 {
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

	existing = append(existing, nextVersion)

	// Async retention with semaphore to limit concurrent goroutines.
	// Derive from the caller's context so that shutdown is propagated,
	// but add a timeout to prevent goroutines from running indefinitely.
	sem := c.retentionSem
	if sem == nil {
		sem = defaultRetentionSem
	}
	select {
	case sem <- struct{}{}:
		go func() {
			defer func() { <-sem }()
			asyncCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			if err := c.enforceVersionRetention(asyncCtx, manifestDesc, existing); err != nil {
				logging.HCLogger().Trace("async retention cleanup failed", "error", err.Error())
			}
		}()
	default:
		// Semaphore full, skip this cleanup (will happen on next Put)
		logging.HCLogger().Trace("async retention skipped: too many pending cleanups")
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
		base, v, ok := splitStateVersionTag(t)
		if !ok || base != c.stateTag {
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
	if c.versioningMaxVersions <= 0 || len(versions) <= c.versioningMaxVersions {
		return nil
	}

	sort.Ints(versions)
	toDeleteCount := len(versions) - c.versioningMaxVersions
	deleteVersions := versions[:toDeleteCount]
	keepVersions := versions[toDeleteCount:]

	deleteTagSet := make(map[string]struct{}, len(deleteVersions))
	keepTagSet := make(map[string]struct{}, len(keepVersions))
	for _, v := range deleteVersions {
		deleteTagSet[c.versionTagFor(v)] = struct{}{}
	}
	for _, v := range keepVersions {
		keepTagSet[c.versionTagFor(v)] = struct{}{}
	}

	groups := c.groupVersionsByDigest(ctx, versions, current.Digest)
	if len(groups) == 0 {
		return nil
	}

	logger := logging.HCLogger().Named("backend.oras")

	for _, g := range groups {
		tagsToDelete, tagsToKeep := classifyTags(g.tags, deleteTagSet, keepTagSet)
		if len(tagsToDelete) == 0 {
			continue
		}

		if len(tagsToKeep) > 0 {
			if err := c.retagToNewManifest(ctx, tagsToKeep, logger); err != nil {
				return err
			}
		}

		if err := c.deleteDigestWithFallback(ctx, g.desc, tagsToDelete[0]); err != nil {
			return err
		}
	}

	return nil
}

func (c *RemoteClient) groupVersionsByDigest(ctx context.Context, versions []int, currentDigest digest.Digest) map[string]*digestGroup {
	groups := make(map[string]*digestGroup)
	for _, v := range versions {
		tag := c.versionTagFor(v)
		desc, err := c.repo.inner.Resolve(ctx, tag)
		if err != nil || desc.Digest == currentDigest {
			continue
		}
		key := desc.Digest.String()
		if g, ok := groups[key]; ok {
			g.tags = append(g.tags, tag)
		} else {
			groups[key] = &digestGroup{desc: desc, tags: []string{tag}}
		}
	}
	return groups
}

func classifyTags(tags []string, deleteSet, keepSet map[string]struct{}) (toDelete, toKeep []string) {
	for _, tag := range tags {
		if _, ok := deleteSet[tag]; ok {
			toDelete = append(toDelete, tag)
		} else if _, ok := keepSet[tag]; ok {
			toKeep = append(toKeep, tag)
		}
	}
	return
}

func (c *RemoteClient) retagToNewManifest(ctx context.Context, tags []string, logger interface{ Debug(string, ...interface{}) }) error {
	if len(tags) == 0 {
		return nil
	}
	logger.Debug("retention: detaching keep tags from digest", "tags", tags)

	m, err := c.fetchManifest(ctx, tags[0])
	if err != nil {
		return err
	}
	if len(m.Layers) == 0 {
		return nil
	}

	newDesc, err := c.packStateManifest(ctx, m.Layers)
	if err != nil {
		return err
	}
	for _, tag := range tags {
		if err := c.repo.inner.Tag(ctx, newDesc, tag); err != nil {
			return err
		}
	}
	return nil
}

func (c *RemoteClient) deleteDigestWithFallback(ctx context.Context, desc ocispec.Descriptor, fallbackTag string) error {
	err := c.repo.inner.Delete(ctx, desc)
	if err == nil || isNotFound(err) {
		return nil
	}
	if !isDeleteUnsupported(err) {
		return err
	}

	if ghErr := tryDeleteGHCRTag(ctx, c.repo, fallbackTag); ghErr != nil {
		return fmt.Errorf("oras backend retention: registry does not support OCI manifest deletion and GHCR API deletion failed for %q: %w", fallbackTag, ghErr)
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

	// Read current generation FIRST (before any modifications).
	// This is critical for race detection: even if we clear a stale lock,
	// we use the generation we read here to compute the next one.
	currentGen, err := c.getLockManifestData(ctx)
	if err != nil && !isNotFound(err) {
		return "", fmt.Errorf("failed to read current lock generation: %w", err)
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
			// Use LeaseExpiry from manifest data (more reliable than Created + TTL)
			if c.isLockStale(currentGen) {
				if err := c.clearLock(ctx, existingDesc); err != nil {
					return "", err
				}
				// After clearing stale lock, continue with generation we read BEFORE clearing.
				// If another process acquires with gen=1, we'll write gen=N+1 (higher),
				// and both post-write verifications will detect the conflict correctly.
			} else {
				return "", &statemgr.LockError{Info: existing, Err: fmt.Errorf("state is locked")}
			}
		}
	} else if !isNotFound(err) {
		return "", err
	}

	// Increment from the generation we read at the start
	newGeneration := int64(1)
	if currentGen != nil && currentGen.Generation > 0 {
		newGeneration = currentGen.Generation + 1
	}

	leaseExpiry := int64(0)
	if c.lockTTL > 0 {
		nowFn := c.now
		if nowFn == nil {
			nowFn = time.Now
		}
		leaseExpiry = nowFn().UTC().Add(c.lockTTL).UnixNano()
	}

	info.Path = c.stateTag
	infoBytes, err := json.Marshal(info)
	if err != nil {
		return "", err
	}

	// Use generation-based lock manifest that includes atomic generation verification
	manifestDesc, err := c.packLockManifestWithGeneration(ctx, info.ID, string(infoBytes), newGeneration, leaseExpiry, info.ID)
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

	// Post-write verification with generation check: Re-read the lock to ensure we actually hold it.
	// This guards against a race condition where two processes both try to write their locks
	// concurrently. We verify that both the generation and the holder ID in the manifest match
	// what we just wrote. Checking HolderID is critical because two processes that read the same
	// base generation would both increment to the same value.
	verified, verifyErr := c.getLockManifestData(ctx)
	if verifyErr != nil {
		// Could not verify - attempt to clean up our lock attempt
		if cleanupDesc, cleanupErr := c.repo.inner.Resolve(ctx, c.lockTag); cleanupErr == nil {
			_ = c.repo.inner.Delete(ctx, cleanupDesc)
		}
		return "", &statemgr.LockError{InconsistentRead: true, Err: fmt.Errorf("failed to verify lock acquisition: %w", verifyErr)}
	}
	if verified == nil || verified.Generation != newGeneration || verified.HolderID != info.ID {
		// Another process won the race - they now hold the lock with a different generation or holder
		existing, _ := c.getLockInfo(ctx)
		return "", &statemgr.LockError{Info: existing, Err: fmt.Errorf("state is locked (lost race)")}
	}

	return info.ID, nil
}

// isLockStale checks if a lock has expired based on its LeaseExpiry.
// This is more reliable than using Created + TTL because:
// 1. The expiry time is calculated when the lock is created
// 2. No dependency on clock synchronization between lock creator and verifier
// 3. Explicit: the manifest contains exactly when the lock expires
func (c *RemoteClient) isLockStale(data *LockManifestData) bool {
	// If lock_ttl is not configured, locks never expire automatically
	if c.lockTTL <= 0 {
		return false
	}
	// If no manifest data or no expiry set, not stale (legacy lock or TTL was 0)
	if data == nil || data.LeaseExpiry <= 0 {
		return false
	}
	nowFn := c.now
	if nowFn == nil {
		nowFn = time.Now
	}
	return nowFn().UTC().UnixNano() > data.LeaseExpiry
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
		desc, err = c.packLockManifest(ctx, "", "")
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
	if m.ArtifactType != "" && m.ArtifactType != artifactTypeLock {
		return nil, fmt.Errorf("unexpected lock manifest artifactType %q for %q", m.ArtifactType, c.lockTag)
	}

	if raw, ok := m.Annotations[annotationLockInfo]; ok && raw != "" {
		var info statemgr.LockInfo
		if err := json.Unmarshal([]byte(raw), &info); err != nil {
			return nil, fmt.Errorf("decoding lock info: %w", err)
		}
		if info.ID == "" {
			info.ID = m.Annotations[annotationLockID]
		}
		if info.Path == "" {
			info.Path = c.stateTag
		}
		return &info, nil
	}

	id := m.Annotations[annotationLockID]
	if id == "" {
		return &statemgr.LockInfo{}, nil
	}
	return &statemgr.LockInfo{ID: id, Path: c.stateTag}, nil
}

func (c *RemoteClient) getLockManifestData(ctx context.Context) (*LockManifestData, error) {
	m, err := c.fetchManifest(ctx, c.lockTag)
	if err != nil {
		return nil, err
	}
	if m.ArtifactType != "" && m.ArtifactType != artifactTypeLock {
		return nil, fmt.Errorf("unexpected lock manifest artifactType %q for %q", m.ArtifactType, c.lockTag)
	}

	if raw, ok := m.Annotations[annotationLockGen]; ok && raw != "" {
		var data LockManifestData
		if err := json.Unmarshal([]byte(raw), &data); err != nil {
			return nil, fmt.Errorf("decoding lock generation data: %w", err)
		}
		return &data, nil
	}

	// No generation data (legacy lock or manually created)
	return &LockManifestData{Generation: 0}, nil
}

type manifest struct {
	ArtifactType string               `json:"artifactType"`
	MediaType    string               `json:"mediaType"`
	Annotations  map[string]string    `json:"annotations"`
	Layers       []ocispec.Descriptor `json:"layers"`
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
	if s == "" || len(s) > 10 {
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
		if v > 1<<30 {
			return "", 0, false
		}
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

func compressGzip(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	gz, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		return nil, err
	}
	if _, err := gz.Write(data); err != nil {
		gz.Close()
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
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
