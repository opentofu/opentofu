## 1.10.0 (Unreleased)

UPGRADE NOTES:

NEW FEATURES:

ENHANCEMENTS:

BUG FIXES:
* `tofu init` command does not attempt to read encryption keys when `-backend=false` flag is set. (https://github.com/opentofu/opentofu/pull/2293)

* Changes in `create_before_destroy` for resources which require replacement are now properly handled when refresh is disabled. ([#2248](https://github.com/opentofu/opentofu/pull/2248))

## Previous Releases

For information on prior major and minor releases, see their changelogs:

- [v1.9](https://github.com/opentofu/opentofu/blob/v1.9/CHANGELOG.md)
- [v1.8](https://github.com/opentofu/opentofu/blob/v1.8/CHANGELOG.md)
- [v1.7](https://github.com/opentofu/opentofu/blob/v1.7/CHANGELOG.md)
- [v1.6](https://github.com/opentofu/opentofu/blob/v1.6/CHANGELOG.md)
