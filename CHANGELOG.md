## 1.8.4 (Unreleased)

BUG FIXES:

* `tofu init` will no longer return a supurious "Backend configuration changed" error when re-initializing a working directory with identical settings, backend configuration contains references to variables or local values, and when the `-backend-config` command line option is used. That combination previously caused OpenTofu to incorrectly treat the backend configuration as invalid. ([#2055](https://github.com/opentofu/opentofu/pull/2055))
* configuration generation should no longer fail when generating sensitive properties
* Provider defined functions are now supported better in child modules
* Fixed an issue where X-Terraform-Get was not being read correctly if a custom module registry returns a 200 statuscode instead of 201

## 1.8.3

SECURITY:
* Added option to enable the sensitive flag for variables used in module sources/versions and backend configurations.
  * This emits a warning by default to prevent breaking compatability with previous 1.8.x versions.
  * It is *highly recommended* to set `TOFU_ENABLE_STATIC_SENSITIVE=1` in any environments using this release.
  * This will be enabled by default as a breaking change in v1.9.0

BUG FIXES:
* Fixed autoloaded test tfvar files being used in non-test scenarios ([#2039](https://github.com/opentofu/opentofu/pull/2039))
* Fixed crash when using sensitive values in module sources/versions and backend configurations ([#2046](https://github.com/opentofu/opentofu/pull/2046))

## 1.8.2

SECURITY:
* Update go version to 1.21.11 to fix CVE-2024-24790

BUG FIXES:
* Better handling of key_provider references ([#1965](https://github.com/opentofu/opentofu/pull/1965))

## 1.8.1

BUG FIXES:
* Fixed crash when module source is not present ([#1888](https://github.com/opentofu/opentofu/pull/1888))

## 1.8.0

UPGRADE NOTES:
* BREAKING CHANGE - `use_legacy_workflow` field has been removing from the S3 backend configuration. ([#1730](https://github.com/opentofu/opentofu/pull/1730))
* SECURITY - Bump github.com/hashicorp/go-getter to fix CVE-2024-6257, may cause performance hit for large modules ([#1751](https://github.com/opentofu/opentofu/pull/1751))

NEW FEATURES:
* Added support for `override_resource`, `override_data` and `override_module` blocks in testing framework. ([#1499](https://github.com/opentofu/opentofu/pull/1499))
* Variables and Locals allowed in module sources and backend configurations (with limitations) ([#1718](https://github.com/opentofu/opentofu/pull/1718))
* Added support to new .tofu extensions to allow tofu-specific overrides of .tf files ([#1738](https://github.com/opentofu/opentofu/pull/1738))
* Added support for `mock_provider`, `mock_resource` and `mock_data` blocks in testing framework. ([#1772](https://github.com/opentofu/opentofu/pull/1772))

ENHANCEMENTS:
* Added `tofu test -json` types to website Machine-Readable UI documentation. ([#1408](https://github.com/opentofu/opentofu/issues/1408))
* Made `tofu plan` with `generate-config-out` flag replace JSON strings with `jsonencode` functions calls. ([#1595](https://github.com/opentofu/opentofu/pull/1595))
* Make state persistence interval configurable via `TF_STATE_PERSIST_INTERVAL` environment variable ([#1591](https://github.com/opentofu/opentofu/pull/1591))
* Improved performance of writing state files and reduced their size using compact json encoding. ([#1647](https://github.com/opentofu/opentofu/pull/1647))
* Allow to reference variable inside the `variables` block of a test file. ([#1488](https://github.com/opentofu/opentofu/pull/1488))
* Allow variables and other static values to be used in encryption configuration. ([#1728](https://github.com/opentofu/opentofu/pull/1728))
* Included provider function in `tofu providers schema` command ([#1753](https://github.com/opentofu/opentofu/pull/1753))

BUG FIXES:
* Fixed validation for `enforced` flag in encryption configuration. ([#1711](https://github.com/opentofu/opentofu/pull/1711))
* Fixed crash in gcs backend when using certain commands. ([#1618](https://github.com/opentofu/opentofu/pull/1618))
* Fixed inmem backend crash due to missing struct field. ([#1619](https://github.com/opentofu/opentofu/pull/1619))
* Added a check in the `tofu test` to validate that the names of test run blocks do not contain spaces. ([#1489](https://github.com/opentofu/opentofu/pull/1489))
* `tofu test` now supports accessing module outputs when the module has no resources. ([#1409](https://github.com/opentofu/opentofu/pull/1409))
* Fixed support for provider functions in tests ([#1603](https://github.com/opentofu/opentofu/pull/1603))
* Only hide sensitive attributes in plan detail when plan on a set of resources ([#1313](https://github.com/opentofu/opentofu/pull/1313))
* Added a better error message on `for_each` block with sensitive value of unsuitable type. ([#1485](https://github.com/opentofu/opentofu/pull/1485))
* Fix race condition on locking in gcs backend ([#1342](https://github.com/opentofu/opentofu/pull/1342))
* Fix bug where provider functions were unusable in variables and outputs ([#1689](https://github.com/opentofu/opentofu/pull/1689))
* Fix bug where lower-case `http_proxy`/`https_proxy` env variables were no longer supported in the S3 backend ([#1594](https://github.com/opentofu/opentofu/issues/1594))
* Fixed issue with migration between versions can cause an update in-place for resources when no changes are needed. ([#1640](https://github.com/opentofu/opentofu/pull/1640))
* Add source context for the 'insufficient feature blocks' error ([#1777](https://github.com/opentofu/opentofu/pull/1777))
* Remove encryption diags from autocomplete ([#1793](https://github.com/opentofu/opentofu/pull/1793))
* Ensure that using a sensitive path for templatefile that it doesn't panic([#1801](https://github.com/opentofu/opentofu/issues/1801))

## Previous Releases

For information on prior major and minor releases, see their changelogs:

- [v1.7](https://github.com/opentofu/opentofu/blob/v1.7/CHANGELOG.md)
- [v1.6](https://github.com/opentofu/opentofu/blob/v1.6/CHANGELOG.md)
