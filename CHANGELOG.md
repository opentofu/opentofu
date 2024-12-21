## 1.10.0 (Unreleased)

UPGRADE NOTES:

NEW FEATURES:

- New builtin provider functions added ([#2306](https://github.com/opentofu/opentofu/pull/2306)) :
  - `provider::terraform::decode_tfvars` - Decode a TFVars file content into an object.
  - `provider::terraform::encode_tfvars` - Encode an object into a string with the same format as a TFVars file.
  - `provider::terraform::encode_expr` - Encode an arbitrary expression into a string with valid OpenTofu syntax.

ENHANCEMENTS:

BUG FIXES:

## Previous Releases

For information on prior major and minor releases, see their changelogs:

- [v1.9](https://github.com/opentofu/opentofu/blob/v1.9/CHANGELOG.md)
- [v1.8](https://github.com/opentofu/opentofu/blob/v1.8/CHANGELOG.md)
- [v1.7](https://github.com/opentofu/opentofu/blob/v1.7/CHANGELOG.md)
- [v1.6](https://github.com/opentofu/opentofu/blob/v1.6/CHANGELOG.md)
