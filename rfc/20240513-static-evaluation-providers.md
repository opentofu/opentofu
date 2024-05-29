# Static Evaluation of Provider Iteration

Issue: https://github.com/opentofu/opentofu/issues/300

Since the introduction of for_each/count, users have been trying to use each/count in provider configurations and resource/module mappings. Providers are a special case throughout OpenTofu and interacting with them either as a user or developer requires significant care.

> [!Note]
> Please read [Provider References](../docs/provider-references.md) before diving into this section!

## Proposed Solution

The approach proposed in the [Static Evaluation RFC](20240513-static-evaluation.md) can be extended to support provider for_each/count with some clever code and lots of testing. It is assumed that the reader has gone through the Static Evaluation thoroughly before continuing here.

### User Documentation

#### Provider Configuration Expansion

The first change is to support static variables in the provider config block:
```hcl
locals {
  regions = {"us": "us-east-1", "eu": "eu-west-1"}
}

provider "aws" {
  for_each = local.regions
  alias = each.key # Defines aliases aws.eu and aws.us
  region = each.value
}

# Uses the AWS US Provider
resource "aws_s3_bucket" "primary" {
  for_each = local.regions
  provider = aws.us # Note the provider alias above.
}

# Uses the AWS EU Provider
module "mod" {
  source = "./mod"
  providers {
    aws = aws.eu # Note the provider alias above.
  }
}
```


#### Provider Alias Mappings
The next step is to support provider aliases indexed by an expression, which is quite a bit trickier.
```hcl
locals {
  regions = {"us": "us-east-1", "eu": "eu-west-1"}
}

provider "aws" {
  for_each = local.regions
  alias = each.key # Could theoretically default to each.key if not specified.  It must use each.key/value in some fashion.
  region = each.value
}

resource "aws_s3_bucket" "primary" {
  for_each = local.regions
  provider = provider.aws[each.key]
}

module "mod" {
  for_each = local.regions
  source = "./mod"
  providers {
    aws = provider.aws[each.key]
  }
}
```

As you can see, the `provider.name[alias]` form is introduced in that example.  This allows providers named "local" or other conflicting names, and clearly shows that it's referencing a particular instance of a given type.

### Technical Approach

#### Provider Configuration Expansion

Expanding provider configurations can be done using the StaticContext availalble in `configs.NewModule()` as defined in the StaticEvaluation RFC. At the end of the NewModule constructor, the configured provider's aliases can be expanded using the each/count, similar to how [module.ProviderLocalNames](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/configs/module.go#L186) is generated. This does not require any special workarounds.

#### Provider Alias Mappings

