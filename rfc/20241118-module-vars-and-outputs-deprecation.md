# Deprecation of module variables and outputs

Issue: https://github.com/opentofu/opentofu/issues/1005

OpenTofu configuration can be split into multiple [modules](https://opentofu.org/docs/language/modules/). Each module
represents some part of the configuration, which can have its own [input variables](https://opentofu.org/docs/language/values/variables/)
and [outputs](https://opentofu.org/docs/language/values/outputs/) to provide additional customization and integration
into the configuration base.

Oftentimes, users who [call modules](https://opentofu.org/docs/language/modules/syntax/#calling-a-child-module) are not the
ones who implemented them, so it requires careful consideration from module authors on how the module should evolve. It also
creates an additional challenge for module authors on how to properly introduce and communicate breaking changes, including
deprecation of module input variables and outputs.

Currently, module authors either keep backward compatiblity or put deprecation notices in the release notes. Both options are
not ideal for the end-user. Therefore, it would be useful if OpenTofu could notify module users about the deprecation of input 
variables or outputs as defined by module authors.

> [!NOTE]
> Module deprecation as a whole is a similar problem, however it is addressed on the OpenTofu Registry level.

## Proposed Solution

OpenTofu should allow module authors to mark input variables and outputs as deprecated with a warning message alongside.
Module users have to see a respective warning message in cases, when deprecated input variable or output is used in their
configuration.

> [!NOTE]
> The result of `tofu validate`, `tofu plan`, `tofu apply` and `tofu test` should include deprecation warnings.

### User Documentation

Module authors should be able to use a new field named `deprecated` in both `variable` and `output` blocks. This field 
should accept a required string value for a warning message:

```hcl
variable "this_is_my_variable" {
  type        = string
  description = "This is a variable for the old way of configuring things."
  deprecated  = "This variable will be removed on 2024-12-31. Use that_is_my_variable instead."
}

output "this_is_my_output" {
  value       = some_resource.some_name.some_field
  description = "This is an output for the old way of using things."
  deprecated  = "This output will be removed on 2024-12-31. Use that_is_my_output instead."
}
```

#### Deprecated module variable warning

If module caller specify non-null value for a deprecated variable, OpenTofu should produce a deprecation warning as 
specified by module author:

```hcl
│ Warning: The variable "this_is_my_variable" is marked as deprecated by module author.
│ 
│   on mod/main.tf line 9, in module call "mod":
│    9:     this_is_my_variable = "something"
│ 
│ This variable will be removed on 2024-12-31. Use that_is_my_variable instead.
```

> [!NOTE]
> It is possible for module user to specify a null value for a deprecated variable and receive no warnings. OpenTofu treats
> variables with explicit null values the same way as if it was ommited completely. This is not an ideal, however an idiomatic
> OpenTofu language approach. See `null` values note [in the docs](https://opentofu.org/docs/language/expressions/types/#types).

#### Deprecated module output warning

If module caller has references to deprecated module outputs in their configuration, OpenTofu should produce a
deprecation warning as specified by module author:

```hcl
│ Warning: The output "this_is_my_output" is marked as deprecated by module author.
│ 
│   on main.tf line 9, in resource "some_resource" "some_name":
│    9:     some_field = module.mod.this_is_my_output
│ 
│ This output will be removed on 2024-12-31. Use that_is_my_output instead.
```

> [!NOTE]
> Module users may rely on [output precondition check](https://opentofu.org/docs/language/values/outputs/#custom-condition-checks),
> however it doesn't require implicit references, so this type of usage will not be counted when generating deprecation warnings. We
> might want to forbid deprecation of outputs with precondition checks so module authors would be required to migrate them beforehand.

### Technical Approach

Technical approach should be different for input variables and outputs. For input variables, we can follow existing approach, where
variables are being validated.

As for module outputs, it is slightly harder to determine if it's is actually referenced in the user configuration, since
`nodeExpandOutput` is included in the graph regardless of its actual usage. On graph walk, this node expands to one or more
`NodeApplyableOutput` nodes, which also validates precondition checks on execution.

In the case we decide to forbid deprecating outputs with precondition checks, we can extend `nodeExpandOutput` with the
deprecation check and then extend `ModuleExpansionTransformer` to not link `nodeExpandOutput` with `nodeCloseModule`. This
way we are getting rid of unused output nodes from the graph. Deprecation check is just ignored if there is an output is unused.

Otherwise, we can extend existing `ReferenceTransformer` (or create a new one) to check if there are any connections to
`nodeExpandOutput` from the outside of its module. We should keep in mind potential performance downgrades since for large
graphs it may be slow to go through all the connections. This approach would also require us to slightly refactor `GraphTransformer`
interface to being able to produce non-critical errors (warnings).

### Open Questions

* Do we want to support silencing of deprecation warnings?
* Should we forbid deprecating module outputs with precondition checks?

### Future Considerations

It is hard to implement generic deprecation mechanism for the OpenTofu language. However, this solution should be generic 
enough from the UX point of view to potentially be extended for other purposes. This way we keep consistent experience for
OpenTofu users.

Also, we want to keep compatibility with Terraform's deprecation mechanisms.

## Potential Alternatives

Here is the list of other potential options (mostly from the UX perspective), which were considered at the time of writing this RFC.

#### HCL comment

Right now, OpenTofu doesn't treat comments as something with a special meaning so this feature would introduce a whole new set
of functionality to be used across the OpenTofu. It would require a separate RFC to define how this notion could potentially
evolve.

```hcl
# @deprecated: This variable will be removed on 2024-12-31. Use that_is_my_variable instead.
variable "this_is_my_variable" {
  type = string
  description = "This is a variable for the old way of configuring things."
}
```

#### Separate `deprecation` block

This approach is too verbose and doesn't align properly with OpenTofu language design. However, this way module authors can
put their deprecation warnings in a separate `.tofu` file to keep compatibility with other tools.

```hcl
variable "this_is_my_variable" {
  type = string
  description = "This is a variable for the old way of configuring things."
}

deprecation "this_is_my_variable" {
  type = "variable"
  message = "This variable will be removed on 2024-12-31. Use that_is_my_variable instead."
}
```

#### Custom variable validation

This option could be reused for generic warning when potentially unwanted behaviour may take place. However, this option is
not suitable for outputs deprecation.

```hcl
variable "this_is_my_variable" {
  type = string
  description = "This is a variable for the old way of configuring things."
  
  validation {
    condition     = var.this_is_my_variable != null
    warning_message = "This variable will be removed on 2024-12-31. Use that_is_my_variable instead."
  }
}
```

#### Extended variable description (rejected by TSC)

```hcl
variable "this_is_my_variable" {
  type = string
  description = "This is a variable for the old way of configuring things. @deprecated{ This variable will be removed on 2024-12-31. Use that_is_my_variable instead. }"
}
```
