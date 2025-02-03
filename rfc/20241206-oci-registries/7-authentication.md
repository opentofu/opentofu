---

This document is part of the [OCI registries RFC](../20241206-oci-registries.md).

| [« Previous](6-modules.md) | [Up](../20241206-oci-registries.md) | [Next »](8-open-questions.md) |

---

# Authentication configuration

OpenTofu today supports defining a [`credentials` block in your .tofurc](https://opentofu.org/docs/cli/config/config-file/#credentials). This block allows you to define access tokens for services related to a hostname. However, critically, this block does not account for the need of usernames and passwords, which is needed for OCI.

We expect OCI registries used within organizations to need authentication, so we anticipate needing at least a username and a password for this purpose.

When dealing with OCI registries, OpenTofu will, without additional configuration, attempt an anonymous authentication against any registries responding with the authentication required header. Additional authentication configuration can be passed in the `.tofurc` file. However, since the OCI ecosystem is different from OpenTofu's own services, it will not use the `credentials` block.

Instead, a separate top-level `oci{}` block will contain all configuration:

```hcl
oci {
  authentication {
    // Options here
  }
}
```

## Integrated Docker mode

OpenTofu will support reading Docker configuration files, such as `~/.docker/config.json`, directly as requested by 53% of respondents in our survey. However, since 25% of respondents indicated that they want OpenTofu to not read Docker configuration files, this is an option that has to be explicitly enabled. You can do so by setting this option:

```hcl
oci {
  authentication {
    use_docker = true
    # Optional:
    # docker_config_path = "~/.docker/config.json"
  }
}
```

Setting this option will use Docker's stored credentials and configured credential helpers.

## Explicit mode

Alternative to the integrated Docker mode, you can also specify credentials directly in the `.tofurc` file. You can specify credentials directly:

```hcl
oci {
  authentication {
    # Specify credentials explicitly for a host:
    host "ghcr.io" {
      token = "token-here"
      # or:
      username = "your-user"
      password = "your-password"
    }
    # Optional, use Docker cred helper:
    docker_credentials_helper = "/path/to/credentials/helper"
  }
}
```

> [!NOTE]
> If you don't specify a credentials helper, OpenTofu will re-request the access tokens for hosts with username and password, or anonymous auth for every session. OpenTofu does not store the gained access token on the disk unencrypted.

---

| [« Previous](6-modules.md) | [Up](../20241206-oci-registries.md) | [Next »](8-open-questions.md) |

---
