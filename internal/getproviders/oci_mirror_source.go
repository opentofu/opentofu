// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"context"
	"fmt"

	"github.com/apparentlymart/go-versions/versions"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
	tfaddr "github.com/opentofu/registry-address"
)

// OCIMirrorSource is a source that reads provider metadata and packages
// from OCI Distribution registries, selected by transforming the
// requested provider source address to an OCI repository address.
type OCIMirrorSource struct {
	getRepositoryAddress func(providerAddr addrs.Provider) (string, tfdiags.Diagnostics)
}

var _ Source = (*OCIMirrorSource)(nil)

// NewOCIMirrorSource constructs and returns a new OCI mirror source with
// the given repository address generation callback.
func NewOCIMirrorSource(repositoryAddressFunc func(providerAddr addrs.Provider) (string, tfdiags.Diagnostics)) *OCIMirrorSource {
	return &OCIMirrorSource{
		getRepositoryAddress: repositoryAddressFunc,
	}
}

// AvailableVersions implements Source.
func (o *OCIMirrorSource) AvailableVersions(_ context.Context, provider tfaddr.Provider) (versions.List, []string, error) {
	repoAddr, err := o.repositoryAddress(provider)
	if err != nil {
		return nil, nil, err
	}

	// TODO: Implement this, once we have an OCI distribution client to implement it with.
	return nil, nil, fmt.Errorf(
		"would have listed available provider versions from %s, but this provider installation method is not yet implemented",
		repoAddr,
	)
}

// PackageMeta implements Source.
func (o *OCIMirrorSource) PackageMeta(_ context.Context, provider tfaddr.Provider, version versions.Version, target Platform) (PackageMeta, error) {
	repoAddr, err := o.repositoryAddress(provider)
	if err != nil {
		return PackageMeta{}, err
	}
	tagName := "v" + version.String()

	// TODO: Implement this, once we have an OCI distribution client to implement it with.
	return PackageMeta{}, fmt.Errorf(
		"would have fetched metadata from %s:%s for %s, but this provider installation method is not yet implemented",
		repoAddr, tagName, target,
	)
}

// ForDisplay implements Source.
func (o *OCIMirrorSource) ForDisplay(provider tfaddr.Provider) string {
	return fmt.Sprintf("%s from an OCI repository", provider)
}

// repositoryAddress calls the getRepositoryAddress callback and then performs some
// postprocessing on its result to make it more convenient to use in the other
// methods of this type.
func (o *OCIMirrorSource) repositoryAddress(providerAddr addrs.Provider) (string, error) {
	ret, diags := o.getRepositoryAddress(providerAddr)
	if diags.HasErrors() {
		// This is an awkward situation where we have a diagnostics-based
		// API wrapped inside an error-based API, and so we need to
		// compromise and adapt the diagnostics into an error object.
		// This means that if the function returns only warnings and
		// no errors then those warnings will be lost. :(
		return "", diags.Err()
	}

	return ret, nil
}
