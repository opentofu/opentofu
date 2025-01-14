## 1.10.0 (Unreleased)

UPGRADE NOTES:

* Using the `ghcr.io/opentofu/opentofu` image as a base image for custom images is no longer supported. Please see https://opentofu.org/docs/intro/install/docker/ for instructions on building your own image.

NEW FEATURES:

- New builtin provider functions added ([#2306](https://github.com/opentofu/opentofu/pull/2306)) :
  - `provider::terraform::decode_tfvars` - Decode a TFVars file content into an object.
  - `provider::terraform::encode_tfvars` - Encode an object into a string with the same format as a TFVars file.
  - `provider::terraform::encode_expr` - Encode an arbitrary expression into a string with valid OpenTofu syntax.

ENHANCEMENTS:
* OpenTofu will now recommend using `-exclude` instead of `-target`, when possible, in the error messages about unknown values in `count` and `for_each` arguments, thereby providing a more definitive workaround. ([#2154](https://github.com/opentofu/opentofu/pull/2154))
* State encryption now supports using external programs as key providers. Additionally, the PBKDF2 key provider now supports chaining via the `chain` parameter. ([#2023](https://github.com/opentofu/opentofu/pull/2023))
* The `element` function now accepts negative indices, which extends the existing "wrapping" model into the negative direction. In particular, choosing element `-1` selects the final element in the sequence. ([#2371](https://github.com/opentofu/opentofu/pull/2371))

BUG FIXES:

- Fixed an issue where an invalid provider name in the `provider_meta` block would crash OpenTofu rather than report an error ([#2347](https://github.com/opentofu/opentofu/pull/2347))
- When assigning an empty map to a variable that is declared as a map of an object type with at least one optional attribute, OpenTofu will no longer create a subtly-broken value. ([#2371](https://github.com/opentofu/opentofu/pull/2371))
- The `format` and `formatlist` functions can now accept `null` as one of the arguments without causing problems during the apply phase. Previously these functions would incorrectly return an unknown value when given `null` and so could cause a failure during the apply phase where no unknown values are allowed. ([#2371](https://github.com/opentofu/opentofu/pull/2371))

## Previous Releases

For information on prior major and minor releases, see their changelogs:

- [v1.9](https://github.com/opentofu/opentofu/blob/v1.9/CHANGELOG.md)
- [v1.8](https://github.com/opentofu/opentofu/blob/v1.8/CHANGELOG.md)
- [v1.7](https://github.com/opentofu/opentofu/blob/v1.7/CHANGELOG.md)
- [v1.6](https://github.com/opentofu/opentofu/blob/v1.6/CHANGELOG.md)
