// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package registry

import (
	"net/url"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/registry/regsrc"
)

// PackageLocation abstractly represents a location that a module package should
// be installed from.
//
// There are exactly two concrete implementations of this interface:
// [PackageLocationDirect] for packages that are hosted as part of the
// registry they were reported from, and [PackageLocationIndirect] (for
// registry packages that are really just aliases for source addresses that
// someone could've specified directly in their configuration).
//
// This is a closed interface. If any new implementations of it are added in
// future then an exhaustive type-switch over these in the module installer
// will need to be updated to support the new variants.
type PackageLocation interface {
	// UILabel returns a label that can be used to concisely refer to this
	// location in the OpenTofu UI, such as when reporting progress or
	// describing problems in error messages.
	//
	// The result is not necessarily a unique identifier for the location. It's
	// just expected to be something a human reader could use to confirm whether
	// OpenTofu is installing from somewhere reasonable and expected.
	UILabel() string

	// Subdir returns a path to a directory within the package that contains
	// the module that is being selected. Returns an empty string if the root
	// of the package contains the selected module.
	Subdir() string

	// This unexported method means that only types within this package can
	// implement this interface.
	packageLocationSigil()
}

// PackageLocationDirect represents a module package location that's considered
// to be a part of the registry that reported it, and so the same registry
// client that reported this location should also be used to install it.
//
// This type is intentionally opaque, since values of this type should be
// passed directly to [Client.InstallPackage] on the same Client instance
// that returned this value. It encapsulates everything that client would need
// to perform the installation.
type PackageLocationDirect struct {
	// module is the registry module address that the package at this location
	// is intended to satisfy. We track this so that the client can decide
	// which credentials (if any) to use when requesting the package.
	module *regsrc.Module

	// packageURL is the absolute HTTP or HTTPS URL where the module package
	// is located. This URL should respond to a GET request by returning a
	// successful response whose body is either a "zip" archive, or is a
	// "tar" archive using either gz, xz, or bzip2 compression.
	packageURL *url.URL

	// useRegistryCredentials records whether the registry directed OpenTofu
	// to reuse the same credentials that were used to request this location
	// (or, at least, functionally-equivalent credentials) when making a GET
	// request to the URL given in archiveURL.
	//
	// This is used for private registries that wish to protect both metadata
	// and packages using the same credentials. If this is false then the
	// request to archiveURL uses no credentials at all and so that URL must
	// either be willing to serve an anonymous request or some sort of
	// credential information must be packed into the URL itself, such as if
	// using a mechanism like AWS S3's "presigned URLs":
	//     https://docs.aws.amazon.com/AmazonS3/latest/userguide/using-presigned-url.html
	useRegistryCredentials bool
}

var _ PackageLocation = PackageLocationDirect{}

// UILabel implements PackageLocation.
func (p PackageLocationDirect) UILabel() string {
	return p.packageURL.String()
}

// Subdir implements PackageLocation.
func (p PackageLocationDirect) Subdir() string {
	return p.module.RawSubmodule
}

// packageLocationSigil implements PackageLocation.
func (p PackageLocationDirect) packageLocationSigil() {}

// PackageLocationIndirect represents a module package that is accessible
// through a "remote" module source address just like what could be written
// directly in a "source" argument in a module call, and so must be installed
// through the normal remote package fetcher instead of through the registry
// client.
//
// For locations of this type, the registry client that produced it is no longer
// involved after the location has been decided.
type PackageLocationIndirect struct {
	// SourceAddr is the remote source address to install from, which should
	// be treated in an equivalent way to how this address would've been treated
	// if specified directly in a module call's "source" argument.
	SourceAddr addrs.ModuleSourceRemote
}

var _ PackageLocation = PackageLocationIndirect{}

// UILabel implements PackageLocation.
func (p PackageLocationIndirect) UILabel() string {
	return p.SourceAddr.ForDisplay()
}

// Subdir implements PackageLocation.
func (p PackageLocationIndirect) Subdir() string {
	return p.SourceAddr.Subdir
}

// packageLocationSigil implements PackageLocation.
func (p PackageLocationIndirect) packageLocationSigil() {}
