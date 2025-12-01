// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"context"
	"sync"

	"github.com/opentofu/opentofu/internal/addrs"
)

// MemoizeSource is a Source that wraps another Source and remembers its
// results so that they can be returned more quickly on future calls to the
// same object.
//
// Each MemoizeSource maintains a cache of response it has seen as part of its
// body. All responses are retained for the remaining lifetime of the object.
// Errors from the underlying source are also cached, and so subsequent calls
// with the same arguments will always produce the same errors.
//
// A MemoizeSource can be called concurrently, with incoming requests processed
// sequentially.
type MemoizeSource struct {
	underlying        Source
	mu                sync.Mutex
	availableVersions map[addrs.Provider]*memoizeAvailableVersionsRet
	packageMetas      map[memoizePackageMetaCall]*memoizePackageMetaRet
}

type memoizeAvailableVersionsRet struct {
	sync.Mutex
	VersionList VersionList
	Warnings    Warnings
	Err         error
}

type memoizePackageMetaCall struct {
	Provider addrs.Provider
	Version  Version
	Target   Platform
}

type memoizePackageMetaRet struct {
	sync.Mutex
	PackageMeta PackageMeta
	Err         error
}

var _ Source = (*MemoizeSource)(nil)

// NewMemoizeSource constructs and returns a new MemoizeSource that wraps
// the given underlying source and memoizes its results.
func NewMemoizeSource(underlying Source) *MemoizeSource {
	return &MemoizeSource{
		underlying:        underlying,
		availableVersions: make(map[addrs.Provider]*memoizeAvailableVersionsRet),
		packageMetas:      make(map[memoizePackageMetaCall]*memoizePackageMetaRet),
	}
}

// AvailableVersions requests the available versions from the underlying source
// and caches them before returning them, or on subsequent calls returns the
// result directly from the cache.
func (s *MemoizeSource) AvailableVersions(ctx context.Context, provider addrs.Provider) (VersionList, Warnings, error) {
	shouldComputeAvailableVersions := false

	s.mu.Lock()

	entry, exists := s.availableVersions[provider]
	if !exists {
		// Add entry to the map
		entry = &memoizeAvailableVersionsRet{}
		s.availableVersions[provider] = entry

		// We are now responsible for computing the entry
		shouldComputeAvailableVersions = true
		// Take the lock early to prevent anyone else from holding it
		entry.Lock()
		defer entry.Unlock()
	}

	s.mu.Unlock()

	if shouldComputeAvailableVersions {
		// Compute result, we already have the lock from above
		entry.VersionList, entry.Warnings, entry.Err = s.underlying.AvailableVersions(ctx, provider)
	} else {
		// Wait for the result to be available
		entry.Lock()
		defer entry.Unlock()
	}

	return entry.VersionList, entry.Warnings, entry.Err
}

// PackageMeta requests package metadata from the underlying source and caches
// the result before returning it, or on subsequent calls returns the result
// directly from the cache.
func (s *MemoizeSource) PackageMeta(ctx context.Context, provider addrs.Provider, version Version, target Platform) (PackageMeta, error) {
	key := memoizePackageMetaCall{
		Provider: provider,
		Version:  version,
		Target:   target,
	}

	shouldComputePackageMeta := false

	s.mu.Lock()

	entry, exists := s.packageMetas[key]
	if !exists {
		// Add entry to the map
		entry = &memoizePackageMetaRet{}
		s.packageMetas[key] = entry

		// We are now responsible for computing the entry
		shouldComputePackageMeta = true
		// Take the lock early to prevent anyone else from holding it
		entry.Lock()
		defer entry.Unlock()
	}

	s.mu.Unlock()

	if shouldComputePackageMeta {
		// Compute result, we already have the lock from above
		entry.PackageMeta, entry.Err = s.underlying.PackageMeta(ctx, provider, version, target)
	} else {
		// Wait for the result to be available
		entry.Lock()
		defer entry.Unlock()
	}

	return entry.PackageMeta, entry.Err
}

func (s *MemoizeSource) ForDisplay(provider addrs.Provider) string {
	return s.underlying.ForDisplay(provider)
}
