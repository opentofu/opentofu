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
	underlying             Source
	mu                     sync.Mutex
	availableVersions      map[addrs.Provider]memoizeAvailableVersionsRet
	availableVersionsLocks map[addrs.Provider]*sync.Mutex
	packageMetas           map[memoizePackageMetaCall]memoizePackageMetaRet
	packageMetasLocks      map[memoizePackageMetaCall]*sync.Mutex
}

type memoizeAvailableVersionsRet struct {
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
	PackageMeta PackageMeta
	Err         error
}

var _ Source = (*MemoizeSource)(nil)

// NewMemoizeSource constructs and returns a new MemoizeSource that wraps
// the given underlying source and memoizes its results.
func NewMemoizeSource(underlying Source) *MemoizeSource {
	return &MemoizeSource{
		underlying:             underlying,
		availableVersionsLocks: make(map[addrs.Provider]*sync.Mutex),
		availableVersions:      make(map[addrs.Provider]memoizeAvailableVersionsRet),
		packageMetas:           make(map[memoizePackageMetaCall]memoizePackageMetaRet),
		packageMetasLocks:      make(map[memoizePackageMetaCall]*sync.Mutex),
	}
}

// AvailableVersions requests the available versions from the underlying source
// and caches them before returning them, or on subsequent calls returns the
// result directly from the cache.
func (s *MemoizeSource) AvailableVersions(ctx context.Context, provider addrs.Provider) (VersionList, Warnings, error) {
	s.mu.Lock()
	if _, ok := s.availableVersionsLocks[provider]; !ok {
		s.availableVersionsLocks[provider] = &sync.Mutex{}
	}
	s.mu.Unlock()

	s.availableVersionsLocks[provider].Lock()
	defer s.availableVersionsLocks[provider].Unlock()

	if existing, exists := s.availableVersions[provider]; exists {
		return existing.VersionList, nil, existing.Err
	}

	ret, warnings, err := s.underlying.AvailableVersions(ctx, provider)
	s.availableVersions[provider] = memoizeAvailableVersionsRet{
		VersionList: ret,
		Err:         err,
		Warnings:    warnings,
	}
	return ret, warnings, err
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

	s.mu.Lock()
	if _, ok := s.packageMetasLocks[key]; !ok {
		s.packageMetasLocks[key] = &sync.Mutex{}
	}
	s.mu.Unlock()

	s.packageMetasLocks[key].Lock()
	defer s.packageMetasLocks[key].Unlock()

	if existing, exists := s.packageMetas[key]; exists {
		return existing.PackageMeta, existing.Err
	}

	ret, err := s.underlying.PackageMeta(ctx, provider, version, target)
	s.packageMetas[key] = memoizePackageMetaRet{
		PackageMeta: ret,
		Err:         err,
	}
	return ret, err
}

func (s *MemoizeSource) ForDisplay(provider addrs.Provider) string {
	return s.underlying.ForDisplay(provider)
}
