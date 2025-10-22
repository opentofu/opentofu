## 1.8.12 (Unreleased)

BUG FIXES:

* Fix missing provider functions when parentheses are used ([#3402](https://github.com/opentofu/opentofu/pull/3402))

## 1.8.11

BUG FIXES:
- Fixed incorrect approach to mocking provider "ReadResource" calls in test. ([#3068](https://github.com/opentofu/opentofu/pull/3068))
- Reduced calls to  ListKeys in azure backend (for rate limiting). ([#3083](https://github.com/opentofu/opentofu/pull/3083))

## 1.8.10

BUG FIXES:
- OpenTofu will no longer crash in a rare case where a dynamically-invalid expression has its error suppressed by `try` or `can` and then that expression becomes relevant for deciding whether to report a "change outside of OpenTofu" in the human-oriented plan diff. ([#2988](https://github.com/opentofu/opentofu/pull/2988))
- Ensure provider downloads into temp are cleaned up correctly on windows. ([#2843](https://github.com/opentofu/opentofu/issues/2843))
- Correctly handle structural typed attributes during test provider mocking. ([#2994](https://github.com/opentofu/opentofu/pull/2994))
- Fix erroneous detection of changes with sensitive resource attributes. ([#3024](https://github.com/opentofu/opentofu/pull/3024))


## 1.8.9

BUG FIXES:

- Provider used in import is correctly identified. ([#2336](https://github.com/opentofu/opentofu/pull/2336))
- `plantimestamp()` now returns unknown value during validation ([#2397](https://github.com/opentofu/opentofu/issues/2397))
- Syntax error in the `required_providers` block does not panic anymore, but yields "syntax error" ([2344](https://github.com/opentofu/opentofu/issues/2344))
- Fix the error message when default value of a complex variable is containing a wrong type ([2394](https://github.com/opentofu/opentofu/issues/2394))
- Fix the way OpenTofu downloads a module that is sourced from a GitHub branch containing slashes in the name. ([2396](https://github.com/opentofu/opentofu/issues/2396))
- Changing Go version to 1.22.11 in order to fix [CVE-2024-45336](https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2024-45336) and [CVE-2024-45341](https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2024-45341) ([#2438](https://github.com/opentofu/opentofu/pull/2438))
- Changing Go version to 1.22.12 in order to fix [CVE-2025-22866](https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2025-22866) and [CVE-2024-45341](https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2024-45341) ([#2438](https://github.com/opentofu/opentofu/pull/2438))


## 1.8.8

SECURITY:
* Upgraded `golang.org/x/crypto` to resolve CVE-2024-45337. ([#2287](https://github.com/opentofu/opentofu/pull/2287))
* Upgraded `golang.org/x/net` to resolve CVE-2024-45338. ([#2311](https://github.com/opentofu/opentofu/pull/2311))

BUG FIXES:
* `tofu test` now removes outputs of destroyed modules between different test runs. ([#2274](https://github.com/opentofu/opentofu/pull/2274))

## 1.8.7

BUG FIXES:
* Error messages related to validation of sensitive input variables will no longer disclose the sensitive value in the UI. ([#2219](https://github.com/opentofu/opentofu/pull/2219))
* Changes to encryption configuration now auto-apply the migration ([#2232](https://github.com/opentofu/opentofu/pull/2232))
* Updated github.com/golang-jwt/jwt/v4 from 4.4.2 to 4.5.1 to make security scanners happy (no vulnerability, see [#2179](https://github.com/opentofu/opentofu/pull/2179))
* `tofu test` is now setting `null`s for dynamic type when generating mock values. ([#2245](https://github.com/opentofu/opentofu/pull/2245))
* Variables declared in test files are now taking into account type default values. ([#2244](https://github.com/opentofu/opentofu/pull/2244))

## 1.8.6

ENHANCEMENTS:
* OpenTofu builds now use Go version 1.22 ([#2050](https://github.com/opentofu/opentofu/issues/2050))

BUG FIXES:
* Extended trace logging for HTTP backend, including request and response bodies. ([#2120](https://github.com/opentofu/opentofu/pull/2120))
* The `tofu test` command doesn't try to validate mock provider definition by its underlying provider schema now. ([#2140](https://github.com/opentofu/opentofu/pull/2140))
* Type validation for mocks and overrides are now less strict in `tofu test`. ([#2144](https://github.com/opentofu/opentofu/pull/2144))

## 1.8.5

BUG FIXES:
* Provider functions will now handle partially unknown arguments per the tfplugin spec ([#2127](https://github.com/opentofu/opentofu/pull/2127))
* `tofu init` will no longer return a spurious "Backend configuration changed" error when re-initializing a working directory with existing initialization of a backend whose configuration schema has required arguments. This was a regression caused by the similar fix in the v1.8.4 release. ([#2135](https://github.com/opentofu/opentofu/pull/2135))

## 1.8.4

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
