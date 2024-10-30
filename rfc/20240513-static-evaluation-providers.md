# Static Evaluation of Provider Iteration

Issue: https://github.com/opentofu/opentofu/issues/300

Since the introduction of for_each/count, users have been trying to use each/count in provider configurations and resource/module mappings. Providers are a special case throughout OpenTofu and interacting with them either as a user or developer requires significant care.

> [!Note]
> Please read [Provider References](../docs/provider-references.md) before diving into this section! This document uses the same terminology and formatting.

## Proposed Solution

The approach proposed in the [Static Evaluation RFC](20240513-static-evaluation.md) can be extended to support provider for_each/count with some clever code and lots of testing. It is assumed that the reader has gone through the Static Evaluation RFC thoroughly before continuing here.

### User Documentation

#### Provider Configuration Expansion

How are multiple configured providers of the same type used today?
```hcl
provider "aws" {
  alias = "us"
  region = "us-east-1"
}
// Copy pasted from above provider, with minor changes
provider "aws" {
  alias = "eu"
  region = "eu-west-1"
}

resource "aws_s3_bucket" "primary_us" {
  provider = aws.us
}
// Copy pasted from above resource, with minor changes
resource "aws_s3_bucket" "primary_eu" {
  provider = aws.eu
}

module "mod_us" {
  source = "./mod"
  providers {
    aws = aws.us
  }
}
// Copy pasted from above module, with minor changes
module "mod_eu" {
  source = "./mod"
  providers {
    aws = aws.eu
  }
}
```
For scenarios where you wish to use the same pattern with multiple providers, copy-paste or multiple workspaces is the only path available. Any copy-paste like this can easily introduce errors and bugs and should be avoided. For this reason, users have been asking to use for_each and count in providers for a long time.

Let's run through what it would look like to enable this workflow.

What is expected when a user adds for_each/count to a provider configuration:
```hcl
locals {
  regions = {"us": "us-east-1", "eu": "eu-west-1"}
}

provider "aws" {
  alias    = "by_region"
  for_each = local.regions

  region = each.value
}
```

At first glance, this looks fairly straightforward. Following the rules in place with resources, we would expect `aws.by_region["us"]` and `aws.by_region["eu"]` to be valid.

What happens if you use `for_each` without `alias`? That would presumably cause reference addresses like `aws["us"]` and `aws["eu"]` which, based on rules elsewhere in the language, end-users would likely assume means the same thing as `aws.us` or `aws.eu`, but that would conflict with a provider configuration that explicitly sets `alias = "us"`.

Therefore we retain the current assumption that a default provider configuration (one without "alias") is always a singleton, and so `for_each` can only be used when `alias` is also set and the instance keys then appear as an extra dynamic index segment at the end of the provider reference syntax. The concept of "default provider configuration" exists in OpenTofu to allow for automatic selection of the provider for a resource in simple cases, and those automatic behaviors rely on the default provider configuration being a singleton.

In the longer term we might allow fully-dynamic expansion and references similar to the existing `for_each` support for resources and module calls, but in prototyping so far we've learned that requires quite significant and risky changes to the core language runtime. This RFC therefore proposes an intermediate step which relies on the ideas set forth in the [Static Evaluation RFC](20240513-static-evaluation.md), which means that the `for_each` argument in a `provider` block and the dynamic index part of a provider reference in a `provider` argument of a resource will initially allow only values known during configuration processing: local values and input variables. Anything dynamic like resources/data will be forbidden for now.

With the provider references clarified, we can now use the providers defined above in resources:

```hcl
resource "aws_s3_bucket" "primary" {
  for_each = local.regions
  provider = aws.by_region["us"] # Extends the existing reference format with an instance key
}

locals {
  region = "eu"
}

module "mod" {
  source = "./mod"
  providers = {
    # The new instance key segment can use arbitrary expressions in the index brackets.
    aws = aws.by_region[local.region]
  }
}
```

