## 1.7.8 (unreleased)

BUG FIXES:

- Provider used in import is correctly identified. ([#2336](https://github.com/opentofu/opentofu/pull/2336))
- `plantimestamp()` now returns unknown value during validation ([#2397](https://github.com/opentofu/opentofu/issues/2397))
- Syntax error in the `required_providers` block does not panic anymore, but yields "syntax error" ([2344](https://github.com/opentofu/opentofu/issues/2344))
- Changing Go version to 1.22.11 in order to fix [CVE-2024-45336](https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2024-45336) and [CVE-2024-45341](https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2024-45341)

## 1.7.7

BUG FIXES:
* Changes to encryption configuration now auto-apply the migration ([#2232](https://github.com/opentofu/opentofu/pull/2232))
* Updated github.com/golang-jwt/jwt/v4 from 4.4.2 to 4.5.1 to make security scanners happy (no vulnerability, see [#2179](https://github.com/opentofu/opentofu/pull/2179))

## 1.7.6

ENHANCEMENTS:

* OpenTofu builds now use Go version 1.22 ([#2050](https://github.com/opentofu/opentofu/issues/2050))

BUG FIXES:
* Extended trace logging for HTTP backend, including request and response bodies. ([#2120](https://github.com/opentofu/opentofu/pull/2120))

## 1.7.5

BUG FIXES:
* Provider functions will now handle partially unknown arguments per the tfplugin spec ([#2127](https://github.com/opentofu/opentofu/pull/2127))

## 1.7.4

ENHANCEMENTS:
* Made `tofu plan` with `generate-config-out` flag replace JSON strings with `jsonencode` functions calls. ([#1595](https://github.com/opentofu/opentofu/pull/1595))

BUG FIXES:
* `tofu init` will no longer return a supurious "Backend configuration changed" error when re-initializing a working directory with identical settings, backend configuration contains references to variables or local values, and when the `-backend-config` command line option is used. That combination previously caused OpenTofu to incorrectly treat the backend configuration as invalid. ([#2055](https://github.com/opentofu/opentofu/pull/2055))
* configuration generation should no longer fail when generating sensitive properties
* Provider defined functions are now supported better in child modules
* Fixed an issue where X-Terraform-Get was not being read correctly if a custom module registry returns a 200 statuscode instead of 201
* Fixed autoloaded test tfvar files being used in non-test scenarios ([#2039](https://github.com/opentofu/opentofu/pull/2039))

## 1.7.3

NEW FEATURES:

ENHANCEMENTS:

BUG FIXES:

* Fix bug where lower-case `http_proxy`/`https_proxy` env variables were no longer supported in the S3 backend ([#1594](https://github.com/opentofu/opentofu/issues/1594))

## 1.7.2

NEW FEATURES:

ENHANCEMENTS:

BUG FIXES:
* Fixed: authorization header is not included for HTTP backends ([#1656](https://github.com/opentofu/opentofu/pull/1656))
* Fixed: bug in import 'to' parsing in JSON configurations ([#1665](https://github.com/opentofu/opentofu/pull/1665))
* Fix bug where provider functions were unusable in variables and outputs ([#1689](https://github.com/opentofu/opentofu/pull/1689))

## 1.7.1

NEW FEATURES:

ENHANCEMENTS:

BUG FIXES:
* Fixed support for provider functions in tests ([#1603](https://github.com/opentofu/opentofu/pull/1603))
* Fixed crash in gcs backend when using certain commands ([#1618](https://github.com/opentofu/opentofu/pull/1618))
* Fix inmem backend crash due to missing struct field ([#1619](https://github.com/opentofu/opentofu/pull/1619))
* Fix for `tofu init` failure when test have spaces in their name. ([1489](https://github.com/opentofu/opentofu/pull/1489))
* `tofu test` now supports accessing module outputs when the module has no resources. ([#1409](https://github.com/opentofu/opentofu/pull/1409))

## 1.7.0

UPGRADE NOTES:
* Backend/S3: The default of `use_legacy_workflow` changed to `false` and is now deprecated. The S3 backend will follow the same behavior as AWS CLI and SDKs for credential search, preferring backend configuration over environment variables. To support the legacy credential search workflow, you can set this option as `true`. It'll be completely removed in a future minor version.

STATE ENCRYPTION
* We're introducing optional end-to-end encryption for state files.
* Available encryption methods as of now are:
  * AES GCM ([#1291](https://github.com/opentofu/opentofu/pull/1291))
* Available key providers:
  * Passphrase, via pbkdf2 ([#1310](https://github.com/opentofu/opentofu/pull/1310))
  * AWS KMS ([#1349](https://github.com/opentofu/opentofu/pull/1349))
  * GCP KMS ([#1392](https://github.com/opentofu/opentofu/pull/1392))
  * OpenBao ([#1436](https://github.com/opentofu/opentofu/pull/1436))

NEW FEATURES:
* Add support for a `removed` block that allows users to remove resources or modules from the state without destroying them. ([#1158](https://github.com/opentofu/opentofu/pull/1158))
* Provider-defined functions are now available.  They may be referenced via `provider::<provider_name>::<funcname>(args)`.  ([#1439](https://github.com/opentofu/opentofu/pull/1439))
* Add support for using `for_each` in `import` blocks ([#1492](https://github.com/opentofu/opentofu/pull/1492))

ENHANCEMENTS:
* Added support to use `.tfvars` files from tests folder. ([#1386](https://github.com/opentofu/opentofu/pull/1386))
* Added `templatestring` function that takes a string and renders it as a template using a supplied set of template variables. ([#1223](https://github.com/opentofu/opentofu/pull/1223))
* Added `base64gunzip` function that takes a base64 encoded gzip string and returns the decompressed data as a string. ([#800](https://github.com/opentofu/opentofu/issues/800))
* Added `cidrcontains` function that determines if an address belongs to a certain prefix. ([#366](https://github.com/opentofu/opentofu/issues/366))
* Added `urldecode` function that will decode a url-encoded string. ([#1234](https://github.com/opentofu/opentofu/issues/1234))
* Added `issensitive` function that returns whether a value is sensitive. ([#1370](https://github.com/opentofu/opentofu/issues/1370))
* Added `-concise` flag to omit the refreshing state logs when tofu plan is run. ([#1225](https://github.com/opentofu/opentofu/pull/1225))
* `nonsensitive` function no longer returns error when applied to values that are not sensitive ([#369](https://github.com/opentofu/opentofu/pull/369))
* Managing large local terraform.tfstate files is now much faster. ([#579](https://github.com/opentofu/opentofu/pull/579))
  * Previously, every call to state.Write() would also Persist to disk. This was not following the intended API and had longstanding TODOs in the code.
  * This change fixes the local state filesystem interface to function as the statemgr API describes.
  * A possible side effect is that a hard crash mid-apply will no longer have a in-progress state file to reference. This matches the other state managers.
* `tofu console` should work in Solaris and AIX as readline has been updated. ([#632](https://github.com/opentofu/opentofu/pull/632))
* Allow test run blocks to reference previous run block's module outputs ([#1129](https://github.com/opentofu/opentofu/pull/1129))
* Support the XDG Base Directory Specification ([#1200](https://github.com/opentofu/opentofu/pull/1200))
* Allow referencing the output from a test run in the local variables block of another run (tofu test). ([#1254](https://github.com/opentofu/opentofu/pull/1254))
* Allow for `templatefile` function recursion (up to 1024 call depth default). ([#1250](https://github.com/opentofu/opentofu/pull/1250))
* Dump state file when `tofu test` fails to clean up resources. ([#1243](https://github.com/opentofu/opentofu/pull/1243))
* Added aliases for `state list` (`state ls`), `state mv` (`state move`), and `state rm` (`state remove`) ([#1220](https://github.com/opentofu/opentofu/pull/1220))
* Added mechanism to introduce automatic retries for provider installations, specifically targeting transient errors ([#1233](https://github.com/opentofu/opentofu/issues/1233))
* Added `-json` flag to `tofu init` and `tofu get` to support output in json format. ([#1453](https://github.com/opentofu/opentofu/pull/1453))
* `import` blocks `to` address can now support dynamic values (like variables, locals, conditions, and references to resources or data blocks) in index keys. ([#1270](https://github.com/opentofu/opentofu/pull/1270))

BUG FIXES:
* Fix view hooks unit test flakiness by deterministically waiting for heartbeats to execute ([$1153](https://github.com/opentofu/opentofu/issues/1153))
* `tofu test` resources cleanup at the end of tests changed to use simple reverse run block order. ([#1043](https://github.com/opentofu/opentofu/pull/1043))
* Fix access to known references when using a import block for module resources ([#1105](https://github.com/opentofu/opentofu/pull/1105))
* Show resource plan even if it failed plan due to `prevent_destroy` ([#1060](https://github.com/opentofu/opentofu/pull/1060))
* `tofu login` now can be interrupted with `Ctrl+C` shortcut. ([#1074](https://github.com/opentofu/opentofu/pull/1074))
* Don't check for version conflicts when doing a force-unlock ([#1123](https://github.com/opentofu/opentofu/pull/1123))
* Fix Global Schema Cache not working in provider acceptance tests ([#1054](https://github.com/opentofu/opentofu/pull/1054))
* Fix `tofu show` and `tofu state show` not working with state files referencing Terraform registry providers in some instances ([#1141](https://github.com/opentofu/opentofu/pull/1141))
* Improved stability on 32-bit architectures ([#1154](https://github.com/opentofu/opentofu/pull/1154))
* Don't show false update action when import resource with sensitive datasource([#1220](https://github.com/opentofu/opentofu/pull/1220))
* Fix panic when provisioner source and content are both null ([#1376](https://github.com/opentofu/opentofu/pull/1376))
* Fix large number will be truncated in plan ([#1382](https://github.com/opentofu/opentofu/pull/1382))
* S3 backend no longer requires to have permissions to use the default `env:` workspace prefix ([#1445](https://github.com/opentofu/opentofu/pull/1445))
* Fixed a crash when using a conditional with Twingate resource ([#1446](https://github.com/opentofu/opentofu/pull/1446))
* Added support for user-defined headers when configuring the HTTP backend ([#1427](https://github.com/opentofu/opentofu/pull/1487))

## Previous Releases

For information on prior major and minor releases, see their changelogs:

- [v1.6](https://github.com/opentofu/opentofu/blob/v1.6/CHANGELOG.md)
