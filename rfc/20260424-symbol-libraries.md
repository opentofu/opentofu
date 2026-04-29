# Symbol Libraries

Issue: https://github.com/opentofu/opentofu/issues/1962

Most engineers using OpenTofu strive to write configuration that is as DRY as possible. The current design of OpenTofu's language has some hard edges that prevent easy re-use of types, logic, and values.

### Types

One common problem is the inability to re-use custom types between constructs (currently just variables), both in the same module and between modules. This makes working with complex types harder, as well as encouraging copy-paste anywhere types are re-used.

The only way to work around this limitation is with code-gen on-top of your OpenTofu setup.

References:
* https://github.com/opentofu/opentofu/issues/1962
* https://github.com/opentofu/opentofu/issues/2704
* https://github.com/hashicorp/terraform/issues/27365

### Logic

Another common problem is the inability to define and re-use logic between expressions, both within the same module and between modules.

The current way to accomplish this is to create a module with only variables, locals, and outputs. This requires a whole new module to be pulled into your configuration for each call to the "module function" and is a lot of heavy lifting for what should be a "simple" feature.

References:
* https://github.com/opentofu/opentofu/issues/4050
* https://github.com/opentofu/opentofu/issues/793
* https://github.com/hashicorp/terraform/issues/27696

### Values

If you want to re-use constant values between modules, you either need to use code-gen, an orchestration tool, or create a module with only outputs. All of these are valid techniques, though it could be useful to have a native solution with less overhead.

Of the three concepts presented in this RFC, this one is the least strong. It could also be implemented using the functions construct above and merely serves as syntactic sugar.

## Proposed Solution

We propose adding symbol libraries to enable easy and well defined re-use of types, logic, and values in OpenTofu. The ideas in this RFC were spawned out of @apparentlymart's comment in https://github.com/opentofu/opentofu/issues/1962#issuecomment-2398192022.

A symbol file may contain constant expressions used to define types, logic, and values. Referencing these symbols should made possible in a consistent and simple fashion.

A symbol file may also import the contents of another symbol library.

### User Documentation


#### Symbol File Contents

```hcl

#### Types #####

typedef "simple_type" {
  # https://opentofu.org/docs/language/expressions/types/
  # Any builtin type may be used here
  type = number
}

typedef "complex_type" {
  type = object({
    ncpus = number
    # Types may reference other types within the same symbol library (or other imported libraries)
    memory_size = symbols::types(simple_type)
  })
}

typedef "defaults_type" {
  type = object({
    # Defaults (via optional()) will be stored alongside the type
    complex = optional(symbols::types(complex_type), { ncpus = 1, memory_size = 1024 })
  })
}

#### Values #####

# Nearly identical concept to "locals" in the tofu configs, but usable externally
values {
  simple = 10
  custom_regex = "<some complex regex>"
  # Most builtin functions are allowed, TBD if we allow "file", "plantimestamp" and other non-constant functions
  upper_regex = upper(value.custom_regex)
}


#### Functions ####

# Simple function definition
function "add" {
  description = "add two numbers together" # Optional
  type        = number                     # Optional
  parameter "a" {
    description = "first number"
    type = number
  }
  parameter "b" {
    description = "second number"
    type = number
  }
  return = param.a + param.b 
}

# Function with parameter validation
# https://opentofu.org/docs/language/expressions/custom-conditions/#input-variable-validation
function "divide" {
  parameter "a" {
    type = number
  }
  parameter "b" {
    type = number
    validation {
      condition     = param.b != 0
      error_message = "Divide by zero"
    }
  }
  return = param.a / param.b 
}

# Function using variadic parameters and multiple expressions
function "greeting" {
  type = list(string)
  parameter "prefix" {
    type    = string
  }
  parameter "name" {
    type     = string
    variadic = true
  }
  locals {
    messages = [for x in param.name: "${param.prefix} ${x}!"]
  }
  return = tolist(local.messages)
}

# Function that uses custom types and calls other functions, both builtin and user defined
typedef "vec3" {
  type = object({ x = number, y = number, z = number})
}
function "vec3_length" {
  parameter "vec" {
    type = symbols::types(vec3)
  }
  locals {
    xx = param.x * param.x
    yy = param.y * param.y
    zz = param.z * param.z
    squared_sum = symbols::add(symbols::add(local.xx, local.yy), local.zz)
  }
  return = sqrt(local.squared_sum)
}

#### Libraries ####

# Import syntax
symbols "namespace" {
  # The package in which the symbols live
  source = "./lib" # Same conventions as registry modules, though without static evaluation

  # One of the decisions we need to make in this RFC is if we allow importing all of the symbols in a given package, 
  # or if a symbol block can only represent the contents of a single file
  file = "contents.sym.hcl"
}

# Usage in types
typedef "exported" {
  type = symbols::namespace::types(type_name)
}

typedef "embedded" {
  type = list(symbols::namespace::types(type_name))
}

# Usage in functions
function "myfunc" {
  return = symbols::namespace::func_name()
}

# Usage in values

values {
  val = symbols.namespace.other_val
}
```



