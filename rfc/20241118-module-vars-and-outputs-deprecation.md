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
  deprecated  = "This variable will be removed on 2024-12-31. Use another_variable instead."
}

output "this_is_my_output" {
  value       = some_resource.some_name.some_field
  description = "This is an output for the old way of using things."
  deprecated  = "This output will be removed on 2024-12-31. Use another_output instead."
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
│ This variable will be removed on 2024-12-31. Use another_variable instead.
```

> [!NOTE]
> It is possible for module user to specify a null value for a deprecated variable and receive no warnings. OpenTofu treats
> variables with explicit null values the same way as if it was ommited completely. This is not an ideal, however an idiomatic
> OpenTofu language approach. See `null` values note [in the docs](https://opentofu.org/docs/language/expressions/types/#types).

#### Deprecated module output warning

If module caller has references to deprecated module outputs in their configuration, OpenTofu should produce a
deprecation warning as specified by module author:

```hcl
│ Warning: Value derived from a deprecated source
│ 
│   on main.tf line 9, in resource "some_resource" "some_name":
│    9:     some_field = module.mod.this_is_my_output
│ 
│ This value is derived from module.mod.this_is_my_output, which is
│ deprecated with the following message:
|
| This output will be removed on 2024-12-31. Use another_output instead.
```

#### Silencing deprecation warnings for dependencies

If the module is not controlled by the configuration author, it makes sense to optionally ignore deprecation warnings raised in such
a module. I will be refering to local and non-local modules, which contain module calls with deprecated variables or outputs. By local
modules, I mean modules fetched from local filesystem and for non-local modules, it means they are fetched from remote systems. This
separation helps with defining what module configuration users control. For example, module calls with deprecation warnings in local
modules (even non-root ones) could be fixed by a configuration author, thus shouldn't be ignored.

Silencing deprecation warnings could be presented in two different ways: globally for all non-local modules or on per-module
basis. This setting shouldn't be a part of a module call block since there could be multiple calls for the same module and each such
call should either produce or silence the warnings.

In this RFC I suggest to allow users to control module deprecation warnings via a CLI flag: `deprecation-warns`, which may
have the following values: `all` (default) or `local` (raise deprecation warnings only from module calls inside the local modules).
This could be extended later to include other options such as `none` to disable the deprecation warnings altogether.

### Technical Approach

Technical approach should be different for input variables and outputs. For input variables, we can follow existing approach, where
variables are being validated.

As for module outputs, we can't follow the same idea, since those values can be used on execution of different graph nodes,
so we need to track deprecation "flags" alongside the values modules produce. This is possible with [go-cty's value marks](https://github.com/zclconf/go-cty/blob/main/docs/concepts.md#marks).

Currently, OpenTofu uses value marks to track sensitive values and a specific output from console-only `type` function. Some
implementations (e.g. `genconfig.GenerateResourceContents`) treat any mark as sensitive and some other implementations (e.g.
`funcs.SensitiveFunc`) erase any non-sensitive marks during its execution. Before the actual implementation of deprecation marks,
we need to carefully review all the places, where OpenTofu operates with marks. This step is crucial to make it possible to 
introduce new marks even beyond deprecation ones.

#### Deprecation marks

Sensitive and type marks are boolean flags which are represented by internal go type of `marks.valueMark`. On the contrary,
deprecation marks must carry more data, including the list of addresses from where this value has been composed. This fact
requires us to introduce a new type (e.g. `marks.Deprecation`), which is a struct with a list of source addresses. Based on
how `go-cty` handles marks (as a set, i.e. map with only keys to be used), we will not be able to use built-in check for mark
presence.

We would need to extend module output nodes to also mark values as deprecated if the user specified so. Then, the mark should
be checked where `tofu.EvalContext` is used to evaluate expressions and blocks (i.e. `EvalContext.EvaluateBlock` and
`EvalContext.EvaluateExpr`). The deprecation mark check must take into account the value of `deprecation-warns` flag
and also it shouldn't fire if the value is used inside the module, which marked this value as deprecated.

This approach would also allow us to reuse the implementation, if we want to mark more values as deprecated from different
sources (i.e. not only module outputs).

### Open Questions

None.

### Future Considerations

It is hard to implement generic deprecation mechanism for the OpenTofu language. However, this solution should be generic 
enough from the UX point of view to potentially be extended for other purposes. This way we keep consistent experience for
OpenTofu users. Deprecation marks could help reuse parts of the implementation (evaluation checks) to handle more `deprecation`
flags in the future.

At the time of writing, Terraform haven't yet released deprecation mechanism for module variables and outputs, so
we are going to mark that feature as experimental in order to tweak UX in the future if needed. That way we are
going to keep compatibility with the upstream project.

## Potential Alternatives

Here is the list of other potential options (mostly from the UX perspective), which were considered at the time of writing this RFC.

#### HCL comment

Right now, OpenTofu doesn't treat comments as something with a special meaning so this feature would introduce a whole new set
of functionality to be used across the OpenTofu. It would require a separate RFC to define how this notion could potentially
evolve.

```hcl
# @deprecated: This variable will be removed on 2024-12-31. Use another_variable instead.
variable "this_is_my_variable" {
  type = string
  description = "This is a variable for the old way of configuring things."
}
```

Allowing comments to be a valid part of the configuration is also constrainted by an internal implementation of `hclsyntax` parser.

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
  message = "This variable will be removed on 2024-12-31. Use another_variable instead."
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
    warning_message = "This variable will be removed on 2024-12-31. Use another_variable instead."
  }
}
```

#### Extended variable description (rejected by TSC)

```hcl
variable "this_is_my_variable" {
  type = string
  description = "This is a variable for the old way of configuring things. @deprecated{ This variable will be removed on 2024-12-31. Use another_variable instead. }"
}
```
