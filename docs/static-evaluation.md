# Implementing Init-time static evaluation of constant variables and locals

This is based on the prototyping done while evaluating RFC #1042 and references work done in [branch] and [branch]

## Progress Overview:
- [ ] Core Implementation
- [ ] Module Iteration
- [ ] Solutions
  - [ ] Module Sources
  - [ ] Module Provider Mappings
  - [ ] Provider Iteration
  - [ ] Backend Configuration
  - [ ] Lifecycle Attributes
  - [ ] Variable defaults/validation?
  - [ ] Provisioners
  - [ ] Moved blocks

## Blockers:
### Testing

Existing testing within OpenTofu is fragmented and more sparse than we would like. Additional test coverage will be needed before, during and after many of the following items.

Code coverage should be inspected before refactoring of a component is undertaken to guide the additional test coverage required. We are not aiming for 100%, but should use it as a tool to understand our current testing.

A comprehensive guide on e2e testing should be written, see #1536.

## Core Implementation:

## Overview of original process and structures

Performing an action in OpenTofu (init/plan/apply/etc...) takes the following steps (simplifed):
* A command in the command package parses the configuration in the current directory
  - The module's configuration is loaded into configs.ModuleFile structures
    - hcl fields like module.source and backend.configuration are evaluated without any eval context (no vars, funcs)
    - config items are validated (which should not be done here, see #1467)
  - configs.ModuleFile structures are merged into configs.Module using various rules
  - configs.Module is used to build config.Config which represents the module and it's location within the module config tree
  - configs.Module.ModuleCalls are iterated through to recursively pull in modules using the same procedure.
* The command constructs a backend from the configuration
* The command excutes the operation using the backend and the configuration
  - The configs.Config module tree is walked and used to build a basic graph
  - The graph is transformed and linked based on references detected between nodes
  - The graph is evaluated by walking each node after it's dependencies have been evaluated.

## Core changes

### Config processing

The config loading process for a given module above will need to be broken into two stages:
* Parse and load the configuration into configs.ModuleFiles without doing any evaluation
* Setup a static evalation context based on the current configs.ModuleFiles

Additionally, variables passed in from the given module's parent will need to be tracked and known if they are static or dynamic.

### Static Evaluation
At the heart of this project lies a simplified evaluator and evaluation scope, similar to what currently exist in the tofu and lang package.

Any static evaluator must be able to:
* Evaluate a hcl expression or block into a single cty value
* Provide detailed insight into why a given expression or block can not be turned into a cty value
* Be scoped to a given context
* Be easily cloned to support for_each/count iterations

There are three potential paths in implementing a static evaluator:
* Build a custom streamlined solution for this specific problem and it's current use cases
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

## Static Module Iteration

Modules may be expanded using for_each and count. This poses a problem for the static evaluation step.

For example:
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

Each "instance" of "mod" will need it's own provider configuration, which is currently only specified as part of "mod" and not any "mod instance". This problem becomes more complex and problematic when considering nested modules.

To solve this, modules which have static for_each and count expressions must be expanded at the config layer. This must happen before the graph building, transformers, and walking.

### Current structure and paths

As specified above, the configs.Config struct is a tree linking config.Modules.  The nodes in that tree are referenced via addr.Module paths (non-instanced).  The whole process of turning Modules and ModuleCalls into a config tree uses those non-instanced paths.

The configs.Config tree is then walked and added into a tofu.Graph. The nodes in this graph have two different addresses: addr.Module and addr.ModuleInstance.  The addr.Module paths of the graph nodes are used to look up the corresponding config structures and other operations on the "unexpanded" view of the world.  The addrs.ModuleInstance paths are built by the module expansion process and are used when operating on the "expanded" view of the world.

### Example represenations:
#### HCL:
```hcl
# main.tf
module "test" {
  for_each = {"a": "first", "b": "second" }
  source = "./mod"
  key = each.key
  value = each.value
}
# mod/mod.tf
variable "key" {}
variable "value" {}
resource "tfcoremock_resource" { string = var.key, other = var.value }
#### configs.Config
root = {
  Root = root
  Parent = nil
  Module = { ModuleCalls = { "test" = { source = "./mod", for_each = hcl.Expression, ... } }, ... }
  Path = addrs.Module[]
  Children = { "test" =  {
    Root = root
    Parent = root
    Module = { ... }
    Path = addrs.Module["test"]
  }}
}
#### tofu.Graph (simplified)
rootExpand = NodeExpandModule {
  Addr = addrs.Module[]
  Config = root
  ModuleCall = nil?
}
testExpand = NodeExpandModule {
  Addr = addrs.Module["test"]
  
}

## Solutions
### Module Sources
### Module Provider Mappings
### Provider Iteration
### Backend Configuration
### Lifecycle Attributes
### Variable defaults/validation?
### Provisioners
### Moved blocks

## Unknowns:
### Providers variables
Providers may have configuration that depends on variables and dynamic values, such as resources from other providers. There is a odd workaround within the internal/tofu package where variable values may be requested during the graph building phase. This is an odd hack and may need to be reworked for the providers iteration above.
### Core functions
Do we want to support the core OpenTofu functions in the static evaluation context? Probably as it would be fairly trivial to hook in.
### Provider functions
Do we want to support provider functions during this static evaluation phase? I suspect not, without a good reason as the development costs may be significant with minimal benefit. It is trivial to detect someone attempting to use a provider function in an expression/body and to mark the expression result as dynamic.
### Module Expansion Disk Copy
As described in #issue, large projects may incur a large cost to build a directory for every remote module.  If we are expanding modules in a static context when possible, that implies that we will also be building a directory for every remote module instance.

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

All modules referenced by a parent module are downloaded and added to the config graph without any understanding of inter dependencies. To implement this, we would need to rewrite the config builder to be aware of the state evaluator and increase the complexity of that component.

I am not sure the engineering effort here is warranted, but it should at least be investigated
