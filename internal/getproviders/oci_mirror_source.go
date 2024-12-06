// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"context"
	"fmt"

	"github.com/apparentlymart/go-versions/versions"
	svchost "github.com/hashicorp/terraform-svchost"
	"github.com/opentofu/libregistry/registryprotocols/ociclient"
	tfaddr "github.com/opentofu/registry-address"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// OCIMirrorSource is a source that reads provider metadata and packages
// from OCI Distribution registries, selected by transforming the
// requested provider source address to an OCI repository address.
type OCIMirrorSource struct {
	client               ociclient.OCIClient
	getRepositoryAddress func(providerAddr addrs.Provider) (OCIRepository, tfdiags.Diagnostics)
}

var _ Source = (*OCIMirrorSource)(nil)

// NewOCIMirrorSource constructs and returns a new OCI mirror source with
// the given repository address generation callback.
func NewOCIMirrorSource(client ociclient.OCIClient, repositoryAddressFunc func(providerAddr addrs.Provider) (OCIRepository, tfdiags.Diagnostics)) *OCIMirrorSource {
	return &OCIMirrorSource{
		client:               client,
		getRepositoryAddress: repositoryAddressFunc,
	}
}

// newOCIMirrorSourceForDirectInstall is used as an implementation detail
// of [DirectSource] when it decides to handle direct installation by
// mapping to an OCI registry.
//
// In this case what we're creating isn't really a "mirror", but shares
// enough behavior with the OCI mirror installation method that we can
// share the implementation but as an internal detail only (subject to
// change in future, if these diverge enough to be worth it).
func newOCIMirrorSourceForDirectInstall(client ociclient.OCIClient, repositoryAddr OCIRepository) *OCIMirrorSource {
	return &OCIMirrorSource{
		client: client,
		getRepositoryAddress: func(_ addrs.Provider) (OCIRepository, tfdiags.Diagnostics) {
			return repositoryAddr, nil
		},
	}
}

// AvailableVersions implements Source.
func (o *OCIMirrorSource) AvailableVersions(ctx context.Context, provider tfaddr.Provider) (versions.List, []string, error) {
	repoAddr, err := o.repositoryAddress(provider)
	if err != nil {
		return nil, nil, err
	}

	refs, warnings, err := o.client.ListReferences(ctx, repoAddr.toClient())
	if err != nil {
		return nil, warnings, fmt.Errorf("failed to list references from OCI repository %s: %w", repoAddr, err)
	}

	var ret versions.List
	for _, ref := range refs {
		// We're only interested in tags whose names are semver-styled version
		// numbers. We'll ignore any other tag, or any reference that isn't a tag.
		tag, isTag := ref.AsTag()
		if !isTag {
			continue
		}
		v, err := versions.ParseVersion(string(tag))
		if err != nil {
			continue
		}
		ret = append(ret, v)
	}

	return ret, warnings, nil
}

// PackageMeta implements Source.
func (o *OCIMirrorSource) PackageMeta(ctx context.Context, provider tfaddr.Provider, version versions.Version, target Platform) (PackageMeta, error) {
	repoAddr, err := o.repositoryAddress(provider)
	if err != nil {
		return PackageMeta{}, err
	}
	tagName := version.String()
	ociAddr := ociclient.OCIAddrWithReference{
		OCIAddr:   repoAddr.toClient(),
		Reference: ociclient.OCIReference(tagName),
	}

	// FIXME: This API was not designed to support warnings from this step, because
	// the main OpenTofu registry protocol only supports whole-provider-level warnings.
	// For now we'll just discard the warnings, but we should alter this API to allow
	// returning them.
	platformSpecificManifestDigest, _, err := o.client.ResolvePlatformImageDigest(
		ctx, ociAddr,

		// OCI and OpenTofu both follow the Go ecosystem's names for operating
		// systems and architectures, so we can just pass these through directly.
		ociclient.WithGOOS(target.OS), ociclient.WithGOARCH(target.Arch),
	)
	if err != nil {
		return PackageMeta{}, fmt.Errorf("failed to get metadata for tag %q in OCI repository %s: %w", tagName, repoAddr, err)
	}

	location := PackageOCIObject{
		repositoryAddr:      repoAddr,
		imageManifestDigest: platformSpecificManifestDigest,
		client:              o.client,
	}

	// FIXME: We should try to find and verify a cosign signature for the selected
	// multi-platform manifest. If that verification fails then we should return
	// an error immediately here and thus not install anything at all. Otherwise
	// we can return a fixed PackageAuthenticationResult describing the outcome.
	//
	// We should also include a set of HashSchemeOCIObject hashes covering each of
	// the platform-specific manifest digests in the multi-platform manifest that
	// we authenticated, since the provider installer can then record them all in
	// the dependency lock file if we indicated that the signature check was
	// successful.
	authentication := NewPrecheckedAuthentication(
		&PackageAuthenticationResult{
			// The location we're returning is itself a manifest digest and so
			// it self-verifies during installation.
			result: verifiedChecksum,
		},
		[]Hash{
			// We'll return the one digest we're selecting as acceptable, but
			// note that this won't actually do anything interesting until we're
			// able to indicate that we verified a signature in the authentication
			// result above, because the installer only trusts the authenticator's
			// additional hashes if it actually authenticated something.
			HashSchemeOCIObject.New(string(platformSpecificManifestDigest)),
		},
	)

	return PackageMeta{
		Provider:       provider,
		Version:        version,
		TargetPlatform: target,

		// TODO: Define an image metadata label that can populate this, so we can give better feedback
		// about unsupported versions in future.
		ProtocolVersions: nil,

		// Filename is synthetic based on the typical naming scheme for provider mirrors, since
		// an OCI image is never manifest as a single file on disk.
		Filename: fmt.Sprintf("terraform-provider-%s_%s_%s_%s.zip", provider.Type, version, target.OS, target.Arch),

		Location:       location,
		Authentication: authentication,
	}, nil
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
	if !validOCIName(ret.Name) {
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
		if !validOCIName(providerAddr.Namespace + "/" + providerAddr.Type) {
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

func (r OCIRepository) String() string {
	return r.Hostname + "/" + r.Name
}

// toClient returns the same repository address expressed as [ociclient.OCIAddr],
// ready to use with the OCI distribution client.
func (r OCIRepository) toClient() ociclient.OCIAddr {
	return ociclient.OCIAddr{
		Registry: ociclient.OCIRegistry(r.Hostname),
		Name:     ociclient.OCIName(r.Name),
	}
}

func validOCIName(given string) bool {
	candidate := ociclient.OCIName(given)
	return candidate.Validate() == nil
}
