
# Module installation in OCI

---

This document is part of the [OCI registries RFC](../20241206-oci-registries.md).

| [« Previous](5-providers.md) | [Up](../20241206-oci-registries.md) | [Next »](7-authentication.md) |

---

In contrast to [providers](5-providers.md), modules already support schemes as part of their addresses. Therefore, modules will be addressable from OCI using the `oci://` prefix in OpenTofu code directly. For example, you will be able to address an AWS ECR registry like this:

```hcl
module "foo" {
  source="oci://AWS_ACCOUNT_ID.dkr.ecr.REGION.amazonaws.com/REPOSITORY"
}
```

By default, this will load the last stable version according to semantic versioning. If you would like to use specific version tags, you can specify the version separately:

```hcl
module "foo" {
  source  = "oci://AWS_ACCOUNT_ID.dkr.ecr.REGION.amazonaws.com/REPOSITORY"
  version = "1.1.0"
}
```

> [!NOTE]
> Module version numbers may contain the `+` sign, such as `1.1.0+something`. This is not a valid OCI reference. OpenTofu will automatically translate the `+` sign to `_` and vice versa when creating or looking for OCI tags.

You can also reference a folder inside a module:

```hcl
module "foo" {
  source  = "oci://AWS_ACCOUNT_ID.dkr.ecr.REGION.amazonaws.com/REPOSITORY//folder1/folder2"
}
```

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

We also intend to provide a tool similar to how [providers work](5-providers.md) that will allow for publishing and mirroring modules.

⚠ TODO: what do we do with SBOM and signature artifacts?

---

| [« Previous](5-providers.md) | [Up](../20241206-oci-registries.md) | [Next »](7-authentication.md) |

---
