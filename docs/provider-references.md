# Provider References Through the OpenTofu Language, Codebase and State

The concept of Providers has changed and evolved over the lifetime of OpenTofu, with many of the legacy configuration options still supported today. This document aims to walk through examples and map them to structures within OpenTofu's code.

## Existing Documentation

It is recommended that you have the following open when reading through the rest of this document:
* https://opentofu.org/docs/language/providers/
* https://opentofu.org/docs/language/providers/requirements/
* https://opentofu.org/docs/language/providers/configuration/

## What is a Provider?

In general terms, a provider is a piece of code which interfaces OpenTofu with resources. For example, the AWS provider describes what resources it is able to read/manage, such as s3 buckets and ec2 instances.

In most cases providers live in a registry, are downloaded into the local path, and executed to provide a versioned GRPC server to OpenTofu. They could potentially be dynamically loaded directly into the running OpenTofu application, but a distinct process helps with fault tolerance and potential isolation issues.

Providers also may define functions that can be called from the OpenTofu configuration. See [Provider Functions](#Provider-Functions) below for more information.

It is HIGHLY recommended to vet all providers you execute locally as they are not sandboxed at all. There are discussions ongoing on how to improve safety in that respect.

Providers also may be configured with values in a HCL block. This allows the provider have some "global" configuration that does not need to be passed in every resource/data instance, a common example being credentials.


## Language References

### History and Addressing of Providers

Provider references and configuration have an interesting history, which leads to the system we have today. Note: some of this history has been summarized or omitted for clarity.

#### Provider Type

Prior to v0.10.0, providers were built directly into the binary and not released/versioned separately. They only had a single identifier, which we now call "Provider Type".

Example:
```hcl
provider "aws" {
  region = "us-east-1"
}

resource "aws_s3_bucket" "foo" {
  bucket_name = "foo"
}
```

This requires the `addrs.Provider{Type = "aws"}` provider, gives it some configuration, and then creates a `s3_bucket` with it due to the type prefix of `aws`. This provider also is referenceable via `addrs.LocalProviderConfig{LocalName = "aws"}`. Note: the Type and LocalName used to be the same field. These are distinct concepts that diverge in later examples.


#### Provider Alias

You also may need to have multiple configurations of the "aws" provider, perhaps with different credentials or regions. These configurations are distinguished by "Provider Alias".


Example:
```hcl
provider "aws" {
  region = "us-east-1"
  alias = "default"
}

resource "aws_s3_bucket" "foo" {
  bucket_name = "foo"
  provider = aws.default
}
```

As with the previous example, this requires the `addrs.Provider{Type = "aws"}` provider, gives it some configuration under the alias "default". The `s3_bucket` resource now refers to the provider explicitly via `addrs.LocalProviderConfig{LocalName = "aws", Alias = "default"}`. Note: the `addrs.Provider{Type = "aws"}` reference is still partially used due to some odd legacy interactions.


#### Provider Versions

Since v0.10.0, providers are distributed via a registry. This allows provider versions to be decoupled from the main application version. Provider bugfixes and new features can be released independently of the main application. All providers/configs with the same `addrs.Provider` must use the same binary and must have compatible version constraints.

```hcl
provider "aws" {
  region = "us-east-1"
  alias = "default"
  version = 0.124 # Deprecated by required_providers
}

resource "aws_s3_bucket" "foo" {
  bucket_name = "foo"
  provider = aws.default
}
```

The result is identical to the previous case, except now the version constraint is tracked in the `config.Module` structure, with `addrs.Provider{Type = "aws"}` as the key. Once all constraints are known, `tofu init` downloads the providers from the registry into a local cache for later execution.

#### Module Provider References

Prior to 0.11.0, modules would share/override provider configurations. There was no distinction between configuration of parent or child module's providers. This implicit inheritance caused a variety of issues and limitations. The `module -> providers` map field was introduced to allow explicit passing of provider configurations to child modules.

```hcl
# main.tf

provider "aws" {
  region = "us-east-1"
  alias = "default"
  version = 0.124 # Deprecated by required_providers
}

module "my_mod" {
  source = "./mod"
  # Only the "unaliased" providers are passed if this is omitted.
  providers = {
    aws = aws.default
  }
}
```

```hcl
# ./mod/mod.tf
provider "aws" { # Deprecated by required_providers
  version = ">= 0.1" # Deprecated by required_providers
}

resource "aws_s3_bucket" "foo" {
  bucket_name = "foo"
  provider = aws
}
```

In the root module (main configuration), we require the `addrs.Provider{Type = "aws"}` with a version constraint of "0.124". A configuration for that provider exists at `addrs.LocalProviderConfig{LocalName = "aws", Alias = "default"}` within the root module and is not automatically accessible from the child module. A new reference is introduced, which can be used globally: `addrs.AbsProviderConfig{Module: Root, Provider: addrs.Provider{Type = "aws"}, Alias = "default"}`.

The child module is passed the `addrs.AbsProviderConfig` and is internally referenceable within the module under `addrs.LocalProviderConfig{LocalName: "aws"}`. That global configuration is copied and merged with the configuration within that module, which in this case adds an additional version constraint.

Within that module, `addrs.LocalProviderConfig{LocalName: "aws"}` now refers to `addrs.Provider{Type = "aws"}` and the merged configuration for that provider.

If multiple instances of the same provider are needed, the alias can be provided in the module's "providers" block

```hcl
# main.tf

provider "aws" {
  region = "us-east-1"
  alias = "default"
  version = 0.124 # Deprecated by required_providers
}

module "my_mod" {
  source = "./mod"
  # Only the "unaliased" providers are passed if this is omitted.
  providers = {
    aws.foo = aws.default
  }
}
```

```hcl
# ./mod/mod.tf
provider "aws" { # Deprecated by required_providers
  version = ">= 0.1" # Deprecated by required_providers
  alias = "foo"
}

resource "aws_s3_bucket" "foo" {
  bucket_name = "foo"
  provider = aws.foo
}
```

The root module's explanation is nearly identical, the primary change is to the addressing in the child module.

The child module is passed the `addrs.AbsProviderConfig` and is internally referenceable within the module under `addrs.LocalProviderConfig{LocalName: "aws", Alias = "foo"}`. That global configuration is copied and merged with the configuration within that module, which in this case adds an additional version constraint.

Within that module, `addrs.LocalProviderConfig{LocalName: "aws", Alias = "foo"}` now refers to `addrs.Provider{Type = "aws"}` and the merged configuration for that provider.

#### Required Providers (Legacy)

With the change in 0.11.0 adding the `providers` field, it is still unclear when a child module's provider is "incorrectly configured" or if the parent module has forgotten an entry in the `providers` field.

To solve this, `terraform -> required_providers` was introduced. The initial version of this feature was a direct mapping between "Provider Type" and "Provider Version Constraint".

```hcl
terraform {
  required_providers {
    aws = "0.124"
  }
}

provider "aws" {
  region = "us-east-1"
  alias = "default"
}

module "my_mod" {
  source = "./mod"
  providers = {
    aws = aws.default
  }
}
```

```hcl
# ./mod/mod.tf
terraform {
  required_providers {
    aws = ">= 0.1"
  }
}

resource "aws_s3_bucket" "foo" {
  bucket_name = "foo"
  provider = aws
}
```

The references are unchanged, except for the dependencies are now more explicit. This form of `required_providers` is no longer supported.


#### Provider Names / Namespaces / Registries

Other organizations started to create providers over time (along with their own registries) and the concept of referencing a provider needed to be expanded. In v0.13.0 the concept of `addrs.Provider` was expanded to include `Namespace` and `Hostname`.

Previously, all providers within the registry had global names "aws", "datadog", "gcp", etc... As forks were introduced and the authoring of providers took off, the `Namespace` concept was introduced. It usually maps to the GitHub user/org that owns it, but it is not a strict requirement (especially in third-party registries).

Organizations also wanted more control over their providers for both development and security purposes. The providers registry hostname was included in the spec.

Additionally, the previous understanding of "datadog" may refer to "datadog/datadog" or "user/datadog" and is unclear if they are both included in the project. By decoupling `addrs.Provider.Type` and `addrs.LocalProviderConfig.LocalName`, both could be used in the same module under different names. Additionally the same concept can be used to have the LocalName "datadog" refer to "user/datadog-fork" without having to rewrite the whole project's config.


```hcl
terraform {
  required_providers {
    awsname = { # name added for clarity, usually Type == LocalName
      #source = "aws"
      #source = "hashicorp/aws"
      source = "registry.opentofu.org/hashicorp/aws"
      version = "0.124"
    }
  }
}

provider "awsname" {
  region = "us-east-1"
  alias = "default"
}

module "my_mod" {
  source = "./mod"
  providers = {
    modaws = awsname.default
  }
}
```

```hcl
# ./mod/mod.tf
terraform {
  required_providers {
    modaws = {
      source = "aws"
      version = ">= 0.1"
    }
  }
}


resource "aws_s3_bucket" "foo" {
  bucket_name = "foo"
  provider = modaws
}
```

The required_providers "source" field in the root module decomposed into `addrs.Provider{Type="aws", Namespace="hashicorp", Hostname="registry.opentofu.org"}`. As the default namespace is "hashicorp" and the default hostname is "registry.opentofu.org", we will continue to use the shorthand `addrs.Provider{Type="aws"}`. Next `addrs.LocalProviderConfig{LocalName: "aws_name"}` is created and within the root module maps to `addrs.Provider{Type="aws"}`. This provider local name is then used in all subsequent references within the root module. The configuration is then mapped to `addrs.AbsProviderConfig{Module: Root, Provider: addrs.Provider{Type = "aws"}, Alias = "default"}` globally.

The child module is passed the `addrs.AbsProviderConfig` and is internally referenceable within the module under `addrs.LocalProviderConfig{LocalName: "modaws"}`.

Within that module, `addrs.LocalProviderConfig{LocalName: "modaws"}` now points at `addrs.Provider{Type = "aws"}` and is effectively replaced with `addrs.AbsProviderConfig{Module: Root, Provider: addrs.Provider{Type = "aws"}, Alias = "default"}` at runtime. This optimizes running as few provider instances as possible.

If a new provider configuration were added to the module:
```hcl
provider "modaws" {
  region = "us-west-2"
}
```
This would negate the override / deduplication above and result in `addrs.AbsProviderConfig{Module: MyMod, Provider: addrs.Provider{Type = "aws"}}`.

#### Multiple Provider Aliases

Multiple provider aliases can be supplied in required_providers via `configuration_aliases`. This requires that a caller of the module provide the requested aliases explicitly.

Example:

```hcl
terraform {
  required_providers {
    awsname = { # name added for clarity, usually Type == LocalName
      source = "registry.opentofu.org/hashicorp/aws"
      version = "0.124"
    }
  }
}

provider "awsname" {
  region = "us-east-1"
  alias = "default"
}

module "my_mod" {
  source = "./mod"
  providers = {
    modaws.foo = awsname.default
    modaws.bar = awsname
  }
}
```

```hcl
# ./mod/mod.tf
terraform {
  required_providers {
    modaws = {
      source = "aws"
      version = ">= 0.1"
      configuration_aliases = [ modaws.foo, modaws.bar ]
    }
  }
}
```

### Multiple Provider Instances

In OpenTofu 1.9.0, we introduced the concept of a "provider instance".  This allows an aliased provider to have multiple instances based on the same configuration differentiated by a for_each expression.

```hcl
# main.tofu

provider "cloudflare" {
  alias = "by_account"
  for_each = local.cloudflare_accounts
  account_id = each.key
  auth_key = each.value
}

locals {
  enabled_cloudflare_accounts = {
    for acct, cfg in local.cloudflare_accounts : acct => cfg
    if cfg.enabled
  }
}

module "my_mod" {
  source = "./foo"
  for_each = local.enabled_cloudflare_accounts
  account_id = each.key

  providers {
    cloudflare = cloudflare.by_account[each.key]
  }
}

resource "cloudflare_r2_bucket" "bucket" {
  for_each = local.cloudflare_accounts 
  account_id = each.key
  provider = cloudflare.by_account[each.key]
}
```

### Representation in State

Resources in the state file note the `addrs.AbsProviderConfig` required to modify them.

Prior to 1.9.0, all instances (for_each/count) of a resource must use the same provider and would have the provider stored in a single field at the resource level, not resource instance level.

After 1.9.0 (provider iteration), the "provider" field can now exist on either the resource or the resource instance and can include a "provider instance key" appended to the `addrs.AbsProviderConfig`.
 * If provider config instances differ per resource instance, the "provider" field will exist only on the resource instances.
   - Although the provider instance key may vary between resource instances of the same resource, the `addrs.AbsProviderConfig` component must be identical.
 * If the provider config instances are identical per resource instance, the provider field will only exist on the resource.

Note: This section should be expanded with examples.

Note: `tofu show -json` and the internal statefile format are different and do not always line up one-to-one.

## Provider Workflow

When `config.Module` is built from `config.Files`, each module maintains:
* ProviderConfigs: map of `provider_name.provider_alias -> config.Provider` from provider config blocks in the parsed config
* ProviderRequirements: map of `provider_local_name -> config.RequiredProvider` from `terraform -> required_providers`
* ProviderLocalNames: map of `addrs.Provider -> provider_name`

The full list of required provider types is collated, downloaded, hashed and cached in the .terraform directory during init.

Providers are then added to the graph in a few transformers:
* ProviderConfigTransformer: Adds configured providers to the graph
* MissingProviderTransformer: Adds unconfigured but required providers to the graph
* ProviderTransformer: Links provider nodes to self reported nodes that require them
* ProviderFunctionTransformer: Links provider nodes to other nodes by inspecting their "OpenTofu Function References"
* ProviderPruneTransformer: Removes provider nodes that are not in use by other nodes

Providers are then managed and scoped by the EvalContextBuiltin where the actual `provider.Interface`s are created and attached to resources.


## Provider Functions

Providers also may supply functions, either unconfigured or configured.
* `providers::aws::arn_parse(var.arn)`
* `providers::aws::us::arn_parse(var.arn)`