The `provider` argument for `resource`/`data` blocks and the `providers` argument for `module` blocks retains much of the rigid static structure previously required, to ensure that it remains possible to statically recognize the relationships between resource configuration blocks and provider configuration blocks since that would be required for a later fully-dynamic version of this feature implemented in the main language runtime: it must be able to construct the dependency graph before evaluating any expressions.

However, the new "index" portion delimited by square brackets is, from a syntax perspective, allowed to contain any valid HCL expression. The only constraints on that expression are on what it is derived from and on its result: it must be derived only from values known during planning, its result must be a value that convert to the type `string`, and the string after conversion must match one of the instance keys of the indicated provider configuration.

Note that in particular this design forces all of the instances of a resource to refer to instances of the same provider configuration, although they are each allowed to refer to a different instance. Again, this is a forward-looking constraint to ensure that it remains possible to build a dynamic evaluation dependency graph where each `resource`/`data` block depends on the appropriate `provider` block before dynamic expression evaluation is possible, even though that isn't strictly required for the static-eval-only implementation.

#### Provider Alias Mappings

Now that we can reference providers via variables, how should this interact with for_each / count in resources and modules?

```hcl
locals {
  regions = {"us": "us-east-1", "eu": "eu-west-1"}
}

provider "aws" {
  alias    = "by_region"
  for_each = local.regions

  region = each.value
}

resource "aws_s3_bucket" "primary" {
  for_each = local.regions
  provider = aws.by_region[each.key]

  # ...
}

module "mod" {
  source   = "./mod"
  for_each = local.regions
  providers = {
    aws = aws.by_region[each.key]
  }

  # ...
}
```

This use of `each.key` is familiar from its existing use in `resource`, `data`, and `module` blocks that use `for_each`. The bracketed portion of the address is evaluated dynamically by the main language runtime, and so (unlike the `for_each` argument in `provider` blocks) this particular expression is _not_ constrained to only static-eval-compatible symbols.

From a user perspective, this provider mapping is fairly simple to understand. As we will show in the Technical Approach, this will be quite difficult to implement.


#### What's not currently allowed

There are scenarios that module authors might think could work, but that we don't intend to support initially so that we can release an initial increment and then react to feedback.

##### Provider references as normal values

```hcl
locals {
  my_provider = aws.by_region["us"]
}

resource "aws_s3_bucket" "primary" {
  provider = local.my_provider
}
```

It's not currenly well-defined what a "provider reference" is outside of a `provider` or `providers` argument. All of the provider references are built as special cases that are handled using HCL static analysis.

Additionally, the OpenTofu language would currently recognize `aws.by_region` as a reference to a `resource "aws" "by_region"` block in any normal expression context, so we cannot easily redefine that existing meaning while retaining backward compatibility.

Syntax compatibility concerns aside, it would be technically possible to treat a provider configuration reference as a value using a concept in the upstream library that provides the low-level building blocks of OpenTofu's type system: [`cty Capsule Types`](https://github.com/zclconf/go-cty/blob/main/docs/types.md#capsule-types). Assuming we also defined a suitable syntax for declaring a new kind of type constraint and referring to a provider in a normal expression, we could technically allow provider instance references to be passed around in a similar way as other values are passed around.

##### One resource with multiple provider configurations

OpenTofu's existing graph construction logic fundamentally assumes that each `resource` block is associated with exactly one provider configuration. This proposal introduces the possibility of a single provider configuration generating multiple provider _instances_, but since this initial proposal aims to avoid making significant changes to the graph transformation logic we will initially require that all instances of a particular resource refer to (potentially different) instances of the _same_ provider configuration.

That restriction is most obvious in the simplest case where a `resource` block has `provider` directly referring to a provider instance in the same module:

