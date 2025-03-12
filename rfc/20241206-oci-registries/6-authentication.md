# Authentication configuration

---

This document is part of the [OCI registries RFC](../20241206-oci-registries.md).

| [« Previous](5-modules.md) | [Up](../20241206-oci-registries.md) | [Next »](7-open-questions.md) |

---

OpenTofu today supports defining a [`credentials` block in your CLI Configuration file](https://opentofu.org/docs/cli/config/config-file/#credentials). This block allows you to define access tokens for OpenTofu-native services offered at a specific hostname.

OCI registries are not an OpenTofu-native service and so have different expectations for authentication. In particular, it's common for registries to support username/password-style authentication instead of (or in addition to) bearer-token-style approaches like OAuth.

When dealing with OCI registries, OpenTofu will by default attempt an anonymous authentication against any registries that indicate that authentication is required. Additional authentication configuration can be taken either from the configuration files managed by tools like Docker CLI and Podman, or from new block types in the OpenTofu CLI configuration language.

## Automatic Discovery of Ambient Credentials

For those who already use OCI registries with other software, it's likely that they already have live credentials on their system created by running a command such as `docker login`, `podman login`, or `oras login`. Commands in this ecosystem typically write credentials either to Docker CLI's configuration files or to a vendor-agnostic alternative location using the same configuration file format.

To support reuse of those existing credentials and minimize credentials sprawl, by default OpenTofu will search for credentials as documented in [`containers-auth.json`](https://github.com/containers/image/blob/c30cc7a54783122c0168d8ad77f712c2469c496c/docs/containers-auth.json.5.md), which supports the credentials locations written by all three of the login commands listed above, along with various others in the ecosystem.

Some survey respondants indicated that they would prefer OpenTofu _not_ to automatically discover ambient credentials in this way, and we also wish to support scenarios involving Docker CLI-style configuration files in unconventional locations, and so the CLI configuration will be extended with an optional new block type `oci_default_credentials` which allows customizing how OpenTofu performs this automatic discovery:

```hcl
oci_default_credentials {
  # Setting discover_ambient_credentials to false completely disables
  # all automatic credentials discovery, forcing OpenTofu to use
  # only credentials provided directly in its own CLI configuration
  # (as described in the next section).
  discover_ambient_credentials = false
}
```

```hcl
oci_default_credentials {
  # Setting docker_style_config_files overrides the default search
  # locations for Docker-style configuration files, allowing
  # OpenTofu to interoperate with other tools in the ecosystem
  # that use Docker's file format but a different filepath for
  # the configuration files.
  docker_style_config_files = [
    "/etc/awesome-oci-tool/auth.json",
  ]

  # When this argument is set, the default search locations are
  # disabled: only exactly the listed files will be searched.
}
```

The `oci_default_credentials` block also allows setting a default [Docker-style credential helper](https://github.com/docker/docker-credential-helpers) which, if specified, will be used by OpenTofu for any repository that doesn't have a more specific set of credentials configured elsewhere:

```hcl
oci_default_credentials {
  docker_credentials_helper = "osxkeychain"
}
```

Future versions of OpenTofu might support other kinds of "ambient" credentials. Each additional credential discovery method must have its own setting in the `oci_default_credentials` block that allows it to be individually disabled. If a future version of OpenTofu supports an additional discovery method that an operator wishes to use _instead of_ the Docker-style config files then that operator can disable the Docker-style method by setting `docker_style_config_files` to an empty list, and then configure their chosen alternative method as necessary.

## OpenTofu-specific Explicit Configuration

For those who are using OCI registries _only_ with OpenTofu, or who prefer to separate the credentials used for OpenTofu from those used by other tools, the OpenTofu CLI Configuration language will support a new block type `oci_credentials` which specifies the credentials to use for all OCI Distribution repositories matching a prefix given in the block label:

```hcl
oci_credentials "example.com/foo/bar" {
  # These credentials are for any repository whose path starts with
  # "foo/bar" in the registry "example.com".
  username = "foobar"
  password = "example"
}
```

Each `oci_credentials` block _must_ set all of the arguments in exactly one of the following mutually-exclusive groups:

- `username` and `password`: for registries that use "basic-auth-style" credentials
- `access_token` and `refresh_token`: for registries that use OAuth-style credentials
- `docker_credentials_helper` alone: provides username and password _indirectly_ using a [Docker-style credential helper](https://github.com/docker/docker-credential-helpers)

> [!NOTE]
> Using a separate top-level block for each repository, rather than grouping all of these settings together under a single block, follows the established precedent for OpenTofu-native service authentication configuration.
>
> That design was chosen to support splitting the CLI configuration over multiple files where, for example, each registry host might have its own configuration file managed by a system-wide configuration management system. It's also used by the `tofu login` command, which intentionally writes the credentials it generates to a separate CLI configuration file from the ones intended to be edited directly by the operator.
>
> The configuration management system situation might equally apply to OCI registries. We do not intend to support a `tofu login`-style command for obtaining OCI Registry credentials in the initial release, but we are considering adding support for that in a later release in which case it might write either to the same CLI configuration file that `tofu login` uses today, or to another separate configuration file reserved for automatically-obtained OCI registry credentials.

## Credentials Selection Precedence

As an extension of the credentials-matching rules used in the Docker CLI configuration format, OpenTofu will select credentials for a particular registry by searching across both the ambient and explicitly-configured credentials sources for all entries that match the requested OCI repository.

OpenTofu will then select the most specific match where, for example:

* `example.com/foo/bar` is more specific than `example.com/foo`.
* `example.com/foo` is more specific than just `example.com`.
* Anything involving a matching domain is more specific than a global setting (and only the default Docker-style credentials helper is a "global" setting).

If there is both an explicit credentials configuration and an "ambient" credentials configuration with the same repository address then the explicit `oci_credentials` block takes precedence. The CLI Configuration parser will reject any CLI configuration that includes multiple `oci_credentials` blocks for the same repository prefix. There is no such constraint on the "ambient" credentials, and so OpenTofu will prefer credentials from files earlier in the automatic discovery sequence, or earlier in the `docker_style_config_files` setting.

---

| [« Previous](5-modules.md) | [Up](../20241206-oci-registries.md) | [Next »](7-open-questions.md) |

---
