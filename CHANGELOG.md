## 1.10.6 (unreleased)

UPGRADE NOTES:

- Upgrade go from 1.24.4 to 1.24.6 to fix [GO-2025-3849](https://pkg.go.dev/vuln/GO-2025-3849) ([3127](https://github.com/opentofu/opentofu/pull/3127))

BUG FIXES:

- Variables with validation no longer interfere with the destroy process ([#3131](https://github.com/opentofu/opentofu/pull/3131))
- Fixed crash when processing multiple deprecated marks on a complex object ([#3105](https://github.com/opentofu/opentofu/pull/3105))

## 1.10.5

BUG FIXES:

- Fixed issue where usage of TF_PLUGIN_CACHE_DIR could result in unexpected lock contention errors ([#3090](https://github.com/opentofu/opentofu/pull/3090))
  - NOTE: It is still highly recommended to have valid .terraform.lock.hcl files in projects using TF_PLUGIN_CACHE_DIR

## 1.10.4

BUG FIXES:

- Fixed crash where sensitive set values used in for_each could cause a panic. ([#3070](https://github.com/opentofu/opentofu/pull/3070))
- Fixed incorrect approach to mocking provider "ReadResource" calls in test. ([#3068](https://github.com/opentofu/opentofu/pull/3068))
- Reduced calls to  ListKeys in azure backend (for rate limiting). ([#3083](https://github.com/opentofu/opentofu/pull/3083))

## 1.10.3

BUG FIXES:

- OpenTofu will no longer crash in a rare case where a dynamically-invalid expression has its error suppressed by `try` or `can` and then that expression becomes relevant for deciding whether to report a "change outside of OpenTofu" in the human-oriented plan diff. ([#2988](https://github.com/opentofu/opentofu/pull/2988))
- Ensure provider downloads into temp are cleaned up correctly on windows. ([#2843](https://github.com/opentofu/opentofu/issues/2843))
- Correctly handle structural typed attributes during test provider mocking. ([#2994](https://github.com/opentofu/opentofu/pull/2994))
- Fix erroneous detection of changes with sensitive resource attributes. ([#3024](https://github.com/opentofu/opentofu/pull/3024))

## 1.10.2

BUG FIXES:

- S3 backend now correctly sends the `x-amz-server-side-encryption` header for the lockfile. ([#2870](https://github.com/opentofu/opentofu/issues/2970))
- A provider source address explicitly using the hostname `registry.terraform.io` will no longer cause errors related to a corresponding provider on `registry.opentofu.org` when executing workflow commands like plan and apply. ([#2979](https://github.com/opentofu/opentofu/issues/2979))

## 1.10.1

BUG FIXES:

- Fix `TF_APPEND_USER_AGENT` handling in the S3 remote state backend. ([#2955](https://github.com/opentofu/opentofu/pull/2955))

## 1.10.0

This release has some changes that might require special attention when upgrading from an earlier release. Refer to "UPGRADE NOTES" below for more information.

NEW FEATURES:

- OpenTofu can now install **module packages from OCI Registries** using the new `oci:` source address scheme. ([#2540](https://github.com/opentofu/opentofu/issues/2540))
- OpenTofu now supports **OCI Registries as a new kind of provider mirror**. ([#2540](https://github.com/opentofu/opentofu/issues/2540))
- **Input variables and output values can now be declared as deprecated**, causing warnings when they are used by other modules. ([#1005](https://github.com/opentofu/opentofu/issues/1005))
- **The `s3` backend can now implement locking without DynamoDB**, using new features recently added to Amazon S3. ([#599](https://github.com/opentofu/opentofu/issues/599))
- **The `pg` backend now supports storing multiple states in a single database**, by specifying specifying `table_name` and `index_name` arguments to spread them across multiple tables. ([#2465](https://github.com/opentofu/opentofu/pull/2465))
- **The global provider cache is now safe for concurrent use by multiple OpenTofu processes**. ([#1878](https://github.com/opentofu/opentofu/pull/1878))

    When the global provider cache is in a filesystem that supports file locking, multiple processes now cooperate to avoid cache corruption and spurious checksum verification errors.

UPGRADE NOTES:

- On Linux, OpenTofu now requires kernel version 3.2 or later.
- On macOS, OpenTofu now requires macOS 11 Big Sur or later. We expect that the next minor release will require macOS 12 Monterey or later.
- Using the `ghcr.io/opentofu/opentofu` image as a base image for custom images is no longer supported. Refer to [Use OpenTofu as Docker Image](https://opentofu.org/docs/intro/install/docker/) for instructions on building your own image.
- OpenTofu v1.10's `pg` backend must not be used in the same database as the `pg` backend from older OpenTofu versions, because the locking implementation has changed. Mixing versions in the same database may allow conflicting writes that can cause data loss.
- On Windows, OpenTofu now has a more conservative definition of "symlink" which is limited only true [symbolic links](https://learn.microsoft.com/en-us/windows/win32/fileio/symbolic-links), and does not include other [reparse point](https://learn.microsoft.com/en-us/windows/win32/fileio/reparse-points) types such as [junctions](https://learn.microsoft.com/en-us/windows/win32/fileio/hard-links-and-junctions#junctions).

    This change fixes a number of edge-cases that caused OpenTofu to interpret paths incorrectly in earlier versions, but may cause new failures if the path you use for the `TEMP` environment variable traverses through directory junctions. Replacing any directory junctions with directory symlinks (e.g. using [`mklink`](https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/mklink) with the `/d` parameter instead of the `/j` parameter) should ensure correct treatment.

ENHANCEMENTS:

- The `element` function now accepts negative indices, which extends the existing "wrapping" model into the negative direction. In particular, choosing element `-1` selects the final element in the sequence. ([#2371](https://github.com/opentofu/opentofu/pull/2371))
- New planning options `-target-file` and `-exclude-file` allow specifying a list of resource instances to target or exclude in a separate file, to allow listing routinely-relevant target addresses in files under version control for easier reuse. ([#2620](https://github.com/opentofu/opentofu/pull/2620))
- The new `-concise` option for `tofu plan` and `tofu apply` prevents OpenTofu from printing "progress-like" messages, focusing only on final results. This is intended for use in automation scenarios where streaming output is not possible and so ongoing progress information is not useful. ([#2549](https://github.com/opentofu/opentofu/issues/2549))
- `tofu test` now accepts remote module sources when specifying an explicit module to test in a `.tftest.hcl` file. ([#2651](https://github.com/opentofu/opentofu/pull/2651))
- `tofu test` allows `provider` blocks to refer to output values from a `run` block in a `.tftest.hcl` file. ([#2543](https://github.com/opentofu/opentofu/pull/2543))
- State encryption now supports using external programs as key providers. Additionally, the PBKDF2 key provider now supports chaining via the `chain` parameter. ([#2023](https://github.com/opentofu/opentofu/pull/2023))
- `moved` blocks now supports moving remote objects between resource instances of different types, with automatic migration of the state data. ([#2370](https://github.com/opentofu/opentofu/pull/2370), [#2481](https://github.com/opentofu/opentofu/pull/2481))
- OpenTofu now includes more information about value types when describing type conversion-related errors, and some other errors relating to local iteration symbols. ([#2815](https://github.com/opentofu/opentofu/pull/2815), [#2816](https://github.com/opentofu/opentofu/pull/2816))
- The `s3` backend now supports the new `mx-central-1` AWS region. ([#2596](https://github.com/opentofu/opentofu/pull/2596))
- The `s3` backend's `skip_s3_checksum` option now additionally disables [the AWS SDK's default S3 integrity checks](https://github.com/aws/aws-sdk-go-v2/discussions/2960), which may improve compatibility for incomplete third-party reimplementations of the Amazon S3 API. ([#2596](https://github.com/opentofu/opentofu/pull/2596))
- The `pg` backend now uses a more granular locking strategy so that locks for multiple separate configurations sharing the same database will not conflict with one another. ([#2411](https://github.com/opentofu/opentofu/pull/2411))
- The `oss` backend, for state storage in Alibaba Cloud OSS, now fully supports all of the typical environment variables for HTTP/HTTPS proxy configuration. Previously it did not support per-origin opt-out using the `NO_PROXY` environment variable. ([#2675](https://github.com/opentofu/opentofu/pull/2675))
- `removed` blocks can now include `lifecycle` and `provisioner` configuration, to configure how OpenTofu should deal with any remaining instances of the resource that has been removed. ([#2556](https://github.com/opentofu/opentofu/issues/2556))
- The `tofu force-unlock` command is now supported by the `http` backend. ([#2381](https://github.com/opentofu/opentofu/pull/2381))
- The `version` argument in `module` blocks can now be set to `null`, which is treated the same as omitting the argument completely. ([#2660](https://github.com/opentofu/opentofu/pull/2660))
- The built-in provider named "terraform" now offers functions for encoding and decoding data in OpenTofu's `.tfvars` file format, and for encoding an arbitrary value as OpenTofu expression syntax. ([#2306](https://github.com/opentofu/opentofu/pull/2306))
- The `tofu show` command now supports a new explicit and extensible usage style, with `-state` and `-plan=PLANFILE` options. The old style with zero or one positional arguments is still supported for backward-compatibility. ([#2699](https://github.com/opentofu/opentofu/pull/2699))
- Dynamic instance keys in the `provider` argument for resources and the `providers` argument for module calls is now automatically converted to string, for consistency with how map indexing typically behaves. ([#2378](https://github.com/opentofu/opentofu/issues/2378))
- The plan and apply summaries now include a count of resource instances being "forgotten", which means that they will be removed from OpenTofu state without destroying the associated object in the remote system. ([#1956](https://github.com/opentofu/opentofu/issues/1956))
- OpenTofu can now produce partial OpenTelemetry trace information, sent to a collector endpoint you control, when run with certain environment variables. This release includes experimental initial support for `tofu init` tracing, but more trace detail is planned for later OpenTofu releases. ([#2665](https://github.com/opentofu/opentofu/pull/2665))
- When running `tofu init` with a dependency lock file that contains entries for certain providers on `registry.terraform.io`, OpenTofu now attempts to select the corresponding version of the equivalent provider on `registry.opentofu.org` as an aid when switching directly from OpenTofu's predecessor. This applies only to the providers that are rebuilt from source and republished on the OpenTofu Registry by the OpenTofu project, because we cannot assume any equivalence for third-party providers published in other namespaces. ([#2791](https://github.com/opentofu/opentofu/pull/2791))
- When installing a provider from a source that offers a `.zip` archive of a provider package but that cannot also offer a signed set of official checksums for the provider, OpenTofu now includes its locally-verified zip archive checksum (`zh:` scheme) in the dependency lock file in addition to the package contents checksum (`h1:` checksum) previously recorded. This makes it more likely that a future reinstall of the same package from a different source will be verified successfully. ([#2656](https://github.com/opentofu/opentofu/pull/2656))
- OpenTofu now recommends using `-exclude` instead of `-target`, when possible, in the error messages about unknown values in `count` and `for_each` arguments, thereby providing a more definitive workaround. ([#2154](https://github.com/opentofu/opentofu/pull/2154)) 
- `tofu init` now includes additional suggestions when provider installation fails and the provider had been chosen implicitly based on the backward-compatibility rules, rather than written explicitly in the configuration. ([#2084](https://github.com/opentofu/opentofu/issues/2084))

BUG FIXES:

- The error message for an unsuitable value nested in a complex-typed input variable now mentions the path to the individual problematic value, rather than incorrectly reporting that the top-level value has the problem. ([#2394](https://github.com/opentofu/opentofu/issues/2394))
- Module source addresses referring to Git branches whose names contain slashes are now handled as described in the documentation. Previously some syntax patterns would cause OpenTofu to misunderstand the slash as a path separator, rather than as part of the branch name. ([#2396](https://github.com/opentofu/opentofu/issues/2396))
- Expiration warnings for provider GPG keys now appear only when _all_ available keys have expired, and not when only a subset of keys have expired. ([#2475](https://github.com/opentofu/opentofu/issues/2475))
- The `format` and `formatlist` functions can now accept `null` as one of the arguments without causing problems during the apply phase. Previously these functions would incorrectly return an unknown value when given `null` and so could cause a failure during the apply phase where no unknown values are allowed. ([#2371](https://github.com/opentofu/opentofu/pull/2371))
- The `transpose` function now returns better error messages when the operation would cause the resulting map to contain a null key, which is impossible. ([#2553](https://github.com/opentofu/opentofu/pull/2553))
- `base64gunzip` no longer exposes sensitive values when returning a base64 decoding error. ([#2503](https://github.com/opentofu/opentofu/pull/2503))
- The `plantimestamp` function now returns an unknown value during validation. ([#2397](https://github.com/opentofu/opentofu/issues/2397))
- When assigning an empty map to a variable that is declared as a map of an object type with at least one optional attribute, OpenTofu no longer creates a subtly-broken value. ([#2371](https://github.com/opentofu/opentofu/pull/2371))
- A syntax error in a `required_providers` block no longer causes OpenTofu to crash. ([#2344](https://github.com/opentofu/opentofu/issues/2344))
- `import` blocks no longer create supurious incorrect provider dependencies that can could cause `tofu init` to fail in some cases. ([#2336](https://github.com/opentofu/opentofu/pull/2336))
- When using `import` with the `-generate-config-out` planning option, generating a `resource` block for a type with nested attributes now works correctly, instead of producing a spurious error that the nested computed attribute is required. ([#2372](https://github.com/opentofu/opentofu/issues/2372))
- The `azurerm` backend now correctly handles blob containers with a large number of blobs. ([#2720](https://github.com/opentofu/opentofu/pull/2720))
- The `azurerm` backend now respects the `timeout_seconds` argument when listing workspaces. ([#2720](https://github.com/opentofu/opentofu/pull/2720))
- OpenTofu no longer creates an incorrect dependency graph containing a cycle when a resource using `create_before_destroy` depends on one that does not use that option. ([#2398](https://github.com/opentofu/opentofu/issues/2398))
- In configurations where multiple encryption key providers and methods are configured, OpenTofu now loads only those needed for the current operation, ensuring correct encryption handling in the `terraform_remote_state` data source. ([#2551](https://github.com/opentofu/opentofu/issues/2551))
- `tofu init` now detects and reports errors when creating the file used to track backend initialization, instead of silently failing to save that information. ([#2798](https://github.com/opentofu/opentofu/pull/2798))
- An invalid provider name in a `provider_meta` block no longer causes OpenTofu to crash. ([#2347](https://github.com/opentofu/opentofu/pull/2347))
- OpenTofu no longer indirectly uses software that was affected by [CVE-2024-45336](https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2024-45336) and [CVE-2024-45341](https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2024-45341). These advisories did not significantly affect OpenTofu, and so this upgrade is purely to reduce false positives in naive security scanners. ([#2438](https://github.com/opentofu/opentofu/pull/2438))

## Previous Releases

For information on prior major and minor releases, see their changelogs:

- [v1.9](https://github.com/opentofu/opentofu/blob/v1.9/CHANGELOG.md)
- [v1.8](https://github.com/opentofu/opentofu/blob/v1.8/CHANGELOG.md)
- [v1.7](https://github.com/opentofu/opentofu/blob/v1.7/CHANGELOG.md)
- [v1.6](https://github.com/opentofu/opentofu/blob/v1.6/CHANGELOG.md)
