## 1.7.0 (Unreleased)

UPGRADE NOTES:
* Backend/S3: The default of `use_legacy_workflow` changed to `false` and is now deprecated. The S3 backend will follow the same behavior as AWS CLI and SDKs for credential search, preferring backend configuration over environment variables. To support the legacy credential search workflow, you can set this option as `true`. It'll be completely removed in a future minor version.


NEW FEATURES:

ENHANCEMENTS:
* `nonsensitive` function no longer returns error when applied to values that are not sensitive ([#369](https://github.com/opentofu/opentofu/pull/369))
* Managing large local terraform.tfstate files is now much faster. ([#579](https://github.com/opentofu/opentofu/pull/579))
 - Previously, every call to state.Write() would also Persist to disk. This was not following the intended API and had longstanding TODOs in the code.
 - This change fixes the local state filesystem interface to function as the statemgr API describes.
 - A possible side effect is that a hard crash mid-apply will no longer have a in-progress state file to reference. This matches the other state managers.
* `tofu console` should work in Solaris and AIX as readline has been updated. ([#632](https://github.com/opentofu/opentofu/pull/632))
* Added "base64gunzip" function. ([$800](https://github.com/opentofu/opentofu/issues/800))
* Added "cidrcontains" function. ([$366](https://github.com/opentofu/opentofu/issues/366))
* Allow test run blocks to reference previous run block's module outputs ([#1129](https://github.com/opentofu/opentofu/pull/1129))

BUG FIXES:
* Fix view hooks unit test flakiness by deterministically waiting for heartbeats to execute ([$1153](https://github.com/opentofu/opentofu/issues/1153))
* `tofu test` resources cleanup at the end of tests changed to use simple reverse run block order. ([#1043](https://github.com/opentofu/opentofu/pull/1043))
* Fix access to known references when using a import block for module resources ([#1105](https://github.com/opentofu/opentofu/pull/1105))
* Show resource plan even if it failed plan due to `prevent_destroy` ([#1060](https://github.com/opentofu/opentofu/pull/1060))
* `tofu login` now can be interrrupted with `Ctrl+C` shortcut. ([#1074](https://github.com/opentofu/opentofu/pull/1074))
* Don't check for version conflicts when doing a force-unlock ([#1123](https://github.com/opentofu/opentofu/pull/1123))
* Fix Global Schema Cache not working in provider acceptance tests ([#1054](https://github.com/opentofu/opentofu/pull/1054))
* Fix `tofu show` and `tofu state show` not working with state files referencing Terraform registry providers in some instances ([#1141](https://github.com/opentofu/opentofu/pull/1141))

## Previous Releases

For information on prior major and minor releases, see their changelogs:

- [v1.6](https://github.com/opentofu/opentofu/blob/v1.6/CHANGELOG.md)
