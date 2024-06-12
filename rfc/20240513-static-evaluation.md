# Init-time static evaluation of constant variables and locals

Issue: https://github.com/OpenTofu/OpenTofu/issues/1042

As initially described in https://github.com/opentofu/opentofu/issues/1042, many users of OpenTofu expect to be able to use variables and locals in a variety of locations that are currently not supported. Common examples include but are not limited to: module sources, provider assignments, backend configuration, encryption configuration.

All of these examples are pieces of information that need to be known during `tofu init` and can not be integrated into the usual `tofu plan/apply` system. Init sets up a static understanding of the project's configuration that plan/apply can later operate on.

## Proposed Solution

In simple terms, the proposal is to enhance the configuration processing in OpenTofu to support evaluation of variable and local references that do not depend on any dynamic information (resources/data/providers/etc...).

### User Documentation

This proposal in and of itself does not explicitly add support for new user facing functionality in OpenTofu. It is designed to be a support system for the examples above.

However, this "support system" will have to interact with the user directly. To properly define this support system, we will look at specific examples that will be built on top of this work.

Note: All errors/warnings are simplified placeholders and would include better wording/formatting as well as source locations.

#### Configurable Backend Examples

Let's first look at a project's backend that depends on both variables and locals:

```hcl
variable "key" {
    type = string
}

locals {
    region = "us-east-1"
    key_check = md5sum(var.key)
}

terraform {
  backend "somebackend" {
    region = local.region
    key = var.key
    key_check = local.key_check
  }
}
```

When first running `tofu init`, two errors will be produced:
1. terraform.backend.key requires variable "key" to be provided
2. terraform.backend.key_check requires local "key_check", which requires variable "key" to be provided

The variable "key" can either be provided with via a `terraform.tfvars` file or a cli flag `-var "key=somevalue"`. This implies that the `-var` cli flag will need to be added to most OpenTofu commands.

