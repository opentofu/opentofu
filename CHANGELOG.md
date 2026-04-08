The v1.13.x release series is supported until **August 1 2027**.

## 1.13.0 (Unreleased)

UPGRADE NOTES:

- The "winrm" connection type for provisioners is no longer supported. ([#4012](https://github.com/opentofu/opentofu/pull/4012))

    This connection type was deprecated in OpenTofu v1.12, and now removed in v1.13. Some of the upstream libraries OpenTofu was using to implement these features are no longer maintained, so it's not viable for us to offer this anymore.

    [Modern Windows versions now support OpenSSH](https://learn.microsoft.com/en-us/windows-server/administration/openssh/openssh_install_firstuse), and so we suggest that anyone currently relying on WinRM plan to migrate to using SSH instead.

ENHANCEMENTS:

- The `local-exec` provisioner now automatically sets the `TRACEPARENT` environment variable in child processes when OpenTelemetry tracing is active, following the W3C Trace Context specification. ([#4014](https://github.com/opentofu/opentofu/issues/4014))

BUG FIXES:

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
