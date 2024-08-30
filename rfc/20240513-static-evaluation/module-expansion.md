This is an ancillary document to the [Static Evaluation RFC](20240513-static-evaluation.md) and is not planned on being implemented. It serves to document the reasoning behind why we are deciding to defer implementation of this complex functionality. For now, we have decided to implement a limited version of this that allows provider aliases *only* to be specified via for_each/count.

This document should be used as a reference for anyone considering implementing this in the future. It is not designed as a comprehensive guide, but instead as documenting the previous exploration of this concept during the prototyping phase of the Static Evaluation RFC.

# Static Module Expansion

Modules may be expanded using for_each and count. This poses a problem for the static evaluation step.

For example:
```hcl
# main.tf
module "mod" {
        for_each = {"us" = "first", "eu" = "second"}
        source = "./my-mod-${each.value}"
        name = each.key
}
```

Each instance of "mod" will have a different source. This is a complex situation that must have intense validation, inputs and outputs must be identical between the two modules.

The example is a bit contrived, but is a simpler representation of why it's difficult to have different module sources for different instances down a configuration tree.

If we want to allow this, modules which have static for_each and count expressions must be expanded at the config layer. This must happen before the graph building, transformers, and walking.

This document assumes you have read the [Static Evaluation RFC](20240513-static-evaluation.md) and understand the concepts in there.

## Current structure and paths

Over half of OpenTofu does not understand module/resource instances. They have a simplified view of the world that is called "pre-expansion".

Relevant components for this document:

Pre-expansion:
* Module structure in configs package
* ModuleCalls structure in configs package
* Config tree in configs package
* Module cache file/filetree
* Graph structure and transformers in tofu package (mixed)
* EvaluationContext (mixed)

Post-expansion
* Graph structure and transformers in tofu package (mixed)
* EvaluationContext (mixed)


## Example representations:

Variables and providers have been excluded for this example.

**HCL:**

```hcl
# main.tf
module "test" {
  for_each = {"a": "first", "b": "second" }
  source = "./mod"
  name = each.key
  description = each.value
}
```

```hcl
# mod/mod.tf
variable "name" {}
variable "description" {}
resource "tfcoremock_resource" { string = var.name, other = var.description }
```

**configs.Config**:

```
root = Config {
  Root = root
  Parent = nil
  Module = Module{
    ModuleCalls = {
      "test" = { source = "./mod", for_each = hcl.Expression, ... }
    }
  }
  Path = addrs.Module[]
  Children = { "test" = test }
}
test = {
  Root = root
  Parent = root
  Module = { ... }
  Path = addrs.Module["test"]
  Children = {}
}
```

**tofu.Graph (simplified)**


Before Expansion:
```
rootExpand = NodeExpandModule {
  Addr = addrs.Module[]
  Config = root
  ModuleCall = nil
}
testExpand = NodeExpandModule {
  Addr = addrs.Module["test"]
  Config = test
  ModuleCall = root.Module.ModuleCalls["test"]
}
testExpandResource = NodeExpandResource {
  NodeResource {
    Addr = addrs.Module["test", "resource"]
    Config = test.Module.Resources["resource"]
  }
}

testExpand -> rootExpand
testExpandResource -> testExpand
```

With Expansion:
```
testExpandResourceA = NodeResourceInstance {
  NodeResource = testExpandResource.NodeResource
  Addr = addrs.ModuleInstance[{"test", Key{"a"}, {"resource", NoKey}]
}
testExpandResourceB = NodeResourceInstance {
  NodeResource = testExpandResource.NodeResource
  Addr = addrs.ModuleInstance[{"test", Key{"b"}, {"resource", NoKey}]
}
```


## Proposed structure and paths

To implement a fully fledged static evaluator which supports for_each and count on modules/providers, the concept of module instances must be brought to all components in the previous section.

One approach is to remove the concept of a "non-instanced" module path and simply deleted addrs.Module entirely and changed all references to addrs.ModuleInstance (among a number of other changes). This is a incredibly complex change with many ramifications.

addrs.Module is simply a []string, while addrs.ModuleInstance is a pair of {string, key} where key is:
* nil/NoKey representing no instances
* CountKey for int count
* ForEachKey for string for_each

## Example representations for Module -> ModuleInstance:
**HCL (identical):**

```hcl
# main.tf
module "test" {
  for_each = {"a": "first", "b": "second" }
  source = "./mod"
  key = each.key
  value = each.value
}
```

```hcl
# mod/mod.tf
variable "key" {}
variable "value" {}
resource "tfcoremock_resource" { string = var.key, other = var.value }
```

**configs.Config**

Changes:
* All addresses are instanced.
* ModuleCalls is expanded into ExpandedModuleCalls using the static evaluator
* The root Children map points to distinct instances of `test["a"]` and `test["b"]`

```
root = Config {
  Root = root
  Parent = nil
  Module = Module{
    ModuleCalls = {
      "test" = { source = "./mod", for_each = hcl.Expression, ... }
    }
    ExpandedModuleCalls = {
      {"test", Key{"a"}} = { source = "./mod", for_each = nil, ... }
      {"test", Key{"b"}} = { source = "./mod", for_each = nil, ... }
    }
  }
  Path = addrs.ModuleInstance[]
  Children = { "test" = { "a": testA, "b": testB } }
}
testA = {
  Root = root
  Parent = root
  Module = { ... }
  Path = addrs.ModuleInstance[{"test", "a"}]
  Children = {}
}
testB = {
  Root = root
  Parent = root
  Module = { ... }
  Path = addrs.ModuleInstance[{"test", "a"}]
  Children = {}
}
```
**tofu.Graph (simplified)**

Changes:
* All addresses are instanced.
* Pre-expanded modules are present in the graph and linked to single instances post-expansion.

Before Expansion:
```
rootExpand = NodeExpandModule {
  Addr = addrs.ModuleInstance[]
  Config = root
  ModuleCall = nil
}
testExpandA = NodeExpandModule {
  Addr = addrs.ModuleInstance[{"test", Key{"a"}}]
  Config = testA
  ModuleCall = root.Module.ExpandedModuleCalls["test"]["a"]
}
testExpandB = NodeExpandModule {
  Addr = addrs.ModuleInstance[{"test", Key{"b"}}]
  Config = testB
  ModuleCall = root.Module.ExpandedModuleCalls["test"]["b"]
}
testExpandResourceA = NodeExpandResource {
  NodeResource {
    Addr = addrs.ModuleInstance[{"test", Key{"a"}}, {"resource", NoKey}]
    Config = testA.Module.Resources["resource"]
  }
}
testExpandResourceB = NodeExpandResource {
  NodeResource {
    Addr = addrs.ModuleInstance[{"test", Key{"b"}}, {"resource", NoKey}]
    Config = testB.Module.Resources["resource"]
  }
}

testExpandA -> rootExpand
testExpandB -> rootExpand
testExpandResourceA -> testExpandA
testExpandResourceB -> testExpandB
```

With Expansion:
```
testExpandResourceA = NodeResourceInstance {
  NodeResource = testExpandResourceA.NodeResource
  Addr = addrs.ModuleInstance[{"test", Key{"a"}, {"resource", NoKey}]
}
testExpandResourceB = NodeResourceInstance {
  NodeResource = testExpandResourceB.NodeResource
  Addr = addrs.ModuleInstance[{"test", Key{"b"}, {"resource", NoKey}]
}
```


