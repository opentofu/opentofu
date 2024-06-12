This is an ancillary document to the [Static Evaluation RFC](20240513-static-evaluation.md) and is not planned on being implemented. It serves to document the reasoning behind why we are deciding to defer implementation of this complex functionality.

# Static Module Expansion

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

TODO Are there any alternatives?

## Current structure and paths

TODO expound on what the below concepts are and link to the above docs.

As specified above, the configs.Config struct is a tree linking config.Modules. The nodes in that tree are referenced via addr.Module paths (non-instanced). The whole process of turning Modules and ModuleCalls into a config tree uses those non-instanced paths.

The configs.Config tree is then walked and added into a tofu.Graph. The nodes in this graph have two different addresses: addr.Module and addr.ModuleInstance. The addr.Module paths of the graph nodes are used to look up the corresponding config structures and other operations on the "unexpanded" view of the world. The addrs.ModuleInstance paths are built by the module expansion process and are used when operating on the "expanded" view of the world.

## Example represenations:

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

TODO Given the above HCL ... (this is how it is transformed/loaded)

```
root = {
  Root = root
  Parent = nil
  Module = { ModuleCalls = { "test" = { source = "./mod", for_each = hcl.Expression, ... } }, ... }
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

Variables and providers have been excluded for the moment.

Before Expansion:
```
rootExpand = NodeExpandModule {
  Addr = addrs.Module[]
  Config = root
  ModuleCall = nil?
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

**Expander structure**

The expander is part of the evaluation context and is a tree that mirrors the configs.Config tree. It is built differently during validate vs plan/apply (validate does not expand).

TODO detailed explanation

## Proposed structure and paths

To implement a fully fledged static evaluator which supports for_each and count on modules/providers, the concept of module instances must be brought to all components in the previous section.

In the prototype, I removed the concept of a "non-instanced" module path and simply deleted addrs.Module entirely and changed all references to addrs.ModuleInstance (among a number of other changes). This worked for the prototype, but the ramifications must be fully considered before implementation starts in earnest.

addrs.Module is simply a []string, while addrs.ModuleInstance is a pair of {string, key} where key is:
* nil/NoKey representing no instances
* CountKey for int count
* ForEachKey for string for_each

## Approaches:
### Replace addrs.Module with addrs.ModuleInstance directly

This approach may be the simplest, but could cause some confusion when inspecting paths. In practice nil would represent both NoKey and NotYetExpanded. In the rough prototype this was not an immediate problem, but could easily be a tripping hazard.

Before:
`addrs.Module["test", "resource"]`

After:
`addrs.ModuleInstance[{"test", nil}, {"resource", NoKey}`

Indistinguishable from:
`addrs.ModuleInstance[{"test", NoKey}, {"resource", NoKey}`

### Replace addrs.Module with addrs.ModuleInstance with additional keys types

DeferredCount and DeferredForEach could be introduced as a placeholder for expansion that is yet to happen. This would clearly differentiate between NoKey and not yet expanded.

Before:
`addrs.Module["test", "resource"]`

After:
`addrs.ModuleInstance[{"test", DeferredForEach}, {"resource", NoKey}`

Clearly distinguishable from:
`addrs.ModuleInstance[{"test", NoKey}, {"resource", NoKey}`

### Modify addrs.Module to be similar to addrs.ModuleInstance with limited keys

Alternatively, addrs.Module could be kept distinct from addrs.ModuleInstance, but follow a nearly identical structure.

TODO pros/cons...

## Example represenations for Module -> ModuleInstance:
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

TODO explain HOW this is different than before

```
root = {
  Root = root
  Parent = nil
  Module = {
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

Variables and providers have been excluded for the moment.

TODO explain HOW this is different than before

Before Expansion:
```
rootExpand = NodeExpandModule {
  Addr = addrs.ModuleInstance[]
  Config = root
  ModuleCall = nil?
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

**Expander structure**

The expander is part of the evaluation context and is a tree that mirrors the configs.Config tree. It is built differently during validate vs plan/apply (validate does not expand).

TODO detailed explanation

