## 1.12.0 (Unreleased)

UPGRADE NOTES:

- On Unix systems OpenTofu now considers the `BROWSER` environment variable as a possible override for the default behavior for launching a web browser.

    If you run OpenTofu in a context where an environment variable of that name is already set, it may cause OpenTofu to now open a web browser in a different way than previous versions would have. Unsetting that environment variable will restore the previous platform-specific behavior.

NEW FEATURES:

* **Experimental: FIPS 140-3 Mode Support:** OpenTofu can now optionally be run in a mode that utilizes FIPS 140-3 validated cryptographic modules provided by the underlying Go 1.24+ runtime. This helps organizations meet certain compliance requirements by ensuring only approved cryptographic algorithms are used for operations like TLS connections. To enable FIPS mode, ensure your OpenTofu binary was built with Go 1.24 or later and set the environment variable `GODEBUG=fips140=on` before running `tofu` commands. **Important Limitation:** Due to current limitations in the underlying OpenPGP library, GPG signature validation for provider packages is **automatically skipped** when FIPS mode is enabled. A warning will be logged when this occurs. In this mode, provider integrity relies on the secure TLS connection to the registry. See the [FIPS Mode documentation](./docs/usage/fips.md) for more details.

ENHANCEMENTS:

- `prevent_destroy` arguments in the `lifecycle` block for managed resources can now use references to other symbols in the same module, such as to a module's input variables. ([#3474](https://github.com/opentofu/opentofu/issues/3474))
- OpenTofu now uses the `BROWSER` environment variable when launching a web browser on Unix platforms, as long as it's set to a single command that can accept a URL to open as its first and only argument. ([#3456](https://github.com/opentofu/opentofu/issues/3456))

BUG FIXES:

- `for_each` inside `dynamic` blocks can now call provider-defined functions. ([#3429](https://github.com/opentofu/opentofu/issues/3429))
- In the unlikely event that text included in a diagnostic message includes C0 control characters (e.g. terminal escape sequences), OpenTofu will now replace them with printable characters to avoid the risk of inadvertently changing terminal state when stdout or stderr is a terminal. ([#3479](https://github.com/opentofu/opentofu/issues/3479))

## Previous Releases

For information on prior major and minor releases, refer to their changelogs:

- [v1.11](https://github.com/opentofu/opentofu/blob/v1.11/CHANGELOG.md)
- [v1.10](https://github.com/opentofu/opentofu/blob/v1.10/CHANGELOG.md)
- [v1.9](https://github.com/opentofu/opentofu/blob/v1.9/CHANGELOG.md)
- [v1.8](https://github.com/opentofu/opentofu/blob/v1.8/CHANGELOG.md)
- [v1.7](https://github.com/opentofu/opentofu/blob/v1.7/CHANGELOG.md)
- [v1.6](https://github.com/opentofu/opentofu/blob/v1.6/CHANGELOG.md)
