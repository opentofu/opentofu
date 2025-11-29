// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package traceattrs

import (
	"go.opentelemetry.io/otel/attribute"
)

// This file contains some functions representing OpenTofu-specific semantic
// conventions, which we use alongside the general OpenTelemetry-specified
// semantic conventions.
//
// These functions tend to take strings that are expected to be the canonical
// string representation of some more specific type from elsewhere in OpenTofu,
// but we make the caller produce the string representation rather than doing it
// inline because this package needs to avoid importing any other packages
// from this codebase so that the rest of OpenTofu can use this package without
// creating import cycles.
//
// We only create functions in here for attribute names that we want to use
// consistently across many different callers. For one-off attribute names that
// are only used in a single kind of span, use the generic functions like
// [String], [StringSlice], etc, instead.

// OpenTofuProviderAddress returns an attribute definition for indicating
// which provider is relevant to a particular trace span.
//
// The given address should be the result of calling [addrs.Provider.String].
func OpenTofuProviderAddress(addr string) attribute.KeyValue {
	return attribute.String("opentofu.provider.address", addr)
}

// OpenTofuProviderVersion returns an attribute definition for indicating
// which version of a provider is relevant to a particular trace span.
//
// The given address should be the result of calling
// [getproviders.Version.String]. This should typically be used alongside
// [OpenTofuProviderAddress] to indicate which provider the version number is
// for.
func OpenTofuProviderVersion(v string) attribute.KeyValue {
	return attribute.String("opentofu.provider.version", v)
}

// OpenTofuTargetPlatform returns an attribute definition for indicating
// which target platform is relevant to a particular trace span.
//
// The given address should be the result of calling
// [getproviders.Platform.String].
func OpenTofuTargetPlatform(platform string) attribute.KeyValue {
	return attribute.String("opentofu.target_platform", platform)
}

// OpenTofuModuleCallName returns an attribute definition for indicating
// the name of a module call that's relevant to a particular trace span.
//
// The given address should be something that would be valid in the
// [addrs.ModuleCall.Name] field.
func OpenTofuModuleCallName(name string) attribute.KeyValue {
	return attribute.String("opentofu.module.name", name)
}

// OpenTofuModuleSource returns an attribute definition for indicating
// which module source address is relevant to a particular trace span.
//
// The given address should be the result of calling
// [addrs.ModuleSource.String], or any other syntax-compatible representation.
func OpenTofuModuleSource(addr string) attribute.KeyValue {
	return attribute.String("opentofu.module.source", addr)
}

// OpenTofuModuleVersion returns an attribute definition for indicating
// which version of a module is relevant to a particular trace span.
//
// The given address should be either the result of calling
// [getproviders.Version.String], or the String method from the "Version" type
// from HashiCorp's "go-version" library.
func OpenTofuModuleVersion(v string) attribute.KeyValue {
	return attribute.String("opentofu.module.version", v)
}

// OpenTofuOCIReferenceTag returns an attribute definition for indicating
// which OCI repository tag is relevant to a particular trace span.
func OpenTofuOCIReferenceTag(name string) attribute.KeyValue {
	return attribute.String("opentofu.oci.reference.tag", name)
}

// OpenTofuOCIReferenceDigest returns an attribute definition for indicating
// which OCI digest reference is relevant to a particular trace span.
func OpenTofuOCIReferenceDigest(digest string) attribute.KeyValue {
	return attribute.String("opentofu.oci.reference.digest", digest)
}

// OpenTofuOCIManifestMediaType returns an attribute definition for indicating
// which OCI manifest media type is relevant to a particular trace span.
func OpenTofuOCIManifestMediaType(typ string) attribute.KeyValue {
	return attribute.String("opentofu.oci.manifest.media_type", typ)
}

// OpenTofuOCIManifestArtifactType returns an attribute definition for indicating
// which OCI manifest artifact type is relevant to a particular trace span.
func OpenTofuOCIManifestArtifactType(typ string) attribute.KeyValue {
	return attribute.String("opentofu.oci.manifest.artifact_type", typ)
}

// OpenTofuOCIManifestSize returns an attribute definition for indicating
// the size in bytes of an OCI manifest that is relevant to a particular
// trace span.
func OpenTofuOCIManifestSize(size int64) attribute.KeyValue {
	return attribute.Int64("opentofu.oci.manifest.size", size)
}

// OpenTofuOCIBlobDigest returns an attribute definition for indicating
// which OCI blob digest is relevant to a particular trace span.
func OpenTofuOCIBlobDigest(digest string) attribute.KeyValue {
	return attribute.String("opentofu.oci.blob.digest", digest)
}

// OpenTofuOCIBlobMediaType returns an attribute definition for indicating
// which OCI blob media type is relevant to a particular trace span.
func OpenTofuOCIBlobMediaType(typ string) attribute.KeyValue {
	return attribute.String("opentofu.oci.blob.media_type", typ)
}

// OpenTofuOCIBlobArtifactType returns an attribute definition for indicating
// which OCI blob artifact type is relevant to a particular trace span.
func OpenTofuOCIBlobArtifactType(typ string) attribute.KeyValue {
	return attribute.String("opentofu.oci.blob.artifact_type", typ)
}

// OpenTofuOCIBlobSize returns an attribute definition for indicating
// the size in bytes of an OCI blob that is relevant to a particular
// trace span.
func OpenTofuOCIBlobSize(size int64) attribute.KeyValue {
	return attribute.Int64("opentofu.oci.blob.size", size)
}

// OpenTofuOCIRegistryDomain returns an attribute definition for indicating
// which OCI registry domain name is relevant to a particular trace span.
func OpenTofuOCIRegistryDomain(domain string) attribute.KeyValue {
	return attribute.String("opentofu.oci.registry.domain", domain)
}

// OpenTofuOCIRepositoryName returns an attribute definition for indicating
// which OCI repository is relevant to a particular trace span.
//
// The value of this should not include the registry domain name. Use a
// separate attribute built from [OpenTofuOCIRegistryDomain] for that.
func OpenTofuOCIRepositoryName(name string) attribute.KeyValue {
	return attribute.String("opentofu.oci.repository.name", name)
}
