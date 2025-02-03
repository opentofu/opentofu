⚠⚠⚠ Work in progress: This file still needs to be adapted to the new RFC.

# Common questions for providers and modules in OCI

> [!NOTE]
> This document is part of the [OCI registries RFC](../20241206-oci-registries.md).

## Authentication

OpenTofu today supports defining a [`credentials` block in your .tofurc](https://opentofu.org/docs/cli/config/config-file/#credentials). This block allows you to define access tokens for services related to a hostname.

With OCI, this credentials block will be extended to allow for a `username` and `password` parameter as well, which will be supported for OCI hosts:

```hcl
credentials "ghcr.io" {
  username = "your_username"
  password = "your_password"
}
```

OpenTofu credential helpers will also be supported for this purpose. In addition, OpenTofu will also support using any Docker or Podman credentials or credential helpers configured if you configure one of the following option:

```hcl
oci {
  use_docker_credentials = true
  use_podman_credentials = true
}
```

TODO how does this interact with `tofu login`?