How do I have "unexported" symbols?

The easiest way to do this without introduing any new syntax is to take an approach similar to go's `internal` pattern.

```hcl
# ./lib/contents.sym.hcl

# Import the internal library
symbols "internal" {
  source = "."
  file  = "./internal/types.sym.hcl"
}


# Re-export the custom type defined in the internal library
typedef "custom" {
  type = symbols::internal::types(custom)
}

```

```hcl
# ./lib/internal/types.sym.hcl

typedef "custom" {
  type = object({id = string, size = number})
}

typedef "other" {
  type = list(symbols::types(custom))
}
```

#### Symbol file usage

```hcl
# ./local-lib/exported.sym.hcl

typedef "items" {
    type = list(string)
}

function "non_empty" {
  parameter "in" {
    type = symbols::types(items) 
  }
  return = alltrue([for x in param.in: length(x) != 0])
}

function "assert_non_empty" {
  parameter "in" {
    type = symbols::types(items)
    validation {
      condition = symbols::non_empty(param.in)
      error_message = "One or more of the elements in ${jsonencode(param.in)} is empty"
    }
  }
  return = param.in
}

values {
    default_items = ["foo", "bar", "baz"]
}
```

```hcl
# main.tofu

# Import ./local-lib/exported.sym.hcl as "lib"
symbols "lib" {
  source = "./local-lib" # Static evaluation not allowed in this source
  file   = "exported.sym.hcl" 
}

# This variable pre-emptively 
variable "my_items" {
  type = symbols::lib::types(items)
  #type = list(string) would also work here
  validation {
    condition = symbols::lib::non_empty(var.my_items)
    error_message = "One or more elements is empty"
  }
  default = symbols.lib.default_items
}

variable "my_items_unchecked" {
  type = symbols::lib::types(items)
}

locals {
  # This will raise an error from within the function validation if var.my_items_unchecked contains empty elements
  my_items_checked = symbols::lib::assert_non_empty(var.my_items_unchecked)
}

resource "provider_type" "ident" {
  value = local.my_items_checked
}
```

