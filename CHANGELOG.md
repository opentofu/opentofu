## 1.12.0 (Unreleased)

UPGRADE NOTES:

- On Unix systems OpenTofu now considers the `BROWSER` environment variable as a possible override for the default behavior for launching a web browser.

    If you run OpenTofu in a context where an environment variable of that name is already set, it may cause OpenTofu to now open a web browser in a different way than previous versions would have. Unsetting that environment variable will restore the previous platform-specific behavior.

ENHANCEMENTS:

- OpenTofu now uses the `BROWSER` environment variable when launching a web browser on Unix platforms, as long as it's set to a single command that can accept a URL to open as its first and only argument. ([#3456](https://github.com/opentofu/opentofu/issues/3456))

BUG FIXES:

- `for_each` inside `dynamic` blocks can now call provider-defined functions. ([#3429](https://github.com/opentofu/opentofu/issues/3429))

## Previous Releases

For information on prior major and minor releases, refer to their changelogs:

- [v1.11](https://github.com/opentofu/opentofu/blob/v1.11/CHANGELOG.md)
- [v1.10](https://github.com/opentofu/opentofu/blob/v1.10/CHANGELOG.md)
- [v1.9](https://github.com/opentofu/opentofu/blob/v1.9/CHANGELOG.md)
- [v1.8](https://github.com/opentofu/opentofu/blob/v1.8/CHANGELOG.md)
- [v1.7](https://github.com/opentofu/opentofu/blob/v1.7/CHANGELOG.md)
- [v1.6](https://github.com/opentofu/opentofu/blob/v1.6/CHANGELOG.md)
