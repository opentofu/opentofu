## 1.11.0 (Unreleased)

This release has some changes that might require special attention when upgrading from an earlier release. Refer to "UPGRADE NOTES" below for more information.

NEW FEATURES:

* **Ephemeral values** allow OpenTofu to work with data and resources that exist only in memory during a single OpenTofu phase, guaranteeing that those values will not be persisted in state snapshots or plan files.

    You can now declare input variables and output values as being ephemeral, and you can use provider plugins that have been updated to include ephemeral resource types (e.g. for fetching a secret) or managed resource types with write-only attributes (e.g. for setting a password without saving it in OpenTofu state).

    For more information, refer to [Ephemerality](https://opentofu.org/docs/language/ephemerality/).

* The new **`enabled` meta-argument** offers an alternative to the existing `count` and `for_each` meta-arguments for situations where a particular resource instance or module instance has either zero or one instances.

    The initial form of this argument is nested inside a `lifecycle` block, rather than directly inside a resource or module declaration, to avoid conflicting with existing input variables or resource type arguments named `enabled`.

    For more information, refer to [the `enabled` meta-argument](https://opentofu.org/docs/main/language/meta-arguments/enabled/).

UPGRADE NOTES:

* OpenTofu on macOS now requires macOS 12 Monterey or later.
* The `azurerm` state storage backend no longer supports certain arguments:
    * `endpoint` and the `ARM_ENDPOINT` environment variable are now deprecated and ignored.
    * `msi_endpoint` and the `ARM_MSI_ENDPOINT` environment variable are now deprecated and ignored. Use the `MSI_ENDPOINT` environment variable instead.
    * `environment` and `metadata_host` are now mutually-exclusive.

    If you wish to adjust your existing backend configuration in an existing OpenTofu working directory, you can use `tofu init -reconfigure` to tell OpenTofu that it should ignore any previously-initialized backend settings and reinitialize from the current configuration. **Do not** use `-migrate-state` because these changes will not cause the state to be stored in a different location and so state migration is not required.
* The `issensitive` function previously incorrectly returned known results when given unknown values, which has now been corrected to avoid confusing consistency check failures during the apply phase, as reported in [issue #2415](https://github.com/opentofu/opentofu/issues/2415).

    If your module was previously assigning something derived from an `issensitive` result to a context where unknown values are not allowed during the planning phase, such as `count`/`for_each` arguments for resources or modules, this will now fail during the planning phase and so you will need to choose a new approach where either the `issensitive` argument is always known during the planning phase or where the sensitivity of an unknown value is not used as part of the decision.
* Testing mocks previously only followed a subset of the rules defined in provider schemas. The generated mock values now follow the provider schema more closely to ensure that the results are valid.

    If your test scenarios previously included invalid mocks or overrides that previous OpenTofu versions did not detect, you will need to fix those invalid configurations to ensure that your tests can continue to pass after upgrading.
* When installing module packages from Amazon S3 buckets using [S3 source addresses](https://opentofu.org/docs/language/modules/sources/#s3-bucket), OpenTofu now uses the same methods for finding AWS credentials as the AWS CLI and SDKs instead of using its own custom credentials search sequence.

    This might mean that OpenTofu v1.11.0 will choose AWS credentials from a different location than previous versions did, if your AWS authentication configuration describes credential sources that were not previously supported. Generally, OpenTofu should choose credentials in the same way that the AWS CLI would by default when accessing the same S3 object.
* OpenTofu no longer accepts SHA-1 signatures in TLS handshakes, as recommended in [RFC 9155](https://www.rfc-editor.org/rfc/rfc9155.html).
* OpenTofu's remote provisioners, when using SSH to connect to a remote server using certificate-based authentication, no longer accept a certificate key as the signature key for a certificate, as required by [draft-miller-ssh-cert-03 section 2.1.1](https://datatracker.ietf.org/doc/html/draft-miller-ssh-cert-03#section-2.1.1).

    This may cause new failures if you are currently using an incorrectly-generated certificate, but does not affect correctly-generated certificates.

ENHANCEMENTS:

* Ephemeral values, ephemeral resources, and write-only attributes are now supported. ([#2834](https://github.com/opentofu/opentofu/issues/2834))
* Resources and modules now support an `enabled` meta-argument, in addition to `count` and `for_each`. ([#3247](https://github.com/opentofu/opentofu/issues/3247))
* When defining the value of an input variable using the object constructor syntax `{ ... }`, OpenTofu now produces a warning if the object constructor includes an attribute name that isn't part of the target object type. ([#3292](https://github.com/opentofu/opentofu/pull/3292))
* OpenTofu will now suggest using `-exclude` if a provider reports that it cannot create a plan for a particular resource instance due to values that won't be known until the apply phase. ([#2643](https://github.com/opentofu/opentofu/pull/2643))
* `tofu validate` can now validate non-root modules that require additional provider configurations using `configuration_aliases`. ([#2905](https://github.com/opentofu/opentofu/pull/2905))
* The `regex` and `regexall` functions now support using `\p` and `\P` sequences with the long-form names for Unicode general character properties. For example, `\p{Letter}` now has the same meaning as `\p{L}`. ([#3166](https://github.com/opentofu/opentofu/pull/3166))
* The `fileset` function can now match filenames that include metacharacters when those metacharacters are escaped with backslashes in the glob pattern. ([#3332](https://github.com/opentofu/opentofu/issues/3332))
* The `mock_provider` block in test scenario configurations now supports the `for_each` meta-argument. ([#3087](https://github.com/opentofu/opentofu/pull/3087))
* OpenTofu now uses less RAM and CPU when working with state for configurations that declare thousands of resource instances. ([#3110](https://github.com/opentofu/opentofu/pull/3110))
* `variable` blocks in test scenario files can now include expressions that call functions. ([#2947](https://github.com/opentofu/opentofu/pull/2947))
* `tofu show` now supports `-config` and `-module=DIR` options, to be used in conjunction with `-json` to produce a machine-readable summary of either the whole configuration or a single module without first creating a plan. ([#2820](https://github.com/opentofu/opentofu/pull/2820), [#3003](https://github.com/opentofu/opentofu/pull/3003))
* [The JSON representation of configuration](https://opentofu.org/docs/internals/json-format/#configuration-representation) returned by `tofu show` in `-json` mode now includes type constraint information for input variables and whether each input variable is required, in addition to the existing properties related to input variables. ([#3013](https://github.com/opentofu/opentofu/pull/3013))
* Multiline string updates in lists are now diffed line-by-line, rather than as a single change per element, making it easier to understand changes in the plan output. ([#3030](https://github.com/opentofu/opentofu/pull/3030))
* Plan UI now explicitly states that the "update in-place" notation is "current -> planned", as part of the existing description of the meaning of each change type symbol. ([#3159](https://github.com/opentofu/opentofu/pull/3159))
* It's now possible to provide input variable values during the apply phase as long as any non-ephemeral variables have the same values as during the planning phase, for the purpose of using input variables to configure state and plan encryption settings. ([#1998](https://github.com/opentofu/opentofu/pull/1998))
* The `s3` state storage backend now allows specifying tags to associate with the S3 objects representing state snapshots and locks. ([#3038](https://github.com/opentofu/opentofu/pull/3038))
* When installing module packages from Amazon S3 source addresses, OpenTofu now follows similar rules for finding AWS credentials as the AWS CLI does, and similar to the S3 backend. In particular this means OpenTofu supports some newer authentication schemes, such as [IAM roles for service accounts](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html). ([#3269](https://github.com/opentofu/opentofu/pull/3269))
* The new `azure_vault` key provider allows using Azure Key Vault as a source for state and plan encryption keys. ([#3046](https://github.com/opentofu/opentofu/pull/3046))
* The `azurerm` state storage backend now supports the following additional configuration options:
  * `use_cli`: set to true by default, this can be set to false to disable command line authentication. ([#3034](https://github.com/opentofu/opentofu/pull/3034))
  * `use_aks_workload_identity`: set to false by default, this allows authentication in Azure Kubernetes when using Workload Identity Federation. ([#3251](https://github.com/opentofu/opentofu/pull/3251))
  * `client_id_file_path`: allows the user to set the `client_id` through a file. ([#3251](https://github.com/opentofu/opentofu/pull/3251))
  * `client_secret_file_path`: allows the user to set the `client_secret` through a file. ([#3251](https://github.com/opentofu/opentofu/pull/3251))
  * `client_certificate`: allows the user to set the certificate directly, as opposed to only setting it through a file. ([#3251](https://github.com/opentofu/opentofu/pull/3251))
* `tofu init` now returns a clearer error message when specifying a [module package sub-directory](https://opentofu.org/docs/language/modules/sources/#modules-in-package-sub-directories) that doesn't exist in the selected module package. ([#3144](https://github.com/opentofu/opentofu/pull/3144))
* `tofu init` now copies module package contents concurrently for shorter runtime when there are many calls to the same module package. ([#3214](https://github.com/opentofu/opentofu/pull/3214))
* It is now possible to configure the registry protocol retry count and request timeout settings in the CLI configuration, in addition to the previously-available environment variables. ([#3256](https://github.com/opentofu/opentofu/pull/3256), [#3368](https://github.com/opentofu/opentofu/pull/3368))
* OpenTelemetry traces describing HTTP requests now follow [the new OpenTelemetry Semantic Conventions for HTTP 1.27.0](https://opentelemetry.io/docs/specs/semconv/non-normative/http-migration/). ([#3372](https://github.com/opentofu/opentofu/pull/3372))
* When running the `stty` program to disable or reenable local echo at a sensitive input prompt, OpenTofu now searches `PATH` for the program rather than requiring it to be at exactly `/bin/stty`. ([#3182](https://github.com/opentofu/opentofu/pull/3182))

BUG FIXES:

* The `s3` state storage backend now correctly sends the `x-amz-server-side-encryption` header when working with S3 objects representing state locks. ([#2970](https://github.com/opentofu/opentofu/issues/2970))
* The `aws_kms` key provider for state and plan encryption no longer returns a confusing error when the `TF_APPEND_USER_AGENT` environment variable is set. ([#3390](https://github.com/opentofu/opentofu/pull/3390))
* The `issensitive` function now returns an unknown result when its argument is unknown, because a sensitive unknown value can potentially become non-sensitive once more information is available. ([#3008](https://github.com/opentofu/opentofu/pull/3008))
* Provider references like `null.some_alias[each.key]` in `.tf.json` files are now accepted in the same way as in native syntax files. ([#2915](https://github.com/opentofu/opentofu/issues/2915))
* Fixed "slice bounds out of range" crash when processing multiple deprecated values inside a complex object. ([#3105](https://github.com/opentofu/opentofu/pull/3105))
* OpenTofu will no longer produce spurious "update" diffs after applying a change that included a sensitive value decided only during the apply phase. ([#3388](https://github.com/opentofu/opentofu/pull/3388))
* The `import` block now correctly validates the `id` property. ([#2416](https://github.com/opentofu/opentofu/issues/2416))
* `tofu import` now correctly checks when its second argument refers to an undeclared instance of the target resource. ([#3106](https://github.com/opentofu/opentofu/pull/3106))
* The `tofu.rc` CLI configuration file now properly takes precedence over `terraform.rc` on Windows. ([#2891](https://github.com/opentofu/opentofu/pull/2891))
* Input variable validation rules no longer cause misbehavior when planning in destroy mode, such as with `tofu destroy`. ([#3131](https://github.com/opentofu/opentofu/pull/3131))
* Mock values generated for `tofu test` now follow the provider schema more closely. ([#3069](https://github.com/opentofu/opentofu/pull/3069))
* `tofu test` no longer crashes when working with a module that declares one or more deprecated output values. ([#3249](https://github.com/opentofu/opentofu/pull/3249))
* The `remote-exec` and `file` provisioners now reject SSH certificates whose signature key is a certificate key, as required by the current SSH Certificate Format specification draft. ([#3180](https://github.com/opentofu/opentofu/pull/3180))
* The `TF_CLI_ARGS` environment variable and all of its subcommand-specific variants now follow typical shell parsing rules more closely when parsing the environment variable values into a sequence of arguments. In particular, pairs of quotes with nothing between them are now understood as zero-length arguments rather than being completely ignored as before. ([#3354](https://github.com/opentofu/opentofu/pull/3354))

## Previous Releases

For information on prior major and minor releases, refer to their changelogs:

- [v1.10](https://github.com/opentofu/opentofu/blob/v1.10/CHANGELOG.md)
- [v1.9](https://github.com/opentofu/opentofu/blob/v1.9/CHANGELOG.md)
- [v1.8](https://github.com/opentofu/opentofu/blob/v1.8/CHANGELOG.md)
- [v1.7](https://github.com/opentofu/opentofu/blob/v1.7/CHANGELOG.md)
- [v1.6](https://github.com/opentofu/opentofu/blob/v1.6/CHANGELOG.md)