Notes:
* Function recursion is not supported due to current limitations in HCL
* A hackathon prototype that includes a partial implementation of these features is available in the [symbol_libraries_experiment branch](https://github.com/opentofu/opentofu/tree/symbol_libraries_experiment)

### Technical Approach

The main approach for this feature is to create a library that allows for "compilation" of symbol libraries and that library's integration into OpenTofu. Although this library may be used externally to OpenTofu some day, the initial plan is to keep it internal.

#### Symbol Library Implementation

Contracts:
* Parse a symbol call
  - Given a `hcl.Body`, validate the body and return a structure that represents the symbol call.
  - This is primarily a helper for standardizing parsing `symbols "namespace" { ... }` and does not perform any complex logic or evaluation.
  - Similar to the existing `configs.decodeThingBlock()` pattern
* Parse a symbol file
  - Given a `hcl.Body`, validate the body and return a structure that represents the symbol file.
  - Similar to the existing `configs.Parser{}.loadConfigFileBody()` pattern
* Compile a symbol library.
  - Given:
    - A set of symbol files
    - A symbol loader function, which takes a symbol call and returns a symbol library
    - A set of builtin functions
  - Return a symbol library which contains:
    - Types compiled from `typedef` blocks
    - Functions compiled from `function` blocks
    - Values compiled from `values` blocks

Most of the proposed implementation follows existing patterns within the `configs` package and will not be covered here. The interesting differences however are in how HCL handles type lookups and in how we need to "compile" the library into a static set of functionality that requires no more evaluation.

The simpler of the two changes is to our fork of the HCL language. We already use the [typeext extension](https://github.com/opentofu/hcl/tree/opentofu/ext/typeexpr) heavily in OpenTofu. It however does not support the addition of custom types. The `typeext` package is fairly simple, and extending it to support custom types is a straightforward task. A possible implementation can be seen in the [patch for the hackathon experiment](https://gist.github.com/cam72cam/ac7d3900c7d5b96c8d87a0ef803302a2).

The other possible change to HCL is in altering the [userfunc extension](https://github.com/opentofu/hcl/tree/opentofu/ext/userfunc). The syntax shown in the user examples above is inspired by, but ultimately more complex than the existing userfunc extension. We will need to decide which syntax to use, and when either it makes sense to keep that implementation within the symbol library implementation or to move it into HCL.

The more complex piece of the library implementation is how to handle the compilation of the symbol files into a library. Ultimately, some form of evaluator will need to be built. It will need to understand the dependencies between the elements within the given symbol files and ensure that they are compiled in the correct order.

The approach taken in the hackathon project was to use the new [workgraph](https://pkg.go.dev/github.com/apparentlymart/go-workgraph) library (also used by the new engine) to build the evaluator/scope constructs. Once the scope was built, we simply iterate through and request all of the known values (which performs the compilation). There are many ways in which to perform this compilation. Given the current limited scope of what is allowed / is possible within symbol files (although complex), it is an easily solved problem.

#### OpenTofu Integration

The biggest challenge to integrating with OpenTofu at this juncture is that we are re-building the tofu engine as part of ongoing work, as well as swapping out the static evaluation implementation with engine integration. Our hope is that the simplicity of the symbol library integration contract means that the integration will not be hampered by or delay the ongoing engine work.

One of the critical touch-points is supporting symbol libraries in variable types. These need to be known prior to static evaluation, as that depends on the variable types. In practice, the current `configs/config_build.go` code can be juggled around to make the symbol library loading and static evaluation steps explicit and properly integrated. In theory, it will be easier to do this integration natively within the new engine.

We also need to determine how and where we install symbol library packages. The hackathon prototype treats them as modules to be installed, but is that the right solution?

The injection of the symbol library data into the OpenTofu engine/evaluator is a trivial concern, if the prototype is anything to go by.

### Open Questions

* How does this impact `tofu show -config -json`?
  - We have integrated that command into the backend of the registry-ui.
  - Will we need to add a new `tofu init -symbols-only` mode to support this use case?
  - Does this have security implementations / prevent us from exposing `file/templatefile`?
* Tooling Integration
  - Tofu-ls?
  - Linting?
  - Dependabot?
* JSON Syntax support
  - Do we want to support json syntax for the symbol files?
  - Are there any features that explicitly can't be supported via the json syntax?
* Types are currently aliases and the actual names do not matter
  - Do users expect the same type definition under different names to be incompatible?
* Does the `symbols::(namespace::)types(type_name) syntax make sense?
  - `symbols(namespace.type_name)`?
  - `symbols::types(namespace.type_name)`?
  - `types(symbols.namespace.type_name)`?
  - Unless we want to make more significant changes to HCL, function call expr syntax is the easiest option here.
* How do we want to represent symbol language versioning?
* As we consider module locking, do we also want to consider symbol locking?

### Future Considerations

* Functions outside of symbol libraries?
  - What use cases are there to support functions in .tofu files themselves?
  - Is that complexity worth it?
* Integration with static eval
  - Would we ever want to support static eval or symbol values within the `source` field of symbol libraries?
* Recursion support
  - HCL in it's current form makes this impossible. It would be neat to try to support it *someday*
