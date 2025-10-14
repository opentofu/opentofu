## 1.11.0 (Unreleased)

UPGRADE NOTES:

* OpenTofu on macOS now requires macOS 12 Monterey or later.
* The `issensitive` function previously incorrectly returned known results when given unknown values, which has now been corrected to avoid confusing consistency check failures during the apply phase, as reported in [issue #2415](https://github.com/opentofu/opentofu/issues/2415).

    If your module was previously assigning something derived from an `issensitive` result to a context where unknown values are not allowed during the planning phase, such as `count`/`for_each` arguments for resources or modules, this will now fail during the planning phase and so you will need to choose a new approach where either the `issensitive` argument is always known during the planning phase or where the sensitivity of an unknown value is not used as part of the decision.
* Testing mocks previously only followed a subset of the rules defined in provider schemas. The provider schema now drives the mocking to ensure the schema is correctly followed. ([#3069](https://github.com/opentofu/opentofu/pull/3069))

    In rare cases this change might result in some previously-passing tests now failing, due to invalid mocks or overrides that were not detected in earlier versions.
* When installing module packages from Amazon S3 buckets using [S3 source addresses](https://opentofu.org/docs/language/modules/sources/#s3-bucket), OpenTofu now uses the same methods for finding AWS credentials as the AWS CLI and SDKs instead of using its own custom credentials search sequence.

    This might mean that OpenTofu v1.11.0 will choose AWS credentials from a different location than previous versions did, if your AWS authentication configuration describes credential sources that were not previously supported. Generally, OpenTofu should choose credentials in the same way that the AWS CLI would by default when accessing the same S3 object.
* OpenTofu no longer accepts SHA-1 signatures in TLS handshakes, as recommended in [RFC 9155](https://www.rfc-editor.org/rfc/rfc9155.html).
* OpenTofu's remote provisioners, when using SSH to connect to a remote server using certificate-based authentication, no longer accept a certificate key as the signature key for a certificate, as required by [draft-miller-ssh-cert-03 section 2.1.1](https://datatracker.ietf.org/doc/html/draft-miller-ssh-cert-03#section-2.1.1).

    This may cause new failures if you are currently using an incorrectly-generated certificate, but does not affect correctly-generated certificates.
* The `azurerm` backend has been heavily rewritten. Deprecated Azure libraries for the `azurerm` backend have been swapped out for modern, offically supported ones. ([#3034](https://github.com/opentofu/opentofu/pull/3034))

ENHANCEMENTS:

* The conditional `enabled` field is now supported for modules within the `lifecycle` block. ([#3244](https://github.com/opentofu/opentofu/pull/3244))
* The conditional `enabled` field is now supported for all types of resources within the `lifecycle` block. ([#3042](https://github.com/opentofu/opentofu/pull/3042))
* OpenTofu will now suggest using `-exclude` if a provider reports that it cannot create a plan for a particular resource instance due to values that won't be known until the apply phase. ([#2643](https://github.com/opentofu/opentofu/pull/2643))
* `tofu validate` now supports running in a module that contains provider configuration_aliases. ([#2905](https://github.com/opentofu/opentofu/pull/2905))
* The `regex` and `regexall` functions now support using `\p` and `\P` sequences with the long-form names for Unicode general character properties. For example, `\p{Letter}` now has the same meaning as `\p{L}`. ([#3166](https://github.com/opentofu/opentofu/pull/3166))
* `tofu show` now supports `-config` and `-module=DIR` options, to be used in conjunction with `-json` to produce a machine-readable summary of either the whole configuration or a single module without first creating a plan. ([#2820](https://github.com/opentofu/opentofu/pull/2820), [#3003](https://github.com/opentofu/opentofu/pull/3003))
* [The JSON representation of configuration](https://opentofu.org/docs/internals/json-format/#configuration-representation) returned by `tofu show` in `-json` mode now includes type constraint information for input variables and whether each input variable is required, in addition to the existing properties related to input variables. ([#3013](https://github.com/opentofu/opentofu/pull/3013))
* Multiline string updates in arrays are now diffed line-by-line, rather than as a single element, making it easier to see changes in the plan output. ([#3030](https://github.com/opentofu/opentofu/pull/3030))
* When defining the value of an input variable using the object constructor syntax `{ ... }`, OpenTofu now produces a warning if the object constructor includes an attribute name that isn't part of the target object type. ([#3292](https://github.com/opentofu/opentofu/pull/3292))
* Add full support for -var, -var-file, and TF_VARS during `tofu apply` to support plan encryption ([#1998](https://github.com/opentofu/opentofu/pull/1998))
* The S3 state backend now supports arguments to specify tags of the state and lock files. [#3038](https://github.com/opentofu/opentofu/pull/3038)
* Plan UI now explicitly states that the "update in-place" notation is "current -> planned", as part of the existing description of the meaning of each change type symbol. ([#3159](https://github.com/opentofu/opentofu/pull/3159))
* When installing module packages from Amazon S3 source addresses, OpenTofu now follows similar rules for finding AWS credentials as the AWS CLI does, and similar to the S3 backend. In particular this means OpenTofu supports some newer authentication schemes, such as [IAM roles for service accounts](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html). ([#3269](https://github.com/opentofu/opentofu/pull/3269))
* Upgrade go from 1.24.4 to 1.24.6 to fix [GO-2025-3849](https://pkg.go.dev/vuln/GO-2025-3849) ([3127](https://github.com/opentofu/opentofu/pull/3127))
* Improved error messages when a submodule is not found in a module ([#3144]https://github.com/opentofu/opentofu/pull/3144)
* Add support for the `for_each` attribute in the `mock_provider` block. ([#3087](https://github.com/opentofu/opentofu/pull/3087))
* Upgrade github.com/openbao/openbao/api/v2 from 2.1.0 to 2.3.0 to fix [GO-2025-3783](https://pkg.go.dev/vuln/GO-2025-3783) ([#3134](https://github.com/opentofu/opentofu/pull/3134))
  * The upgrade is necessary to silence the security scanner and does not affect the actual state encryption provided by OpenBao.
* Add logs for the DynamoDB operations in the S3 backend ([#3103](https://github.com/opentofu/opentofu/pull/3103))
* When running the `stty` program to disable or reenable local echo at a sensitive input prompt, OpenTofu will now search `PATH` for the program rather than requiring it to be at exactly `/bin/stty`. ([#3182](https://github.com/opentofu/opentofu/pull/3182))
* Reduced the CPU and Memory overhead of managing large state files in OpenTofu. ([#3110](https://github.com/opentofu/opentofu/pull/3110))
  * These improvements are primarilly visible in projects with thousands of resources
* It is now possible to configure the registry protocol retry count and request timeout settings in the CLI configuration, in addition to the previously-available environment variables. ([#3256](https://github.com/opentofu/opentofu/pull/3256))
* Upgrade github.com/hashicorp/go-getter to v1.7.9 to fix [GO-2025-3892](https://pkg.go.dev/vuln/GO-2025-3892). ([#3227](https://github.com/opentofu/opentofu/pull/3227))
* The module installer will copy files in parallel to improve performance of `init` ([#3214](https://github.com/opentofu/opentofu/pull/3214))
* Encryption can now take advantage of Azure Key Vaults with the new Azure Key Provider. ([#3046](https://github.com/opentofu/opentofu/pull/3046))
* The following configuration options have been added to the `azurerm` backend:
  * `use_cli`: set to true by default, this can be set to false to disable command line authentication. ([#3034](https://github.com/opentofu/opentofu/pull/3034))
  * `use_aks_workload_identity`: set to false by default, this allows authentication in Azure Kubernetes when using Workload Identity Federation. ([#3251](https://github.com/opentofu/opentofu/pull/3251))
  * `client_id_file_path`: allows the user to set the `client_id` through a file. ([#3251](https://github.com/opentofu/opentofu/pull/3251))
  * `client_secret_file_path`: allows the user to set the `client_secret` through a file. ([#3251](https://github.com/opentofu/opentofu/pull/3251))
  * `client_certificate`: allows the user to set the certificate directly, as opposed to only setting it through a file. ([#3251](https://github.com/opentofu/opentofu/pull/3251))
* OpenTelemetry traces describing HTTP requests now follow [the new OpenTelemetry Semantic Conventions for HTTP 1.27.0](https://opentelemetry.io/docs/specs/semconv/non-normative/http-migration/). ([#3372](https://github.com/opentofu/opentofu/pull/3372))
* Upgrade github.com/go-viper/mapstructure/v2 to v2.4.0 to fix [GO-2025-3900](https://pkg.go.dev/vuln/GO-2025-3900). ([#3229](https://github.com/opentofu/opentofu/pull/3229))

BUG FIXES:

* The `tofu.rc` configuration file now properly takes precedence over `terraform.rc` on Windows ([#2891](https://github.com/opentofu/opentofu/pull/2891))
* S3 backend now correctly sends the `x-amz-server-side-encryption` header for the lockfile ([#2870](https://github.com/opentofu/opentofu/issues/2970))
* The `import` block now correctly validates the `id` property. ([#2416](https://github.com/opentofu/opentofu/issues/2416)
* Allow function calls in test variable blocks ([#2947](https://github.com/opentofu/opentofu/pull/2947))
* The `issensitive` function now returns an unknown result when its argument is unknown, since a sensitive unknown value can potentially become non-sensitive once more information is available. ([#3008](https://github.com/opentofu/opentofu/pull/3008))
* The `fileset` function can now match filenames that include metacharacters when those metacharacters are escaped with backslashes in the glob pattern. ([#3332](https://github.com/opentofu/opentofu/issues/3332))
* Provider references like "null.some_alias[each.key]" in .tf.json files are now correctly parsed ([#2915](https://github.com/opentofu/opentofu/issues/2915))
* Fixed crash when processing multiple deprecated marks on a complex object ([#3105](https://github.com/opentofu/opentofu/pull/3105))
* Variables with validation no longer interfere with the destroy process ([#3131](https://github.com/opentofu/opentofu/pull/3131))
* Ensure that generated mock values for testing correctly follows the provider schema. ([#3069](https://github.com/opentofu/opentofu/pull/3069))
* Remote provisioners now reject SSH certificates whose signature key is a certificate key, as required by the current SSH Certificate Format specification draft. ([#3180](https://github.com/opentofu/opentofu/pull/3180))
* `tofu import` command now correctly validates when the target address contains non-existent for_each key ([#3106](https://github.com/opentofu/opentofu/pull/3106))
* Fix crash in tofu test when using deprecated outputs ([#3249](https://github.com/opentofu/opentofu/pull/3249))
* The `TF_CLI_ARGS` environment variable and all of its subcommand-specific variants now follow typical shell parsing rules more closely when parsing the environment variable values into a sequence of arguments. In particular, pairs of quotes with nothing between them are now understood as zero-length arguments rather than being completely ignored as before. ([#3354](https://github.com/opentofu/opentofu/pull/3354))

BREAKING CHANGES:
* In the `azurerm` backend, the following backend variables have been changed ([#3034](https://github.com/opentofu/opentofu/pull/3034)):
  * `endpoint` and the `ARM_ENDPOINT` environment variable are deprecated: these are now unused and have no affect on execution
  * `msi_endpoint` and the `ARM_MSI_ENDPOINT` environment variable deprecated: please use the `MSI_ENDPOINT` environment variable instead
  * You cannot set both an `environment` and `metadata_host`.

## Previous Releases

For information on prior major and minor releases, refer to their changelogs:

- [v1.10](https://github.com/opentofu/opentofu/blob/v1.10/CHANGELOG.md)
- [v1.9](https://github.com/opentofu/opentofu/blob/v1.9/CHANGELOG.md)
- [v1.8](https://github.com/opentofu/opentofu/blob/v1.8/CHANGELOG.md)
- [v1.7](https://github.com/opentofu/opentofu/blob/v1.7/CHANGELOG.md)
- [v1.6](https://github.com/opentofu/opentofu/blob/v1.6/CHANGELOG.md)
