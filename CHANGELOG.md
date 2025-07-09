## 1.11.0 (Unreleased)

UPGRADE NOTES:

* The `issensitive` function previously incorrectly returned known results when given unknown values, which has now been corrected to avoid confusing consistency check failures during the apply phase, as reported in [issue #2415](https://github.com/opentofu/opentofu/issues/2415).

    If your module was previously assigning something derived from an `issensitive` result to a context where unknown values are not allowed during the planning phase, such as `count`/`for_each` arguments for resources or modules, this will now fail during the planning phase and so you will need to choose a new approach where either the `issensitive` argument is always known during the planning phase or where the sensitivity of an unknown value is not used as part of the decision.

ENHANCEMENTS:

* OpenTofu will now suggest using `-exclude` if a provider reports that it cannot create a plan for a particular resource instance due to values that won't be known until the apply phase. ([#2643](https://github.com/opentofu/opentofu/pull/2643))
* `tofu validate` now supports running in a module that contains provider configuration_aliases. ([#2905](https://github.com/opentofu/opentofu/pull/2905))
* `tofu show` now supports a `-config` option, to be used in conjunction with `-json` to produce a machine-readable summary of the configuration without first creating a plan. ([#2820](https://github.com/opentofu/opentofu/pull/2820))

BUG FIXES:

* The `tofu.rc` configuration file now properly takes precedence over `terraform.rc` on Windows ([#2891](https://github.com/opentofu/opentofu/pull/2891))
* S3 backend now correctly sends the `x-amz-server-side-encryption` header for the lockfile ([#2870](https://github.com/opentofu/opentofu/issues/2970))
* Allow function calls in test variable blocks ([#2947](https://github.com/opentofu/opentofu/pull/2947))
* The `issensitive` function now returns an unknown result when its argument is unknown, since a sensitive unknown value can potentially become non-sensitive once more information is available. ([#3008](https://github.com/opentofu/opentofu/pull/3008))

## Previous Releases

For information on prior major and minor releases, refer to their changelogs:

- [v1.10](https://github.com/opentofu/opentofu/blob/v1.10/CHANGELOG.md)
- [v1.9](https://github.com/opentofu/opentofu/blob/v1.9/CHANGELOG.md)
- [v1.8](https://github.com/opentofu/opentofu/blob/v1.8/CHANGELOG.md)
- [v1.7](https://github.com/opentofu/opentofu/blob/v1.7/CHANGELOG.md)
- [v1.6](https://github.com/opentofu/opentofu/blob/v1.6/CHANGELOG.md)
