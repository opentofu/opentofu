## 1.10.0 (Unreleased)

UPGRADE NOTES:

* Using the `ghcr.io/opentofu/opentofu` image as a base image for custom images is no longer supported. Please see https://opentofu.org/docs/intro/install/docker/ for instructions on building your own image.
* OpenTofu 1.10 with `pg` backend must not be used in parallel with older versions. It may lead to unsafe state writes, when the database is shared across multiple projects.

NEW FEATURES:

- New builtin provider functions added ([#2306](https://github.com/opentofu/opentofu/pull/2306)) :
    - `provider::terraform::decode_tfvars` - Decode a TFVars file content into an object.
    - `provider::terraform::encode_tfvars` - Encode an object into a string with the same format as a TFVars file.
    - `provider::terraform::encode_expr` - Encode an arbitrary expression into a string with valid OpenTofu syntax.

ENHANCEMENTS:
* OpenTofu will now recommend using `-exclude` instead of `-target`, when possible, in the error messages about unknown values in `count` and `for_each` arguments, thereby providing a more definitive workaround. ([#2154](https://github.com/opentofu/opentofu/pull/2154))
* State encryption now supports using external programs as key providers. Additionally, the PBKDF2 key provider now supports chaining via the `chain` parameter. ([#2023](https://github.com/opentofu/opentofu/pull/2023))
* Added count of forgotten resources to plan and apply outputs. ([#1956](https://github.com/opentofu/opentofu/issues/1956))
* The `element` function now accepts negative indices, which extends the existing "wrapping" model into the negative direction. In particular, choosing element `-1` selects the final element in the sequence. ([#2371](https://github.com/opentofu/opentofu/pull/2371))
* `moved` now supports moving between different types ([#2370](https://github.com/opentofu/opentofu/pull/2370))
* `moved` block can now be used to migrate from the `null_resource` to the `terraform_data` resource. ([#2481](https://github.com/opentofu/opentofu/pull/2481))
* Warn on implicit references of providers without a `required_providers` entry. ([#2084](https://github.com/opentofu/opentofu/issues/2084))

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
- `pg` backend doesn't fail on workspace creation for paralel runs, when the database is shared across multiple projects. ([#2411](https://github.com/opentofu/opentofu/pull/2411))
- Generating an OpenTofu configuration from an `import` block that is referencing a resource with nested attributes now works correctly, instead of giving an error that the nested computed attribute is required. ([#2372](https://github.com/opentofu/opentofu/issues/2372)) 
- `base64gunzip` now doesn't expose sensitive values if it fails during the base64 decoding. ([#2503](https://github.com/opentofu/opentofu/pull/2503))

## Previous Releases

For information on prior major and minor releases, see their changelogs:

- [v1.9](https://github.com/opentofu/opentofu/blob/v1.9/CHANGELOG.md)
- [v1.8](https://github.com/opentofu/opentofu/blob/v1.8/CHANGELOG.md)
- [v1.7](https://github.com/opentofu/opentofu/blob/v1.7/CHANGELOG.md)
- [v1.6](https://github.com/opentofu/opentofu/blob/v1.6/CHANGELOG.md)
