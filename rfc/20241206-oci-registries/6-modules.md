
# Module installation in OCI

---

This document is part of the [OCI registries RFC](../20241206-oci-registries.md). Please leave your comments on [this pull request](https://github.com/opentofu/opentofu/pull/2163) as a review.

| [« Previous](5-providers.md) | [Up](../20241206-oci-registries.md) | [Next »](7-authentication.md) |

---

In contrast to [providers](5-providers.md), modules already support schemes as part of their addresses. Therefore, modules will be addressable from OCI using the `oci://` prefix in OpenTofu code directly. For example, you will be able to address an AWS ECR registry like this:

```hcl
module "foo" {
  source = "oci://AWS_ACCOUNT_ID.dkr.ecr.REGION.amazonaws.com/REPOSITORY"
}
```

By default, this will inspect all the tags whose name can be parsed as a semantic versioning-style version number, and select the one with the highest precedence according to [the semantic versioning specification](https://semver.org/). If there are multiple tags that all share the highest precedence, OpenTofu will prefer one without a build identifier segment if available, or will otherwise arbitrarily select the one whose build metadata would have the greatest precedence if treated as a prerelease identifier instead.

> [!NOTE]
> Semantic versioning numbers may contain the `+` sign delimiting a build identifier, such as `1.1.0+something`. That character is not valid in an OCI reference, so OpenTofu will automatically translate the `_` symbol to `+` when attempting to parse a tag name as a version number.

If you preferred to override the automatic selection, or to use a tag whose name does not conform to the semantic versioning syntax at all, you can specify a specific tag using the optional `ref` argument:

```hcl
module "foo" {
  source = "oci://AWS_ACCOUNT_ID.dkr.ecr.REGION.amazonaws.com/REPOSITORY?ref=1.1.0"
}
```

Alternatively, you can specify a specific digest as a reference:

```hcl
module "foo" {
  source = "oci://AWS_ACCOUNT_ID.dkr.ecr.REGION.amazonaws.com/REPOSITORY?ref=sha256:1d57d25084effd3fdfd902eca00020b34b1fb020253b84d7dd471301606015ac"
}
```

> [!NOTE]
> OpenTofu currently doesn't support semantic versioning outside of traditional OpenTofu/Terraform registries. See [this issue](https://github.com/opentofu/opentofu/issues/2495) for details.

By default, OpenTofu expects to find a module in the root of the artifact, but you can optionally specify a subdirectory using [the usual subdirectory syntax](https://opentofu.org/docs/language/modules/sources/#modules-in-package-sub-directories):

```hcl
module "foo" {
  source  = "oci://AWS_ACCOUNT_ID.dkr.ecr.REGION.amazonaws.com/REPOSITORY//dir1/dir2"
}
```

To combine that with the explicit `ref` argument, place the query string after the subdirectory portion: `oci://example.com/repository//directory?ref=example`.

## Artifact layout

Publishing modules in OpenTofu will be performed as a single, non-multi-platform ORAS-style artifact with the `artifactType` attribute as `application/vnd.opentofu.module`. OpenTofu will refuse to use multi-platform artifacts. Specifically:

1. The module must be packaged into a single ZIP file and published as a blob in OCI.
2. The main manifest is an image manifest (not an index manifest) and declares the `artifactType` of `application/vnd.opentofu.module` on the main manifest. The layer must have the `artifactType` of `archive/zip`.
3. Tag names must follow existing versioning rules for modules in the OpenTofu registry. OpenTofu will ignore any incorrectly formatted tags, including `latest`.

## Tooling

The artifact layout described is compatible with [ORAS](https://oras.land/), so you can use ORAS to push modules:

```
oras push \
    --artifact-type application/vnd.opentofu.module \
    ghcr.io/yourname/terraform-your-module \
    terraform-your-module.zip:archive/zip
```

We also intend to provide a tool similar to how [providers work](5-providers.md) that will allow for publishing and mirroring modules. Similar to providers, the mirroring tool will attach detected SBOM and attestation artifacts to the modules in OCI. Specifically, the mirroring tool will detect:

- `*.spdx.json` as `application/spdx+json` containing an SPDX SBOM file.

## OCI-based Modules through an OpenTofu Module Registry

The existing [OpenTofu Module Registry Protocol](https://opentofu.org/docs/internals/module-registry-protocol/) works as a facade over arbitrary remote [module source addresses](https://opentofu.org/docs/language/modules/sources/), and so any OpenTofu version that supports OCI-based module installation will also support module registries that respond to [Download Source Code for a Specific Module Version](https://opentofu.org/docs/internals/module-registry-protocol/#download-source-code-for-a-specific-module-version) with a location property using the `oci://` prefix as described above.

For example, a module registry would be allowed to respond to such a request by returning an address like the following:

```json
{"location":"oci://example.com/repository?digest=sha256:1d57d25084effd3fdfd902eca00020b34b1fb020253b84d7dd471301606015ac"}
```

Such a design would use the module registry protocol to hide the implementation detail that the packages are actually coming from an OCI registry, but at the expense of needing to run an additional OpenTofu-specific module registry service. We do not currently anticipate this being a common need, but it is a natural consequence of the existing module registry protocol design.

> [!WARNING]
> Classic OpenTofu Registry implementations should consider that references to OCI addresses will only work with OpenTofu version 1.10 and up. Older versions will be unable to access modules referenced in such a way.

---

| [« Previous](5-providers.md) | [Up](../20241206-oci-registries.md) | [Next »](7-authentication.md) |

---
