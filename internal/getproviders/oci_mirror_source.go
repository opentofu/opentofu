// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"context"
	"fmt"
	"regexp"

	"github.com/apparentlymart/go-versions/versions"
	svchost "github.com/hashicorp/terraform-svchost"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
	tfaddr "github.com/opentofu/registry-address"
)

// OCIMirrorSource is a source that reads provider metadata and packages
// from OCI Distribution registries, selected by transforming the
// requested provider source address to an OCI repository address.
type OCIMirrorSource struct {
	getRepositoryAddress func(providerAddr addrs.Provider) (OCIRepository, tfdiags.Diagnostics)
}

var _ Source = (*OCIMirrorSource)(nil)

// NewOCIMirrorSource constructs and returns a new OCI mirror source with
// the given repository address generation callback.
func NewOCIMirrorSource(repositoryAddressFunc func(providerAddr addrs.Provider) (OCIRepository, tfdiags.Diagnostics)) *OCIMirrorSource {
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
func (o *OCIMirrorSource) repositoryAddress(providerAddr addrs.Provider) (OCIRepository, error) {
	ret, diags := o.getRepositoryAddress(providerAddr)
	if diags.HasErrors() {
		// This is an awkward situation where we have a diagnostics-based
		// API wrapped inside an error-based API, and so we need to
		// compromise and adapt the diagnostics into an error object.
		// This means that if the function returns only warnings and
		// no errors then those warnings will be lost. :(
		return OCIRepository{}, diags.Err()
	}

	if !svchost.IsValid(ret.Hostname) {
		return ret, fmt.Errorf("an OCI mirror's translation template transformed provider address %q into an OCI distribution registry address with invalid hostname %q", providerAddr, ret.Hostname)
	}

	// The result must be a valid name as defined in the OCI distribution specification.
	if !ociDistributionNamePattern.MatchString(ret.Name) {
		// OpenTofu's provider address syntax permits a wider repertiore of
		// Unicode characters than the OCI distribution name pattern allows,
		// so a likely way to get here is to try to install a provider
		// whose namespace or name includes non-ASCII characters. We'll
		// return a special error message for that case.
		//
		// Aside from the character repertiore the OpenTofu provider address
		// syntax is compatible enough with the OCI repository address syntax
		// that we can use a string representation of the provider address
		// with the same regular expression pattern.
		if !ociDistributionNamePattern.MatchString(providerAddr.Namespace + "/" + providerAddr.Type) {
			// (TODO: Should the CLI configuration template for this offer some
			// functions to help users transform a non-ASCII provider address
			// segment into a reasonable ASCII equivalent? Non-ASCII provider
			// namespaces and types are pretty rare, so we'll wait to see if
			// that's needed.)
			return ret, fmt.Errorf("requested provider address %q contains characters that are not valid in an OCI distribution repository name, so this provider cannot be installed from an OCI repository as %q", providerAddr, ret.Name)
		}
		// We'd get here if the invalidity was caused by a literal part
		// of the template, regardless of the given provider address
		// components.
		return ret, fmt.Errorf("an OCI mirror's translation template transformed provider address %q into OCI distribution registry name %q, which is not valid OCI address syntax", providerAddr, ret.Name)
	}

	return ret, nil
}

// OCIRepository represents an OCI Distribution repository address.
type OCIRepository struct {
	Hostname string
	Name     string
}

// ociDistributionNamePattern is a compiled regular expression pattern corresponding to the
// "name" pattern as defined in the OCI distribution specification.
var ociDistributionNamePattern = regexp.MustCompile(`^[a-z0-9]+((\.|_|__|-+)[a-z0-9]+)*(\/[a-z0-9]+((\.|_|__|-+)[a-z0-9]+)*)*$`)

func (r OCIRepository) String() string {
	return r.Hostname + "/" + r.Name
}
