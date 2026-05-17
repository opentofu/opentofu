The v1.13.x release series is supported until **August 1 2027**.

## 1.13.0 (Unreleased)

UPGRADE NOTES:

- The "winrm" connection type for provisioners is no longer supported. ([#4012](https://github.com/opentofu/opentofu/pull/4012))

    This connection type was deprecated in OpenTofu v1.12, and now removed in v1.13. Some of the upstream libraries OpenTofu was using to implement these features are no longer maintained, so it's not viable for us to offer this anymore.

    [Modern Windows versions now support OpenSSH](https://learn.microsoft.com/en-us/windows-server/administration/openssh/openssh_install_firstuse), and so we suggest that anyone currently relying on WinRM plan to migrate to using SSH instead.

ENHANCEMENTS:

- The `cidrsubnets` function now supports prefix extensions greater than 32 bits when the base CIDR block uses an IPv6 address. ([#4042](https://github.com/opentofu/opentofu/pull/4042))
- The `local-exec` provisioner now automatically sets the `TRACEPARENT` environment variable in child processes when OpenTelemetry tracing is active, following the W3C Trace Context specification. ([#4014](https://github.com/opentofu/opentofu/issues/4014))
- When installing provider and module packages from OCI Distribution registries, OpenTofu now tracks separate transient credentials for each repository to support registry implementations that issue repository-scoped tokens.  ([#3316](https://github.com/opentofu/opentofu/issues/3316))

BUG FIXES:

- The built-in function `contains` now accepts `null` as its second argument, to test whether a collection contains any null values. ([#4043](https://github.com/opentofu/opentofu/issues/4043))
- The built-in function `merge` no longer fails when its only argument is a null value of an object type. ([#4043](https://github.com/opentofu/opentofu/issues/4043))
- The built-in function `cidrhost` no longer returns a "panic" error when called with an out-of-range host number represented in more than 64 bits. ([#4056](https://github.com/opentofu/opentofu/pull/4056))
- provisioner output is no longer suppressed when `-show-sensitive` is passed. ([#3927](https://github.com/opentofu/opentofu/issues/3927))
- In the `azurerm` backend's OpenID Connect authorization method, when `audience` is provided as a query parameter in the URL, it will be passed through instead of being overwritten by a default value. ([#4037](https://github.com/opentofu/opentofu/pull/4037))

DOCUMENTATION:

- Added DNF as an install option alongside Yum in the RPM-based Linux install guide. ([#1205](https://github.com/opentofu/opentofu/issues/1205))

## Previous Releases

For information on prior major and minor releases, refer to their changelogs:

- [v1.12](https://github.com/opentofu/opentofu/blob/v1.12/CHANGELOG.md)
- [v1.11](https://github.com/opentofu/opentofu/blob/v1.11/CHANGELOG.md)
- [v1.10](https://github.com/opentofu/opentofu/blob/v1.10/CHANGELOG.md)
- [v1.9](https://github.com/opentofu/opentofu/blob/v1.9/CHANGELOG.md)
- [v1.8](https://github.com/opentofu/opentofu/blob/v1.8/CHANGELOG.md)
- [v1.7](https://github.com/opentofu/opentofu/blob/v1.7/CHANGELOG.md)
- [v1.6](https://github.com/opentofu/opentofu/blob/v1.6/CHANGELOG.md)
