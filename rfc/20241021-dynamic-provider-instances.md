# Dynamic Provider Instances and Instance Assignment

Related issues:
- [Dynamic provider configuration assignment](https://github.com/opentofu/opentofu/issues/300)
- [Allow provider declarations in submodules](https://github.com/opentofu/opentofu/issues/1896)

The earlier RFC [Static Evaluation of Provider Iteration](https://github.com/opentofu/opentofu/blob/9c379c0dc02a6e2da4ea6bc9d3e9b8178ceb3dc7/rfc/20240513-static-evaluation-providers.md) proposed to use the configuration-level early expression evaluation mechanism to provide some limited support for programmatically generating multiple instances of a provider based on input variable values and for associating different instances of one resource with different instances of a provider each.

The decisions made in that proposal are reasonable if we assume that early evaluation is the final end state for this feature, but the original request was for _dynamic_ provider instantiation and assignment and so we don't have enough evidence to rule out later requests for making this behavior more dynamic.

The possibilities for fully-dynamic evaluation are somewhat more constrained than for early evaluation because dynamic evaluation must take into account the correct ordering of externally-visible side-effects, whereas early evaluation can assume that all needed data is immediately available and that configuration loading never has externally-visible side-effects.

This proposal therefore describes how fully-dynamic provider instantiation and assignment could potentially work, both to prepare for the likely future request for this feature and to draw attention to some additional constraints implied by dynamic evaluation that we may wish to also artifically apply to the early eval implementation to maximize our changes of being able to transition to this dynamic approach later without causing breaking changes for existing modules relying on the first implementation.

## Proposed Solution

The subsequent sections will describe dynamic provider instantiation and assignment as if it were being implemented as the first incarnation of the feature, without reference back to the early eval design, but because our primary goal here is to inform potential changes to the early eval design we'll begin with a summary of what additional constraints this proposal implies as compared to the early eval proposal:

- Provider dynamic instance keys are _in addition to_ the existing idea of "alias", rather than replacing it.

    OpenTofu's strategy for "dynamic expansion" (instantiating new objects based on the results of expression evaluation) fundamentally relies on constructing a dependency graph between all of the _statically-declared_ configuration elements -- `resource` blocks, `data` blocks, `provider` blocks, etc -- and then walking that dependency graph and "expanding" each of those into zero or more instances only when its graph node is visited.

    That approach relies on two main related constraints:

    - Each configuration block has some sort of statically-decided unique identifier so that the configuration can unambiguously describe the static relationships between the configuration objects without relying on the not-yet-evaluated dynamic instance keys.

        For example, a reference to `aws_instance.example[each.key]` has enough information for us to learn that it refers to the `resource "aws_instance" "example"` block, even though we won't know what `each.key` evaluates to or exactly which instance keys that resource block will have until we're already walking the graph.

    - Each configuration block that can "expand" has its own independent namespace of "instance keys", which means we can assume that no two configuration blocks can possibly generate the same dynamic instance key.

        For example, a `resource "aws_instance" "example1"` block and a `resource "aws_instance" "example2"` block that both use `for_each` cannot possibly both dynamically generate the same resource instance address, because `aws_instance.example1` and `aws_instance.example2` are each separate namespaces and each resource block can only contribute instances to one of them.

    `provider` blocks unfortunately use an inconsistent syntax where the "unique identifier" is expressed via the nested `alias` argument instead of using a block label. This inconsistency results in large part from the fact that early versions of the product allowed only one configuration per provider and thus the provider's name _was_ originally a sufficient identifier, although we can justify that retroactively by noting the concept of "default provider configurations" -- those which don't set `alias` at all -- which OpenTofu relies on for various "magical" behaviors that are intended to make life easier for those who are new to the product, so they can defer learning about the complexities of provider configuration until later.

    That difference of syntax aside, the `alias` argument (or absense thereof) serves as part of the static unique identifier of a `provider` block -- the `configs` package rejects any configuration that declares a duplicate provider configuration based on aliases -- and so we need to retain that as static so that our graph building and execution can assume that each graph node can "expand" independently of all others.

- In `provider` blocks, `for_each` is allowed only when `alias` is also present.

    The concept of "default provider configurations" has a number of special "magical" behaviors attached to it in current OpenTofu, such as automatically inheriting downward across module calls, and also even having synthetic empty default configurations created automatically whenever a resource doesn't specify `provider` and there is no explicit configuration for it to default-select.

    Making a default provider configuration be treated as effectively a map of multiple provider instances undermines those assumptions, because it's no longer meaningful to check for and select the _one_ default instance for a provider.

    The original early eval proposal didn't need to consider this case because it specified that `for_each` effectively _generates_ aliases, and so the resulting instances would always be "additional" (aka "aliased") by definition. Since this proposal calls for retaining alias in its previous form and adding optional new instance keys _in addition_, it is theoretically possible for us to allow a "default provider configuration" (one without an alias) to have multiple instances, but that would be conceptually confusing and we'd need to carefully define how that should interact with all of our existing special treatment of the singleton default provider configurations.

    It's overall simpler and easier to explain to require all multi-instance provider configurations to be "additional" (to have `alias` specified), and our documentation and error messages can directly encourage setting `alias` to something which gives a hint as to what the instance keys might represent. For example, `aws.by_region["us-east-2"]`.

- The `provider` argument in a `resource` block, if specified, must specify the provider local name and alias statically. The instance key can be dynamically-selected.

    Our graph construction process relies on static analysis of expression syntax, rather than on expression evaluation. We currently determine that a resource with `provider = aws.foo` depends on a `provider "aws"` block with `alias = "foo"` by directly inspecting the expression syntax.

    For the reasons described above, we need to be able to determine which single provider configuration block a particular resource depends on before evaluating expressions so that we can then use the dependency graph to correctly evaluate the expressions. Making instance key be an optional extra _in addition to_ the provider local name and alias means that we can support an expression like `provider = aws.by_region[each.value.region]` by first noticing the static relationship with `aws.by_region` during graph construction, and then finally resolving the `each.value.region` part during the evaluation phase at which point we can assume that _all_ instances of `aws.by_region` will be ready.

    This constraint also applies to the entries in the `providers` argument in a `module` block, since that behaves as a translation table between the provider alias namespaces in different modules so that we can resolve provider dependencies across module call boundaries.

- All machine-readable output, such as the `tofu show -json` output, must expose specific instance keys only as part of dynamic artifacts like plan and state, and _not_ as part of the config description.

As noted above, the sections below will present the proposed functionality as if the early-eval variant were never implemented just so this document can focus on the end state rather than the intermediate step, but this RFC also doubles as an implicit proposal to do at one of the following (the first being the most likely outcome):

1. Make an adjusted version of the early-eval proposal that incorporates the above constraints while still implementing the behavior using early-eval.

    This would mean that we'd still be able to continue with the early eval goal for the next release, while reducing the risk of technical design challenges should we wish to implement something like _this_ proposal in a later release. However, there is still some risk of "unknown unknowns" on this path, since early eval is still a very new concept in OpenTofu and we have no practical experience of migrating an existing feature from early eval to full dynamic eval retroactively.

2. Put the provider expansion feature on hold for now and consider having this dynamic eval proposal be the _first_ implementation meeting these use-cases.

    This path is non-ideal because it means postponing what was intended to be a headline feature of the next release. Its primary consequence would be only to buy us more time for more detailed analysis and prototyping so that we can potentially further reduce the risk of shipping something that is challenging to evolve; we could still choose to build and ship the early eval variation first, if we later convince ourselves the risk is sufficiently low, but it would be in a some release rather than the next one.

The decision between these is ultimately a risk tradeoff. Much of the remainder of this proposal focuses on the end state rather than the path to get there so that the decision between the above two options is effectively out of scope and can be made as a project management decision separately from this technical proposal.

### User Documentation

From an end-user perspective, this feature has two main parts: the ability to declare that a single `provider` block actually declares zero or more provider instances dynamically, and the ability for each dynamic instance of a resource to refer to a different dynamic instance of its associated provider configuration.

There are some other language changes that follow from those two new capabilities, which are also discussed in subsequent sections below.

#### Multi-instance provider configuration blocks

The schema for `provider` blocks is extended to allow optionally using the `for_each` argument in conjunction with the `alias` argument. For example:

```hcl
variable "aws_regions" {
  type = map(object({
    vpc_cidr_block = string
  }))
}

terraform {
  required_providers {
    aws = {
      source = "hashicorp/aws"
    }
  }
}

provider "aws" {
  alias    = "by_region"
  for_each = var.aws_regions

  region = each.key
}
```

The `provider "aws"` block in this example declares one additional provider configuration (also sometimes called "alternate" or "aliased" provider configurations) which has a dynamically-chosen set of instances. Each element of the map provided in `var.aws_regions` corresponds to one provider instance belonging to this block.

Using `alias` and `for_each` together like this means that `aws.by_region` is no longer a valid provider instance address on its own: a fully-qualified instance address for instances from this configuration block must have an additional _instance key_ portion, like `aws.by_region["us-east-2"]`.

The non-meta arguments in this `provider` block can refer to `each.key` and `each.value`, with the same meaning as those symbols would take in a `resource` block with a `for_each` argument.

The `for_each` argument is allowed only when the `alias` argument is also set. A provider configuration without `alias` is a special "default provider configuration", which must always be a singleton to allow OpenTofu to select it automatically or even to generate a synthetic empty default configuration for the provider when needed. Authors are encouraged to set `alias` to an identifier which somehow suggests what the instance keys represent; in this case `by_region` suggests that the instance keys are AWS region names.

#### Dynamic selection of provider instance for each resource instance

The `provider` meta-argument in a `resource` block defines a rule for connecting each instance of the resource to exactly one provider instance that is responsible for managing it.

The `provider` argument's expression must start with a static reference to a provider configuration block:

- `provider = aws` refers to the default configuration of whichever provider has the local name "aws" in the current module, which might be a synthetic empty configuration block if none was explicitly written.

    In most cases this situation is best expressed by omitting the `provider` argument completely and allowing OpenTofu to select the default configuration for the provider whose local name matches the first segment of the resource type name, such as selecting `aws` for `aws_s3_bucket`.
    
    Explicitly assigning a default provider configuration is needed only when the module's choice of local name for the provider does not match the prefix of its resource type names, which is rare but can occur if a module depends on two providers that use the same prefix. A practical situation where that can occur is when a module is using both the `hashicorp/google` and `hashicorp/google-beta` providers together, or when only using the `hashicorp/google-beta` provider but using the local name `google-beta` to refer to it, in which case a `google_compute_address` resource managed by an instance of the `hashicorp/google-beta` provider would need to specify `provider = google-beta` to avoid implicitly selecting the default configuration of the `hashicorp/google` provider.

- `provider = aws.secondary` refers to an additional/alternate/aliased configuration of whichever provider has the lcoal name "aws" in the current module. Additional provider configurations must always be explicitly written when needed; empty configuration is never implied for these.

    Each module has its own namespace of aliases, so the alias `secondary` might refer to different provider instances in different modules.

When using the two-part form that specifies both a local name and an alias, an author can optionally follow it with indexing brackets `[ ]` containing an arbitrary dynamic expression that evaluates to a string to be used as a key to dynamically select a provider instance. For example, `provider = aws.by_region[each.key]` dynamically selects the instance of `aws.by_region` whose instance key matches the instance key of the current resource instance.

If the `provider` expression includes the instance key segment, the expression in the brackets must not return an unknown value. In particular this means that the expressions should typically not refer to a result attribute of another resource unless the author is confident that the value will always be available during the planning phase. OpenTofu needs to select a single provider instance for each resource instance during the planning phase because the provider plugin is responsible for generating the plan for any resource instance that belongs to it, so it isn't possible to wait until the apply phase to decide which provider instance to use.

Note that while this syntax allows dynamic selection of a provider instance, it _doesn't_ allow dynamic selection of which `provider` block the instance is taken from. All instances of a specific resource must refer to dynamic instances of a single `provider` block, because OpenTofu needs to find the dependency relationships between `resource` and `provider` blocks before it can evaluate the dynamic expression provided in brackets.

The above uses managed resources (`resource` blocks) to describe the mechanism, but the same behavior applies to resource blocks of all modes: `provider` works the same way in `data` blocks and would presumably also work the same way for any other new resource modes that we might add in future.

#### Explicitly passing provider instances between modules

Each module instance has its own namespace of provider instance addresses, but it's possible for a provider instance address in one module to refer to a provider configuration block defined in one of its ancestor modules.

A module author can declare that their module expects to be passed provider instances from its caller using the optional `configuration_aliases` argument in a `required_providers` entry. For example:

```hcl
terraform {
  required_providers {
    aws = {
      source = "hashicorp/aws"
      configuration_aliases = [
        aws.foo,
        aws.bar,
      ]
    }
  }
}
```

The above declares that this module has local provider instance addresses `aws.foo` and `aws.bar`, but which provider configuration they actually refer to is decided by the parent module. The parent module's `module` block calling this module must therefore include a `providers` argument that describes how to map from the parent module's address namespace into the child module's address namespace:

```hcl
provider "aws" {
  alias = "use1"

  region = "us-east-1"
}

provider "aws" {
  alias = "euw2"

  region = "eu-west-2"
}

module "example" {
  source = "./example" # This is the directory containing the previous example block
  providers = {
    aws.foo = aws.use1
    aws.bar = aws.euw2
  }

  # ...
}
```

The above configuration means that `aws.foo` in the child module refers to the same provider instance as `aws.use1` refers to in the parent module.

The mapping assigned to the `providers` argument is a static mapping, so it must always use the object constructor syntax with braces `{ }`; it's not valid to assign a dynamic expression that would generate a map.

To build the dependency graph OpenTofu needs to bind each provider configuration address in the child module to exactly one provider configuration address in the parent module before evaluating any expressions. It's valid to use dynamic instance expressions on the right side of the mappings in the `providers` block, but as with resources all instances of a module call must refer to instances of the same provider configuration:

```hcl
provider "aws" {
  alias    = "by_region"
  for_each = var.aws_regions

  region = each.key
}

module "example2" {
  source = "./another-example" # This is _not_ the directory containing the earlier example
  for_each  = var.aws_regions
  providers = {
    aws = aws.by_region[each.key]
  }
}
```

The above means that in each instance of `module.example2` the default instance of "aws" is dynamically decided by evaluating `each.key`. The resources using `provider = aws` (implicitly or explicitly) in that module all depend on the `aws.by_region` configuration from the parent module in the dependency graph, and then choose one of its instances dynamically at runtime after evaluating `each.key`.

The final interesting case is passing a whole multi-instance provider configuration between modules. For this we extend the `configuration_aliases` syntax to allow declaring that the module expects to be passed a multi-instance configuration rather than a singleton configuration:

```hcl
variable "aws_regions" {
  type = map(object({
    cidr_block = string
  }))
}

terraform {
  required_providers {
    aws = {
      source = "hashicorp/aws"
      configuration_aliases = [
        aws.by_region[*],
      ]
    }
  }
}

resource "aws_vpc" "main" {
  for_each = var.aws_regions
  provider = aws.by_region[each.key]

  cidr_block = each.value.cidr_block
  # ...
}
```

When a module is configured in this way, the `providers` argument in any call _must_ include an entry for `aws.by_region` whose selected configuration also uses `for_each`, like this:

```hcl
provider "aws" {
  alias    = "by_region"
  for_each = var.aws_regions

  region = each.key
}

module "example" {
  source = "./example" # This is the directory containing the previous example block
  providers = {
    aws.by_region = aws.by_region
  }

  # ...
}
```

In _this_ situation `aws.by_region` in both the parent and child modules refers to the same multi-instance provider configuration, with the same instance keys. The child module allows the parent module to decide which instance keys to use, so the child module can be written generically.

> [!NOTE]
> Since this proposal doesn't include any way for a normal value expression to retrieve a set of the instance keys associated with a provider configuration, the example with `configuration_aliases` set to `aws.by_region[*]` would initially require the caller to _also_ pass the set of region names as a normal input variable so that the child module can know which instance keys it ought to expect to find for its `aws.by_region`. It would be the caller's responsibility to ensure that the instance keys passed by input variable are always a subset of the instance keys provided with the provider configuration.
>
> That is a typical hazard that results from the historical quirk of provider instances being passed "out of band" rather than as normal values. The "Future Considerations" section below includes [Provider Instance References as Normal Values](#provider-instance-references-as-normal-values) which discusses how we could potentially generalize this further in future, by making some considerably more invasive changes to the language syntax.

#### Automatic inheritance of provider instances between modules

Whenever a `module` block does not include an explicit `providers` argument, OpenTofu implicitly passes the default configuration for each provider that both parent and child depend on, and does _not_ pass any additional provider configurations for any provider.

For example, if both parent and child depend (implicitly or explicitly) on `hashicorp/aws` and the parent module uses the local name `aws` to refer to that provider then OpenTofu would infer an implied `providers` argument equivalent to the following:

```hcl
  providers = {
    aws = aws
  }
```

Because this mechanism is only for default provider configurations, and because dynamic instances are only allowed for additional provider configurations, this proposal does not change this implied behavior in any way.

#### `provider` blocks in nested modules

Current OpenTofu documentation calls for `provider` blocks to be placed only in the root module of a configuration (in [Provider Configuration](https://opentofu.org/docs/language/providers/configuration/#provider-configuration-1)):

> Provider configurations belong in the root module of an OpenTofu configuration. (Child modules receive their provider configurations from the root module; for more information, see [The Module `providers` Meta-Argument](https://opentofu.org/docs/language/meta-arguments/module-providers/) and [Module Development: Providers Within Modules](https://opentofu.org/docs/language/modules/develop/providers/).)

Despite the strong wording, this is actually more a suggestion than a requirement: for backward compatibility with earlier versions, OpenTofu _does_ allow `provider` blocks in non-root modules, but only if none of the `module` blocks on the path to the module make use of `count`, `for_each`, or `depends_on` arguments.

This strong suggestion to place `provider` blocks only in the root module is primarily to avoid the problem described in [Removal of dynamic provider instances with resource instances present](#removal-of-dynamic-provider-instances-with-resource-instances-present) below: OpenTofu generally requires a provider instance to be defined in configuration for at least one more plan/apply round than all of the resource instances it manages, because OpenTofu uses the provider configuration to configure the provider for any "destroy" operations for resource instances that are no longer declared.

If we either somehow resolve that constraint or decide to ignore it for the sake of supporting dynamic provider instances then we've also either resolved or decided to ignore the main blocker for [Allow provider declarations in submodules](https://github.com/opentofu/opentofu/issues/1896), and so there's no longer any strong reason to block using `provider` blocks in any non-root modules.

Making that change would not introduce any new syntax to the language, but would permit using the `for_each`, `count`, and `depends_on` arguments with a module that includes a `provider` block, whereas today that situation always returns an error.

### Technical Approach

#### Address Representations

OpenTofu uses [`package addrs`](https://pkg.go.dev/github.com/opentofu/opentofu/internal/addrs) to represent addresses of the various different types of objects used by the system in a way that allows the Go compiler to check for consistent usage, and hopefully helps readers of the code understand the intended meaning of and relationships between these objects.

Unfortunately for historical reasons the vocabulary of address types for provider configurations is inconsistent both with current usage and with the naming schemes used elsewhere in `package addrs`. Since this proposal introduces for the first time a strong distinction between the concept of "provider configuration" and "provider instance" -- a provider configuration can now have zero or more instances, whereas before this was always 1:1 -- I strongly recommend starting by updating the addressing types to better describe the new relationships.

Specifically:

- [`AbsProviderConfig`](https://pkg.go.dev/github.com/opentofu/opentofu@v1.8.3/internal/addrs#AbsProviderConfig) would be renamed to `ConfigProviderConfig` and would now represent a not-yet-expanded provider configuration.

    (The two appearances of the word "Config" in the name are unfortunate but consistent with the existing naming scheme elsewhere in `package addrs`, where the `Config` prefix represents the address of a static configuration block while the `Abs` prefix with the same suffix represents the potentially-many instances it "expands" into. `Config`-prefixed addresses typically have a `Module Module` field, whiile `Abs`-prefixed addresses typically have a `Module ModuleInstance` field.)
- A new type `AbsProviderInstance` represents a fully-expanded provider instance address:

    ```go
    type AbsProviderInstance struct {
        Module   ModuleInstance
        Provider Provider
        Alias    string
        Key      InstanceKey
    }
    ```

    This type is the most specific type and would become the lookup key in the language runtime's internal map of the `providers.Interface` objects for each active provider instance.
- [`LocalProviderConfig`](https://pkg.go.dev/github.com/opentofu/opentofu@v1.8.3/internal/addrs#LocalProviderConfig) remains but its scope is reduced only to describing which provider configuration block each `resource` or `data` block is associated with during graph construction, so that we can add the appropriate dependency edges.
- A new type `LocalProviderInstance` represents the local-scoped equivalent of `AbsProviderInstance` as described above:

    ```go
    type LocalProviderInstance struct {
        LocalName string
        Alias     string
        Key       InstanceKey
    }
    ```

    An address of this type is the result of fully-evaluating the expression in the `provider` argument of a `resource` or `data` block, once the instance key expression has been evaluated. An address of this type can then in turn be translated into an `AbsProviderInstance` using the local provider name tables in each module to finally look up the correct `providers.Interface` to use for actions related to a resource instance.
- The interface type [`ProviderConfig`](https://pkg.go.dev/github.com/opentofu/opentofu@v1.8.3/internal/addrs#ProviderConfig) is used only to deal with the annoying situation during graph construction where configuration-based graph nodes produce "local" addresses while state-based and plan-based graph nodes produce "absolute" addresses.

    Its name has always been a little confusing since an unprefixed name like this would normally represent an address in a single module's namespace that isn't yet qualified by a module. I propose to rename it to `LocalOrConfigProviderConfig` to make its name directly represent what it is.

    Under this new model, the two types that implement this interface would be `LocalProviderConfig` and `ConfigProviderConfig`, since those are the two address types we need to use when creating the graph dependency edges from resource nodes to provider configuration nodes. (Graph construction does not consider instance keys because they haven't been evaluated yet.)

    A desire for completeness might call for us also having `LocalOrAbsProviderInstance` implemented by `LocalProviderInstance` and `AbsProviderInstance`, but there doesn't seem to be any current need for that adaptation because code working with instance addresses currently always knows statically whether it's dealing with local or absolute addresses, and explicitly translates between them.

The `Local` prefix on `LocalProviderConfig` and `LocalProviderInstance` remains inconsistent with other names in this package: the type containing only the module-local part of an address without any module path information is conventionally given no prefix at all, as in the triad of `Resource`, `ConfigResource`, and `AbsResource`. The different prefix for this particular situation is justified by the fact that -- unlike all other examples of this distinction -- there is no purely-syntactic translation from `LocalProviderConfig` to `ConfigProviderConfig` or from `LocalProviderInstance` to `AbsProviderInstance`: the "local" prefix is representing that these elements use `LocalName string` instead of `Provider Provider` fields, and so extra context is always required to translate between these two families of address types.

#### Graph Construction

Since the initial dependency graph only captures the relationships between whole resources and whole provider configurations, no significant behavioral change is required to the graph builders, although the handling of the `provider` argument in `resource` blocks and `providers` argument in `module` blocks would need to allow (but ignore) the new optional instance key expression syntax.

Although the graph construction process does not need any significant changes, the following summarizes the high-level thought process using the updated address terminology from the previous section just for completeness:

1. Walk the configuration tree to find all `provider` blocks. Add a graph node for each one whose identity is an `addrs.ConfigProviderConfig` address.
2. Iterate over all of the nodes that were already in the graph before the previous step -- which includes a node for each resource whose existence is implied by the configuration and/or state -- and find the subset of them that are "provider consumers" (expressed by implementing a particular extension interface).

    Each provider consumer is required to return a single `addrs.LocalOrAbsProviderConfig` value, which can be translated into an `addrs.ConfigProviderConfig` in a different way depending on the concrete type:

    - `LocalProviderConfig`: Use the provider local name table of the node's module to find which `addrs.Provider` the local name refers to, and walk up the configuration tree through `configuration_aliases` entries to find which `addrs.Module` the provider config _actually_ belongs to and what its alias is in that module, if any. That provides all of the values required to instantiate `addrs.ConfigProviderConfig`.
    - `ConfigProviderConfig`: A type assertion is sufficient to deal with this case.

3. Create a dependency edge pointing from every node that is a "provider consumer" to the provider config node that owns the discovered `addrs.ConfigProviderConfig` address.

    If there is no such provider config node, and the `addrs.ConfigProviderConfig` has an empty alias and therefore represents a "default provider configuration", insert a new node with a nil `*configs.Provider` representing an implicitly-generated empty default configuration for that provider and use that as the dependency for each node that refers to that address.

The project to implement these new capabilities would be a good opportunity to reflect on the existing implementation and hopefully simplify it by removing some crufty complexity not included in the high-level description above, like "proxy" provider nodes, but that's an implementation decision to be made by the final implementer depending on how much time for refactoring work was included in the project schedule.

#### During Evaluation

The main evaluation phase consists of visiting each node in the graph in an order which respects the dependency edges. Each visited node has the opportunity to perform two optional actions:

- "Execute" with [`GraphNodeExecutable`](https://pkg.go.dev/github.com/opentofu/opentofu@v1.8.3/internal/tofu#GraphNodeExecutable): performs any calculations or side-effects related to the individual node itself.
- "DynamicExpand" with [`GraphNodeDynamicExpandable`](https://pkg.go.dev/github.com/opentofu/opentofu@v1.8.3/internal/tofu#GraphNodeDynamicExpandable): constructs a "dynamic subgraph", which is a typically-small extra graph that the language runtime walks completely before it considers the node to be "complete".

    Although this technically generates a graph, it's completely separate from the main execution graph and so it can't include any new dependencies on nodes outside of the subgraph. These dynamic subgraphs therefore usually have no edges at all and are really just a flat set of nodes to visit in an unspecified order.

In today's OpenTofu at the time of writing this document, all of the behavior for the provider configuration graph nodes occurs in the "Execute" phase, because each provider configuration block always has exactly one instance.

To implement dynamic-expanding provider configurations, we need an extra level of indirection: the nodes added to the initial graph implement `DynamicExpand` instead of `Execute`, and then the `DynamicExpand` implementation returns a graph containing one node for each of the zero-or-more instances generated for that provider configuration.

In address-type terms, the main graph has nodes that each have one `addrs.ConfigProviderConfig` addresse, while the dynamic subgraph contains nodes that each have one `addrs.AbsProviderInstance` addresses. All of the nodes in one subgraph have `addrs.AbsProviderInstance` addresses that share the same provider type, alias, and static module path but vary can vary in instance keys for any step in the module path and the instance key of the provider instance itself.

The following code is taken from a partial prototype of the behavior described in this proposal, and describes the above more concretely:

```go
func (n *nodeProviderConfigExpand) DynamicExpand(globalCtx EvalContext) (*Graph, error) {
	var diags tfdiags.Diagnostics

	// There are two different modes of dynamic expansion possible for providers:
	//
	// 1. A provider block can contain its own for_each argument that
	//    causes zero or more additional ("aliased") provider configurations
	//    to be generated in the same module.
	// 2. A provider block can appear inside a module that was itself
	//    called with for_each or count, in which case each instance
	//    of the module has its own provider instance.
	//
	// This function handles both levels of expansion together in the
	// nested loops below.

	g := &Graph{}

	recordInst := func(instAddr addrs.AbsProviderInstance, config *configs.Provider, keyData instances.RepetitionData) bool {
		log.Printf("[TRACE] nodeProviderConfigExpand: found %s", instAddr)

		nodeAbstract := &nodeAbstractProviderInstance{
			Addr:   instAddr,
			Config: config,
            // (we'll need to track some further information here to allow
            // evaluating each.key/each.value in the provider block, etc)
		}
		node := n.concrete(nodeAbstract)
		g.Add(node)

		return true
	}

	allInsts := globalCtx.InstanceExpander()
	config := n.config
	moduleInsts := allInsts.ExpandModule(n.addr.Module)
	for _, moduleInstAddr := range moduleInsts {
		// We need an EvalContext bound to the module where this
		// provider block has been instantiated so that we can
		// evaluate expressions in the correct scope.
		modCtx := globalCtx.WithPath(moduleInstAddr)

		switch {
		case config.Alias == "" && config.ForEach == nil:
			// A single "default" (aka "non-aliased") instance.
			recordInst(addrs.AbsProviderInstance{
				Module:   moduleInstAddr,
				Provider: n.addr.Provider,
				Key:      addrs.NoKey, // Default configurations have no instance key
			}, config, EvalDataForNoInstanceKey)
		case config.Alias != "" && config.ForEach == nil:
			// A single "additional" (aka "aliased") instance.
			recordInst(
				addrs.AbsProviderInstance{
					Module:   moduleInstAddr,
					Provider: n.addr.Provider,
					Alias:    n.addr.Alias,
					Key:      addrs.NoKey,
				},
				config,
				EvalDataForNoInstanceKey,
			)
		case config.Alias != "" && config.ForEach != nil:
			// Zero or more "additional" (aka "aliased") instances, with
			// dynamically-selected instance keys.
			instMap, moreDiags := evaluateForEachExpression(config.ForEach, modCtx)
			diags = diags.Append(moreDiags)
			if moreDiags.HasErrors() {
				continue
			}
			for k := range instMap {
				instKey := addrs.StringKey(k)
				recordInst(
					addrs.AbsProviderInstance{
						Module:   moduleInstAddr,
						Provider: n.addr.Provider,
						Alias:    n.addr.Alias,
						Key:      instKey,
					},
					config,
					EvalDataForInstanceKey(instKey, instMap),
				)
			}
		default:
			// No other situation should be possible if the config
			// decoder is correctly implemented, so this is just a
			// low-quality error for robustness.
			diags = diags.Append(fmt.Errorf("provider block has invalid combination of alias and for_each arguments; it's a bug that this wasn't caught during configuration loading, so please report it"))
		}
	}

	addRootNodeToGraph(g)
	return g, diags.ErrWithWarnings()
}
```

The above preserves the existing concept of initially constructing an "abstract" node type and then passing it to a "concrete" function to wrap it in a specific implementation. During all phases except the special "eval" phase used for the `tofu console` command, the final node type is [`NodeApplyableProvider](https://pkg.go.dev/github.com/opentofu/opentofu@v1.8.3/internal/tofu#NodeApplyableProvider), whose `Execute` implementation is responsible for actually starting up the provider plugin (if it isn't a builtin) and calling `ConfigureProvider` on it. The resulting `providers.Interface` value is then stored in a map keyed by `addrs.AbsProviderInstance` so that other graph nodes can find it later.

The other significant change during evaluation is in the implementation of the graph nodes representing individual resource instances. Because today's OpenTofu forces a one-to-one relationship between provider configuration and provider instance, the resource instance nodes already "know" exactly which provider instance they are bound to once the initial graph has been constructed. The implementation of these node types must now perform the final step of evaluating the optional "instance key" expression in the `provider` argument, to complete the information required to construct an `addrs.AbsProviderInstance` which can be used to look up the correct `providers.Interface` to use when performing the real operations related to that resource instance.

#### State and Plan Format Changes

The original state snapshot format version 4 tracks a provider configuration address for each entire resource, and then all of the instances of the resource are assumed to share that same provider configuration address. To support the features in this proposal requires two changes:

- Each _resource_ should record the `addrs.ConfigProviderConfig` address it was statically bound to in the most recently-applied configuration. This is technically not really a change at all, since `addrs.ConfigProviderConfig` is effectively just a rename of the current `addrs.AbsProviderConfig` and that's what OpenTofu already tracks on a per-resource basis, but it's worth noting just to complete the conceptual model.

    This resource-level address would be used during graph construction where resource instances are not yet expanded. This is used only when the `resource` block for this resource has been removed from the configuration and thus the state is the only record we have remaining of which provider configuration the resource was most recently managed by.
- Each _resource instance_ should also now record the `addrs.AbsProviderInstance` it was dynamically bound to in the most recent round, which allows us to deal with the fact that each resource instance is now allowed to refer to a different instance of the provider.

    The `addrs.AbsProviderInstance` on each resource instance _must_ conform to the `addrs.ConfigProviderConfig` of the resource it is nested under, or the state snapshot is invalid. OpenTofu will ensure that this invariant is always maintained when generating state snapshots, and so this invalid case can arise only if someone manually edits a state snapshot (whimsically known as "state surgery").

The state snapshot format has a three-level nested heirarchy of resource, resource instance, and resource instance object because state snapshots are part of the input used to construct a graph for planning, which is always done in terms of static configuration objects rather than dynamic instances. It's for this reason that we need to be able to retain both a single provider configuration address and possibly-many provider instance addresses as separate data in the state snapshot format.

The _plan_ format is only consumed when constructing a graph for the apply phase, and in the apply phase the full set of dynamic instances is always already known and so the main graph begins in the fully-expanded state. Therefore the plan format does not follow the same heirarchical shap and is instead just a flat set of _resource instance objects_, each of which refers to a provider instance. Those references must now be based on `addrs.AbsProviderInstance` values instead of `addrs.ConfigProviderConfig` values to retain the dynamic instance key information for each resource instance object.

#### Changes to machine-readable output formats

[OpenTofu's JSON output formats](https://opentofu.org/docs/internals/json-format/) provide a compatibility-constrained for external software to consume certain information about configurations, saved plans, and state snapshots.

Earlier work on these formats intentionally withheld detailed information about the relationships between resources and specific provider instances in anticipation of future work like what's described in this proposal. The only information _strictly required_ in the JSON output formats is the provider source address -- e.g. `registry.opentofu.org/hashicorp/aws` -- because that must be combined with the resource type name and the schema version to uniquely identify the specific schema that the resource instance data is following. However, additional detail is often appreciated by those who are writing highly-specific policy rules that are intended to check that, for example, a particular object is being declared in the correct AWS region.

To allow some iteration flexibility for an initial release of this feature I propose to leave these JSON formats unchanged, but once we feel confident that the relationships between our system's object types are "settled" and unlikely to need to change significantly in future we could update the JSON output formats to reflect similar information as what we'd be capturing in the internal state snapshot and plan file formats:

- In the [values representation](https://opentofu.org/docs/internals/json-format/#values-representation) that we use to describe the prior state and planned new state, the JSON objects under the `resources` property are actually really _resource instance objects_.

    The existing `provider_name` property is intended for use with information in the "configuration representation" to find the corresponding provider source address. We can extend that with a new `provider_instance` property which contains a string representation of the `addrs.AbsProviderInstance` that the resource instance of each resource instance object was most recently associated with.
- In the [plan representation](https://opentofu.org/docs/internals/json-format/#plan-representation) used to describe planned changes, the JSON objects under the `resources` property are again actually _resource instance objects_. There's also similar information under `resource_drift` which describes any changes OpenTofu detected between the original input state and the current (not yet changed) state of each remote object

    It's a known existing gap in this format that there's currently not actually any information about provider association _at all_ for the planned changes, and so consumers of this format must cross-reference with the values representation of the "planned new state" to find the provider that will be responsible for each planned change.

    We can fill that gap at the same time as introducing instance information by adding both `provider_name` and `provider_instance`, with the same meanings as for the values representation in the previous point.
- The [configuration representation](https://opentofu.org/docs/internals/json-format/#configuration-representation) describes the static configuration as written, without any expression evaluation. Therefore it can't directly capture any information about dynamic provider instances or the resource instances that dynamically refer to them, but we do currently return a `provider_config_key` property that is effectively a string representation of an `addrs.LocalProviderConfig` address.

    We might choose to build on that by adding `provider_instance_expression` that uses the [expression representation](https://opentofu.org/docs/internals/json-format/#expression-representation) to describe either a statically-specified provider instance key _or_ the set of references that it would be derived from during dynamic evaluation, matching how we represent dynamic expressions elsewhere in the configuration. This object would describe only the expression written in brackets at the end of the `provider` argument, if any, since the full `provider` expression is not actually ever evaluated to produce a value and so the "expression representation" is a not appropriate for the expression as a whole.

### Open Questions

#### Removal of dynamic provider instances with resource instances present

OpenTofu uses provider plugins to plan and apply the "delete" (aka "destroy") action for any managed resource instance that is present in the prior state but not present in the desired state described by the current configuration (unless the author explicitly declared that it should not be destroyed using a `removed` block).

Therefore it's a long-standing hazard that any `provider` block in the configuration must always exist for at least one more plan/apply round after all of its associated managed resources have been removed from the configuration. It's this hazard that led to the current strong recommendation that `provider` blocks should exist only in the root module, because if a child module includes both a `provider` block and a `resource` block that's associated with it then removing the calling `module` block from the parent module effectively causes both the provider configuration and the resource configuration to be removed at once, leaving OpenTofu unable to plan and apply the delete action for any instances of that resource. Instead, OpenTofu returns the following frustrating error:

```
Error: Provider configuration not present

To work with module.foo.aws_instance.bar its original provider configuration
at module.foo.provider["registry.opentofu.org/hashicorp/aws"] is required,
but it has been removed. This occurs when a provider configuration is removed
while objects created by that provider still exist in the state. Re-add the
provider configuration to destroy module.foo.aws_instance.bar, after which
you can remove the provider configuration again.
```

The text of the error message is correctly describing what OpenTofu needs -- re-adding the `provider` block while keeping the `resource` block absent -- but that resolution simply isn't possible when the operator is interacting with these blocks only indirectly through a single `module "foo"` block. Unless the module author had the foresight to provide an input variable to dynamically disable all of the nested `resource` blocks, the operator now needs to learn some very arcane details of how OpenTofu works, to perform some highly-unnatural act like changing the `source` in the `module "foo"` block to refer to a new temporary module that _only_ contains a `provider "aws`" block suitable for destroying all of the existing instances.

This error belongs to a troublesome but thankfully-small category of errors that OpenTofu can catch only after infrastructure already exists, and therefore operators can become "trapped" in a situation which they don't know how to resolve but they also cannot make any further changes until they've resolved it, and therefore the situation is often percieved as an emergency or incident.

All proposals that allow for adding and removing provider configurations without actually modifying the configuration source code effectively create even-more-subtle versions of this problem. That is already true for [the existing early-eval proposal](https://github.com/opentofu/opentofu/blob/9c379c0dc02a6e2da4ea6bc9d3e9b8178ceb3dc7/rfc/20240513-static-evaluation-providers.md), and it's also true of this new proposal in both of the following situations

```hcl
variable "regions" {
  type = set(string)
}

module "per_region" {
  # Let's assume that ./per_region contains a "provider" block.
  # That would not be allowed in today's OpenTofu, but is permitted
  # under this proposal.
  source   = "./per_region"
  for_each = var.regions

  # ...
}
```

```hcl
variable "regions" {
  type = set(string)
}

provider "aws" {
  alias    = "per_region"
  for_each = var.regions

  region = each.value
}

resource "aws_s3_object" "example" {
  for_each = var.regions
  provider = aws[each.value]

  # ...
}
```

The first example is a direct dynamic analogy to the original problem described above: if the previous round included instance `module.per_region["us-west-2"]` and the next round doesn't then everything inside that module instance has been removed simultaneously, with the same effect as removing that entire `module` block has in today's OpenTofu.

The second example achieves the same broken outcome without using child modules: removing an element from `var.regions` removes an instance of `aws_s3_object.example` and its corresponding instance of `aws.per_region` at the same time.

We currently have no settled solution to this problem. It remains to be decided whether we're willing to ship any form of input-variable-based provider expansion, whether early-eval or fully dynamic, without first solving this problem. However, it's likely that whatever solution we devise for this would simultaneously solve the problem for the early-eval approach, this dynamic-eval approach, _and_ for the provider-config-in-child-module variant of the problem.

### Future Considerations

#### Provider instance references as normal values

A lot of the strange restrictions on how provider configurations can be used in OpenTofu trace back to the fact that the surface-level language syntax for them is entirely separate from most other concepts, despite the use-cases being pretty analogous to passing of values:

- Both values and provider configurations can be passed from parent module to child, but values go through input variables while provider configurations use the specialized `providers` argument in the parent and `configuration_aliases` in the child.
- Both values and provider configurations have use-cases where multiple ought to be declared dynamically and passed around as a collection, but so far only values actually support that while provider configurations are highly rigid and static.

If we had the freedom to start over with a new language, it would be most ideal if a reference to a provider configuration was just a special kind of value that can be passed around like any other value, and included as part of larger data structures. Achieving that would require a few different building blocks:

- A syntax for describing "provider configuration reference" as a type constraint in a `variable` block, such as `type = map(providerconfig(aws))` to describe a map of provider configuration references for whatever provider has the local name `aws` in the current module.
- A syntax for referring to provider configurations directly in expressions, such as `provider.aws.foo` where the `provider.` prefix is similar to the prefixes used on other special symbol types like `var.example`, `local.example`, `module.foo.bar`, etc.
- A way to represent provider configuration addresses as `cty.Value` so that they can interact with the dynamic expression capabililties in the rest of the language. This could be handled using [`cty`'s Capsule Types](https://github.com/zclconf/go-cty/blob/main/docs/types.md#capsule-types), with a separate capsule type for each distinct `addrs.Provider` whose encapsulated value is an `addrs.AbsProviderInstance` where the `Provider` field always matches that of the capsule type.

Work to implement the features proposed elsewhere in this RFC could potentially rework the _internals_ of OpenTofu's language runtime to treat provider references as if they were normal expressions using the building blocks described above, which could then be a good first step towards a more general solution in future.

However, [the v1.x compatibility promises](https://opentofu.org/docs/language/v1-compatibility-promises/) make it very challenging to change the surface-level syntax to match that hypothetical general implementation. In particular (but likely not limited to):

- An expression like `provider.aws.foo` currently refers to an attribute of the singleton instance of the `resource "provider" "aws"` block. Although it is rather unlikely that there's an OpenTofu provider in the world with a resource type literally named "provider", we cannot prove that there isn't one. (This is an example of the general problem of [Reserved Words and Extension Points](https://log.martinatkins.me/2021/09/25/future-of-the-terraform-language/#reserved-words-and-extension-points) I described in an earlier blog post about how we might evolve the language in future.)
- To retain backward compatibility with older modules we'd need to continue to support the special side-channels for passing providers between modules, which includes the `providers` argument in `module` blocks, the `configuration_aliases` argument in `required_providers`, and (most annoyingly) the magical behavior of automatically-inheriting default provider configurations.

    Indeed, the very concept of "default provider configurations" makes the provider reference syntax confusing and difficult to implement with the existing language building-blocks: if `provider.aws.foo` refers to an additional provider configuration for that provider then `provider.aws` must therefore be of a map or object type so that the `.foo` operation is valid, and so it couldn't simultaneously be of the special capsule type used to represent a reference to a provider configuration.
- Currently there is no value in the OpenTofu language that can't be successfully serialized with the `jsonencode` and `yamlencode` functions, aside from the special situation where unknown values cause the result to be an unknown string. There's no reasonable JSON or YAML serialization of a provider configuration reference and so we'd need to decide what those functions ought to do in those cases. If we decide that they should return an error then that invites a feature request for a function that takes an arbitrary data structure and replaces all of the unserializable values with `null`, or similar.

    We also have a special type constraint `any` which currently causes OpenTofu to infer a type based on the provided value. The original intended use for that was to allow modules to accept arbitrary data structures to pass to functions like `jsonencode`, where the module treats the given value as totally opaque. Unless we did something quite special the new provider configuration reference types would be accepted for `any` too, which could allow values of that type to end up inside modules that were never intended to deal with them, causing confusing errors.

Perhaps we could eventually use a new [language edition](https://log.martinatkins.me/2021/09/25/future-of-the-terraform-language/#terraform-language-editions) to introduce these changes, but this particular problem is not an easy fit for language editions because provider configuration references cross module boundaries and so we'd still need to decide what the rules are for passing provider configuration references from a new-edition module into an old-edition module, or vice-versa.

#### References to "all resources of a type" or "all provider configurations of a type"

An intermittent but repeated feature request has been [to allow treating "all resources of a type"](https://github.com/opentofu/opentofu/issues/1418) as a value in itself, whereas today a reference expression like `aws_instance.foo` is an atomic unit that cannot be shortened to just `aws_instance`.

The current design represents the true nature of the underlying language model: OpenTofu needs to be able to build a dependency graph from referrer to referent, and so it needs to know that `aws_instance.foo` refers to `resource "aws_instance" "foo"` before evaluating any expressions.

Supporting an expression like `aws_instance[var.dynamic_thing]` or `[for i in aws_instance: i.id]` _is_ technically possible if we were willing to significantly rework the language runtime's "reference transformer", teaching it that the way to resolve `aws_instance` alone is to search the current module for all resources of that type and make the referrer depend on _all of them_. That would then achieve much the same effect as what happens when authors use the current approach of manually constructing a mapping to refer to:

```hcl
locals {
  instances = {
    a = aws_instance.a
    b = aws_instance.b
  }

  dynamic = [for i in local.instances: i.id]
}
```

OpenTofu correctly infers that `local.dynamic` depends on `local.instances` and `local.instances` depends on both `aws_instance.a` and `aws_instance.b`, and so the resulting dependency graph is correct.

Nothing here is directly relevant to dynamic provider instances as described in this proposal, but I'm writing this section in anticipating of a similar request to refer to "all provider configurations of a type" -- e.g. writing multiple `provider "aws"` blocks each with a different `alias` and then trying to use the aliases as dynamic instance keys similar to what the early eval variant of this proposal allowed.

As far as I can see, the best way to allow for such capabilities in future is to change the language runtime to internally treat dynamic provider instances as much like dynamic resource instances as possible, and also consider the ideas in [Provider instance references as normal values](#provider-instance-references-as-normal-values) to extend that analogy even to the surface language syntax, and then any solution we might devise for allowing this sort of "all (something) of a type" reference in future could treat all of the "somethings" in the same way, rather than needing a separate solution for provider configurations as for other object types.

## Potential Alternatives

While preparing this document I tried two other variations that aimed to hew closer to the original early-eval-based proposal, which informed the constraints I proposed above. The following sections therefore document some specific challenges we'd need to overcome if we wanted to retain the flexibility of the current early eval proposal while retroactively introducing dynamic behavior.

# One graph node per distinct provider

An interesting implication of the original static eval proposal, when viewed through the lens of the dynamic evaluation dependency graph, is that it effectively considers all of the `provider` blocks in a particular module as if they were a single object: allowing the "alias" to be dynamically decided based on expression evaluation means that the only remaining static identifier is the provider type _itself_.

Consider the following example:

```hcl
terraform {
  required_providers {
    aws = {
      source = "hashicorp/aws"
    }
  }
}

variable "dynamic_aws_provider_instances" {
  type = map(object({
    # ...
  }))
}

provider "aws" {
  alias = "a"

  # ...
}

provider "aws" {
  for_each = var.dynamic_aws_provider_instances

  # ...
}

resource "aws_instance" "example" {
  provider = aws.a

  # ...
}
```

With a dynamic-eval reinterpretation of the early eval design, both of these provider blocks are effectively contributing new "aliases" to the namespace of "aws" provider instances in this module. _Something_ needs to check and return an error if `var.dynamic_aws_provider_instances` includes an element with the key `"a"` because otherwise we'd have two provider instances with the same address `aws.a`, and thus the configuration would be ambiguous.

As long as the `for_each` is something early-evaluable this is not a big deal, because we can catch it immediately during configuration loading. However, if we allow that `for_each` to include references to dynamic data then we can't resolve the full set of aliases for "aws" until we're already walking the graph, which means that everything using the AWS provider -- regardless of which instance it refers to -- needs to depend on a single graph node that's responsible for finalizing the set of declared aliases and making sure they are all unique.

That design would be possible in principle if we were unconstrained by existing behavior, but unfortunately today's OpenTofu already allows one instance of a provider to indirectly depend on another instance of the same provider:

```hcl
terraform {
  required_providers {
    aws = {
      source = "hashicorp/aws"
    }
  }
}

provider "aws" {
  alias = "a"

  # ...
}

data "aws_ssm_parameter" "region" {
  provider = aws.a

  name = "primary_region"
}

provider "aws" {
  alias = "b"

  region = data.aws_ssm_parameter.region.value
  # ...
}
```

If we retroactively decide that both of those provider blocks flatten to a single node in the dependency graph then that node must now depend on `data "aws_ssm_parameter" "region"`, but that resource would also depend on the provider's single graph node and so there's now a dependency cycle, making this previously-valid configuration invalid.

# One graph node per `provider` block without unique addresses

In the introduction to this proposal I asserted that each graph node needs to have a unique identifier, but that's isn't _strictly_ true: viewed purely from the low-level graph representation, OpenTofu doesn't actually care about uniqueness of graph nodes beyond the fact that each one must have a distinct heap allocation and thus a unique address in memory.

Therefore I next tried a variation where each `provider` block still gets its own graph node and the graph builder just does its best to draw the dependency edges as precisely as possible, but can add extra dependency edges in any case where we don't have enough information to choose a single unique provider block to depend on.

However, that approach is troublesome for two somewhat-independent reasons:

1. If there are two `provider` blocks that both set `for_each` in the same module, or one block that sets `for_each` and another that sets `alias`, we fundamentally still need to have one single place where we verify that there are no conflicting alias declarations between the provider blocks.

    If there were any conflicting declarations then a reference like `provider = aws.foo` would be ambiguous and would therefore need to depend on _all_ `provider "aws"` blocks in the current module, despite the `foo` identifier being statically configured here, making this degenerate to being equivalent to the situation in the previous section.

2. Conservatively depending on more provider-configuration graph nodes than strictly necessary would have knock-on impacts on various existing behaviors that rely on the dependency edges between resource nodes and provider nodes, including:

    - "Pruning" of unused provider instances: OpenTofu makes some effort to avoid configuring any provider that wouldn't have any work to do, and some existing users with somewhat-large configurations rely on that to constrain the peak memory usage of OpenTofu runs. Making the graph mode conservative means that this pruning would be less successful, and thus possibly a peak memory usage regression or other performance regression.
    - Delayed closure of provider instances: Related to the previous point, OpenTofu tries to arrange to "close" a provider as soon as possible after its work is done as another way to potentially reduce peak memory usage. This behavior is derived from the dependency relationships from resources to providers -- it effectively introduces the opposite dependency edges from the "close" nodes to the resource nodes -- and so making the provider dependencies more conservative also implies making the close dependencies more conservative, potentially causing providers to stay running longer than they technically need to.
    - Apply phase challenges with create vs. destroy nodes: the dependency graph during an _apply_ phase is classically troublesome due to the possibility of mixed "create" and "destroy" nodes and the fact that destroy nodes have inverted dependencies: a resource must be destroyed only after everything that depends on it has been destroyed.

        There is unfortunately lots of existing fragile logic doing special work to narrowly avoid creating dependency cycles in different tricky cases, and previous experience tells me that introducing even more edges pointing into provider graph nodes is likely to upset that balance in ways that are hard to predict.

Therefore although at a low level it's not _technically_ required for each node to have a corresponding unique reference address -- we can technically deal with ambiguity by just adding more possibly-unnecessary nodes to the graph -- the higher-level functionality dealing with provider graph nodes in particular has a number of tricky implementation details that are hard to reason about and risky to change. Although it _may_ be practical to change these rules in future, we don't yet have enough information to be confident about it.

# Allowing `for_each` without `alias`

The main differences in this proposal vs. the original early eval proposal are in the treatment of instance keys: this proposal calls for them to be added _in addition to_ aliases rather than as a different way of specifying aliases, and then further proposes that `for_each` only be allowed in conjunction with `alias`.

The second part of that is not techncially required for correctness and is instead motivated by user experience concerns. Consider the following hypothetical configuration:

```hcl
provider "aws" {
  alias = "a"

  # ...
}

provider "aws" {
  for_each = {
    "a" = {}
  }

  # ...
}
```

If we allow this then we need some syntax for distinguishing between these two cases. We _do_ have the syntax tools required for this, and existing conventions would perhaps call for the first block's singleton instance to be called `aws.a` while the second's instance is called `aws["a"]`. 

OpenTofu would have no technical problem with those two addresses being distinct, but there is no other place in the OpenTofu language where `.a` and `["a"]` are both allowed but yet have different meanings.

Requiring `alias` to always be set when `for_each` is set is a relatively-small concession that means that we have only three forms to deal with and each builds on the previous in a similar way to how other similar language features work:

* `aws`: default provider configuration
* `aws.secondary`: singleton additional provider configuration
* `aws.by_region["us-east-2"]`: multi-instance additional provider configuration

Although it's a much less significant concern than user confusion, this constraint also allows for simpler address decoding logic: the number of traversal steps immediately distinguishes between the three cases, and each position has only one valid syntax element: the sequence is always root name, attribute access, index. Simpler decoding logic typically also allows better error messages for invalid input, because the error message doesn't need to hedge so much between multiple possible user intents.

Since the concept of "default provider configurations" exists primarily to allow OpenTofu to magically select or even generate them when there's no explicit selection, and that behavior is only possible if the default configuration is a singleton, I think it's justified to require a static alias so that there are fewer situations to explain in the documentation and no risk of conflict with existing assumptions about "default provider configurations".
