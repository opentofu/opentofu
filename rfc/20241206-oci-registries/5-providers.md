# Provider installation in OCI

---

This document is part of the [OCI registries RFC](../20241206-oci-registries.md).

| [« Previous](4-registry-changes.md) | [Up](../20241206-oci-registries.md) | [Next »](6-modules.md) |

---

As stated in [Design Considerations](3-design-considerations.md), in this iteration we will focus on serving the mirroring and private registry use case and leave solving the public OCI registry use case for a later iteration.

## Configuring a provider in OCI

In order to configure OCI as a provider source in OpenTofu, you will have to modify your [.tofurc](https://opentofu.org/docs/cli/config/config-file/) file. Specifically, you will need to add a `provider_installation` block with the following syntax (syntax subject to change):

```hcl
provider_installation {
   oci {
      registry_template = "example.com/examplenet-mirror/${namespace}-${type}"
      include           = ["example.net/*/*"]
   }
   oci {
      registry_template = "example.com/exampleorg-mirror/${namespace}-${type}"
      include           = ["example.org/*/*"]
   }
   direct {
      exclude = ["example.net/*/*", "example.org/*/*"]
   }
}
```

In this case, provider addresses matching `include` blocks would be redirected to the specified OCI registry. The templating here is required because OCI registry addresses work differently to OpenTofu provider addresses and some registries require specific prefixes.

> [!TIP]
> For example, Amazon ECR registries have the format of *aws_account_id*.dkr.ecr.*region*.amazonaws.com/*repository*:*tag*, which does not cleanly map to the OpenTofu provider address understanding of *hostname*/*namespace*/*type*.
> 
> You could work around this by configuring your organizational provider registry as follows:
> 
> ```hcl
> provider_installation {
>   oci {
>     registry_template = "YOUR_AWS_ACCOUNT_ID.dkr.ecr.${namespace}.amazonaws.com/${type}"
>     include           = ["yourcompany.org/*/*"]
>   }
> }
> ```
> 
> Assuming you mirrored the AWS provider into the us-east-1 region, you could now use the following configuration:
> 
> ```hcl
> terraform {
>   required_providers {
>     source  = "yourcompany.org/us-east-1/aws"
>     version = "5.64.0"
>   }
> }
> ```

## Storage in OCI

OpenTofu takes some inspiration from how [ORAS](1-oci-primer.md#oras) stores files, but with a few key differences. It is our hope that [ORAS will soon support multi-platform images](https://github.com/oras-project/oras/issues/1053) and this implementation will be compatible.

1. Each OpenTofu provider OS and architecture (e.g. linux_amd64) will be stored as a ZIP file directly in an OCI blob. OpenTofu will not use tar files as it would be typical for a classic container image.
2. Each provider OS and architecture will have an image manifest with a single layer with the `mediaType` of `archive/zip` and the `org.opencontainers.image.title` annotation containing the original file name of the ZIP file.
3. The main manifest of the image will be an index manifest, containing separate entries for each provider OS and architecture. Additionally, the main manifest must declare the `artifactType` attribute as `application/vnd.opentofu.provider` in order for OpenTofu to accept it as a provider image.
4. The provider artifact must be tagged with the same version number as for the non-OCI use case. OpenTofu will ignore any versions it cannot identify as a semver version number, including the `latest` tag.
5. The index manifest may reference additional artifacts, such as SBOM manifests, under their corresponding MIME types. OpenTofu will ignore any artifacts without a known `artifactType`.

> [!NOTE]
> Provider version numbers may contain the `+` sign, such as `1.1.0+something`. This is not a valid OCI reference. OpenTofu will automatically translate the `+` sign to `_` and vice versa when creating or looking for OCI tags.

When necessary, you can attach artifacts containing signatures or attestations under a separate manifest using the `subject` field in the manifest. This will enable security tools like Trivy to automatically discover, for example, SBOMs.

> [!WARNING]
> As of this RFC, OpenTofu will not use these artifacts directly, but may do so in a later version based on the MIME type. This section describes the files we intend to use for mirroring below, but will not otherwise process in this release.

Manifests attached to a single platform manifest:

- `terraform-provider-YOURNAME_VERSION_PLATFORM_ARCH.zip.gpg` as `application/pgp-signature` containing the GPG Signature of the ZIP file.
- `terraform-provider-YOURNAME_VERSION_PLATFORM_ARCH.spdx.json` as `application/spdx+json` containing an SPDX SBOM file specific to this provider ZIP.
- `terraform-provider-YOURNAME_VERSION_PLATFORM_ARCH.intoto.jsonl` as `application/vnd.in-toto+json` containing an [in-toto attestation framework](https://github.com/in-toto/attestation)/[SLSA Provenance](https://slsa.dev/spec/v1.0/provenance) file for the OS/architecture ZIP.

For the top level (index) manifest:

- `terraform-provider-YOURNAME.spdx.json` as `application/spdx+json` containing an SPDX SBOM file covering all OS/architecture combinations.
- `terraform-provider-YOURNAME.intoto.jsonl` as `application/vnd.in-toto+json` containing an [in-toto attestation framework](https://github.com/in-toto/attestation)/[SLSA Provenance](https://slsa.dev/spec/v1.0/provenance) file covering all OS/architecture combinations.
- `terraform-provider-YOURNAME_SHA256SUMS` as `text/plain`, containing the SHA256 checksums for the individual platform ZIP files. Additionally, `terraform-provider-YOURNAME_SHA256SUMS.gpg` may be attached to the current file as `application/pgp-signature`.

> [!WARNING]
> Provider artifacts in OCI *must* have multi-platform (index) manifests. OpenTofu will refuse to download and use non-multi-platform artifacts as provider manifests. In contrast, [modules](6-modules.md) *must* have non-multi-platform manifests.

## Publishing or mirroring a provider

Currently, there is no third-party tool capable of pushing an OCI artifact in the format we need for this RFC. We hope that ORAS will support this layout, but we do not want to make this proposal dependent on the ORAS implementation. Therefore, we propose that there should be a command line tool, either integrated into OpenTofu or as a standalone tool that allows you to:

1. Publish a set of ZIP files and related artifacts.
2. Directly mirror a provider from an existing OpenTofu registry.

In both cases we expect to find ZIP files that are [correctly named](https://search.opentofu.org/docs/providers/publishing#manually-for-the-adventurous) for providers, which will be published with individual multi-platform manifests. Additionally, the tool will process and upload any files matching file names described above.

The tool will also have a way to hook in external tools (such as [Syft](https://github.com/anchore/syft)) to generate SBOM files at the time of publication or mirroring.

---

| [« Previous](4-registry-changes.md) | [Up](../20241206-oci-registries.md) | [Next »](6-modules.md) |

---