```hcl
provider "example" {
  alias    = "foo"
  for_each = toset(["bar", "baz"])

  # ...
}

resource "example_thing" "example" {
  for_each = toset(["bar", "baz"])

  # The part in brackets is dynamic, but the initial reference to the
  # provider configuration block remains static, so effectively all
  # of the instances of this resource _must_ belong to the one
  # single provider configuration block above.

  # This is valid as "example.foo" is statically known
  provider = example.foo[each.key]

  # Whereas something like this is not supported as it could refer to
  # many potential providers depending on the result of local.alias
  provider = example[local.alias][each.key]
}
```

This restriction also applies when passing provider configurations indirectly through module calls. Each instance of a module can have its own provider configuration namespace populated with different provider instances defined in the parent, but they must still all refer to the same configuration block:

```hcl
provider "example" {
  alias    = "foo"
  for_each = toset(["bar", "baz"])

  # ...
}

module "example" {
  source   = "./example"
  for_each = toset(["bar", "baz"])

  providers = {
    # All of the resource blocks in this module can just assume there's
    # a default (unaliased) "example" provider instance available to
    # use. Each one is bound to a different instance from the provider
    # block above, but they must still nonetheless all be bound to
    # the same block.
    example = example.foo[each.key] # Supported

    # However, if we add uncertainty to which provider configuration is
    # being referenced, the above assertions do not hold and become much
    # more complex to reason about.
    example = example[local.alias][each.key] # Unsupported
  }
}
```

We should be able to loosen this restriction in a future RFC, either as part of a fully-dynamic design where provider references are normal expressions as described in the previous section or via some specialized syntax that's only used for provider configurations. The main requirement, regardless of how the surface syntax is designed, is that the graph builder change to support creating multiple dependency edges from a single resource node to multiple provider configuration nodes, so that evaluation of the resource node is delayed until after all of the possibly-referred-to provider configurations have been visited and thus their provider instances are running.

##### Passing collections of provider instances between modules.

There was previously no syntax for causing a single provider configuration to produce multiple provider instances, and so we also have no existing syntax to use to differentiate between a reference to one provider instance vs. a reference to a collection of provider instances.

In this first implementation it's forbidden to refer to a multi-instance provider configuration without specifying an instance key.

For example, consider the following configuration:

```hcl
provider "example" {
  alias    = "foo"
  for_each = toset(["bar", "baz"])

  # ...
}

resource "example_thing" "a" {
  # The following is invalid: must include the instance key part to
  # tell OpenTofu how to select exactly one provider instance.
  provider = example.foo
}

module "child" {
  source = "./child"
  providers = {
    # The right-hand side of this item is invalid: must include the
    # instance key part to tell OpenTofu how to select exactly one
    # provider instance.
    example.foo = example.foo
  }
}
```

The consequences of `example.foo` being invalid don't really matter for the `resource` block because it doesn't make sense to specify multiple provider instances for a single resource anyway.

It _would_ be useful to be able to pass a collection of provider instances to a child module as shown in the second example, but that raises various new design questions about how the child module should declare that it's expecting to be passed a collection of provider instances, since today provider instance references are not normal values and so it isn't really meaningful to describe "a map of `example` provider instances".

Leaving this unsupported for the first release again gives us room to consider various different design options in later RFCs as our understanding of the use-cases grows based on feedback. The following are some early potential designs we've considered so far, but it isn't clear yet which (if any) is the best fit:

- Borrow the "splat operator" syntax to talk about recieving a collection of provider instances in the `configuration_aliases` argument of a module's `required_providers` block:

    ```hcl
    terraform {
      required_providers {
        example = {
          source = "tf.example.com/example/example"

          configuration_aliases = [
            # The following could potentially mean something similar to
            # a provider "example" block with alias = "foo" and a
            # for_each argument, specifying that this module expects its
            # caller to provide a collection of provider instances.
            example.foo[*]
          ]
        }
      }
    }
    ```

