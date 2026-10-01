The v1.14.x release series is supported until **February 1 2028**.

## 1.14.0 (Unreleased)

UPGRADE NOTES:

- We are no longer producing official builds for 32-bit CPU architectures (`*_386` and `*_arm` platforms). ([#4530](https://github.com/opentofu/opentofu/pull/4530))

    If you are currently relying on our official releases of OpenTofu on one of these platforms then you will need to migrate to running OpenTofu on a 64-bit CPU architecture (`*_amd64` or `*_arm64` platforms) before upgrading from OpenTofu v1.13.

    Third parties may continue to offer their own OpenTofu builds targeting platforms that we don't officially support. This only affects the official packages published directly by the OpenTofu project in this repository's release artifacts.

- `provider_meta` blocks in `terraform` blocks are now just silently ignored by OpenTofu, behaving as if they were not declared at all. ([#4600](https://github.com/opentofu/opentofu/pull/4600))

ENHANCEMENTS:

- `tofu plan` no longer prints iterative warnings for multiple resources but instead it shows one warning with all of the affected resources. ([#4201](https://github.com/opentofu/opentofu/issues/4201))
- For provider installation, registry and network mirror sources can now optionally forward authentication credentials to the package download URL if the server explicitly opts in. Previously the package download requests were always made without any credentials. ([#4584](https://github.com/opentofu/opentofu/issues/4584))
- `mock_provider` now supports the `source` argument that can get a file or directory with specific provider overrides ([#4532](https://github.com/opentofu/opentofu/pull/4532))
- The `tofu` cli will now read `*.tfrc` file to load its configuration from system standard directories. ([#4624](https://github.com/opentofu/opentofu/pull/4624))

BUG FIXES:

- `tofu fmt`: Fixed wrong resolution of paths when the working directory is a symlink; output now shows absolute file paths instead of giving error `Invalid file or directory path`. ([#3879](https://github.com/opentofu/opentofu/issues/3879))
- OpenTelemetry tracing support now honors the standard OpenTelemetry environment variable `OTEL_RESOURCE_ATTRIBUTES` for specifying arbitrary additional resource attribute values to be added to traces. Previously we only supported `OTEL_SERVICE_NAME` for overriding the `service.name` attribute. ([#4621](https://github.com/opentofu/opentofu/issues/4621))

## Previous Releases

For information on prior major and minor releases, refer to their changelogs:

- [v1.13](https://github.com/opentofu/opentofu/blob/v1.13/CHANGELOG.md)
- [v1.12](https://github.com/opentofu/opentofu/blob/v1.12/CHANGELOG.md)
- [v1.11](https://github.com/opentofu/opentofu/blob/v1.11/CHANGELOG.md)
- [v1.10](https://github.com/opentofu/opentofu/blob/v1.10/CHANGELOG.md)
- [v1.9](https://github.com/opentofu/opentofu/blob/v1.9/CHANGELOG.md)
- [v1.8](https://github.com/opentofu/opentofu/blob/v1.8/CHANGELOG.md)
- [v1.7](https://github.com/opentofu/opentofu/blob/v1.7/CHANGELOG.md)
- [v1.6](https://github.com/opentofu/opentofu/blob/v1.6/CHANGELOG.md)
