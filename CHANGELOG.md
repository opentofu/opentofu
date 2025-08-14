## 1.11.0 (Unreleased)

UPGRADE NOTES:

* OpenTofu on macOS now requires macOS 12 Monterey or later.
* The `issensitive` function previously incorrectly returned known results when given unknown values, which has now been corrected to avoid confusing consistency check failures during the apply phase, as reported in [issue #2415](https://github.com/opentofu/opentofu/issues/2415).

    If your module was previously assigning something derived from an `issensitive` result to a context where unknown values are not allowed during the planning phase, such as `count`/`for_each` arguments for resources or modules, this will now fail during the planning phase and so you will need to choose a new approach where either the `issensitive` argument is always known during the planning phase or where the sensitivity of an unknown value is not used as part of the decision.
* OpenTofu no longer accepts SHA-1 signatures in TLS handshakes, as recommended in [RFC 9155](https://www.rfc-editor.org/rfc/rfc9155.html).

* Testing mocks previously only followed a subset of the rules defined in provider schemas. The provider schema now drives the mocking to ensure the schema is correctly followed. ([#3069](https://github.com/opentofu/opentofu/pull/3069))

    In rare cases this change might result in some previously-passing tests now failing, due to invalid mocks or overrides that were not detected in earlier versions.

ENHANCEMENTS:

* OpenTofu will now suggest using `-exclude` if a provider reports that it cannot create a plan for a particular resource instance due to values that won't be known until the apply phase. ([#2643](https://github.com/opentofu/opentofu/pull/2643))
* `tofu validate` now supports running in a module that contains provider configuration_aliases. ([#2905](https://github.com/opentofu/opentofu/pull/2905))
* The `regex` and `regexall` functions now support using `\p` and `\P` sequences with the long-form names for Unicode general character properties. For example, `\p{Letter}` now has the same meaning as `\p{L}`. ([#3166](https://github.com/opentofu/opentofu/pull/3166))
* `tofu show` now supports `-config` and `-module=DIR` options, to be used in conjunction with `-json` to produce a machine-readable summary of either the whole configuration or a single module without first creating a plan. ([#2820](https://github.com/opentofu/opentofu/pull/2820), [#3003](https://github.com/opentofu/opentofu/pull/3003))
* [The JSON representation of configuration](https://opentofu.org/docs/internals/json-format/#configuration-representation) returned by `tofu show` in `-json` mode now includes type constraint information for input variables and whether each input variable is required, in addition to the existing properties related to input variables. ([#3013](https://github.com/opentofu/opentofu/pull/3013))
* Multiline string updates in arrays are now diffed line-by-line, rather than as a single element, making it easier to see changes in the plan output. ([#3030](https://github.com/opentofu/opentofu/pull/3030))
* Add full support for -var, -var-file, and TF_VARS during `tofu apply` to support plan encryption ([#1998](https://github.com/opentofu/opentofu/pull/1998))
* The S3 state backend now supports arguments to specify tags of the state and lock files. [#3038](https://github.com/opentofu/opentofu/pull/3038)
* Upgrade go from 1.24.4 to 1.24.6 to fix [GO-2025-3849](https://pkg.go.dev/vuln/GO-2025-3849) ([3127](https://github.com/opentofu/opentofu/pull/3127))
* Improved error messages when a submodule is not found in a module ([#3144]https://github.com/opentofu/opentofu/pull/3144)
* Add support for the `for_each` attribute in the `mock_provider` block. ([#3087](https://github.com/opentofu/opentofu/pull/3087))
* Upgrade github.com/openbao/openbao/api/v2 from 2.1.0 to 2.3.0 to fix [GO-2025-3783](https://pkg.go.dev/vuln/GO-2025-3783) ([3134](https://github.com/opentofu/opentofu/pull/3134))
  * The upgrade is necessary to silence the security scanner and does not affect the actual state encryption provided by OpenBao.
* Add logs for the DynamoDB operations in the S3 backend ([#3103](https://github.com/opentofu/opentofu/pull/3103))

BUG FIXES:

* The `tofu.rc` configuration file now properly takes precedence over `terraform.rc` on Windows ([#2891](https://github.com/opentofu/opentofu/pull/2891))
* S3 backend now correctly sends the `x-amz-server-side-encryption` header for the lockfile ([#2870](https://github.com/opentofu/opentofu/issues/2970))
* The `import` block now correctly validates the `id` property. ([#2416](https://github.com/opentofu/opentofu/issues/2416)
* Allow function calls in test variable blocks ([#2947](https://github.com/opentofu/opentofu/pull/2947))
* The `issensitive` function now returns an unknown result when its argument is unknown, since a sensitive unknown value can potentially become non-sensitive once more information is available. ([#3008](https://github.com/opentofu/opentofu/pull/3008))
* Provider references like "null.some_alias[each.key]" in .tf.json files are now correctly parsed ([#2915](https://github.com/opentofu/opentofu/issues/2915))
* Fixed crash when processing multiple deprecated marks on a complex object ([#3105](https://github.com/opentofu/opentofu/pull/3105))
* Variables with validation no longer interfere with the destroy process ([#3131](https://github.com/opentofu/opentofu/pull/3131))
* Ensure that generated mock values for testing correctly follows the provider schema. ([#3069](https://github.com/opentofu/opentofu/pull/3069))

BREAKING CHANGES:

## Previous Releases

For information on prior major and minor releases, refer to their changelogs:

- [v1.10](https://github.com/opentofu/opentofu/blob/v1.10/CHANGELOG.md)
- [v1.9](https://github.com/opentofu/opentofu/blob/v1.9/CHANGELOG.md)
- [v1.8](https://github.com/opentofu/opentofu/blob/v1.8/CHANGELOG.md)
- [v1.7](https://github.com/opentofu/opentofu/blob/v1.7/CHANGELOG.md)
- [v1.6](https://github.com/opentofu/opentofu/blob/v1.6/CHANGELOG.md)
