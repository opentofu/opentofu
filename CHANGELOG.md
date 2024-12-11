## 1.9.0 (Unreleased)

UPGRADE NOTES:

* Using the `ghcr.io/opentofu/opentofu` image as a base image for custom images is deprecated and this will be removed in OpenTofu 1.10.

    Please refer to https://opentofu.org/docs/main/intro/install/docker/ for instructions on building your own image.

NEW FEATURES:

* **`for_each` in provider configuration blocks:** An alternate (aka "aliased") provider configuration can now have multiple dynamically-chosen instances using the `for_each` argument:

    ```hcl
    provider "aws" {
      alias    = "by_region"
      for_each = var.aws_regions

      region = each.key
    }
    ```

    Each instance of a resource can also potentially select a different instance of the associated provider configuration, making it easier to declare infrastructure that ought to be duplicated for each region.

* **`-exclude` planning option:** similar to `-target`, this allows operators to tell OpenTofu to work on only a subset of the objects declared in the configuration or tracked in the state.

    ```shell
    tofu plan -exclude=kubernetes_manifest.crds
    ```

    While `-target` specifies the objects to _include_ and skips everything not needed for the selected objects, `-exclude` instead specifies objects to skip. OpenTofu will exclude the selected objects and everything that depends on them.

ENHANCEMENTS:
* OpenTofu builds now use Go version 1.22 ([#2050](https://github.com/opentofu/opentofu/issues/2050))
* `provider` blocks now support `for_each`. ([#2123](https://github.com/opentofu/opentofu/issues/2123))
* The new `-exclude` planning option complements `-target`, specifying what to exclude rather than what to include. ([#1900](https://github.com/opentofu/opentofu/pull/1900))
* State encryption key providers now support customizing the metadata key via `encrypted_metadata_alias`. ([#2080](https://github.com/opentofu/opentofu/pull/2080))
* OpenTofu will now prompt for values for input variables needed for early evaluation. ([#2047](https://github.com/opentofu/opentofu/pull/2047))
* Various commands now accept `-consolidate-warnings` and `-consolidate-errors` options to enable or disable OpenTofu's summarization of diagnostic messages. ([#1894](https://github.com/opentofu/opentofu/pull/1894))
* `-show-sensitive` option causes `tofu plan`, `tofu apply`, and other commands that can return data from the configuration or state to unmask sensitive values. ([#1554](https://github.com/opentofu/opentofu/pull/1554))
* `tofu console` now accepts expressions split over multiple lines, when the newline characters appear inside bracketing pairs or when they are escaped using a backslash. ([#1875](https://github.com/opentofu/opentofu/pull/1875))
* Improved performance for large graphs when debug logs are not enabled. ([#1810](https://github.com/opentofu/opentofu/pull/1810))
* Improved performance for large graphs with many submodules. ([#1809](https://github.com/opentofu/opentofu/pull/1809))
* Extended trace logging for HTTP backend, including request and response bodies. ([#2120](https://github.com/opentofu/opentofu/pull/2120))
* `tofu test` now throws errors instead of warnings for invalid override and mock fields. ([#2220](https://github.com/opentofu/opentofu/pull/2220))
* `tofu test` now supports `override_resource` and `override_data` blocks in the scope of a single `mock_provider`. ([#2168](https://github.com/opentofu/opentofu/pull/2168))
* Changes to encryption configuration now auto-apply the migration ([#2232](https://github.com/opentofu/opentofu/pull/2232))
* References to vars, data, etc. are now usable in variable validation ([#2216](https://github.com/opentofu/opentofu/pull/2216))
* `AzureRM` backend now support `timeout_seconds` with default timeout of 300 seconds ([#2263](https://github.com/opentofu/opentofu/pull/2263))
* Adds warning about provider version `0.0.0` ([#2281](https://github.com/opentofu/opentofu/pull/2281))

BUG FIXES:
* `templatefile` no longer crashes if the given filename is derived from a sensitive value. ([#1801](https://github.com/opentofu/opentofu/issues/1801))
* Configuration loading no longer crashes when a `module` block lacks the required `source` argument. ([#1888](https://github.com/opentofu/opentofu/pull/1888))
* The `tofu force-unlock` command now returns a relevant error when used with a backend that is not configured to support locking. ([#1977](https://github.com/opentofu/opentofu/pull/1977))
* Configuration generation during import no longer crashes if an imported object includes sensitive values. ([#1986](https://github.com/opentofu/opentofu/pull/1986), [#2077](https://github.com/opentofu/opentofu/pull/2077))
* `.tfvars` files from the `tests` directly are no longer incorrectly loaded for non-test commands. ([#2039](https://github.com/opentofu/opentofu/pull/2039))
* `tofu console`'s interactive mode now handles the special `exit` command correctly. ([#2086](https://github.com/opentofu/opentofu/pull/2086))
* Provider-contributed functions are now called correctly when used in the `validation` block of an input variable declaration. ([#2052](https://github.com/opentofu/opentofu/pull/2052))
* Sensitive values are now prohibited in early evaluation of backend configuration and module source locations, because otherwise they would be exposed as a side-effect of initializing the backend or installing a module. ([#2045](https://github.com/opentofu/opentofu/pull/2045))
* `tofu providers mirror` no longer crashes when the dependency lock file has missing or invalid entries. ([#1985](https://github.com/opentofu/opentofu/pull/1985))
* OpenTofu now respects a provider-contributed functions' request to be called only when its arguments are fully known, for compatibility with functions that cannot handle unknown values themselves. ([#2127](https://github.com/opentofu/opentofu/pull/2127))
* `tofu init` no longer duplicates diagnostic messages produced when evaluating early-evaluation expressions. ([#1890](https://github.com/opentofu/opentofu/pull/1890))
* `tofu plan` change description now includes information about configuration blocks generated using a `dynamic` block with an unknown `for_each` value. ([#1948](https://github.com/opentofu/opentofu/pull/1948))
* Error message about a provider type mismatch now correctly identifies which module contains the problem. ([#1991](https://github.com/opentofu/opentofu/pull/1991))
* The `yamldecode` function's interpretation of scalars as numbers now conforms to the YAML 1.2 specification. In particular, the scalar value `+` is now interpreted as the string `"+"` rather than returning a parse error trying to interpret it as an integer. ([#2044](https://github.com/opentofu/opentofu/pull/2044))
* A `module` block's `version` argument now accepts prerelease version selections using a "v" prefix before the version number. Previously this was accepted only for non-prerelease selections. ([#2124])(https://github.com/opentofu/opentofu/issues/2124)
* The `tofu test` command doesn't try to validate mock provider definition by its underlying provider schema now. ([#2140](https://github.com/opentofu/opentofu/pull/2140))
* Type validation for mocks and overrides are now less strict in `tofu test`. ([#2144](https://github.com/opentofu/opentofu/pull/2144))
* Skip imports blocks logic on `tofu destroy` ([#2214](https://github.com/opentofu/opentofu/pull/2214))
* Updated github.com/golang-jwt/jwt/v4 from 4.4.2 to 4.5.1 to make security scanners happy (no vulnerability, see [#2179](https://github.com/opentofu/opentofu/pull/2179))
* `tofu test` is now setting `null`s for dynamic type when generating mock values. ([#2245](https://github.com/opentofu/opentofu/pull/2245))
* Variables declared in test files are now taking into account type default values. ([#2244](https://github.com/opentofu/opentofu/pull/2244))
* `tofu test` now removes outputs of destroyed modules between different test runs. ([#2274](https://github.com/opentofu/opentofu/pull/2274))

INTERNAL CHANGES:

* The Makefile now includes `build` and `help` targets. ([#1925](https://github.com/opentofu/opentofu/pull/1925), [#1927](https://github.com/opentofu/opentofu/pull/1927))
* The Makefile is now configured to allow only POSIX standard make syntax, without implementation-specific extensions. ([#1811](https://github.com/opentofu/opentofu/pull/1928))

## Previous Releases

For information on prior major and minor releases, see their changelogs:

- [v1.8](https://github.com/opentofu/opentofu/blob/v1.8/CHANGELOG.md)
- [v1.7](https://github.com/opentofu/opentofu/blob/v1.7/CHANGELOG.md)
- [v1.6](https://github.com/opentofu/opentofu/blob/v1.6/CHANGELOG.md)
