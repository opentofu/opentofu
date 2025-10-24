# OpenTofu glossary
This document is intended for anyone who wants to gain more knowledge about the terms and vocabulary used to talk about different concepts in OpenTofu.

> [!NOTE]
> This was created with the intent of gathering more knowledge over time.
> The state that you find this in right now could be incomplete. 
> Once you discover and learn about a new concept that would benefit others,
> feel free to open a PR to update this.

> [!NOTE]
> When adding new content to this document, try to place it in such a way to match the alphabetical order of the already existing content.
> 
> Optionally, also add a reference link if possible (GitHub conversation, issue, other docs, etc).

## OpenTofu
### Attribute/argument/field
* Attribute - a named key inside an object type.
* Argument - a name used for an individual setting inside a configuration block.

It is recommended to avoid using popular programming language terms such as "field" or "property" to describe an element of an object in OpenTofu.

Reference: [link](./diagnostics.md#diagnostic-description-writing-style)

### Data sources/data resource
* Data source - the remote thing that the data resource reads from.
* Data resource - refers to a block of type `data`, and the associated object it declares.
* Data resource type - is what is represented by the first label in a `data` block header, and the associated declarations and code for it in the provider plugin.

Reference: [link](https://github.com/opentofu/opentofu/pull/3389#discussion_r2440264786)

### Diagnostic
"Diagnostic" is the general term we use to describe the error or warning
message that OpenTofu returns when there are problems with the configuration,
or when interactions with external systems fail.

Reference: [link](./diagnostics.md)

### Mark/value mark
OpenTofu uses cty.Value to represent the result of expressions (and other data).
Occasionally, we will want to annotate that data with additional properties, without actually modifying the underlying value.
Marks are used for that purpose and are the preferred method of doing so with go-cty.

### Resource/resource instance/resource type
* Resource - is what is declared by a `resource`, `data` or `ephemeral` block.
* Resource instance - is what such a block can declare zero or more of, when using the `count`, `for_each`, or `enabled` arguments.
* Resource type - the type of a "resource". E.g., `aws_instance` is a "resource type".

It is recommended to use the following terms when discussing about "resource" blocks:
* managed resource - a block declared as `resource "type" "name" {}`
* data resource - a block declared as `data "type" "name" {}`
* ephemeral resource - a block declared as `ephemeral "type" "name" {}`

Reference: [link](./diagnostics.md#diagnostic-description-writing-style)

### Unknown value/Computed value
* Unknown value - Unknown values are the result of expressions that have unknown inputs. E.g.: a value that will not be known until a resource is created.
  Right now the main source of this type of values is resources, but we are considering adding others like unknown inputs.
  Another place where an unknown value can be encountered is from using some of the built-in functions like `timestamp`, `bcrypt` and `uuid`.
* Computed value - Computed is more of a resource specific concept that a provider can specify in its resource schema. 
  When set to true, the provider does not expect a value and may instead produce one that may or may not be unknown. 
  With other flags, the actual functionality is a bit more subtle.

References: [link](https://github.com/opentofu/opentofu/blob/490762343322eff42c0586f7a4c267b579fe80ef/internal/configs/configschema/schema.go#L65), [link](https://github.com/opentofu/opentofu/blob/490762343322eff42c0586f7a4c267b579fe80ef/internal/lang/functions.go#L22)
## HCL
### Evaluation context (HCL)
A set of already known functions, input values, local values, resources, etc. that is used to evaluate an expression that can reference any of the concepts listed above.

The list of concepts above, in the context of HCL evaluation, are called [variables](#variable-hcl).

### Expression
An expression is any right hand side of an assignment that will be evaluated to generate the value that will be associated with key on the left hand side of the assignment. 
The simplest expressions are just literal values, like "hello" or 5, but the OpenTofu language also allows more complex 
expressions such as references to data exported by resources, arithmetic, conditional evaluation, and a number of built-in and provider-defined functions.

Reference: [link](https://opentofu.org/docs/language/expressions/)

### Variable (HCL)
Anything that's available to refer to in the current evaluation context.