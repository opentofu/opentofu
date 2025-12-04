## 1.12.0 (Unreleased)

UPGRADE NOTES:

- On Unix systems OpenTofu now considers the `BROWSER` environment variable as a possible override for the default behavior for launching a web browser.

    If you run OpenTofu in a context where an environment variable of that name is already set, it may cause OpenTofu to now open a web browser in a different way than previous versions would have. Unsetting that environment variable will restore the previous platform-specific behavior.

ENHANCEMENTS:

- `prevent_destroy` arguments in the `lifecycle` block for managed resources can now use references to other symbols in the same module, such as to a module's input variables. ([#3474](https://github.com/opentofu/opentofu/issues/3474), [#3507](https://github.com/opentofu/opentofu/issues/3507))
- New `lifecycle` meta-argument `destroy` for altering resource destruction behavior. When set to `false` OpenTofu will not retain resources when they are planned for destruction. ([#3409](https://github.com/opentofu/opentofu/pull/3409))
- OpenTofu now uses the `BROWSER` environment variable when launching a web browser on Unix platforms, as long as it's set to a single command that can accept a URL to open as its first and only argument. ([#3456](https://github.com/opentofu/opentofu/issues/3456))
- Improve performance around provider checking and schema management. ([#2730](https://github.com/opentofu/opentofu/pull/2730))
- `tofu init` now fetches providers and their metadata in parallel. Depending on provider size and network properties, this can reduce provider installation and checking time. ([#2729](https://github.com/opentofu/opentofu/pull/2729))

BUG FIXES:

- `for_each` inside `dynamic` blocks can now call provider-defined functions. ([#3429](https://github.com/opentofu/opentofu/issues/3429))
- In the unlikely event that text included in a diagnostic message includes C0 control characters (e.g. terminal escape sequences), OpenTofu will now replace them with printable characters to avoid the risk of inadvertently changing terminal state when stdout or stderr is a terminal. ([#3479](https://github.com/opentofu/opentofu/issues/3479))
- Fixed `length(module.foo)` returning 0 for module instances without outputs, even when `count` or `for_each` is set. ([#3067](https://github.com/opentofu/opentofu/issues/3067))

## Previous Releases

For information on prior major and minor releases, refer to their changelogs:

- [v1.11](https://github.com/opentofu/opentofu/blob/v1.11/CHANGELOG.md)
- [v1.10](https://github.com/opentofu/opentofu/blob/v1.10/CHANGELOG.md)
- [v1.9](https://github.com/opentofu/opentofu/blob/v1.9/CHANGELOG.md)
- [v1.8](https://github.com/opentofu/opentofu/blob/v1.8/CHANGELOG.md)
- [v1.7](https://github.com/opentofu/opentofu/blob/v1.7/CHANGELOG.md)
- [v1.6](https://github.com/opentofu/opentofu/blob/v1.6/CHANGELOG.md)
