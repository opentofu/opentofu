# Overview of custom type related issues

## OpenTofu Issues:

Add the ability to create custom types: https://github.com/opentofu/opentofu/issues/1962
Problem:
- Reduce Copy-Paste of variable blocks.
Requested:
- Some way to re-use type definitions with defaults.
Comments:
- Requested ability to re-use validation.
References:
- https://github.com/hashicorp/terraform/issues/27365

Define variable type from provider schema: https://github.com/opentofu/opentofu/issues/2704
Problem:
- Copy paste of variable types (and subtypes) representing provider resources is cumbersome.
Requested:
- Allow using a provider resource as a variable type.
- Some way of turning `tofu providers schema` resources into type definitions.
Comments:
- Module types being independent from providers is intentional, the module API does not change when a provider is updated.
  - This is not entirely true as outputs can change if a provider is updated and the above only applies to variables currently.
  - There is ongoing discussion on adding type constraints to output blocks.
- A "object type" != "type constraint" within opentofu and supporting libraries
References:
- [dupe] https://github.com/opentofu/opentofu/issues/3829

Validation of Output Types: https://github.com/opentofu/opentofu/issues/2831
Problem:
- Ensure module outputs conform to a specific type
Requested:
- type property on output blocks
Comments:
- Currently accomplished using a "type validation module" which is cumbersome and cryptic
- Should this represent an "exhaustive type" or one that could grow to include new attributes.
  - Current type constraint system does not shoot us in the foot regarding that future decision
References:
- https://github.com/opentofu/opentofu/issues/2704

## Terraform Issues

Ability to create custom types: https://github.com/hashicorp/terraform/issues/27365
Problem:
- Too much variable type boiler plate in large projects
Requested:
- Defining custom types in top level blocks
Comments:
- Export a type definition from a module (re-use)
- Validate and other variable specific fields requested
- Referencing a child module's input as a type
- Provider types (and subtypes) as variable types
References:
- [dupe] https://github.com/hashicorp/terraform/issues/32101
- [dupe] https://github.com/hashicorp/terraform/issues/27905
- [dupe] https://github.com/hashicorp/terraform/issues/36361
- [dupe] https://github.com/hashicorp/terraform/issues/22683
- [dupe] https://github.com/hashicorp/terraform/issues/24761
- [dupe] https://github.com/hashicorp/terraform/issues/32320
- [dupe] https://github.com/hashicorp/terraform/issues/30386

Introduce "union" type for variables: https://github.com/hashicorp/terraform/issues/32587
Problem:
- Multiple one of many "payload types" wanted in a module variable
Requested:
- Support for "tagged union" types with new types + syntax
Comments:
- Currently possible in $tool, though requires a bit of hand holding with variable validation and locals
- Distilled to adding syntax sugar to make this pattern easier.
References:
- https://github.com/hashicorp/terraform/issues/33916


Declare a variable with a resource type: https://github.com/hashicorp/terraform/issues/25466
Problem:
- Desires ability to pass a resource in as a variable verbatium.
- Core issue is actually depends_on, not just typing
Requested:
- Changes to the variable block to take a resource type argument
Comments:
- Martin: Loose couping by design, depends_on does not need an actual type
- User: Explicitly wants this to be a resource type as the hidden dependency is crucial

## Summary

Most of the issues above are directly solved with what is currently in the RFC.

The only things not entirely covered are:
- Named vs unnamed types
- Tagged union types
- Syntax to select subtypes of type definitions

The named vs unnamed types in the above is actually a more specific question about *requiring* a resource instace as a variable type for use in depends_on. In practice, most of the problem seems to be a misunderstanding in how depends_on functions in the original issue. That misunderstanding aside, introducing named types would be a drastic diversion from everything already added to the language and would be a massive stumbling blocks. It implies a very different language design and usage patterns that are outside the scope of what this RFC is trying to accomplish.


Although this RFC does not propose introducing something as complex as tagged unions, they are a somewhat frequently requested feature that libraries could make easier to implement.
```hcl
typedef "foo" {
    type = any
}
typedef "bar" {
    type = any
}

typedef "union" {
    object({
        foo = symbols::foo()
        bar = symbols::bar()
    })
}


function union_foo {
    parameter "foo" {
        type = symbols::foo()
    }
    return = { foo = param.foo }
}

function ensure_union {
    parameter "union" {
        type = symbols::union()
        validation {
            condition     = length([for o in param.union: o if o != null]) == 1 # borrowed from @apparentlymart
            error_message = "Invalid union, expected one attribute set"
        }
    }
    return = param.union
}

function union_type {
    parameter "union" {
        type = symbols::union()
    }
    return = one([select k, o in symbols::ensure_union(param.union): k if o != null]) # borrowed from @apparentlymart
}
function union_attr {
    parameter "union" {
        type = symbols::union()
    }
    return = param.union[symbols::union_type(param.union)]
}

// Usage

module "example" {
    union = symbols::mylib::union_foo(var.foo)
    # union = { foo = var.foo }
}

// inside module

variable "union" {
    type = symbols::mylib::union()
}

resource "example_resource" "foo" {
    lifecycle { enabled = symbols::mylib::union_type(var.union) == "foo"}
    value = symbols::mylib::union_attr(var.union)
    // OR value = var.union["foo"]
}

```


Is there any value in allowing users to select sub-portions of given types? Something akin to:
```hcl
typedef "foo" {
  type = object({
    id = string
    attrs = object({
        stuff = any
    })
  })
}

// Usage

variable "foo" {
  type = symbols::mylib::foo.attrs()
}
```

In practice, this could be re-written as
```hcl
typedef "foo_attrs" {
  type = object({
    stuff = any
  })
}
typedef "foo" {
  type = object({
    id = string
    attrs = symbols::foo_attrs()
  })
}

// Usage

variable "foo" {
  type = symbols::mylib::foo_attrs()
}
```
which feels like a more idiomatic tf/tofu pattern.
