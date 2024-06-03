## 1.8.0 (Unreleased)

UPGRADE NOTES:

NEW FEATURES:

ENHANCEMENTS:
* Added `tofu test -json` types to website Machine-Readable UI documentation ([1408](https://github.com/opentofu/opentofu/issues/1408))
* Made `tofu plan` with `generate-config-out` flag replace JSON strings with `jsonencode` functions calls. ([#1595](https://github.com/opentofu/opentofu/pull/1595))
* Make state persistence interval configurable via `TF_STATE_PERSIST_INTERVAL` environment variable ([#1591](https://github.com/opentofu/opentofu/pull/1591))
* Improved performance of writing state files and reduced their size using compact json encoding. ([#1647](https://github.com/opentofu/opentofu/pull/1647))
* Allow to reference variable inside the `variables` block of a test file. ([1488](https://github.com/opentofu/opentofu/pull/1488))

BUG FIXES:
* Fixed crash in gcs backend when using certain commands ([#1618](https://github.com/opentofu/opentofu/pull/1618))
* Fix inmem backend crash due to missing struct field ([#1619](https://github.com/opentofu/opentofu/pull/1619))
* Added a check in the `tofu test` to validate that the names of test run blocks do not contain spaces. ([#1489](https://github.com/opentofu/opentofu/pull/1489))
* `tofu test` now supports accessing module outputs when the module has no resources. ([#1409](https://github.com/opentofu/opentofu/pull/1409))
* Fixed support for provider functions in tests ([#1603](https://github.com/opentofu/opentofu/pull/1603))
* Added a better error message on `for_each` block with sensitive value of unsuitable type. ([#1485](https://github.com/opentofu/opentofu/pull/1485))
* Fix race condition on locking in gcs backend ([#1342](https://github.com/opentofu/opentofu/pull/1342))
* Fix bug where provider functions were unusable in variables and outputs ([#1689](https://github.com/opentofu/opentofu/pull/1689))

## Previous Releases

For information on prior major and minor releases, see their changelogs:

- [v1.7](https://github.com/opentofu/opentofu/blob/v1.7/CHANGELOG.md)
- [v1.6](https://github.com/opentofu/opentofu/blob/v1.6/CHANGELOG.md)
