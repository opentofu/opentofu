## 1.11.0 (Unreleased)

UPGRADE NOTES:

* The `issensitive` function previously incorrectly returned known results when given unknown values, which has now been corrected to avoid confusing consistency check failures during the apply phase, as reported in [issue #2415](https://github.com/opentofu/opentofu/issues/2415).

    If your module was previously assigning something derived from an `issensitive` result to a context where unknown values are not allowed during the planning phase, such as `count`/`for_each` arguments for resources or modules, this will now fail during the planning phase and so you will need to choose a new approach where either the `issensitive` argument is always known during the planning phase or where the sensitivity of an unknown value is not used as part of the decision.

ENHANCEMENTS:

* OpenTofu will now suggest using `-exclude` if a provider reports that it cannot create a plan for a particular resource instance due to values that won't be known until the apply phase. ([#2643](https://github.com/opentofu/opentofu/pull/2643))
* `tofu validate` now supports running in a module that contains provider configuration_aliases. ([#2905](https://github.com/opentofu/opentofu/pull/2905))
* `tofu show` now supports `-config` and `-module=DIR` options, to be used in conjunction with `-json` to produce a machine-readable summary of either the whole configuration or a single module without first creating a plan. ([#2820](https://github.com/opentofu/opentofu/pull/2820), [#3003](https://github.com/opentofu/opentofu/pull/3003))
* [The JSON representation of configuration](https://opentofu.org/docs/internals/json-format/#configuration-representation) returned by `tofu show` in `-json` mode now includes type constraint information for input variables and whether each input variable is required, in addition to the existing properties related to input variables. ([#3013](https://github.com/opentofu/opentofu/pull/3013))
* Multiline string updates in arrays are now diffed line-by-line, rather than as a single element, making it easier to see changes in the plan output. ([#3030](https://github.com/opentofu/opentofu/pull/3030))

BUG FIXES:

* The `tofu.rc` configuration file now properly takes precedence over `terraform.rc` on Windows ([#2891](https://github.com/opentofu/opentofu/pull/2891))
* S3 backend now correctly sends the `x-amz-server-side-encryption` header for the lockfile ([#2870](https://github.com/opentofu/opentofu/issues/2970))
* The `import` block now correctly validates the `id` property. ([#2416](https://github.com/opentofu/opentofu/issues/2416)
* Allow function calls in test variable blocks ([#2947](https://github.com/opentofu/opentofu/pull/2947))
* The `issensitive` function now returns an unknown result when its argument is unknown, since a sensitive unknown value can potentially become non-sensitive once more information is available. ([#3008](https://github.com/opentofu/opentofu/pull/3008))
* Provider references like "null.some_alias[each.key]" in .tf.json files are now correctly parsed ([#2915](https://github.com/opentofu/opentofu/issues/2915))

## Previous Releases

For information on prior major and minor releases, refer to their changelogs:

- [v1.10](https://github.com/opentofu/opentofu/blob/v1.10/CHANGELOG.md)
- [v1.9](https://github.com/opentofu/opentofu/blob/v1.9/CHANGELOG.md)
- [v1.8](https://github.com/opentofu/opentofu/blob/v1.8/CHANGELOG.md)
- [v1.7](https://github.com/opentofu/opentofu/blob/v1.7/CHANGELOG.md)
- [v1.6](https://github.com/opentofu/opentofu/blob/v1.6/CHANGELOG.md)
