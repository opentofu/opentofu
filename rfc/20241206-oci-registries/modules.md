⚠⚠⚠ Work in progress: This file still needs to be adapted to the new RFC.

# Module installation in OCI

> [!NOTE]
> This document is part of the [OCI registries RFC](../20241206-oci-registries.md).

In contrast to [providers](providers.md), modules already support schemes as part of their addresses. Therefore, modules will be addressable from OCI using the `oci://` prefix in OpenTofu code directly. For example, you will be able to address an AWS ECR registry like this:

```hcl
module "foo" {
  source="oci://AWS_ACCOUNT_ID.dkr.ecr.REGION.amazonaws.com/REPOSITORY"
}
```

By default, this will load the `latest` tag. If you would like to use specific version tags, you can specify the version separately:

```hcl
module "foo" {
  source  = "oci://AWS_ACCOUNT_ID.dkr.ecr.REGION.amazonaws.com/REPOSITORY"
  version = "1.1.0"
}
```

⚠ TODO: do we want to follow the git schema here or the version tag?

## Publishing a module

Modules for OpenTofu are stored in OCI as standard **non-multiarch** container images. If OpenTofu encounters a multiarch container image, it will refuse to use it as a module. Also make sure to add the label of `org.opentofu.package-type=modules` to the image. Without this label OpenTofu will refuse to use the image as a provider image.

The easiest way to produce a compliant image is:

1. Create a `FROM scratch` container image.
2. Add the `org.opentofu.package-type=modules` label.
3. Copy all files of your module into the root directory.

