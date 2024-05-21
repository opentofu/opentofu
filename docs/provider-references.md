# Provider References Through the OpenTofu Language, Codebase and State

The concept of Providers has changed and evolved over the lifetime of OpenTofu, with many of the legacy configuration options still supported today.

This document is a work in progress, but aims to at least document some of the history and the current state of the codebase.  The explanations may not be 100% correct, but the progress through the scenarios shows how the complexity grows over time.

TODO don't use made up names like `provider_ref` and instead use real links into the addrs package!!  They should be renamed in the addrs package as they are quite confusing...

## What is a Provider?

In generall terms, a provider is a piece of code which interfaces OpenTofu with resources. For example, the AWS provider describes what resources it is able to read/manage, such as s3 buckets and ec2 instances.

In most cases providers live in a registry, are downloaded into the local path, and executed to provide a versioned GRPC server to OpenTofu. They could potentially be dynamically loaded directly into the running OpenTofu application, but a distinct process helps with fault tollerance and potential isolation issues.

Providers also may define functions that can be called from the OpenTofu configuration.  See [Provider Functions](#Provider-Functions) below for more information.

It is HIGHLY recommended to vet all providers you execute locally as they are not sandboxed at all.  There are discussions ongoing on how to improve safetey in that respect.

Providers also may be configured with values in a HCL block. This allows the provider have some "global" configuration that does not need to be passed in every resource/data instance.

TODO explain provider_meta

## Language References

### Provider Type vs Name vs Alias

This history is probably wrong, but gets the point across and can be fixed later.


#### Provider Type
Originally, a provider was only known by it's "type".  All providers were supplied by HashiCorp and did not need any namespaces.

Example:
```hcl
provider "aws" {
  region = "us-east-1"
}

resource "aws_s3_bucket" "foo" {
  bucket_name = "foo"
}
```
In "legacy" terms, this requires HashiCorp's `provider[type = "aws"]` provider, gives it some configuration, and then creates a "s3_bucket" with it due to the type prefix of "aws".  It also defines a version constraint to determine what version of the provider to download and start.

#### Provider Alias

You also may need to have multiple instances of the "aws" provider, perhaps with different credentials or regions.  Providers may have different instances spun up with different configuration linked by an alias.  All providers of the same type must use the same binary and must have compatible version constraints.


Example:
```hcl
provider "aws" {
  region = "us-east-1"
  alias = "default"
  version = 0.124 # Deprecated
}

resource "aws_s3_bucket" "foo" {
  bucket_name = "foo"
  provider = aws.default
}
```

As before, this requires HashiCorp's `provider[type = "aws"]` provider, gives it some configuration under the alias "default" now referencable via `provider[type = "aws", alias = "default"]`

I'm assuming Alias came before Names, this should all be fact checked.

#### Provider Names

Other organizations started to create providers over time and the concept of referencing a provider needed to be expanded.  Thus was introduced the concept of a "provider name" and "required_providers".

In the previous example, we would now say we require a provider of `provider[type = "aws", namespace = default_namespace, registry = default_registry]` with a `provider_ref[name = "aws"]`.  Notice how that by default the "provider name" is identical to the "provider type".  This can make puzzling out complex scenarios tricky.

```hcl
terraform {
  required_providers {
    my_aws = {
        # All sources here are identical, with different levels of specificity
        #source = "aws"
        #source = "hashicorp/aws"
        source = "registry.opentofu.org/hashicorp/aws"
        version = 0.124
    }
  }
}

provider "my_aws" {
  region = "us-east-1"
  alias = "default"
}

resource "aws_s3_bucket" "foo" {
  bucket_name = "foo"
  provider = my_aws.default
}
```

This now fully fledged example can be described as thus:
* The provider named "my_aws" is supplied by a provider of `provider[type = "aws", namespace = default_namespace, registry = default_registry]` and is locked to version 0.124.
* An alias of the provider named "my_aws" is created with the supplied configuration and an alias of "default", now referencable via `provider_ref[name = "my_aws", alias = "default"]`
* The "aws_s3_bucket" resources requires a provider with `type = "aws"` due to the "aws_" prefix.  It then uses the provider referred to via `provider_ref[name = "my_aws", alias = "default"]`.

This means there are technically three different provider_refs available for use here:
* The default "type-named" aws reference `provider_ref[name = "aws"]`
* The named aws reference `provider_ref[name = "my_aws"]`
* The named and aliased aws reference `provider_ref[name = "my_aws", alias = "default"]` (has configuration linked to it)

#### Module Provider References

To add additional complexity, `provider_ref`s can be passed via module calls.  This means that `provider_ref` now also contains a module prefix!


```hcl
# main.tf
terraform {
  required_providers {
    my_aws = {
        source = "aws"
    }
  }
}

provider "my_aws" {
  region = "us-east-1"
  alias = "default"
}

module "my_mod" {
    source = "./mod"
    providers = {
     mod_aws = my_aws.default
    }
}
```

```hcl
#mod/mod.tf
terraform {
  required_providers {
    mod_aws = {
        source = "aws"
    }
  }
}

resource "aws_s3_bucket" "foo" {
  bucket_name = "foo"
  provider = mod_aws
}

provider "mod_aws" {
  region = "us-east-2"
  alias = "sub"
}

resource "aws_s3_bucket" "bar" {
  bucket_name = "bar"
  provider = mod_aws.sub
}

```

If `providers` was not specifed in the module call, it would default to passing in the non-aliased `provider_ref`s in the root module `provider_ref[name = "my_aws"]` (I think, something like this happens).

In this example, there are now six provider_refs
* Root Module
  * The default "type-named" aws reference `provider_ref[name = "aws"]`
  * The named aws reference `provider_ref[name = "my_aws"]`
  * The named and aliased aws reference `provider_ref[name = "my_aws", alias = "default"]`
* Module "my_mod"
  * The default "type-named" aws reference `my_mod.provider_ref[name = "aws", mod="my_mod"]`
  * The named aws reference `provider_ref[name = "mod_aws", mod="my_mod"]`
  * The named and aliased aws reference `provider_ref[name = "mod_aws", alias = "sub", mod="my_mod"]`

However many are actually internally mapped to the same provider_ref in the config package.  This de-duping in done by looking at if/what configuration is mapped.
* `provider_ref[name = "aws"]` represents the unconfigured providers
* `provider_ref[name = "my_aws"]` has no configuration and maps to `provider_ref[name = "aws"]`.
* `provider_ref[name = "my_aws", alias = "default"]` has configuration and is kept unique
* `provider_ref[name = "aws", mod="my_mod"]` is unconfigured and maps to `provider_ref[name = "aws"]`
* `provider_ref[name = "mod_aws", mod="my_mod"]` via the module call "providers" block is mapped to `provider_ref[name = "my_aws" alias = "default"]`
* `provider_ref[name = "mod_aws", alias = "sub", mod="my_mod"]` has configuration and is kept unique
Giving us a final list of:
* `provider_ref[name = "aws"]`
* `provider_ref[name = "my_aws", alias = "default"]`
* `provider_ref[name = "mod_aws", alias = "sub", mod="my_mod"]`


This can be also seen by inspecting state files.  The TL;DR is that although there is a complex reference system it boils down to a `provider[type,namespace,registry]` and a configuration or lack thereof.



TODO discuss provider aliases in the required_providers blocks...


### Representation in State


## Provider Workflow

When `config.Module` is built from `config.Files`, each module maintains:
* ProviderConfigs: map of `provider_name.provider_alias -> config.Provider` from provider config blocks in the parsed config
* ProviderRequirements: map of `provider_name -> config.RequiredProvider` from `terraform -> required_providers`
* ProviderLocalNames: map of `addrs.Provider -> provider_name`
* ProviderMetas: Explanation TODO

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
