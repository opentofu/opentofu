The v1.13.x release series is supported until **August 1 2027**.

## 1.14.0 (Unreleased)

UPGRADE NOTES:

- We are no longer producing official builds for 32-bit CPU architectures (`*_386` and `*_arm` platforms). ([#4530](https://github.com/opentofu/opentofu/pull/4530))

    If you are currently relying on our official releases of OpenTofu on one of these platforms then you will need to migrate to running OpenTofu on a 64-bit CPU architecture (`*_amd64` or `*_arm64` platforms) before upgrading from OpenTofu v1.13.

    Third parties may continue to offer their own OpenTofu builds targeting platforms that we don't officially support. This only affects the official packages published directly by the OpenTofu project in this repository's release artifacts.

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
