# Provider installation implementation details

---

This document is part of the [OCI registries RFC](../20241206-oci-registries.md).

| [« Previous](8-auth-implementation-details.md) | [Up](../20241206-oci-registries.md) | [Next »](10-module-implementation-details.md) |

---

This appendix discusses implementation details related to [installing provider packages from OCI registries](4-providers.md).

## OCI Mirror Provider Installation Source

Each installation method block type that OpenTofu accepts in [the CLI Configuration's `provider_installation` block](https://opentofu.org/docs/cli/config/config-file/#provider-installation) corresponds to an implementation of [`getproviders.Source`](https://pkg.go.dev/github.com/opentofu/opentofu/internal/getproviders#Source). At the time of writing, we have:

* `direct` corresponding to [`getproviders.RegistrySource`](https://pkg.go.dev/github.com/opentofu/opentofu/internal/getproviders#RegistrySource)
* `filesystem_mirror` corresponding to [`getproviders.FilesystemMirrorSource`](https://pkg.go.dev/github.com/opentofu/opentofu/internal/getproviders#FilesystemMirrorSource)
* `network_mirror` corresponding to [`getproviders.HTTPMirrorSource`](https://pkg.go.dev/github.com/opentofu/opentofu/internal/getproviders#HTTPMirrorSource)

This project will therefore introduce `getproviders.OCIMirrorSource` to correspond with the new `oci_mirror` installation method. This particular one needs to be configured with a means to obtain an ORAS-Go OCI Distribution registry client object and with a function that takes an `addrs.Provider` (our internal representation of a provider source address) and returns an OCI registry domain name and a repository path:

```go
func NewOCIMirrorSource(
    getRepositoryAddress(addrs.Provider) (registryDomainName, repositoryPath string, err error),
    getRegistryClient func(ctx context.Context, registryDomainName, repositoryPath string) (*orasregistry.Registry, error),
) *OCIMirrorSource
```

As usual, `package main` (in `provider_installation.go`) is responsible for providing the concrete implementations of these two functions so that we can follow the dependency-inversion principle, where:

- `getRepositoryAddress` will actually delegate to a function provided in `package cliconfig` for evaluating the `repository_template` argument from the CLI configuration as a HCL-style [string template](https://opentofu.org/docs/language/expressions/strings/#string-templates).
- `getRegistryClient` will first use the `ociauthconfig.CredentialsConfigs` object produced by `package cliconfig` (refer to [OCI Registry Credentials Policy Layer](9-auth-implementation-details.md#oci-registry-credentials-policy-layer)) to select suitable credentials (if any) for the requested repository address, and then use those credentials to instantiate [the ORAS library's registry client type](https://pkg.go.dev/oras.land/oras-go/v2@v2.5.0/registry/remote#Registry).

The registry client type can then in turn produce [a repository-specific client](https://pkg.go.dev/oras.land/oras-go/v2@v2.5.0/registry#Repository), which provides the functions that we'll need for our `getproviders.Source` implementation:

- enumerate all of the available tags ([`Tags`](https://pkg.go.dev/oras.land/oras-go/v2@v2.5.0/registry#Tags))
- fetch manifests ([`ManifestStore`](https://pkg.go.dev/oras.land/oras-go/v2@v2.5.0/registry#ManifestStore))
- fetch blobs ([`BlobStore`](https://pkg.go.dev/oras.land/oras-go/v2@v2.5.0/registry#BlobStore))

We expect that the implementation of `OCIMirrorSource` will be very similar to the existing `HTTPMirrorSource`, but will of course use the OCI Distribution protocol when making requests instead of OpenTofu's own "network mirror protocol".

> [!NOTE]
> The above signature for `NewOCIMirrorSource` is intended to be illustrative rather than concrete. In practice it may end up taking a single argument that is an implementation of some interface with two methods similar to the function arguments shown, but that's a level of design detail we will consider during the implementation phase, with consideration to whether it turns out to be shareable with the similar dependency inversion design we'll be using for the module installer.

## Package checksums and signing

OpenTofu tracks in its [dependency lock file](https://opentofu.org/docs/language/files/dependency-lock/) the single selected version of each previously-installed provider, and a set of checksums that are considered acceptable for that version of that provider.

Additionally, whenever possible OpenTofu retrieves a GPG signature and associated public key for a provider (with both decided by its origin registry) and uses that signature to verify that the fetched provider package matches one of the checksums that was covered by the GPG signature. If that verification succeeds, OpenTofu assumes that all other checksums covered by the same signature are also trusted, and so records _all_ of them in the dependency lock file for future use. This mechanism is important to ensure that the dependency lock file ends up recording checksums for _all_ platform-specific packages offered for that provider version, even though OpenTofu only downloads and verifies the package for the current platform.

OpenTofu's existing provider registry protocol always uses `.zip` archives as provider packages and requires the provider developer's signature to cover a document containing SHA256 checksums of all of the `.zip` archives for a particular version. These translate into `zh:`-schemed checksums in the dependency lock file, but since those checksums can only be verified against a not-yet-unpacked `.zip` archive OpenTofu also generates a more general `h1:` checksum based on the _contents_ of the package it downloaded, which it can then use to verify already-unpacked provider packages in the local filesystem.

[Our OCI artifact layout for provider packages](4-providers.md#storage-in-oci) intentionally uses `archive/zip` layers so that we can use byte-for-byte identical copies of a provider developer's signed `.zip` packages as the layer blobs, and therefore we can directly translate the `sha256:` digest of each layer into a `zh:`-style checksum for dependency locking purposes, without having to download the package and recalculate the checksum locally.

We are not intending to support OCI artifact signing in our first implementation, since we are focusing initially only on the "OCI mirror" use-case where the mirror is assumed to be trusted by whoever configured it. Without any signatures, we'll capture in the dependency lock file only the checksums (both `h1:` and `zh:`) of the specific artifact we downloaded, for consistency with the guarantees we make from installing from unsigned sources in today's OpenTofu. When we later add support for optionally signing the index manifest, we can begin using those signatures to justify including the `zh:` checksums from _all_ of the per-platform artifacts, announcing the ID of the signing key as part of the `tofu init` output just as we do for registry-distributed signatures today. It would remain the operator's responsibility to verify that the key ID is one they expected before committing the new checksums in the dependency lock file.

As with OpenTofu's current provider registry protocol, an OCI provider artifact manifest cannot provide us any trustworthy representation of the `h1:` checksum of a provider package, and so OpenTofu will calculate that locally based on the already-downloaded package(s) only if the artifact also matches one of the previously-discovered `zh:` checksums, as usual. Operators will be able to use the existing `tofu providers lock` command to force OpenTofu to calculate and record `h1:` checksums for other platforms if desired, just as is possible today with all of the existing provider installation methods.

> [!NOTE]
> The `tofu providers lock` command by default ignores the operator's configured provider installation methods and instead always attempts to install providers from their origin registries. This default is to support the workflow where `tofu providers lock` is used to obtain the checksums of the "official" releases from the origin registry and then subsequent `tofu init` against a mirror source can verify that the mirrored packages match those official release packages without needing to directly access the origin registry.
>
> There are `-net-mirror` and `-fs-mirror` command line options that effectively substitute for the similarly-named installation method blocks in the CLI Configuration, to handle the less-common situation where a particular provider is _only_ available from a mirror. In a future release we ought to add a similar `-oci-mirror` option to restore parity with the full set of installation methods, but we'll exclude that from the initial release just to limit scope and so that we will have fewer backward-compatibility constraints if we need to make subsequent changes in response to feedback.
>
> At some point during the first phase of development of these features we will create a separate feature request issue to represent the addition of an `-oci-mirror` option to the `tofu providers lock` command, which will presumably take as an argument a repository address template string similar to the one used in the `repository_template` argument in an `oci_mirror` installation method configuration block.

---

| [« Previous](8-auth-implementation-details.md) | [Up](../20241206-oci-registries.md) | [Next »](10-module-implementation-details.md) |

---
