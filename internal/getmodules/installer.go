// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getmodules

import (
	"context"
	"fmt"
)

// PackageFetcher is a low-level utility for fetching remote module packages
// into local filesystem directories in preparation for use by higher-level
// module installer functionality implemented elsewhere.
//
// A PackageFetcher works only with entire module packages and never with
// the individual modules within a package.
//
// A particular PackageFetcher instance remembers the target directory of
// any successfully-installed package so that it can optimize future calls
// that have the same package address by copying the local directory tree,
// rather than fetching the package from its origin repeatedly. There is
// no way to reset this cache, so a particular PackageFetcher instance should
// live only for the duration of a single initialization process.
type PackageFetcher struct {
	getter reusingGetter
}

// NewPackageFetcher constructs a new [PackageFetcher] that interacts with
// the rest of the system and with the execution environment using the
// given [PackageFetcherEnvironment] implementation.
//
// It's valid to set "env" to nil, but that will make certain module
// package source types unavailable for use and so that concession is
// intended only for use in unit tests.
func NewPackageFetcher(env PackageFetcherEnvironment) *PackageFetcher {
	env = preparePackageFetcherEnvironment(env)
	_ = env // TODO: Actually use this, once we have an OCI Distribution getter
	return &PackageFetcher{
		getter: reusingGetter{},
	}
}

// FetchPackage downloads or otherwise retrieves the filesystem inside the
// package at the given address into the given local installation directory.
//
// packageAddr must be formatted as if it were the result of an
// addrs.ModulePackage.String() call. It's only defined as a raw string here
// because the getmodules package can't import the addrs package due to
// that creating a package dependency cycle.
//
// PackageFetcher only works with entire packages. If the caller is processing
// a module source address which includes a subdirectory portion then the
// caller must resolve that itself, possibly with the help of the
// getmodules.SplitPackageSubdir and getmodules.ExpandSubdirGlobs functions.
func (f *PackageFetcher) FetchPackage(ctx context.Context, instDir string, packageAddr string) error {
	return f.getter.getWithGoGetter(ctx, instDir, packageAddr)
}

// PackageFetcherEnvironment is an interface used with [NewPackageFetcher]
// to allow the caller to define how the package fetcher should interact
// with the rest of OpenTofu and with OpenTofu's execution environment.
//
// This interface may grow to include new methods if we learn of new
// requirements, so implementations of this interface should all be
// inside this codebase. If we decide to factor out module installation
// to a separate library for broader use in future then we should consider
// carefully whether a single interface aggregating all of the fetcher's
// concerns is still the best design for that different context.
type PackageFetcherEnvironment interface {
	OCIRepositoryStore(ctx context.Context, registryDomainName, repositoryPath string) (OCIRepositoryStore, error)
}

// preparePackageFetcherEnvironment takes a [PackageFetcherEnvironment]
// value provided directly by a caller to [NewPackageFetcher] and
// optionally wraps or replaces it as needed to better suit the
// implementation details of this package.
//
// Currently its only special behavior is that it replaces a nil
// PackageFetcherEnvironment with a default implementation that stubs
// out all of the methods, so that the other code in this package can
// then assume that the environment is always valid to call.
func preparePackageFetcherEnvironment(given PackageFetcherEnvironment) PackageFetcherEnvironment {
	if given == nil {
		return noopPackageFetcherEnvironment{}
	}
	return given
}

type noopPackageFetcherEnvironment struct{}

// OCIRepositoryStore implements PackageFetcherEnvironment.
func (n noopPackageFetcherEnvironment) OCIRepositoryStore(ctx context.Context, registryDomainName string, repositoryPath string) (OCIRepositoryStore, error) {
	return nil, fmt.Errorf("module installation from OCI repositories is not available in this context")
}