> [!NOTE]
> Instead of producing an error in this scenario, we could instead ask the user to provide values for the required variables. This already occurs as part of the provider configuration process. We could unify that process as well to reduce code duplication and [odd workarounds](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/tofu/context_input.go#L22-L48).

Let's now consider what happens when `tofu apply` is run. If `terraform.tfvars` or `-var "key="` have changed, the backend configuration will no longer match the configuration during `tofu init` and will return an error to the user. This will require users to be considerate of what vars they are allowing in backends and how they are managed as a team. That said, there are clear guide-rails already in place for most scenarios where configuration does not match expectation.

As shown by users looking to use values in backends today, someone will eventually try to use a "dynamic" value (resource/data) in a backend configuration:

```hcl
resource "mycloud_account" {
}

locals {
    account_id = mycloud.account.id
}

terraform {
  backend "somebackend" {
    account_id = local.account_id
  }
}
```

In this scenario, the following error message will be produced:
* terraform.backend.account_id is unable to be resolved
  - The field "account_id" depends on local "account_id", which depends on resource "mycloud.account" which is not allowed here.

These examples are designed to convey the importance of reference tracking when explaining to users clearly why something they are attempting to do is not allowed.

#### Module Sources Example

Modules are a complex concept that static evaluation will need to interact with. We will need to define how users interact with them and how to convey errors and limitations encountered.

First, let's look at a simple example that does not involve any child modules:

```hcl
# main.tf

variable "version" {
    type = string
}

module "helper" {
    source = "git@github.com:org/my-utils?ref=${var.version}"
}
```

This will be subject to the same type of error messages defined above in the backend examples.

Next, let's look at a more complex example with child module sources:
```hcl
# main.tf
variable "version" {
    type = string
}

module "common_first" {
    source = "./common"
    version = var.version
    region = "us-east-1"
}
module "common_second" {
    source = "./common"
    version = var.version
    region = "us-east-2"
}
```

```hcl
# ./common/main.tf
variable "version" {
    type = string
}
variable "region" {
    type = string
}

module "helper" {
    source = "git@github.com:org/my-utils?ref=${var.version}"
    region = var.region
}
```

This now adds an additional dimension to reference errors. Without the variable "version" supplied, the following errors will occur:
* Module common_first.helper's source can not be determined
  - common_first.helper.source depends on variable common_first.version, which depends on variable "version" which has not been supplied
* Module common_second.helper's source can not be determined
  - common_second.helper.source depends on variable common_second.version, which depends on variable "version" which has not been supplied

Once a value for "version" is provided, everything should work as intended.

This configuration can be cleaned up a bit further using `for_each`:
```hcl
# main.tf
variable "version" {
    type = string
}

module "common" {
    for_each = {first = "us-east-1", second = "us-east-2"}
    source = "./common"
    version = var.version
    region = each.value
}
```

This would produce a single error instead of multiple:
* Module common.helper's source can not be determined
  - common.helper.source depends on variable "common.version", which depends on variable "version" which has not been supplied

Notice that the for_each does not interact with the static reference system at all.

This begs the question, what if a user tries to use each.value in a field that must be statically known?

```hcl
# main.tf
variable "version" {
    type = string
}

module "common" {
    for_each = {first = "us-east-1", second = "us-east-2"}
    source = "./common"
    version = "${var.version}_${each.key}"
    region = each.value
}
```

This will produce the following error, regardless of the value of the variable "version":
* Module common.helper's source can not be determined
  - common.helper.source depends on variable "common.version", which depends on a for_each key and is forbidden here!

There are some significant technical roadblocks to supporting `for_each`/`count` in static expressions. For the purposes of this RFC, we are forbidding it. For more information, see [Static Module Expansion](20240513-static-evaluation/module-expansion.md).

### Technical Approach

#### Background

Although mostly limited in scope to one or two packages in OpenTofu, it is important to understand what complex systems it will be resembling and interacting with.

> [!NOTE]
> It is HIGHLY recommended to read the [Architecture document](../docs/architecture.md) before diving too deep into this document. Below, many of the concepts in the Architecture doc are expanded upon or viewed from a different angle for the purposes of understanding this proposal.

##### Expressions

The evaluation of expressions (`1 + var.bar` for example) depends on referenced values and functions used in the expression. In that example, you would need to know the value of `var.bar`. That dependency is known via a concept called "[HCL Traversals](https://pkg.go.dev/github.com/hashicorp/hcl/v2#Traversal)", which represent an attribute access path and can be turned into strongly typed "OpenTofu References". In practice, you would say "the expression depends on an OpenTofu Variable named bar".

Once you know what the requirements are for an expression ([hcl.Expression](https://pkg.go.dev/github.com/hashicorp/hcl/v2#Expression)), you can build up an evaluation context ([hcl.EvalContext](https://pkg.go.dev/github.com/hashicorp/hcl/v2#EvalContext)) to provide those requirements or return an error. In the above example, the evaluation context must include `{"var": {"bar": <somevalue>}`.

Expression evaluation is currently split up into two stages: config loading and graph reference evaluation.

##### Config Loading

During configuration loading, the HCL or JSON config is pulled apart into Blocks and Attributes by the hcl package. A Block can contain Attributes and nested Blocks. Attributes are simply named expressions (`foo = 1 + var.bar` for example).

```hcl
some_block {
    some_attribute = "some value"
}
```

These Blocks and Attributes are abstract representations of the configuration which have not yet been evaluated into actionable values. When processing a block or attribute, a decision is made to either evaluate it immediately if required or to keep the abstract block/attribute for later processing. If it is kept in the abstract representation, it will later be turned into a value by [Graph Reference Evaluation](#graph-reference-evaluation).

As a concrete example, the `module -> source` field must be known during configuration loading as it is required to continue the next iteration of the loading process. However, attributes like `module -> for_each` may depend on attribute values from resources or other pieces of information not known during config loading and are therefore stored as an expression for the [Graph Reference Evaluation](#graph-reference-evaluation).

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

No evaluation context is built or provided during the entire config loading process. **Therefore, no functions, locals, or variables may be used during config loading due to the lack of an evaluation context. This limitation is what we wish to resolve.**

##### Graph Reference Evaluation

After the config is fully loaded, it is [transformed and processed](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/docs/architecture.md#graph-builder) into nodes in a [graph (DAG)](https://en.wikipedia.org/wiki/Directed_acyclic_graph). These nodes use the "[OpenTofu References](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/addrs/parse_ref.go#L174)" present in their blocks/attributes (the ones not evaluated in config loading) to build both the dependency edges in the graph, and [eventually an evaluation context](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/docs/architecture.md#expression-evaluation) once those references are available.

This theoretically simple process is deeply complicated by the module dependency tree and [expansion therein](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/docs/architecture.md#sub-graphs). A sub graph is dynamically created due to `for_each` and `count` being evaluated as their required references are made available. The majority of the logic in this process exists within the `tofu` and `lang` packages, which are tightly coupled.

For example, a module's `for_each` statement may require data from a resource: `for_each = resource.aws_s3_bucket.foo.tags`. Before it can be evaluated, the module must wait for "OpenTofu Resource Reference `aws_s3_bucket.foo`" to be available. This would be represented as a dependency edge between the module node and the specific resource node. The evaluation context would then include `{"resource": {"aws_s3_bucket": {"foo": {"tags": <provided value>}}}}`.

> [!NOTE]
> A common misconception is that modules are "objects". However, modules more closely resemble "namespaces" and can cross-reference each other's vars/outputs as long as there is no reference loop.



##### Background Summary

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

By utilizing Traversals/References, we can track what values are statically known throughout the config loading process. This will follow a similar pattern to the graph reference evaluation, with limitations in what can be resolved. It may or may not re-use much of the existing graph reference evaluators code, limited by complex dependency tracing required for error handling.

When evaluating an Attribute/Block into a value, any missing reference must be properly reported in a way that the user can easily debug and understand. For example, a user may try to use a `local` that depends on a resource's value in a module's source. The user must then be told that the `local` can not be used in the module source field as it depends on a resource which is not available during the config loading process. Variables used through the module tree must also be passed with their associated information, such as their references.

### Development Approach

Given the scope of what needs to be changed to build this foundation for static evaluation, we are talking about a significant amount of work, potentially spread across multiple releases.

We can not take the approach of hacking on a feature branch for months or freezing all related code. It's unrealistic and unfair to other developers.

Instead, we can break this work into smaller discrete and testable components, some of which may be easy to work on in parallel.

With this piece by piece approach, we can also add testing before, during, and after each component is added/modified.

The OpenTofu core team should be the ones to do the majority of the core implementation. If community members are interested, much of the issues blocked by the static evaluation are isolated and well defined enough for them to be worked on independently of the core team.

### Current Load/Evaluation Flow

Much of this process is covered by the Architecture document linked above and should be used as reference throughout this section.

Performing an action in OpenTofu (init/plan/apply/etc...) takes the following steps (simplified):

A [command](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/command/init.go#L193) in the command package is created based on the CLI arguments and is executed. It performs the following actions:

#### Parse and Load the configuration modules

Starting at the root module (current directory), a `config.Config` structure is created. This structure is the root node of a tree representing all of the module calls (`module {}`) that make up the project. Each node in the tree contains a `config.Module` and a `addrs.Module` path.

The tree is built by: installing a module's source, loading the module, inspecting the module calls, recursing in a depth first pattern.
```go
// Pseudocode
// https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/configs/config_build.go#L27
func buildConfig(source string) configs.Config {
    c := configs.Config{}

    // https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/initwd/module_install.go#L147
    path = installModule(source)
    c.module = loadModule(path) // https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/configs/parser_config_dir.go#L41-L58

    // https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/configs/config_build.go#L132
    for name, call := range c.module.calls {
        c.children[name] = buildConfig(call.source)
    }
    return c
}

root = buildConfig(".")
```


The [configs.Module](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/configs/module.go#L22-L63) structure is a representation of the module that is a mixture of fields that are computed during the config process and fields who's evaluation is deferred until later. For example: the `module -> source` must be known for config loading to complete, but a resource's config body can be deferred until during graph node evaluation.

Module loading takes a directory, turns each hcl/json file into a [configs.File](https://github.com/opentofu/opentofu/blob/290fbd6/internal/configs/module.go#L76) structure, merges them together, and returns a `configs.Module`.

```go
// Pseudocode
// https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/configs/parser_config_dir.go#L41-L58
// https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/configs/module.go#L122
func loadModule(path string) configs.Module {
    var files = []file.File
    for filepath in range(list_files(path)) {
        files = append(files, loadFile(filepath))
    }
    module := configs.Module{}
    for _, file in range(files) {
        module.appendFile(file) // https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/configs/module.go#L205
    }
    return module
}

// https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/configs/parser_config.go#L54
func loadFile(filepath string) configs.File {
    file := configs.File{}

    hclBody = hcl.parse(filepath)
    for _, hclBlock in range(hclBody) {
        switch (hclBlock.Type) {
            case "module":
                file.ModuleCalls = append(file.ModuleCalls, decodeModuleCall(hclBlock))
            case "variable":
                file.Variables = append(file.Variables, decodeVariable(hclBlock))
            // omitted cases pattern for all remaining supported blocks
        }
    }

    return file
}
```

#### Backend is loaded

The command [constructs a backend](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/command/apply.go#L111) from the configuration

The backend is what interfaces with the state storage and is in charge of actually executing and managing state operations (plan/apply)

#### Operation is executed

The command executes the [operation](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/command/apply.go#L119) using the [backend and the configuration](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/command/apply.go#L135).


A graph is [built from the loaded configuration](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/tofu/transform_config.go#L16-L27) and is transformed such that it can be walked.

This transformation is a complex process, but the some of the key pieces are:
* [Transformation and linking based on references detected between nodes](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/tofu/transform_reference.go#L119)
     - Node dependencies are determined by inspecting [blocks](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/tofu/transform_reference.go#L584) and [attributes](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/tofu/node_resource_abstract.go#L159)
       - The blocks and attributes are [turned into references](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/lang/references.go#L56) in the `lang` package
* Resources nodes are linked to their required provider nodes
* Module variables are mapped from parent to child via their module calls

The graph is then [evaluated](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/tofu/graph.go#L86) by [walking each node](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/tofu/graph.go#L43) after it's dependencies have been evaluated.

When evaluating a node in the graph, the [tofu.EvalContext](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/tofu/eval_context.go#L117-L125) (implemented by [tofu.BuiltinEvalContext](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/tofu/eval_context_builtin.go#L271-L284)) is used to [build and utilize](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/tofu/eval_context_builtin.go#L504) a `lang.Scope` based on the references that the node specifies, all of which should have already been evaluated due to the dependency structure represented by the transformed graph.

The `lang.Scope` handles the specific details of [taking OpenTofu references and building](https://github.com/opentofu/opentofu/blob/290fbd66d3f95d3fa413534c4d5e14ef7d95ea2e/internal/lang/eval.go#L295) a `hcl.EvalContext` from the values and functions currently available to the EvaluationContext/State.

### Proposed Changes

We will need to modify the above design to track references/values in different scopes during the config loading process. This will be called a Static Evaluation Context, with the requirements and potential implementation paths below.

When loading a module, a static context must be supplied. When called from an external package like `command`, the static context will contain tfvars from both cli options and .tfvars files. When called from within the `configs.Config` tree building process, it will pass static references to values from the `config.ModuleCall` in as supplied variables. In either case, builtin OpenTofu commands are available.

```go
// Pseudocode
func buildConfig(source string, ctx StaticContext) configs.Config {
    c := configs.Config{}

    path = installModule(source)
    c.module = loadModule(path, ctx)

    for name, call := range c.module.calls {
        // Should have the required information at this point to evaluate the child's source field
        source := ctx.Evaluate(call.source)

        // Build a new StaticContext based on the call's configuration attributes.
        childCtx = ctx.FromModuleCall(call.Name, call.Config)

        c.children[name] = buildConfig(source, childCtx)
    }
    return c
}

func loadModule(path string, ctx StaticContext) configs.Module {
    var files = []file.File
    for filepath in range(list_files(path)) {
        files = append(files, loadFile(filepath))
    }
    module := configs.Module{}
    for _, file in range(files) {
        module.appendFile(file)
    }

    // Link in current variables and locals
    ctx.AddVariables(module.Variables)
    ctx.AddLocals(module.Locals)

    // Additional processing of module's fields can be done here using the StaticContext

    return module
}


root = buildConfig(".", StaticContextFromTFVars(command.TFVars))
```

#### Static Context Design
At the heart of the project lies an evaluation context, similar to what currently exist in the `tofu` and `lang` packages. It must serve a similar purpose, but has some differing requirements.

Any static evaluator must be able to:
* Evaluate a hcl expression or block into a single cty value
  - Provide detailed insight into why a given expression or block can not be turned into a cty value
* Be constructed with variables derived from a parent static context corresponding to parent modules
  - This is primarily for passing values down the module call stack, while maintaining references
* Understand current locals and their dependencies

There are three potential paths in implementing a static evaluator:
* Build a custom solution for this specific problem and it's current use cases
  - This will not be shared with the existing evaluation context
  - This approach was taken in the prototypes
  - Can be flexible during development
  - Does not break other packages
  - Tests must be written from scratch
* Re-use existing components of the tofu and lang packages with new plumbing
  - Can build on top of existing tested logic
  - Somewhat flexible as components can be swapped out as needed
  - May require refactoring existing components we wish to use
  - May accidentally break other packages due to poor existing testing
* Re-use current evaluator/scope constructs in tofu and lang packages
  - Would require re-designing these components to function in either mode
  - Would come with most of the core logic already implemented
  - High likelihood of breaking other packages due to poor existing testing
  - Would likely require some ergonomic gymnastics depending on scale of refactoring

This will need to be investigated and roughly prototyped, but all solutions should fit a similar enough interface to not block development of dependent tasks. We should design the interface first, based on the requirements of the initial prototype. Alternatively this could be a more iterative approach where the interface is designed at the same time as being implemented by multiple team members.

We are deferring this decision to the actual implementation of this work. It is a deeply technical investigation and discussion that does not significantly impact the proposed solution in this RFC.

#### Overview of dependent issues

To better understand the exact solution we are trying to solve, a limited overview of problems that could be solved using the static evaluation context are provided.

##### Backend Configuration

Once the core is implemented, this is probably the easiest solution to implement.

Notes from initial prototyping:
* The configs.Backend saves the config body during the config load and does not evaluate it
* backendInitFromConfig() in command/meta_backend.go is what evaluates the body
  - This happens before the graph is constructed / evaluated and can be considered an extension of the config loading stage.
* We can stash a copy of the StaticContext in the configs.Backend and use it in backendInitFromConfig() to provide an evaluation context for decoding into the given backend schema.
  - There are a few ways to do this, stashing it there was a simple way to get it working in the prototype.
* Don't forget to update the configs.Backend.Hash() function as that's used to detect any changes

##### Module Sources

Module sources must be known at init time as they are downloaded and collated into .terraform/modules. This can be implemented by inspecting the source hcl.Expression using the static evaluator scoped to the current module.

Notes from initial prototyping:
* Create a SourceExpression field in config.ModuleCall and don't set the "config.ModuleCall.Source" field initially
* Use the static context available during NewModule constructor to evaluate all of the config.ModuleCall source fields and check for bad references and other errors.

Many of the other blocked issues follow an extremely similar pattern of "store the expression in the first part of config loading and evaluate when needed" and are therefore omitted.

##### Encryption

Encryption attempts to be a standalone package that tries hard to limit dependence on OpenTofu code, potentially allowing it to be used independently at some point.

It uses the hcl libraries directly and does not follow the same patterns as the rest of OpenTofu codebase. This may have a significant impact on the design of the Static Evaluation Context interface.

##### Provider Iteration

This will be described in the [Provider Evaluation RFC](20240513-static-evaluation-providers.md) due to expansion complexity.

#### Blockers

Existing testing within OpenTofu is fragmented and more sparse than we would like. Additional test coverage will be needed before, during and after each stage of implementation.

Code coverage should be inspected before refactoring of a component is undertaken to guide the additional test coverage required. We are not aiming for 100%, but should use it as a tool to understand our current testing.

A comprehensive guide on e2e testing should be written, see https://github.com/opentofu/opentofu/issues/1536.

#### Performance Considerations

##### Multiple calls to parse config
Due to the partially refactored command package, the configuration in the pwd is loaded, parsed, and evaluated multiple times during many steps. We will be adding more overhead to that action and may wish to focus some effort on easy places to cut out multiple configuration loads. An issue should be created or updated to track the cleanup of the command package.
##### Static evaluator overhead
We should keep performance in mind for which solution we choose for the static evaluator above

### Open Questions

Do we want to support asking for variable values when required but not provided? This is already an established pattern, but may require additional work. It may be prudent to defer this until a later iteration. See above note on provider configuration in the first user error example.

Do we want to support the core OpenTofu functions in the static evaluation context? Probably as it would be fairly trivial to hook in.

Do we want to support provider functions during this static evaluation phase? I suspect not, without a good reason as the development costs may be significant with minimal benefit. It is trivial to detect someone attempting to use a provider function in an expression/body and to mark the expression result as dynamic.

### Future Considerations

[Static Module Expansion](20240513-static-evaluation/module-expansion.md) is currently forbidden due to the significant architectural changes required. The linked document serves as an exploration into what that architectural change could look like if the need arises.

#### Static Module Outputs
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

## Potential Alternatives

Tools like terragrunt offer an abstraction layer on top of OpenTofu, which many users find beneficial. Building some of these features into OpenTofu means that out of the box you do not need additional tooling. Additionally, terragrunt can focus more on more complex problems that occur when orchestrating complex infrastructure among multiple OpenTofu projects instead of patching around OpenTofu limitations.

A distinct pre-processor is another option, but that would require a completely distinct language to be designed and implemented to pre-process the configuration. Additionally, it would not integrate easily with any existing OpenTofu constructs.
