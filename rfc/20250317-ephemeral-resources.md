# Ephemeral resources, variables, outputs, locals and write-only arguments

Issue: https://github.com/opentofu/opentofu/issues/1996

Right now, OpenTofu information for resources and outputs are written to state as it is. This is presenting a security risk
as some of the information from the stored objects can contain sensitive bits that are visible to whoever is having access to the state file.

In order to provide a better solution for the aforementioned situation, OpenTofu introduces the concept of "ephemerality".

To make this work seamlessly with most of the blocks that OpenTofu supports, the following functionalities need to be able to work with the ephemeral concept:
* `resource`'s `write-only` attributes
* variables
* outputs
* locals
* ephemeral `resource`s
* providers
* provisioners
* `connection` block

## Proposed Solution

In the attempt of providing to the reader an in-depth understanding of the ephemerality implications in OpenTofu,
this section will try to explain the functional implications of the new concept in each existing feature.

### Write-only attributes
Write-only attributes is a new concept that allows any existing `resource` to define attributes in its schema that can be only written without the ability to retrieve the value afterward.

By not being readable, this also means that an attribute configured by a provider this way, will not be written to the state or plan file either.
Therefore, these attributes are suitable for configuring specific resources with sensitive data, like passwords, access keys, etc.

A write-only attribute can accept an ephemeral or a non-ephemeral value, even though it's recommended to use ephemeral values for such attributes.

Because these attributes are not written to the plan file, the update of a write-only attribute it's getting a little bit trickier.
Provider implementations do generally include also a "version" field linked to the write-only one.
For example having a write-only field called `secret`, providers should also include
a non-write-only field called `secret_version`. Every time the user wants to update the value of `secret`, it needs to change the value of `secret_version` to trigger a change.
The provider implementation is responsible with handling this particular case: because the version field is stored also in the state, the provider needs to compare the value from the state with the one from the configuration
and in case it differs, it will trigger the update of the `secret` field.

The write-only attributes are supported momentarily by a low number of providers and resources.
Having the `aws_db_instance` as one of those, here is an example on how to use the write-only attributes:
```hcl
resource "aws_db_instance" "example" {
  // ...
  password_wo         = "your-initial-password"
  password_wo_version = 1
  // ...
}
```
By updating **only** the `password_wo`, on the `tofu apply`, the password will not be updated.
To do so, the `password_wo_version` needs to be incremented too:
```hcl
resource "aws_db_instance" "example" {
  // ...
  password_wo         = "new-password"
  password_wo_version = 2
  // ...
}
```

