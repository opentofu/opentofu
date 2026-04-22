The v1.13.x release series is supported until **August 1 2027**.

## 1.13.0 (Unreleased)

UPGRADE NOTES:

- The "winrm" connection type for provisioners is no longer supported. ([#4012](https://github.com/opentofu/opentofu/pull/4012))

    This connection type was deprecated in OpenTofu v1.12, and now removed in v1.13. Some of the upstream libraries OpenTofu was using to implement these features are no longer maintained, so it's not viable for us to offer this anymore.

    [Modern Windows versions now support OpenSSH](https://learn.microsoft.com/en-us/windows-server/administration/openssh/openssh_install_firstuse), and so we suggest that anyone currently relying on WinRM plan to migrate to using SSH instead.

ENHANCEMENTS:

- The `cidrsubnets` function now supports prefix extensions greater than 32 bits when the base CIDR block uses an IPv6 address. ([#4042](https://github.com/opentofu/opentofu/pull/4042))
- The `local-exec` provisioner now automatically sets the `TRACEPARENT` environment variable in child processes when OpenTelemetry tracing is active, following the W3C Trace Context specification. ([#4014](https://github.com/opentofu/opentofu/issues/4014))
- Some commands now produce warning diagnostics when execution relied on various workarounds in the Go runtime library ("GODEBUG" settings), encouraging the reader to report any situations where those workarounds are necessary so that we can identify a more sustainable solution, because these workarounds can be removed or change behavior in future versions of Go outside of our control and are not covered by OpenTofu's compatibility promises. ([#4049](https://github.com/opentofu/opentofu/pull/4049))

BUG FIXES:

- The built-in function `contains` now accepts `null` as its second argument, to test whether a collection contains any null values. ([#4043](https://github.com/opentofu/opentofu/issues/4043))
- The built-in function `merge` no longer fails when its only argument is a null value of an object type. ([#4043](https://github.com/opentofu/opentofu/issues/4043))
- provisioner output is no longer suppressed when `-show-sensitive` is passed. ([#3927](https://github.com/opentofu/opentofu/issues/3927))

## Previous Releases

For information on prior major and minor releases, refer to their changelogs:

- [v1.12](https://github.com/opentofu/opentofu/blob/v1.12/CHANGELOG.md)
- [v1.11](https://github.com/opentofu/opentofu/blob/v1.11/CHANGELOG.md)
- [v1.10](https://github.com/opentofu/opentofu/blob/v1.10/CHANGELOG.md)
- [v1.9](https://github.com/opentofu/opentofu/blob/v1.9/CHANGELOG.md)
- [v1.8](https://github.com/opentofu/opentofu/blob/v1.8/CHANGELOG.md)
- [v1.7](https://github.com/opentofu/opentofu/blob/v1.7/CHANGELOG.md)
- [v1.6](https://github.com/opentofu/opentofu/blob/v1.6/CHANGELOG.md)
