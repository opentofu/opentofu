## 1.9.0 (Unreleased)

UPGRADE NOTES:

* Using the `ghcr.io/opentofu/opentofu` image as a base image for custom images is deprecated and this will be removed in OpenTofu 1.10. Please see https://opentofu.org/docs/intro/install/docker/ for instructions on building your own image.

NEW FEATURES:

ENHANCEMENTS:
* Added user input prompt for static variables. ([#1792](https://github.com/opentofu/opentofu/issues/1792))
* Added `-show-sensitive` flag to tofu plan, apply, state-show and output commands to display sensitive data in output. ([#1554](https://github.com/opentofu/opentofu/pull/1554))
* Improved performance for large graphs when debug logs are not enabled. ([#1810](https://github.com/opentofu/opentofu/pull/1810))
* Improved performance for large graphs with many submodules. ([#1809](https://github.com/opentofu/opentofu/pull/1809))
* Added multi-line support to the `tofu console` command. ([#1307](https://github.com/opentofu/opentofu/issues/1307))
* Added a help target to the Makefile. ([#1925](https://github.com/opentofu/opentofu/pull/1925))
* Added a simplified Build Process with a Makefile Target ([#1926](https://github.com/opentofu/opentofu/issues/1926))
* Ensures that the Makefile adheres to POSIX standards ([#1811](https://github.com/opentofu/opentofu/pull/1928))
* Added for-each support to providers. ([#300](https://github.com/opentofu/opentofu/issues/300))

BUG FIXES:
* Ensure that using a sensitive path for templatefile that it doesn't panic([#1801](https://github.com/opentofu/opentofu/issues/1801))
* Fixed crash when module source is not present ([#1888](https://github.com/opentofu/opentofu/pull/1888))
* Added error handling for `force-unlock` command when locking is disabled for S3, HTTP, and OSS backends. [#1977](https://github.com/opentofu/opentofu/pull/1977)
* Ensured that using a sensitive path for templatefile that it doesn't panic([#1801](https://github.com/opentofu/opentofu/issues/1801))
* Fixed a crash when module source is not present ([#1888](https://github.com/opentofu/opentofu/pull/1888))
* Fixed a crash when importing an empty optional sensitive string ([#1986](https://github.com/opentofu/opentofu/pull/1986))
* Fixed autoloaded test tfvar files being used in non-test scenarios ([#2039](https://github.com/opentofu/opentofu/pull/2039))

## Previous Releases

For information on prior major and minor releases, see their changelogs:

- [v1.8](https://github.com/opentofu/opentofu/blob/v1.8/CHANGELOG.md)
- [v1.7](https://github.com/opentofu/opentofu/blob/v1.7/CHANGELOG.md)
- [v1.6](https://github.com/opentofu/opentofu/blob/v1.6/CHANGELOG.md)
