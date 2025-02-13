# Authentication configuration

---

This document is part of the [OCI registries RFC](../20241206-oci-registries.md).  Please leave your comments on [this pull request](https://github.com/opentofu/opentofu/pull/2163) as a review.

| [« Previous](6-modules.md) | [Up](../20241206-oci-registries.md) | [Next »](8-open-questions.md) |

---

OpenTofu today supports defining a [`credentials` block in your .tofurc](https://opentofu.org/docs/cli/config/config-file/#credentials). This block allows you to define access tokens for services related to a hostname. However, critically, this block does not account for the need of usernames and passwords, which is needed for OCI.

We expect OCI registries used within organizations to need authentication, so we anticipate needing at least a username and a password for this purpose.

When dealing with OCI registries, OpenTofu will, without additional configuration, attempt an anonymous authentication against any registries responding with the authentication required header. Additional authentication configuration can be passed in the `.tofurc` file. However, since the OCI ecosystem is different from OpenTofu's own services, it will not use the `credentials` block.

Instead, a separate top-level blocks will contain all configuration:

```hcl
oci_default_credentials {
  use_container_engine_authentication = "off"
  docker_credentials_helper           = "example"
}

oci_credentials "example.com" {
  # These are the default settings for the example.com registry as a whole
  docker_credentials_helper = "osxkeychain"
}

oci_credentials "example.com/foo/bar" {
  # These are special settings used only for repositories under the foo/bar
  # prefix on example.com
  username = "foobar"
  password = "example"
}
```

## Integrated Docker mode

OpenTofu will support reading [Docker configuration files](https://github.com/moby/moby/blob/131e2bf12b2e1b3ee31b628a501f96bbb901f479/cliconfig/config.go#L49), such as `~/.docker/config.json`, directly as requested by 53% of respondents in our survey. However, since 25% of respondents indicated that they want OpenTofu to not read Docker configuration files, this is an option can be disabled. You can do so by setting this option:

```hcl
oci_default_credentials {
  # Use the container engine configuration present on the current device.
  # Defaults to: "auto"
  # Possible values: "auto", "docker", "off
  # Note that currently "auto" and "docker" are equivalent, but this behavior
  # may later change.
  use_container_engine_authentication = "auto"
}

oci_credentials "example.com" {
  # Ignore the container engine configuration for example.com:
  use_container_engine_authentication = "off"
}
```

By default, OpenTofu will default to auto-detecting which container engine is present and use their configuration paths for credential helpers and credential helper configuration. OpenTofu users can disable this functionality by changing `use_container_engine_authentication = "off"`.

## Explicit mode

Alternative to the integrated Docker mode, you can also specify credentials directly in the `.tofurc` file. You can specify credentials directly:

```hcl
oci_credentials "example.com" {
  # Disable reading Docker and other configuration files for this domain:
  use_container_engine_authentication = "off"

  # Authenticate with username and password:
  username = "your-user"
  password = "your-password"

  # Use a Docker-style domain-specific credentials helper:
  docker_credentials_helper = "/path/to/credentials/helper"
}
```

> [!NOTE]
> If you don't specify a credentials helper, OpenTofu will re-request the access tokens for hosts with username and password, or anonymous auth for every session. OpenTofu does not store the gained access token on the disk unencrypted.

---

| [« Previous](6-modules.md) | [Up](../20241206-oci-registries.md) | [Next »](8-open-questions.md) |

---
