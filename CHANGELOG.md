## 1.11.0 (Unreleased)

ENHANCEMENTS:

* OpenTofu will now suggest using `-exclude` if a provider reports that it cannot create a plan for a particular resource instance due to values that won't be known until the apply phase. ([#2643](https://github.com/opentofu/opentofu/pull/2643))
* `tofu validate` now supports running in a module that contains provider configuration_aliases. ([#2905](https://github.com/opentofu/opentofu/pull/2905))
* `tofu show` now supports a `-config` option, to be used in conjunction with `-json` to produce a machine-readable summary of the configuration without first creating a plan. ([#2820](https://github.com/opentofu/opentofu/pull/2820))

## Previous Releases

For information on prior major and minor releases, refer to their changelogs:

- [v1.10](https://github.com/opentofu/opentofu/blob/v1.10/CHANGELOG.md)
- [v1.9](https://github.com/opentofu/opentofu/blob/v1.9/CHANGELOG.md)
- [v1.8](https://github.com/opentofu/opentofu/blob/v1.8/CHANGELOG.md)
- [v1.7](https://github.com/opentofu/opentofu/blob/v1.7/CHANGELOG.md)
- [v1.6](https://github.com/opentofu/opentofu/blob/v1.6/CHANGELOG.md)