- Borrow the "splat operator" syntax instead for use at the _call site_, so the calling module can declare that it's intending to pass a collection of instances:

    ```hcl
    module "child" {
      # ...
      providers = {
        aws.each_region[*] = aws.by_region[*]
      }
    }
    ```

    This is similar to the previous idea except that it makes the assertion of intent to pass multiple instances appear on the caller side rather than on the callee side. It's technically redundant to use extra syntax here since the presence or absense of `for_each` in the `provider "aws"` block that has `alias = "by_region"` is already sufficient to represent whether `aws.by_region` is a single instance or a collection of instances, 

    (This and the previous option could potentially be combined together to allow both caller and callee to explicitly declare what they are intending to do.)

- Wait until we're ready to support provider instance references being treated as normal values of a new type, and then encourage module authors to pass collections of provider configurations via input variables that use a new type constraint syntax, effectively deprecating the current provider-specific sidechannel.

    This generalized approach would allow for various more interesting combinations, such as sending provider instances along with other data as part of a single object per region:

    ```hcl
    variable "aws_regions" {
      type = map(object({
        cidr_block        = string
        provider_instance = providerinst(aws)
      }))
    }

    resource "aws_vpc" "example" {
      for_each = var.aws_regions
      provider = each.value.provider_instance

      cidr_block = each.value.cidr_block
    }
    ```

    ```hcl
    module "example" {
      # ...

      aws_regions = {
        us-west-2 = {
          cidr_block        = "10.1.0.0/16"
          provider_instance = provider.aws.usw2
        }
        eu-west-1 = {
          cidr_block        = "10.2.0.0/16"
          provider_instance = provider.aws.euw2
        }
      }
    }
    ```

##### Using the `count` meta-argument in `provider` blocks

The existing `resource`, `data`, and `module` blocks support either `count` or `for_each` as two different strategies for causing a single configuration block to dynamically declare multiple instances.

`count` is most appropriate for situations where the multiple instances are "fungible". For example, the instances could be considered "fungible" if the process of reducing the number of instances doesn't need to consider which of the instances will be destroyed. A collection of virtual machines all booted from the same disk image, with the same configuration, and running the same software could be considered fungible.

`for_each` is the better option when each instance has at least one unique characteristic that makes it functionally or meaningfully distinct from the others. A collection of virtual network objects where each is used to host different services are _not_ fungible because each network has its own separate role to play in the overall system.

The technical design we've adopted here could potentially allow supporting `count` in `provider` blocks, generating instances with integer numbers as keys just as with `count` in other blocks. However, we don't yet know of any clear use-case for a collection of "fungible" provider configurations: the only reasons we know of to have multiple instances of a provider involve each one being configured differently, such as using a different AWS region. It's important for correctness for the resource instances associated with these provider instances to retain their individual bindings as the set of provider instances changes, because an object created using one provider instance will often appear to be non-existent if requested with another provider instance.

Because of how crucial it is to preserve the relationships between resource instances and provider instances between runs, we've chosen to intentionally exclude `count` support from the initial design. However, the `count` argument remains reserved in `provider` blocks so that we can potentially implement it in future if feedback suggests a significant use-case for it. We hope that the future discovered use-case(s) will also give us some ideas on how we could help protect against the inevitable misbehavior caused by a resource instance getting accidentally reassociated with the wrong provider instance.

### Technical Approach

The following describes the high level changes. A potential implementation has been proposed here: https://github.com/opentofu/opentofu/pull/2105

Our goal is to implement this feature with as little risk as possible. We prioritize minimizing the feature and changeset to minimize the risk. Additional refactoring will likely be performed as this feature grows over time and we choose to implement the most clearly defined and useful parts first.

The core of the changes needed to implement what is described above:
* Allow for_each in provider configuration blocks and evaluate them in the static context
* Initialize and configure a provider instance on every `for_each` entry (exec binary + pass configuration)
* Allow provider key expressions in `resource > provider` and `module > providers` configuration blocks
* Evaluate the provider key expressions when needed during the evaluation graph to determine which provider instance a resource should use.
* Update state storage to understand per-resource-instance provider keys.

#### Provider Configuration Blocks

