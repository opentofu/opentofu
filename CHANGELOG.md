The v1.12.x release series is supported until **February 1 2027**.

## 1.12.0 (Unreleased)

UPGRADE NOTES:

- The `OPENTOFU_USER_AGENT` environment variable, which allowed fully overriding the default User-Agent header on all HTTP requests has been removed.
- This is the last OpenTofu release series that will support macOS 12 Monterey. We expect that OpenTofu v1.13 will require macOS 13 Ventura or later.
- On Unix systems OpenTofu now considers the `BROWSER` environment variable as a possible override for the default behavior for launching a web browser.

    If you run OpenTofu in a context where an environment variable of that name is already set, it may cause OpenTofu to now open a web browser in a different way than previous versions would have. Unsetting that environment variable will restore the previous platform-specific behavior.

- If you are installing providers from the registry (most users), you should expect to see additional `h1:value` provider hashes in your `.terraform.lock.hcl` file.

    We have improved the OpenTofu registry to serve both `zh:value` and `h1:value` hashes, as well as instructing OpenTofu in how to integrate this data into its existing provider trust chain. Including these additional hashes will reduce friction in cross-platform environments. These and other related changes below should subsume the need to use `tofu providers lock` in most scenarios, simplifying many existing cross-platform workflows. For more information, see the [corresponding RFC](rfc/20251027-provider-registry-hashes.md) and [discussion](https://github.com/opentofu/opentofu/pull/3434)

- The OpenTofu project is planning to stop providing official release packages for 32-bit CPU architectures (`*_386` and `*_arm` platforms) in a future release series.

    We intend to continue producing packages for these platforms at least throughout the v1.12.x and v1.13.x series and so no immediate action is required, but if you are currently relying on our official packages for these platforms then we suggest that you begin planning to migrate to running OpenTofu on a 64-bit CPU architecture (`*_amd64` or `*_arm64` platforms).

ENHANCEMENTS:

- A `prevent_destroy` argument in the `lifecycle` block for managed resources can now refer to other symbols in the same module, such as to the module's input variables. ([#3474](https://github.com/opentofu/opentofu/issues/3474), [#3507](https://github.com/opentofu/opentofu/issues/3507))
- New `lifecycle` meta-argument `destroy`: when set to `false` OpenTofu will plan to just remove the affected object from state without asking the provider to destroy it first, similar to `destroy = false` in `removed` blocks. ([#3409](https://github.com/opentofu/opentofu/pull/3409))
- Comparing an object or other complex-typed value to `null` using the `==` operator now returns a sensitive boolean result only if the object as a whole is sensitive, and not when the object merely contains a sensitive value nested inside one of its attributes. This means that comparisons to null can now be used in parts of the configuration where sensitive values are not allowed, such as in the `enabled` meta-argument on resources and modules. ([#3793](https://github.com/opentofu/opentofu/pull/3793))
- Resources using `replace_triggered_by` in their `lifecycle` block are now replaced when a resource they refer to is itself being replaced, whereas before this triggered only when it was being updated. ([#3714](https://github.com/opentofu/opentofu/issues/3714))
- The `yamldecode` function now supports the "merge" tag, most commonly written as `<<` where a map key would be expected, with sequences of mappings rather than just individual mappings. ([#3607](https://github.com/opentofu/opentofu/pull/3607))
- A new configuration block type `language` offers a more general way to define version constraints that separates OpenTofu constraints from other software. Note that module authors should delay adopting this new syntax until they are ready to require OpenTofu v1.12.0 or later, but there is an interim solution available that is backward-compatible with earlier OpenTofu versions. ([#3300](https://github.com/opentofu/opentofu/issues/3300))
- New CLI argument `-json-into=<outfile>` allows emitting both human-readable and machine-readable logs. ([#3606](https://github.com/opentofu/opentofu/pull/3606))
- Provider installation now makes concurrent requests to download provider packages, which may allow `tofu init` to complete faster. ([#2729](https://github.com/opentofu/opentofu/pull/2729))
- Provider checksum verification and schema loading are now better optimized, including no longer verifying checksums for providers that are present in the local cache but will not be used by a particular command. ([#2730](https://github.com/opentofu/opentofu/pull/2730))
- `tofu init` now includes a full set of checksums for all supported platforms when updating a dependency lock file, using additional information now reported by the provider registry. This should remove the need to run `tofu providers lock` in many situations where it was previously required. ([#3868](https://github.com/opentofu/opentofu/pull/3868))
- The `network_mirror` configuration now includes an option to trust all hashes reported by the mirror. This also simplifies managing lockfiles in cross-platform environments. ([3885](https://github.com/opentofu/opentofu/pull/3885))
- Module registries can now specify that package downloads should use the same credentials as the registry's API calls, without needing to configure credentials separately in a `.netrc` file. This approach is helpful when the module packages are served by the registry itself, rather than when the registry just links to an external location such as a GitHub repository. ([#3313](https://github.com/opentofu/opentofu/issues/3313))
- `tofu destroy` now supports `-suppress-forget-errors` to suppress errors and exit with a zero status code when resources are forgotten during destroy operations. ([#3588](https://github.com/opentofu/opentofu/issues/3588))
- `tofu console` now supports `-lock=false` and `-lock-timeout=DURATION` to control whether and how this command uses state locks. ([#3800](https://github.com/opentofu/opentofu/pull/3800))
- `tofu login` now uses the `BROWSER` environment variable when launching a web browser on Unix platforms, as long as it's set to a single command that can accept a URL to open as its first and only argument. ([#3456](https://github.com/opentofu/opentofu/issues/3456))
- The `s3` backend now automatically discovers and uses AWS credentials issued using [the `aws login` command](https://docs.aws.amazon.com/cli/latest/reference/login/) in AWS CLI. ([#3767](https://github.com/opentofu/opentofu/pull/3767))
- The `azurerm` backend now supports authentication using Azure DevOps and Azure Pipelines workload identity federation. ([#3820](https://github.com/opentofu/opentofu/pull/3820))

BUG FIXES:

* `local-exec` provisioner output is no longer suppressed when `-show-sensitive` is passed. ([#3927](https://github.com/opentofu/opentofu/issues/3927))
- `length(module.example)` now returns the correct result for a module that has no output values when called using `count` or `for_each`. It would previously incorrectly return zero unless at least one output - A call to a module containing `check` blocks can now use `depends_on` without causing a dependency cycle error. ([#3060](https://github.com/opentofu/opentofu/issues/3060))
value was declared inside the module. ([#3067](https://github.com/opentofu/opentofu/issues/3067))
- `for_each` arguments in `dynamic` blocks can now call provider-defined functions. ([#3429](https://github.com/opentofu/opentofu/issues/3429))
- Calls to provider-defined functions in the `id` argument of an `import` block no longer cause "BUG: Uninitialized function provider" error. ([#3803](https://github.com/opentofu/opentofu/issues/3803))
- `local-exec` and `file` provisioners no longer crash when their `command` or `destination` arguments are set to `null`. ([#3783](https://github.com/opentofu/opentofu/issues/3783))
- Modules containing nested provider configurations now reject the `enabled` argument, matching the existing behavior for `count`, `for_each`, and `depends_on`. ([#3680](https://github.com/opentofu/opentofu/pull/3680))
- In JSON syntax, `key_provider` expressions can now use references written directly in quotes, without using template interpolation syntax. Previously only the template syntax was allowed, which was inconsistent with other parts of the encryption configuration. ([#3794](https://github.com/opentofu/opentofu/issues/3794))
- In JSON syntax, the state encryption method configuration now allows specifying keys using both normal expression syntax and using template interpolation syntax. Previously only the template interpolation syntax was allowed, which was inconsistent with other parts of the encryption configuration. ([#3654](https://github.com/opentofu/opentofu/issues/3654))
- OpenTofu no longer returns spurious errors about incorrectly-detected provider reference problems when modules fail to load during the construction of a configuration tree. ([#3681](https://github.com/opentofu/opentofu/pull/3681))
- State lock now released correctly when `tofu apply` is interrupted using Ctrl+C while using the `http` backend. ([#3624](https://github.com/opentofu/opentofu/issues/3624))
- `tofu init` no longer crashes when a module `version` refers to an input variable and the module is used in an expression from a test file. ([#3686](https://github.com/opentofu/opentofu/issues/3686))
- `tofu test` with `mock_provider` no longer fails during cleanup when a resource's `ignore_changes` argument refers to a block. ([#3644](https://github.com/opentofu/opentofu/issues/3644))
- In the unlikely event that text included in a diagnostic message includes C0 control characters (e.g. terminal escape sequences), OpenTofu will now replace them with printable characters to avoid the risk of inadvertently changing terminal state when stdout or stderr is a terminal. ([#3479](https://github.com/opentofu/opentofu/issues/3479))
- The `azurerm` backend's MSI authentication method now respects the provided client ID. ([#3586](https://github.com/opentofu/opentofu/issues/3586))
- The `gcs` backend now supports a `universe_domain` option to support sovereign GCP services. ([#3758](https://github.com/opentofu/opentofu/issues/3758))
- OpenTofu now consistently sends "null" to `key_provider "external"` programs when only encryption the key is requested. ([#3672](https://github.com/opentofu/opentofu/pull/3672))

## Previous Releases

For information on prior major and minor releases, refer to their changelogs:

- [v1.11](https://github.com/opentofu/opentofu/blob/v1.11/CHANGELOG.md)
- [v1.10](https://github.com/opentofu/opentofu/blob/v1.10/CHANGELOG.md)
- [v1.9](https://github.com/opentofu/opentofu/blob/v1.9/CHANGELOG.md)
- [v1.8](https://github.com/opentofu/opentofu/blob/v1.8/CHANGELOG.md)
- [v1.7](https://github.com/opentofu/opentofu/blob/v1.7/CHANGELOG.md)
- [v1.6](https://github.com/opentofu/opentofu/blob/v1.6/CHANGELOG.md)
