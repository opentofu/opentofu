The v1.12.x release series is supported until **February 1 2027**.

## 1.12.0 (Unreleased)

UPGRADE NOTES:

- This is the last OpenTofu release series that will support macOS 12 Monterey. We expect that OpenTofu v1.13 will require macOS 13 Ventura or later.
- On Unix systems OpenTofu now considers the `BROWSER` environment variable as a possible override for the default behavior for launching a web browser.

    If you run OpenTofu in a context where an environment variable of that name is already set, it may cause OpenTofu to now open a web browser in a different way than previous versions would have. Unsetting that environment variable will restore the previous platform-specific behavior.

ENHANCEMENTS:

- `prevent_destroy` arguments in the `lifecycle` block for managed resources can now use references to other symbols in the same module, such as to a module's input variables. ([#3474](https://github.com/opentofu/opentofu/issues/3474), [#3507](https://github.com/opentofu/opentofu/issues/3507))
- New `lifecycle` meta-argument `destroy` for altering resource destruction behavior. When set to `false` OpenTofu will not retain resources when they are planned for destruction. ([#3409](https://github.com/opentofu/opentofu/pull/3409))
- New `-suppress-forget-errors` flag for the `tofu destroy` command to suppress errors and exit with a zero status code when resources are forgotten during destroy operations. ([#3588](https://github.com/opentofu/opentofu/issues/3588))
- OpenTofu now uses the `BROWSER` environment variable when launching a web browser on Unix platforms, as long as it's set to a single command that can accept a URL to open as its first and only argument. ([#3456](https://github.com/opentofu/opentofu/issues/3456))
- Improve performance around provider checking and schema management. ([#2730](https://github.com/opentofu/opentofu/pull/2730))
- `tofu init` now fetches providers and their metadata in parallel. Depending on provider size and network properties, this can reduce provider installation and checking time. ([#2729](https://github.com/opentofu/opentofu/pull/2729))
- The `yamldecode` function now supports the "merge" tag, most commonly written as `<<` where a map key would be expected, with sequences of mappings rather than just individual mappings. ([#3607](https://github.com/opentofu/opentofu/pull/3607))
- New CLI argument `-json-into=<outfile>` has been added to support emitting both human readable and machine readable logs ([#3606](https://github.com/opentofu/opentofu/pull/3606))

BUG FIXES:

- Modules containing local provider configurations now also reject the `enabled` argument, matching existing behavior for `count`, `for_each`, and `depends_on`. ([#3680](https://github.com/opentofu/opentofu/pull/3680))
- Fixed state lock not being released when `tofu apply` is interrupted with Ctrl+C while using the HTTP backend. ([#3624](https://github.com/opentofu/opentofu/issues/3624))
- Fixed dependency cycle error when a module with check blocks is referenced via `depends_on` by another module. ([#3060](https://github.com/opentofu/opentofu/issues/3060))
- `for_each` inside `dynamic` blocks can now call provider-defined functions. ([#3429](https://github.com/opentofu/opentofu/issues/3429))
- In the unlikely event that text included in a diagnostic message includes C0 control characters (e.g. terminal escape sequences), OpenTofu will now replace them with printable characters to avoid the risk of inadvertently changing terminal state when stdout or stderr is a terminal. ([#3479](https://github.com/opentofu/opentofu/issues/3479))
- Fixed `length(module.foo)` returning 0 for module instances without outputs, even when `count` or `for_each` is set. ([#3067](https://github.com/opentofu/opentofu/issues/3067))
- Fixed `tofu test` with `mock_provider` failing during cleanup when `lifecycle { ignore_changes }` references a block. ([#3644](https://github.com/opentofu/opentofu/issues/3644))
- In JSON syntax, the state encryption method configuration now allows specifying keys using both normal expression syntax and using template interpolation syntax. Previously only the template interpolation syntax was allowed, which was inconsistent with other parts of the encryption configuration. ([#3654](https://github.com/opentofu/opentofu/issues/3654))
- No longer generate spurious error messages about incorrectly-detected provider reference problems when modules fail to load during the construction of a configuration tree. ([#3681](https://github.com/opentofu/opentofu/pull/3681))
- OpenTofu consistently sends "null" to external `key_provider` programs when only encryption key is requested ([#3672](https://github.com/opentofu/opentofu/pull/3672))
- The `azurerm` backend's MSI authentication method will now respect the provided client ID ([#3586](https://github.com/opentofu/opentofu/issues/3586))
- Add `universe_domain` option in the `gcs` backend to support sovereign GCP services ([#3758](https://github.com/opentofu/opentofu/issues/3758))

## Previous Releases

For information on prior major and minor releases, refer to their changelogs:

- [v1.11](https://github.com/opentofu/opentofu/blob/v1.11/CHANGELOG.md)
- [v1.10](https://github.com/opentofu/opentofu/blob/v1.10/CHANGELOG.md)
- [v1.9](https://github.com/opentofu/opentofu/blob/v1.9/CHANGELOG.md)
- [v1.8](https://github.com/opentofu/opentofu/blob/v1.8/CHANGELOG.md)
- [v1.7](https://github.com/opentofu/opentofu/blob/v1.7/CHANGELOG.md)
- [v1.6](https://github.com/opentofu/opentofu/blob/v1.6/CHANGELOG.md)
