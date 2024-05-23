# Implementing Init-time static evaluation of constant variables and locals

As initially described in https://github.com/opentofu/opentofu/issues/1042, many users of OpenTofu expect to be able to use variables and locals in a variety of locations that are currently not supported.

```hcl
variable "accesskey" {
}

terraform {
  backend "somebackend" {
    accesskey = var.accesskey
  }
}
```

To understand why this is, we need to peek under the hood and understand how and why OpenTofu evaluates expressions in configuration.

> [!NOTE]
> It is HIGHLY recommended to read the [Architecture document](./architecture.md) before diving too deep into this document. Below, many of the concepts in the Architecture doc are expanded upon or viewed from a different angle for the purposes of understanding this proposal.

## Expressions

The evaluation of expressions (`1 + var.bar` for example) depends on required values and functions used in the expression. In that example, you would need to know the value of `var.bar`. That dependency is known via a concept called "[HCL Traversals](https://pkg.go.dev/github.com/hashicorp/hcl/v2#Traversal)", which represent an attribute access path and can be turned into strongly typed "OpenTofu References". In practice, you would say "the expression depends on an OpenTofu Variable named bar".

Once you know what the requirements are for an expression ([hcl.Expression](https://pkg.go.dev/github.com/hashicorp/hcl/v2#Expression)), you can build up an evaluation context ([hcl.EvalContext](https://pkg.go.dev/github.com/hashicorp/hcl/v2#EvalContext)) to provide those requirements or return an error.  In the above example, the evaluation context would include `{"var": {"bar": <somevalue>}`.

Expression evaluation is currently split up into two stages: config loading and graph reference evaluation

## Config Loading

During configuration loading, the hcl or json config is pulled apart into Blocks and Attributes. A Block contains Attributes and nested Blocks. Attributes are simply named expressions (`foo = 1 + var.bar` for example).

```hcl
some_block {
    some_attribute = "some value
}
```

These Blocks and Attributes are abstract representations of the configuration which have not yet been evaluated into actionable values. Depending on the handing of the given block/attribute, either the abstract representation is kept or it is evaluated into a real value for use.

As a concrete example, the `module -> source` field must be known during configuration loading as it is required to continue the next iteration of the loading process.  However attributes like `module -> for_each` may depend on attribute values from resources or other pieces of information not known during config loading and therefore are stored as an expression for later evaluation.

```hcl
resource "aws_instance" "example" {
  name  = "server-${count.index}"
  count = 5
  # (other resource arguments...)
}

module "dnsentries" {
  source   = "./dnsentries"
  hostname = each.value
  for_each = toset(aws_instance.example.*.name)
}
```

No evaluation context is built or provided during the entire config loading process.  **Therefore, no functions, locals, or variables may be used during config loading due to the lack of context.  This limitation is what we wish to resolve**.

## Graph Reference Evaluation

After the config is fully loaded, it is transformed and processed into nodes in a [graph (DAG)]( https://en.wikipedia.org/wiki/Directed_acyclic_graph). These nodes use the "[OpenTofu References](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/addrs/parse_ref.go#L174)" present in their blocks/attributes (the ones not evaluated in config loading) to build both the dependency edges in the graph, and eventually an evaluation context once those references are available.

This theoretically simple process is deeply complicated by the module dependency tree and expansion therein. The graph is dynamically modified due to `for_each` and `count` being evaluated as their required references are made available. The majority of the logic in this process exists within the `tofu` and `lang` package which are somewhat tightly coupled.

For example, a module's `for_each` statement may require data from a resource: `for_each = resource.aws_s3_bucket.foo.tags`. Before it could be evaluated, the module must wait for "OpenTofu Resource Reference aws_s3_bucket.foo" to be available. This would be represented as a dependency edge between the module node and the specific resource node. The evaluation context would then include `{"resource": {"aws_s3_bucket": {"foo": {"tags": <provided value>}}}}`.

> [!NOTE]
> A common misconception is that modules are "objects". However, modules more closely resemble "namespaces" and can cross reference each other's vars/outputs as long as there is no reference loop.


## Initial implementation

As you can see above, the lack of evaluation contexts during the config loading stage prevents any expressions with references from being expanded. Only primitive types and expressions are currently allowed during that stage.

By introducing the ability to build and manage evaluation contexts during config loading, we would open up the ability for *certain* references to be evaluated during the config loading process.

For example, many users expect to be able to use `local` values within `module -> source` to simplify upgrades and DRY up their configuration. This is not currently possible as the value of `module -> source` *must* be known during the config loading stage and can not be deferred until graph evaluation.

```hcl
local {
  gitrepo = "git://..."
}
module "mymodule" {
  source = locals.gitrepo
}
```

By utilizing Traversals/References, we can track what values are statically known throughout the config loading process. This will follow a similar pattern to the graph reference evaluation (with limitations) and may or may not re-use much of it's code.

When evaluating an Attribute/Block into a value, any missing reference must be properly reported in a way that the user can easily debug and understand. For example, a user may try to use a `local` that depends on a resource's value in a module's source. The user must then be told that the `local` can not be used in the module source field as it depends on a resource which is not yet available.  Variables used through the module tree must also be passed with their associated information. In practice this is fairly easy to track and has been prototyped during the exploration of [#1042](https://github.com/opentofu/opentofu/issues/1042).

Implementing this initial concept will allow many of the [solutions](#solutions) below to be fully implemented.  However, there are some limitations due to module expansion which are worth considering.

## Additional Complexity due to Module Expansion

The concepts of `for_each` and `count` were grafted on to the codebase in a way that has added significant complexity and limitations. When a module block contains a `for_each` or `count` all of the nodes (resources/variables/locals/etc...) will be created multiple times, one copy per "instance".

One common example of a limitation would be to use different providers for different module instances:
```hcl
# main.tf
module "mod" {
        for_each = {"us" = "first", "eu" = "second"}
        source = "./mod"
        name = each.key
        providers {
          aws = provider.aws[each.value]
        }
}
```

As the provider requirements are baked into the module itself, the multiple "instances" don't have any concept of providers per instance. This becomes even more complex when you consider that these providers might be passed through a complex tree of modules before they are directly used.

There are two potential solutions detailed below in [Provider Iteration](#provider-iteration), each with their own tradeoffs.  Depending on the decision made there, we may not allow each.key/vaue and count to be used in module sources (at least for now).

## Plan of Attack

Between the initial implementation, the simple solutions, and the provider complexity, we are talking about a significant amount of work likely spread across multiple releases.

We can not take the approach of hacking on a feature branch for months or freezing all related code. It's unrealistic and unfair to other developers.

Instead, we can break this work into smaller discrete and testable components, some of which may be easy to work on in parallel.

If we design an interface for the static evaluator and wire a noop implementation through the config package, work on all of the major solutions can be started. Those solutions will then become functional as pieces of the static evaluator are implemented.

With this piece by piece approach, we can also add testing before, during, and after each component is added/modified.

The OpenTofu core team should be the ones to do the majority of the core implementation and the module expansion work.  If community members are interested, many of the solutions are isolated and well defined enough for them to be worked on independently of the core team.

## Progress Overview:
- [ ] Plan Approved by Core Team
- [ ] [Core Implementation](#core-implementation)
  - [ ] Define Static Evaluator Interface
  - [ ] Pick Static Evaluator Approach
  - [ ] Implement Static Evaluator
  - [ ] Wire Static Evaluator through the config package
  - [ ] Implement one of the simple solutions to validaten
- [ ] [Solutions](#static-context-design)
  - [ ] [Module Sources](#module-sources)
  - [ ] [Provider Iteration](#provider-iteration)
  - [ ] [Backend Configuration](#backend-configuration)
  - [ ] [Lifecycle Attributes](#lifecycle-attributes)
  - [ ] [Variable defaults/validation?](#variable-defaultsvalidation)
  - [ ] [Provisioners](#provisioners)
  - [ ] [Moved blocks](#moved-blocks)
  - [ ] [Encryption block](#encryption)

## Core Implementation:

Before implementation starts, the relevant parts of the current config loading / graph process must be well understood by all developers working on it.

### Current Design / Workflow

Performing an action in OpenTofu (init/plan/apply/etc...) takes the following steps (simplified):
* A [command](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/command/init.go#L193) in the command package [parses the configuration in the current directory](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/configs/parser_config_dir.go#L41-L58)
  - [The module's configuration is loaded](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/configs/parser_config.go#L54) into [configs.File](https://github.com/opentofu/opentofu/blob/290fbd6/internal/configs/module.go#L76) structures
    - Fields like [module -> source](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/configs/module_call.go#L79) are evaluated without a evaluation context (nil)
    - config items are [validated](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/configs/module_call.go#L145-L150) (which should not be done here, see [#1467](https://github.com/opentofu/opentofu/issues/#1467))
  - `configs.File` [structures used to create](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/configs/module.go#L122) a [configs.Module](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/configs/module.go#L22) using [various rules](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/configs/module.go#L205)
  - `configs.Module` is [used to build](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/configs/config_build.go#L173) a [config.Config](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/configs/config.go#L30) which represents the [module](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/configs/config.go#L57) and it's [location](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/configs/config.go#L48) within the module [config tree](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/configs/config.go#L53)
  - [configs.Module.ModuleCalls](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/configs/module_call.go#L20) are iterated through to [recursively pull in modules using the same procedure](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/configs/config_build.go#L118)
* The command [constructs a backend](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/command/apply.go#L111) from the configuration
* The command executes the [operation](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/command/apply.go#L119) using the [backend and the configuration](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/command/apply.go#L135)
  - The `configs.Config` module tree is [walked and used to populate a basic graph](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/tofu/transform_config.go#L16-L27)
  - The graph is [transformed and linked based on references detected between nodes](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/tofu/transform_reference.go#L119)
    - Node dependencies are determined by inspecting [blocks](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/tofu/transform_reference.go#L584) and [attributes](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/tofu/node_resource_abstract.go#L159)
      - The blocks and attributes are are [turned into references](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/lang/references.go#L56) in the lang package
  - The graph is [evaluated](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/tofu/graph.go#L86) by [walking each node](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/tofu/graph.go#L43) after it's dependencies have been evaluated.

### Proposed Design Changes

As explained in [Initial implementation](#Initial-implementation), we will need to modify the above workflow to track references/values in different scopes during the config loading process. This will be called a [Static Context](#Static-Context), with the requirements and potential implementation paths below.

When loading a module, a static context must be supplied. When called from an external package like command, the static context will contain tfvars from both cli options and .tfvars files. When called from within the `configs.Config` tree building process, it will pass static references to values from the `config.ModuleCall` in as supplied variables.

Example:

main.tf
```hcl
variable "input_value" {}
locals {
  hash = md5sum(var.input_value)
}
module "mod" {
  source = "./some-source"
  value = local.hash
}
```

pseudocode
```go
buildConfig(path = ".", ctx = StaticContextFromTFVars(command.TFVars))
  config = Config{}

  config.Module = loadModule(path, ctx)
    module = localModuleFiles(".")
    ctx.AddVariables(module.Variables)
    ctx.AddLocals(module.Locals)
    return module

  for call in config.Module.ModuleCalls {
    args = ctx.Evaluate(call.Arguments)
    subCtx := ctx.SubContext(call.Source, args)
    buildConfig(call.Source, subCtx)
  }
```

### Static Context Design
At the heart of the project lies an evaluation context, similar to what currently exist in the tofu and lang package. It must serve a similar purpose, but has some differing requirements.

Any static evaluator must be able to:
* Evaluate a hcl expression or block into a single cty value
  - Provide detailed insight into why a given expression or block can not be turned into a cty value
* Be constructed with variables derived from a parent static context
  - This is primarlly for passing values down the module call stack, while maintaining references

There are three potential paths in implementing a static evaluator:
* Build a custom streamlined (?) solution for this specific problem and it's current use cases
  - This approach was taken in the prototypes
  - Can be flexible during development
  - Does not break other packages
  - Tests must be written from scratch
* Re-use existing components of the tofu and lang packages with new plumbing
  - Can build on top of existing tested logic
  - Somewhat flexible as components can be swapped out as needed
  - May require refactoring existing componets we wish to use
  - May accidentally break other packages due to poor existing testing
* Re-use current evaluator/scope constructs in tofu and lang packages
  - Would require re-designing these components to function in either mode
  - Would come with most of the core logic already implemented
  - High likelyhood of breaking other packages due to poor existing testing
  - Would likely require some ergonomic gymnastics depending on scale of refactoring

This will need to be investigated and roughly prototyped, but all solutions should fit a similar enough interface to not block development of dependent tasks. We should design the interface first, based on the requirements of the initial prototype.


## Solutions
### Module Sources
Module sources must be known at init time as they are downloaded and collated into .terraform/modules. This can be implemented by inspecting the source hcl.Expression using the static evaluator scoped to the current module.

This is relatively straight forward once the core is implemented, but will require some more in-depth changes to support for_each/count later on.

Without module pre-expansion / support for for_each/count, the process would look like:
* Create a SourceExpression field in config.ModuleCall and don't set the "config.ModuleCall.Source" field initially
* Use the static context available during NewModule constructor to evaluate all of the config.ModuleCall source fields and check for bad references and other errors.

### Provider Iteration

In [#300](https://github.com/opentofu/opentofu/issues/300), users describe how supporting for_each for configuring providers will allow much DRYer and simpler configurations.

> [!Note]
> Please read [Provider References](provider-references.md) before diving into this section!

#### Proposed Changes

The first change is to support static variables in the provider config block.  This can then be extended to support for_each/count and be expanded at the end of the `config.NewModule()` function, similar to how [module.ProviderLocalNames](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/configs/module.go#L186) is generated.  This piece is fairly straightforward and can be done relatively easily.

```hcl
locals {
  regions = {"us": "us-east-1", "eu": "eu-west-1"}
}

provider "aws" {
  for_each = local.regions
  alias = each.key
  region = each.value
}

# Uses the AWS US Provider
resource "aws_s3_bucket" "primary" {
  for_each = local.regions
  provider = aws.us
}

# Uses the AWS EU Provider
module "mod" {
  source = "./mod"
  providers {
    aws = aws.eu
  }
}
```


The next step is to support provider aliases indexed by an expression, which is quite a bit trickier.
Example:
```hcl
locals {
  regions = {"us": "us-east-1", "eu": "eu-west-1"}
}


provider "aws" {
  for_each = local.regions
  alias = each.key
  region = each.value
}

resource "aws_s3_bucket" "primary" {
  for_each = local.regions
  provider = provider.aws[each.key]
}

module "mod" {
  source = "./mod"
  providers {
    aws = provider.aws[each.key]
  }
}
```

As you can see, the `provider.name[alias]` form is introduced in that example.  This allows providers named "local" or other conflicting names, and clearly shows that it's referencing a particular instance of a given type.

At this point, we don't have a clear path to implementation, but we can enumerate some of the challenges that are faced:

* Introducing an alternate provider address method and updating documentation
* Provider mappings for modules and resources are hard-coded at config load time on an *unexpanded* view of the config/graph structures
* Provider configurations are [pruned during graph processing](./provider-references.md#Provider-Workflow)

Potential implementation paths are explored in [Static Evaluation of Providers](./static-evaluation-providers.md)

#### Questions

Should variables be allowed in required_providers now or in the future?  Could help with versioning / swapping out for testing?

In #300, there's also a discussion on allowing variable names in provider aliases.
Example:
```
# Why would you want to do this?  It looks like terraform deprecated this some time after 0.11.
provider "aws" {
  alias = var.foo
}
```

### Backend Configuration

Once the core is implemented, this is probably the easiest solution to implement.

Notes from initial prototyping:
* The configs.Backend saves the config body during the config load and does not evaluate it
* backendInitFromConfig() in command/meta_backend.go is what evaluates the body
  - This happens before the graph is constructed / evaluated and can be considered an extension of the config loading stage.
* We can stash a copy of the StaticContext in the configs.Backend and use it in backendInitFromConfig() to provide an evaluation context for decoding into the given backend scheam.
  - There are a few ways to do this, stashing it there was a simple way to get it working in the prototype.
* Don't forget to update the configs.Backend.Hash() function as that's used to detect any changes

### Lifecycle Attributes

Not yet investigated in depth.

### Variable defaults/validation?

Not sure we are doing this one at this juncture. It may have been passed on before due to complexity around providers, or simply that coalesce + locals exists.

### Provisioners

Not yet investigated in depth.

### Moved blocks

Not yet investigated in depth.

## Encryption

Not yet investigated in depth.

## Blockers:
### Testing

Existing testing within OpenTofu is fragmented and more sparse than we would like. Additional test coverage will be needed before, during and after many of the following items.

Code coverage should be inspected before refactoring of a component is undertaken to guide the additional test coverage required. We are not aiming for 100%, but should use it as a tool to understand our current testing.

A comprehensive guide on e2e testing should be written, see #1536.

## Unknowns:
### Providers variables
Providers may have configuration that depends on variables and dynamic values, such as resources from other providers. There is a odd workaround within the internal/tofu package where [variable values may be requested during the graph building phase](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/tofu/context_input.go#L22-L39). This is an odd hack and may [need to be reworked](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/tofu/context_input.go#L46-L48) for the providers iteration above.
### Core functions
Do we want to support the core OpenTofu functions in the static evaluation context? Probably as it would be fairly trivial to hook in.
### Provider functions
Do we want to support provider functions during this static evaluation phase? I suspect not, without a good reason as the development costs may be significant with minimal benefit. It is trivial to detect someone attempting to use a provider function in an expression/body and to mark the expression result as dynamic.
### Module Expansion Disk Copy
As described in #issue, large projects may incur a large cost to build a directory for every remote module.  *If* we are expanding modules in a static context when possible, that implies that we will also be building a directory for every remote module instance.

Potential solutions include:
* Optimizing the copy process - fairly straightforward low hanging fruit
* Only expanding when static expansion is required (hard to detect?)

## Performance:
### Multiple calls to parse config
Due to the partially refactored command package, the configuration in the pwd is loaded, parsed, and evaluated multiple times during many steps. We will be adding more overhead to that action and may wish to focus some effort on easy places to cut out multiple configuration loads. An issue should be created or updated to track the cleanup of the command package.
### Static evaluator overhead
We should keep performance in mind for which solution we choose for the static evaluator above

## Future Work:
### Static Module Outputs
It would be quite useful to pull in a single module which defined sources and versions of dependencies across multiple projects within an organization. This would enable the following example:
```hcl
module "mycompany" {
  source = "git::.../sources"
}

module "capability" {
  source = ${module.mycompany.some_component}
}

module "other_capability" {
  source = ${module.mycompany.other_component}
}
```

All modules referenced by a parent module are downloaded and added to the config graph without any understanding of inter-dependencies. To implement this, we would need to rewrite the config builder to be aware of the state evaluator and increase the complexity of that component.

I am not sure the engineering effort here is warranted, but it should at least be investigated