A "provider configuration" is the `provider "type" {}` block in the OpenTofu Language. This is located within either the root module or a child module. Provider configurations can not be declared in child modules which have been invoked with for_each and count.

Each `provider` block in the configuration is decoded into a `configs.Provider` struct and added to it's respective `configs.Module` at the correct location within the module tree. The existence of this provider block is heavily used when validating the providers within the module tree, see `configs/provider_validation.go`.

Each provider block will need to contain it's static Expansion information.  "Expansion" is the process of deciding the set of zero or more instance keys that are declared for an object using either `count` or `for_each`. For this initial iteration of the feature, the `for_each` expression will be evaluated as part of the main config loader using the static evaluation context as defined in [Init-time static evaluation of constant variables and locals](https://github.com/opentofu/opentofu/blob/main/rfc/20240513-static-evaluation.md).

The `configs.Provider` struct representing each configured `provider` block will now contain new field that contains the static mapping from "provider instance key" to "provider repetition data". In practice, this is a map of `addrs.InstanceKey` to `instances.RepetitionData`.  This map can be built using the StaticContext available in [configs.NewModule()](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/configs/module.go#L122) as defined in the Static Evaluation RFC.

At the end of the `configs.NewModule` constructor, all provider configurations that contain a for_each or count will have their new "instance mapping" field set (or an error message produced). This should not change the majority of provider validation logic in the configuration package as the name/type/alias information *has not changed*.

##### Provider Node Execution

Each "provider configuration block" is turned into a "provider node" (NodeApplyableProvider) within the tofu graph. When a provider node is executed, it starts up and configures a `providers.Interface` of the corresponding provider. This "interface" is stored in the `tofu.EvalContext` and is available for use by other nodes via the context (referenced below).

`NodeApplyableProvider` now effectively represents all `provider.Interface`s of a particular `provider` configuration block at once, and its "execute" step now involves a loop performing for each instance similar behavior that was previously always singleton:
1. Evaluate the configuration using the appropriate `instances.RepetitionData`, thereby causing `each.key` and `each.value` to be available with instance-specific values. The result is a `cty.Value` representing an object of the type implied by the provider's configuration schema.
2. Create a child process running the appropriate provider plugin executable, creating a running provider interface.
3. Call the `ConfigureProvider` function from the provider protocol on the new provider interface, passing it the `cty.Value` representing its instance-specific configuration.
4. Save the provider interface in the shared `tofu.EvalContext` for use by downstream resource nodes.

A special case exists here for `tofu validate`. The validate codepath does not configure any providers or deal with module/resource "expansion" (for_each/count). A validate graph walk should only initialize and register one `providers.Interface`.  It can still however validate provider configurations using the provider's schema and `providers.Interface.ValidateProviderConfiguration` call for each set of provider instance data on the single instance.


##### Selecting a provider instance for each resource instance

In the initial dependency graph constructed during the planning phase, there are graph nodes representing whole `provider` and `resource`/`data` blocks, but not yet their individual instances. This is because the _existing_ expansion behaviors all use dynamic expression evaluation and so `for_each` or `count` has not yet been evaluated at the time of graph construction, and the expressions in those arguments can potentially imply additional dependencies that need to be included in the graph for the expression to ultimately evaluate successfully.

The initial graph node type for a `resource` or `data` block is `nodeExpandPlannableResource`, which ultimately represents the entire process of deciding which dynamic instances are declared for the block and performing the per-instance configuration evaluation for each of the instances.

The per-instance evaluation process now grows to include the task of evaluating the dynamic instance key portion of the `provider` argument. If an author wrote `provider = aws.by_region[each.key]` then each instance selects a different instance of `aws.by_region` based on its own value of `each.key`.

OpenTofu does not need to know the final provider instance for a resource instance until just before it begins making requests to the provider, so we can safely wait until visiting/executing an individual resource instance before deciding its provider configuration but when doing so we must deal with the possibility that the target provider instance might be accessed only indirectly through the provider instances passed by the parent module, and therefore we must now track for each resource the following new information:

- The `hcl.Expression` representing the expression in the brackets at the end of the reference expression.
- The `addrs.Module` of which module in the chain contains the dynamic instance key expression, and therefore which scope the instance key needs to be evaluated in.

    The initial rule against passing entire collections of providers between modules means that in practice there will always be exactly one reference per resource that carries a dynamic instance key expression but that reference might actually be in a calling `module` block rather than in the `resource` block itself. (Future generalizations to support passing collections of providers will require tracking some additional detail here.)

Each resource _instance_ must also now track the final `addrs.InstanceKey` that was the result of evaluating the resource's `hcl.Expression` instance key expression using that instance's containing scope and repetition data. That combines with the already-tracked provider configuration address to produce a fully-qualified provider instance address.

##### Dynamic provider instance keys in `module` blocks

When the dynamic provider instance selection occurs in a `module` block, rather than directly in a `resource`/`data` block, it appears as part of the existing capability of projecting some or all of the provider configurations in the calling module to also appear (potentially with different aliases) in the callee. For example:

```hcl
provider "aws" {
  alias    = "by_region"
  for_each = var.aws_regions

  # ...
}

module "example" {
  source   = "./example"
  for_each = var.aws_regions
  providers = {
    aws = aws.by_region[each.key]
  }
}
```

This configuration means that each _instance_ of `module "example"` has its default (unaliased) configuration for `hashicorp/aws` acting as an alias for one of the dynamic instances of the calling module's `aws.by_region`.

Therefore when resolving the dynamically-chosen provider instance for each resource in the child module, the resource instance node for each resource instance in the child module needs to be able to effectively walk up the module tree to find out that any reference to `aws` in the child module (whether explicit or implicit) must be resolved by evaluating `aws.by_region[each.key]` _in the calling module_, using the calling module's scope and the appropriate module call instance's `each.key` value.

Fotunately for us, this process is already encapsulated well within the `ProviderConfigTransformer` and can be extended for this purpose. The `ProviderConfigTransformer` adds nodes to the graph that either represent "provider configurations" directly (`NodeApplyableProvider`) or proxies to those providers. These proxies know how to walk up the tree to locate the actual provider configuration.

The proxy structure can easily be extended to also include the "provider key expression" required to perform the mapping. The `ProviderTransformer` is in charge of determining what "provider configuration node" is used by all of the resource nodes in the graph. Additional information from the proxy traversal can be added to each resource node's SetProvider() function call to help it determine which instance it should be using.

At the end of this process, each resource node should know both what "provider configuration" it should be trying to use, as well as if there is a provider key expression which needs to be evaluated within a given module path.

##### Resource Instance Visiting/Execution

When a resource instance is visited (Execute method called), it's first task is to determine which provider it should be using.

With all of the previously described machinery in place, it has access to:
* The required provider configuration block address (ProviderTransformer)
* The module level provider key expression + module path (ProviderTransformer/ProviderConfigTransformer)
* The resource provider key expression (configs.Resource)

The resource instance then determines if it should be using the module or resource provider key expression (only one is allowed). It evaluates that expression within the specified context and produces a "provider instance key".

It then asks the EvalContext for the `providers.Interface` determined by "provider configuration block address" and "provider instance key".

Once the provider interface has been determined (resolved), it can continue it's usual unmodified execution path.

##### Providers Instances in the State

Currently OpenTofu assumes that all instances of a resource must be bound to the same provider instance, because each provider configuration can have only one instance and each resource can be bound to only one provider configuration. This assumption is unfortunately exposed in the state snapshot format, which tracks the provider configuration address as a property of the overall resource rather than of each instance separately.

To allow for future generalization without the need to make further breaking changes to the state snapshot format, and to avoid making rollback to an older OpenTofu version difficult for anyone who has never used these new features, we will extend the state snapshot format to _also_ track provider instance information at the resource instance level.

Today's state snapshot format includes the following information for each resource (irrelevant properties excluded):

```json
{
  "resources": [{
    "module": "module.example",
    "type": "aws_instance",
    "name": "example",
    "provider": "module.example.provider[\"registry.opentofu.org/hashicorp/aws\"].by_region",
    "instances": [
      {
        "index_key": "first"
      },
      {
        "index_key": "second"
      }
    ]
  }]
}
```

We will add a similar `"provider"` property to each of the objects under `"instances"` which tracks the same information but might additionally capture a trailing instance key for the specific selected provider instance:

```json
{
  "resources": [{
    "module": "module.example",
    "type": "aws_instance",
    "name": "example",
    "instances": [
      {
        "index_key": "first",
        "provider": "module.example.provider[\"registry.opentofu.org/hashicorp/aws\"].by_region[\"us-west-2\"]"
      },
      {
        "index_key": "second",
        "provider": "module.example.provider[\"registry.opentofu.org/hashicorp/aws\"].by_region[\"eu-east-1\"]"
      }
    ]
  }]
}
```

The state snapshot loader will parse both the resource-level address and the instance-level addresses and determine which to use for each resource instance. It will, for now, return an error if any of the instance addresses are not identical aside from the optional trailing instance key. This preserves our current simplification of each resource still being bound to exactly one provider configuration while offering a more general syntax that we can retain in future if we weaken that constraint. We don't yet know exactly what flexibility we will need for future features, and so capturing the entire provider instance address (and then verifying their consistency) gives us the freedom to refer to literally any valid provider instance address in future versions, without changing the syntax and thus without the need for new state snapshot format upgrade logic.

We could retain the `"provider"` argument at the resource level, despite it now being technically redundant. It would offer a measure of backward-compatibility with older versions of OpenTofu. When the core team discussed this in depth, we decided that it would likely be better to omit this duplicate information when possible to force the user to roll-back and apply their configuration before downgrading their OpenTofu version.  This is not a supported path, but we want to make sure that the proper steps are taken when downgrading.

This section described changes only to the internal state snapshot format that is not documented for external use. We do not intend to extend the documented `tofu show -json` output formats yet as part of this change, because we want to retain design flexibility for later work and to learn more about what real-world use-cases exist for machine-consumption of the provider instance relationships before we design a final format that will then be subject to compatibilty constraints.

### Open Questions

#### Dynamic or early-evaluated provider dependencies

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

Initial discussion with Core Team suggests that this should be moved to it's own issue as it would be useful, but does not impact the work done in this issue.

#### Dynamic or early-evaluated provider aliases

Early discussion for this feature considered possibly allowing more than just constant strings in the `alias` argument. If we supported this it would likely be limited only to early-evaluation to preserve the requirement that each `provider` block have a known alias when we build the main language runtime's dependency graph.

Example:
```hcl
# Why would you want to do this?
provider "aws" {
  alias = var.foo
}
```

We have intentionally omitted this for now because we don't know of any real use-cases for it, and it's conceptually easier to imagine `alias` as being effectively equivalent to the second label in the header of a `resource` block -- a statically-chosen unique identifier for use in references elsewhere -- than as another possibly-dynamic element in addition to instance keys.

We can safely add early-eval-based aliases in a later release without any breaking changes, so we will await specific use-cases for this before we decide whether and how to support it.

### Future Considerations

A potential fully-dynamic version of this feature is discussed in another RFC: [Dynamic Provider Instances and Instance Assignment](https://github.com/opentofu/opentofu/pull/2088). We don't intend to support that immediately but have "designed ahead" to reduce the risk that backward-compatibility with the static eval implementation would block a later dynamic implementation.

If we ever decide to implement Static Module Expansion, how will that interact with the work proposed in this RFC?

## Potential Alternatives

Go the route of expanded modules/resources as detailed in [Static Module Expansion](20240513-static-evaluation/module-expansion.md)
- Concept has been explored for modules
- Not yet explored for resources
- Massive development and testing effort!

