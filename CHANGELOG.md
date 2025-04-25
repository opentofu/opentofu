## 1.10.0 (Unreleased)

UPGRADE NOTES:

* On Linux, OpenTofu now requires kernel version 3.2 or later.
* On macOS, OpenTofu now requires macOS 11 Big Sur or later. We expect that the next minor release will require macOS 12 Monterey or later.
* Using the `ghcr.io/opentofu/opentofu` image as a base image for custom images is no longer supported. Refer to https://opentofu.org/docs/intro/install/docker/ for instructions on building your own image.
* OpenTofu 1.10 with `pg` backend must not be used in parallel with older versions. It may lead to unsafe state writes, when the database is shared across multiple projects.
* On Windows, OpenTofu now has a more conservative definition of "symlink" which is limited only true [symbolic links](https://learn.microsoft.com/en-us/windows/win32/fileio/symbolic-links), and does not include other [reparse point](https://learn.microsoft.com/en-us/windows/win32/fileio/reparse-points) types such as [junctions](https://learn.microsoft.com/en-us/windows/win32/fileio/hard-links-and-junctions#junctions).

    This change fixes a number of edge-cases that caused OpenTofu to interpret paths incorrectly in earlier versions, but may cause new failures if the path you use for the `TEMP` environment variable traverses through directory junctions. Replacing any directory junctions with directory symlinks (e.g. using [`mklink`](https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/mklink) with the `/d` parameter instead of the `/j` parameter) should ensure correct treatment.

NEW FEATURES:

- Can now use OCI Registries as a new kind of provider mirror. ([#2540](https://github.com/opentofu/opentofu/issues/2540))
- Can now install module packages from OCI Registries using the new `oci:` source address scheme. ([#2540](https://github.com/opentofu/opentofu/issues/2540))
- New builtin provider functions added ([#2306](https://github.com/opentofu/opentofu/pull/2306)) :
    - `provider::terraform::decode_tfvars` - Decode a TFVars file content into an object.
    - `provider::terraform::encode_tfvars` - Encode an object into a string with the same format as a TFVars file.
    - `provider::terraform::encode_expr` - Encode an arbitrary expression into a string with valid OpenTofu syntax.
- Added support for S3 native locking ([#599](https://github.com/opentofu/opentofu/issues/599))
- Backend `pg` now allows the `table_name` and `index_name` to be specified. This enables a single database schema to support multiple backends via multiple tables. ([#2465](https://github.com/opentofu/opentofu/pull/2465))
- Module variables and outputs can now be marked as `deprecated` to indicate their removal in the future. ([#1005](https://github.com/opentofu/opentofu/issues/1005))
- OpenTelemetry tracing has been added to the `init` command for provider installation. Note: This feature is experimental and subject to change in the future. ([#2665](https://github.com/opentofu/opentofu/pull/2665))
- Global Provider Cache Locking is now supported ([#1878](https://github.com/opentofu/opentofu/pull/1878). As long as your filesystem supports file level locking, you can now run multiple instances of OpenTofu that use the same global provider file system cache without worrying about them clobbering each other.

ENHANCEMENTS:

* OpenTofu will now recommend using `-exclude` instead of `-target`, when possible, in the error messages about unknown values in `count` and `for_each` arguments, thereby providing a more definitive workaround. ([#2154](https://github.com/opentofu/opentofu/pull/2154))
* State encryption now supports using external programs as key providers. Additionally, the PBKDF2 key provider now supports chaining via the `chain` parameter. ([#2023](https://github.com/opentofu/opentofu/pull/2023))
* New planning options `-target-file` and `-exclude-file` allow specifying a list of resource instances to target or exclude in a separate file, to allow listing routinely-relevant target addresses in files under version control for easier reuse. ([#2620](https://github.com/opentofu/opentofu/pull/2620))
* Added count of forgotten resources to plan and apply outputs. ([#1956](https://github.com/opentofu/opentofu/issues/1956))
* The `element` function now accepts negative indices, which extends the existing "wrapping" model into the negative direction. In particular, choosing element `-1` selects the final element in the sequence. ([#2371](https://github.com/opentofu/opentofu/pull/2371))
* Remove restriction on test module sources now allowing all source types for modules during tests ([#2651]https://github.com/opentofu/opentofu/pull/2651)
* `moved` now supports moving between different types ([#2370](https://github.com/opentofu/opentofu/pull/2370))
* `moved` block can now be used to migrate from the `null_resource` to the `terraform_data` resource. ([#2481](https://github.com/opentofu/opentofu/pull/2481))
* Warn on implicit references of providers without a `required_providers` entry. ([#2084](https://github.com/opentofu/opentofu/issues/2084))
* The test `run` outputs can now be used in the test `provider` blocks defined in test files. ([#2543](https://github.com/opentofu/opentofu/pull/2543))
* Provider instance keys now automatically converted to string ([#2378](https://github.com/opentofu/opentofu/issues/2378))
* Remove progress messages from commands using -concise argument ([#2549](https://github.com/opentofu/opentofu/issues/2549))
* Upgrade aws-sdk version to include `mx-central-1` region. ([#2596](https://github.com/opentofu/opentofu/pull/2596))
* When installing a provider from a source that offers a `.zip` archive of a provider package but that cannot also offer a signed set of official checksums for the provider, OpenTofu will now include its locally-verified zip archive checksum (`zh:` scheme) in the dependency lock file in addition to the package contents checksum (`h1:` checksum) previously recorded. This makes it more likely that a future reinstall of the same package from a different source will be verified successfully. ([#2656](https://github.com/opentofu/opentofu/pull/2656))
* The `tofu show` command now supports a new explicit and extensible usage style, with `-state` and `-plan=PLANFILE` options. The old style with zero or one positional arguments is still supported for backward-compatibility. ([#2699](https://github.com/opentofu/opentofu/pull/2699))
* `removed` now supports `lifecycle` and `provisioner` configuration. ([#2556](https://github.com/opentofu/opentofu/issues/2556))
* "force-unlock" option is now supported by the HTTP backend. ([#2381](https://github.com/opentofu/opentofu/pull/2381))

BUG FIXES:

- Fixed an issue where an invalid provider name in the `provider_meta` block would crash OpenTofu rather than report an error ([#2347](https://github.com/opentofu/opentofu/pull/2347))
- When assigning an empty map to a variable that is declared as a map of an object type with at least one optional attribute, OpenTofu will no longer create a subtly-broken value. ([#2371](https://github.com/opentofu/opentofu/pull/2371))
- The `format` and `formatlist` functions can now accept `null` as one of the arguments without causing problems during the apply phase. Previously these functions would incorrectly return an unknown value when given `null` and so could cause a failure during the apply phase where no unknown values are allowed. ([#2371](https://github.com/opentofu/opentofu/pull/2371))
- Provider used in import is correctly identified. ([#2336](https://github.com/opentofu/opentofu/pull/2336))
- `plantimestamp()` now returns unknown value during validation ([#2397](https://github.com/opentofu/opentofu/issues/2397))
- Syntax error in the `required_providers` block does not panic anymore, but yields "syntax error" ([2344](https://github.com/opentofu/opentofu/issues/2344))
- Changing Go version to 1.22.11 in order to fix [CVE-2024-45336](https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2024-45336) and [CVE-2024-45341](https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2024-45341) ([#2438](https://github.com/opentofu/opentofu/pull/2438))
- Fix the error message when default value of a complex variable is containing a wrong type ([2394](https://github.com/opentofu/opentofu/issues/2394))
- Fix the way OpenTofu downloads a module that is sourced from a GitHub branch containing slashes in the name. ([2396](https://github.com/opentofu/opentofu/issues/2396))
- `pg` backend doesn't fail on workspace creation for parallel runs, when the database is shared across multiple projects. ([#2411](https://github.com/opentofu/opentofu/pull/2411))
- Generating an OpenTofu configuration from an `import` block that is referencing a resource with nested attributes now works correctly, instead of giving an error that the nested computed attribute is required. ([#2372](https://github.com/opentofu/opentofu/issues/2372))
- `base64gunzip` now doesn't expose sensitive values if it fails during the base64 decoding. ([#2503](https://github.com/opentofu/opentofu/pull/2503))
- Fix the issue with unexpected `create_before_destroy` (CBD) behavior, when CBD resource was depending on a non-CBD resource. ([#2398](https://github.com/opentofu/opentofu/issues/2398))
- Fix loading only the necessary encryption key providers and methods for better `terraform_remote_state` support. ([2551](https://github.com/opentofu/opentofu/issues/2551))
- Better error messages when using `null` in invalid positions in the argument to the `transpose` function. ([#2553](https://github.com/opentofu/opentofu/pull/2553))
- Provider GPG Expiration warnings no longer show when only one of the keys have expired. Only once all are expired or invalid. ([#2475](https://github.com/opentofu/opentofu/issues/2475))
- The "oss" backend, for state storage in Alibaba Cloud OSS, now fully supports all of the typical environment variables for HTTP/HTTPS proxy configuration. Previously it supported configuring a proxy using variables like `HTTPS_PROXY`, but did not support per-origin opt-out using the `NO_PROXY` environment variable. ([#2675](https://github.com/opentofu/opentofu/pull/2675))

INTERNAL CHANGES:
- `skip_s3_checksum=true` now blocks the [aws-sdk new default S3 integrity checks](https://github.com/aws/aws-sdk-go-v2/discussions/2960) ([#2596](https://github.com/opentofu/opentofu/pull/2596))

## Previous Releases

For information on prior major and minor releases, see their changelogs:

- [v1.9](https://github.com/opentofu/opentofu/blob/v1.9/CHANGELOG.md)
- [v1.8](https://github.com/opentofu/opentofu/blob/v1.8/CHANGELOG.md)
- [v1.7](https://github.com/opentofu/opentofu/blob/v1.7/CHANGELOG.md)
- [v1.6](https://github.com/opentofu/opentofu/blob/v1.6/CHANGELOG.md)
