# Provider installation implementation details

---

This document is part of the [OCI registries RFC](../20241206-oci-registries.md). Please leave your comments on [this pull request](https://github.com/opentofu/opentofu/pull/2163) as a review.

| [« Previous](9-auth-implementation-details.md) | [Up](../20241206-oci-registries.md) | [Next »](10-provider-implementation-details.md) |

---

This appendix discusses implementation details related to [installing provider packages from OCI registries](5-providers.md).

> [!WARNING]
> This appendix is still under construction, subject to change based on feedback on the earlier chapters, and may not yet be up-to-date with the latest changes in the earlier chapters.

## OCI Mirror Provider Installation Source

> **TODO:** Describe something like what we did for OCI Mirror sources in [the earlier prototype](https://github.com/opentofu/opentofu/pull/2170), with adjustments for the latest modifications to the proposed artifact layout.
>
> For now we can ignore the parts of that prototype related to treating an OCI registry as if it is an OpenTofu registry directly, since we're not planning to implement that in the first round.

## Package checksums and signing

OpenTofu tracks in its [dependency lock file](https://opentofu.org/docs/language/files/dependency-lock/) the single selected version of each previously-installed provider, and a set of checksums that are considered acceptable for that version of that provider.

Additionally, whenever possible OpenTofu retrieves a GPG signature and associated public key for a provider (with both decided by its origin registry) and uses that signature to verify that the fetched provider package matches one of the checksums that was covered by the GPG signature. If that verification succeeds, OpenTofu assumes that all other checksums covered by the same signature are also trusted, and so records _all_ of them in the dependency lock file for future use. This mechanism is important to ensure that the dependency lock file ends up recording checksums for _all_ platform-specific packages offered for that provider version, even though OpenTofu only downloads and verifies the package for the current platform.

OpenTofu's existing provider registry protocol always uses `.zip` archives as provider packages and requires the provider developer's signature to cover a document containing SHA256 checksums of all of the `.zip` archives for a particular version. These translate into `zh:`-schemed checksums in the dependency lock file, but since those checksums can only be verified against a not-yet-unpacked `.zip` archive OpenTofu also generates a more general `h1:` checksum based on the _contents_ of the package it downloaded, which it can then use to verify already-unpacked provider packages in the local filesystem.

[Our OCI artifact layout for provider packages](5-providers.md#storage-in-oci) intentionally uses `archive/zip` layers so that we can use byte-for-byte identical copies of a provider developer's signed `.zip` packages as the layer blobs, and therefore we can directly translate the `sha256:` digest of each layer into a `zh:`-style checksum for dependency locking purposes, without having to download the package and recalculate the checksum locally.

We are not intending to support OCI artifact signing in our first implementation, since we are focusing initially only on the "OCI mirror" use-case. Without any signatures, we'll capture in the dependency lock file only the checksum of the specific artifact we downloaded, for consistency with the guarantees we make from installing from unsigned sources in today's OpenTofu. When we later add support for optionally signing the index manifest, we can begin using those signatures to justify including the `zh:` checksums from _all_ of the per-platform artifacts, announcing the ID of the signing key as part of the `tofu init` output just as we do for registry-distributed signatures today. It remains the user's responsibility to verify that the key ID is one they expected before committing the new checksums in the dependency lock file.

As with OpenTofu's current provider registry protocol, an OCI provider artifact cannot provide us any trustworthy representation of the `h1:` checksum of a provider package, and so OpenTofu will calculate that locally based on the already-downloaded package(s), assuming that they also match one of the previously-discovered `zh:` checksums, as usual.

---

| [« Previous](9-auth-implementation-details.md) | [Up](../20241206-oci-registries.md) | [Next »](10-provider-implementation-details.md) |

---