To fully implement static provider alias mappings with for_each/count, the provider reference system throughout the configs package and the tofu package must be significantly changed:
* Introduce an alternate provider address method and updating documentation
* Provider mappings for modules and resources are hard-coded at config load time on an *unexpanded* view of the config/graph structures, see `configs.NewModule` and `configs.Module`'s provider fields.
* Provider configurations are [pruned during graph processing](../docs/provider-references.md#Provider-Workflow)

Approach explored in this RFC:
* Make the provider reference system a bit looser until the point in which it's actually needed
  - The main challenge is the convoluted reference system and graph transforms built around it.
  - An *unexpanded* module or resource could depend on a single provider type, but refer to multiple aliases
    - The for_each/count values would be known and could be used to determine the aliases required
  - Expanded modules/resources could then refer to a specific alias of the provider required by the unexpanded parent.


For now this, is mostly a brain dump of the initial exploration.

We assume that `provider.aws["foo"]` is equivalent to `aws.foo` in the provider config reference as it's easiest if they both use the same alias.  Therefore, the provider for_each expansion is trivial and can be completely calculated during the config phase (replace mod.ProviderConfigs with an expanded view using the static context).


Let's first consider the simpler case of specifying a resource's provider. The resource provider field is either a name or name+alias.  These both refer to providers within the current module and do not have any prefix/path associated.  Let's limit ourselves to not support different provider names within a `provider =` field and only allow the alias to be manipulated. `provider = provider[var.name][var.alias]` vs `provider = provider.name[var.alias]`.

With this limitation, a provider's name is always known and therefore the type will always be known.  The alias, however is malleable.  The majority of the opentofu codebase only cares about names/types and is quite happy when that is unchanging (not altered in for_each).  The alias however is much more flexible and only used in a few critical places, mostly in the provider transformer with values from resource nodes.

Let's consider the resource above. It depends on the provider with a name 'aws' and it is known at config time that it's eventual expanded instances will require aws.us and aws.eu. We could have the unexpanded resource node depend on both providers being configured and ready for use before performing the expansion and assigning each provider to their respective expanded resource instance.

```
Graph:
resource.aws_s3_bucket -> provider.aws.us, provider.aws.eu
Expanded:
resource.aws_s3_bucket["us"] -> provider.aws.us
resource.aws_s3_bucket["eu"] -> provider.aws.eu
```

The unexpanded resource depending on both provider nodes is critical for two reasons:
* The provider transformers try to identify unused providers and remove them from the graph.  This happens pre-expansion before the instanced links are established.
* A core assumption of the code is that expanded instances depend on identical or a subset of references that the unexpanded nodes do.


Although the code ergonomics of modifying the transformers and resource nodes may not be great, it's a fairly surgical change that won't disrupt much of the rest of the codebase and can be tested and validated in isolation.

Let's now consider the module scenario above.  Modules link providers by name through the config tree. Author's note: I don't fully grok this flow and this should be taken with a big grain of salt.

```hcl
# main.tf
terraform {
  required_providers { aws = { source = "hashicorp/aws" } }
}
provider "aws" {
  for_each = local.regions
  alias = each.key
  region = each.value
}
module "mod" {
  source = "./mod"
  for_each = local.regions
  providers {
    magicsauce = provider.aws[each.key]
  }
}
```
```hcl
# mod/main.tf
terraform {
  required_providers { magicsauce = { source = "hashicorp/aws" } }
}
resource "aws_s3_bucket" "primary" {
  provider = magicsauce
}
```

This would imply:
* The root module requires a provider of type "hashicorp/aws" with name "aws"
* There are two aliased provider configurations for provider with name "aws": aws.eu and aws.us
* The ModuleCall "mod" maps a provider named "magicsauce" to both aws.eu and aws.us, depending on the instance it's used in (not known at config time).
* The resulting module "mod" requires a provider of type "hashicorp/aws" with name "magicsauce", this is linked in via the module call.
  - There are two options that won't be known which is used until later on.
* The resource "aws_s3_bucket" (which could also be "magicsauce_s3_bucket") depends on a provider named or typed: "aws" or "margicsauce"


Consider:
* How are these mappings represented currently in code structures? It's a bit convoluted as would be expected
* How much does the config package understand about aliasing and can it be abstracted out?
* How does the resource decide which "magicsauce" to use?
  - The unexpanded resource would depend on both instances
  - Once expanded, the module expansion key would be used to look up the provider instance
  - This implies aliased providers addresses can be resolved given a ModuleInstance path?
* How does this impact state?
  - Resource instances all currently point to a single provider type + alias
  - We could introduce "provider_alias" in state and bump the version number
  - Could also be a good reason to consider pre-expansion?
* What happens if "magicsauce" is passed into another module?
  - During config loading, the name/type is known but the alias can be multiple things?
  - Each level down providers aliases must be made "deferred"
  - This implies aliased providers addresses can be resolved given a ModuleInstance path?



#### Conclusion:

The provider mapping system is quite bodged together and we will need to understand it's idiosyncrasies before making large changes.  It's fun to think of how this evolved from older versions of this code (v0.11 for example).  This has started to be pieced together in [Provider References](./docs/provider-references.md)


### Open Questions

Should variables be allowed in required_providers now or in the future?  Could help with versioning / swapping out for testing?
```hcl
variable "version" {
    type = string
}
terraform {
    required_providers {
        aws = {
            source = "hashicorp/aws"
            version = var.aws_version
        }
    }
}
```

There's also an ongoing discussion in the linked issue on allowing variable names in provider aliases.
Example:
```
# Why would you want to do this?  It looks like terraform deprecated this some time after 0.11.
provider "aws" {
  alias = var.foo
}
```

### Future Considerations

## Potential Alternatives

Go the route of expanded modules/resources as detailed in [Static Module Expansion](20240513-static-evaluation/module-expansion.md)
- Concept has been explored for modules
- Not yet explored for resources
- Massive development and testing effort!

At this point, we now need to determine if pre-expanding modules and resources is simpler / less risk than changing provider mappings to be deferred through the entire application. Initial gut feeling is that provider mappings are fairly isolated to a few complex pieces of code, whereas module/resource expansion complexity is deeply ingrained in the whole codebase.