As seen in this particular change of the [terraform-plugin-framework](https://github.com/hashicorp/terraform-plugin-framework/commit/ecd80f67daed0b92b243ae59bb1ee2077f8077c7), the write-only attribute cannot be configured for set attributes, set nested attributes and set nested blocks.
> [!NOTE]
> 
> Why so? I need additional information here. Why MapNestedAttribute can be write-only but not SetAttribute, SetNestedAttribute and SetNestedBlock?
> Some info [here](https://github.com/hashicorp/terraform-plugin-framework/pull/1095).

Write-only attributes cannot generate a plan diff because the prior state does not contain a value that OpenTofu can use to compare the new value against and also the planned value of a write-only argument will always be empty.
### Variables
Any `variable` block can be marked as ephemeral.
```hcl
variable "ephemeral_var" {
  type      = string
  ephemeral = true
}
```
OpenTofu will allow usage of these variables only in other ephemeral contexts:
* write-only arguments
* other ephemeral variables
* ephemeral outputs
* local values
* ephemeral resources
* provisioner blocks
* connection blocks
* provider configuration

Usage in any other place will raise an error.

OpenTofu will not store ephemeral variable(s) in plan files. 
If a plan is generated from a configuration that is having at least one ephemeral variable, 
when the planfile will be applied, the value(s) for the ephemeral variable(s) needs to passed again by 
using `-var` or `-var-file` arguments.

### Outputs
An `output` block can be configured as ephemeral as long as it's
not from the root module. 
This limitation is natural since ephemeral outputs are meant to be skipped from the state file. Therefore, there is no usage of such a defined output block in a root module.

Ephemeral outputs are useful when a child module returns sensitive data, forcing the caller to use the value of that output only in ephemeral contexts.

To mark an output as ephemeral, use the following syntax:
```hcl
output "test" {
  // ...
  ephemeral = true
}
```

The ephemeral outputs are available during plan and apply phase and can be accessed only in specific contexts:
* ephemeral variables
* other ephemeral outputs
* write-only attributes
* ephemeral resources

> [!NOTE]
> 
> Check other blocks like `provider`, `provisioner` and `connection` for early evaluated ephemeral values. Maybe with the OpenTofu early eval feature, at least the `provider` should be able to reference ephemeral values  

### Locals
Local values are automatically marked as ephemeral if any of value that is used to compute the local is already an ephemeral one.

Eg:
```hcl
variable "a" {
  type = string
  default = "a value"
}

variable "b" {
  type = string
  default = "b value"
  ephemeral = true
}

locals {
  a_and_b = "${var.a}_${var.b}"
}
```
Because variable `b` is marked as `ephemeral`, then the local `a_and_b` is marked as `ephemeral` too.

### Ephemeral resource
In contrast with the write-only arguments where only specifically tagged attributes are skipped from the state/plan file, `ephemeral` resources are skipped entirely.
This is adding a new idea of generating a resource every time. 
For example, you can have an ephemeral resource that is retrieving the password from a secret manager, password that can be passed later into a write-only attribute of another ephemeral resource.

Ephemeral resources can be referenced only in specific contexts:
* other ephemeral resources
* ephemeral variables
* ephemeral outputs
* locals
* to configure `provider` blocks
* in `provisioner` and `connection` blocks
* in write-only arguments

### Providers
`provider` block is ephemeral by nature, meaning that the configuration of this is never stored into state/plan file.

Therefore, this block should be able to receive values from ephemeral variables/resources.

### `provisioner` block
As `provisioner` information is not stored into the plan/state file, this can reference ephemeral values like ephemeral variables, outputs, locals and values from ephemeral resources.

Whenever doing so, the output of the provisioner execution should be suppressed.
### `connection` block
When the `connection` block is configured, this will be allowed to use ephemeral values from variables, outputs, locals and values from ephemeral resources. 

## User Documentation

Describe what the user would encounter when attempting to interact with what is being proposed. Provide clear and detailed descriptions, code examples, diagrams, etc... Starting point for the documentation that will be added during implementation.

This documentation will help the community have a better understanding how they will be interacting with this proposal and have an easier time discussing it in depth.

## Technical Approach

<!-- Technical summary, easy to understand by someone unfamiliar with the codebase. -->
<!---->
<!-- Link to existing documentation and code, include diagrams if helpful. -->
<!---->
<!-- Include pseudocode or link to a Proof of Concept if applicable. -->
<!---->
<!-- Describe potential limitations or impacts on other areas of the codebase. -->

In this section, as in the "Proposed Solution" section, we'll go over each concept, but this time in a more technical point of view.

### Write-only arguments
Most of the write-only arguments logic is already in the [provider-framework](https://github.com/hashicorp/terraform-plugin-framework):
* [Initial implementation](https://github.com/hashicorp/terraform-plugin-framework/pull/1044)
* [Sets comparisson enhancement](https://github.com/hashicorp/terraform-plugin-framework/pull/1064)
  * This seems to be related to the reason why sets of any kind are not allowed to be marked as write-only
* [Dynamic attribute validation](https://github.com/hashicorp/terraform-plugin-framework/pull/1090)
* [Prevent write-only for sets](https://github.com/hashicorp/terraform-plugin-framework/pull/1095)
* [Nullifying write-only attributes moved to an earlier stage](https://github.com/hashicorp/terraform-plugin-framework/pull/1097)

On the OpenTofu side the following needs to be tackled:
* Update [Attribute](https://github.com/opentofu/opentofu/blob/ff4c84055065fa2d83d318155b72aef6434d99e4/internal/configs/configschema/schema.go#L44) to add a field for the WriteOnly flag.
* Update the validation of the provider generated plan in such a way to allow nil values for the fields that are actually having a value defined in the configuration. This is necessary because the plugin framework is setting nil any values that are marked as write-only.
  * Test this in-depth for all the block types except sets of any kind (Investigate and understand why sets are not allowed by the plugin framework).
    * Add a new validation on the provider schema to check against, set nested attributes and set nested blocks with writeOnly=true. Tested this with a version of terraform-plugin-framework that allowed writeOnly on sets and there is an error returned. (set attributes are allowed based on my tests)
    In order to understand this better, maybe we should allow this for the moment and test OpenTofu with the [plugin-framework version](https://github.com/hashicorp/terraform-plugin-framework/commit/0724df105602e6b6676e201b7c0c5e1d187df990) that allows sets to be write-only=true.

## Open Questions

List questions that should be discussed and answered during the RFC process.

## Future Considerations

What are some potential future paths this solution could take?  What other features may interact with this solution, what should be kept in mind during implementation?

Docs to be added:
* write-only - add also some hands-on with generating an ephemeral value and pass it into a write-only attribute
* variables - add an in-depth description of the ephemeral attribute in the variables page
* outputs - add an in-depth description of the ephemeral attribute in the outputs page
## Potential Alternatives

List different approaches and briefly compare with the proposal in this RFC. It's important to explore and understand possible alternatives before agreeing on a solution.
