# Ephemeral resources, variables, outputs, locals and write-only arguments

Issue: https://github.com/opentofu/opentofu/issues/1996

Right now, OpenTofu information for resources and outputs are written to state as it is. This is presenting a security risk
as some of the information from the stored objects can contain sensitive bits that can become visible to whoever is having access to the state file.

In order to provide a better solution for the aforementioned situation, OpenTofu introduces the concept of "ephemerality".

To make this work seamlessly with most of the blocks that OpenTofu supports, the following functionalities need to be able to work with the ephemeral concept:
* `resource`'s `write-only` attributes
* variables
* outputs
* locals
* `ephemeral` resources
* providers
* provisioners
* `connection` block

## Proposed Solution

In the attempt of providing to the reader an in-depth understanding of the ephemerality implications in OpenTofu,
this section will try to explain the functional implications of the new concept in each existing feature.

### Write-only attributes
This is a new concept that allows any existing `resource` to define attributes in its schema that can be only written without the ability to retrieve the value afterwards.

By not being readable, this also means that an attribute configured by a provider this way, will not be written to the state or plan file either.
Therefore, these attributes are suitable for configuring specific resources with sensitive data, like passwords, access keys, etc.

A write-only attribute can accept an ephemeral or a non-ephemeral value, even though it's recommended to use ephemeral values for such attributes.

Because these attributes are not written to the plan file, the update of a write-only attribute it's getting a little bit trickier.
Provider implementations do generally include also a "version" argument linked to the write-only one.
For example having a write-only argument called `secret`, providers should also include
a non-write-only argument called `secret_version`. Every time the user wants to update the value of `secret`, it needs to change the value of `secret_version` to trigger a change.
The provider implementation is responsible with handling this particular case: because the version attribute is stored also in the state, the provider needs to compare the value from the state with the one from the configuration and in case it differs, it will trigger the update of the `secret` attribute.

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

Usage in any other place will raise an error:
```shell
│ Error: Invalid use of an ephemeral value
│
│   with playground_secret.store_secret,
│   on main.tf line 30, in resource "playground_secret" "store_secret":
│   30:   secret_name = var.password
│
│ "secret_name" cannot accept an ephemeral value because it is not a write-only attribute which means that will be written to the state.
╵
```

OpenTofu will not store ephemeral variable(s) in plan files. 
If a plan is generated from a configuration that is having at least one ephemeral variable, 
when the planfile will be applied, the value(s) for the ephemeral variable(s) needs to passed again by 
using `-var` or `-var-file` arguments.

### Outputs
An `output` block can be configured as ephemeral as long as it's
not from the root module. 
This limitation is natural since ephemeral outputs are meant to be skipped from the state file. Therefore, there is no usage of such a defined output block in a root module.

Ephemeral outputs are useful when a child module returns sensitive data, allowing the caller to use the value of that output in other ephemeral contexts.

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

Locals marked as ephemeral are available during plan and apply phase and can be referenced only in specific contexts:
* ephemeral variables
* other ephemeral locals
* write-only attributes
* ephemeral resources
* provider blocks configuration
* `connection` and `provisioner` blocks

### Ephemeral resource
In contrast with the write-only arguments where only specifically tagged attributes are skipped from the state/plan file, `ephemeral` resources are skipped entirely.
This is adding a new idea of generating a resource every time. 
For example, you can have an ephemeral resource that is retrieving the password from a secret manager, password that can be passed later into a write-only attribute of another normal `resource`.

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

Therefore, this block should be configurable by using also ephemeral values.

### `provisioner` block
As `provisioner` information is not stored into the plan/state file, this can reference ephemeral values like ephemeral variables, outputs, locals and values from ephemeral resources.

Whenever doing so, the output of the provisioner execution should be suppressed.
### `connection` block
When the `connection` block is configured, this will be allowed to use ephemeral values from variables, outputs, locals and values from ephemeral resources. 

## User Documentation

Describe what the user would encounter when attempting to interact with what is being proposed. Provide clear and detailed descriptions, code examples, diagrams, etc... Starting point for the documentation that will be added during implementation.




## Technical Approach
In this section, as in the "Proposed Solution" section, we'll go over each concept, but this time with a more technical focus.

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

> [!NOTE]
>
> Write-only attributes will be presented in the OpenTofu's UI as `(write-only attribute)` instead of the actual value.

### Variables

For enabling ephemeral variables, these are the basic steps that need to be taken:
* Update config to support the `ephemeral` attribute.
* Mark the variables with a new mark ensure that the marks are propagated correctly.
* Based on the marks, ensure that the variable cannot be used in other contexts than the ephemeral ones (see the User Documentation section for more details on where this is allowed).
* Check the state of [#1998](https://github.com/opentofu/opentofu/pull/1998). If that is merged, in the changes where variables from plan are verified against the configuration ones, we also need to add a validation on the ephemerality of variables. If the variable is marked as ephemeral, then the plan value is allowed (expected) to be missing. 
* Ensure that when the prompt is shown for an ephemeral variable there is an indication of that:
  ```hcl
  var.password (ephemeral)
     Enter a value:
  ````

We should use boolean marks, as no additional information is required to be carried. When introducing the marks for these, extra care should be taken in *all* the places marks are handled and ensure that the existing implementation around marks is not affected.

### Outputs

For enabling ephemeral outputs, these are the basic steps that need to be taken:
* Update config to support the `ephemeral` attribute.
* Mark the outputs with a new mark and ensure that the marks are propagated correctly.
  * We should use boolean marks, as no additional information is required to be carried. When introducing the marks for these, extra care should be taken in *all* the places marks are handled and ensure that the existing implementation around marks is not affected.
* Based on the marks, ensure that the output cannot be used in other contexts than the ephemeral ones (see the User Documentation section for more details on where this is allowed).

> [!TIP]
>
> For an example on how to properly introduce a new mark in the outputs, you can check the [PR](https://github.com/opentofu/opentofu/pull/2633) for the deprecated outputs.

Strict rules:
* A root module cannot define ephemeral outputs since are useless.
* Any output that wants to use an ephemeral value, it needs to be marked as ephemeral too. Otherwise, it needs to show an error:
   ```hcl
    │ Error: Output not marked as ephemeral
    │
    │   on mod/main.tf line 33, in output "password":
    │   33:   value = reference.to.ephemeral.value
    │
    │ In order to allow this output to store ephemeral values add `ephemeral = true` attribute to it.
   ```
* Any output from a root module that is referencing a write only attribute needs to be marked as sensitive too. Otherwise, an error should be raised
  ```hcl
    │ Error: An output referencing a sensitive value needs to be marked with sensitive too
    │
    │   on main.tf line 32:
    │   32: output "write_only_out" {
    │
    │ For security reasons, OpenTofu requires any output that is referencing a sensitive value to also be configured the same. If the root module really wants to export this sensitive value, you need to annotate it with the following argument:
    │     sensitive = true
  ```

Considering the rules above, root modules cannot have any ephemeral outputs defined.

### Locals
Any `local` declaration will be marked as ephemeral if in the expression that initialises it an ephemeral value is used:
```hcl
variable "var1" {
  type = string
}

variable "var2" {
  type = string
}

variable "var3" {
  type      = string
  ephemeral = true
}

locals {
  eg1 = var.var1 == "" ? var.var2 : var.var1 // not ephemeral
  eg2 = var.var2 // not ephemeral
  eg3 = var.var3 == "" ? var.var2 : var.var1 // ephemeral because of var3 conditional
  eg4 = var.var1 == "" ? var.var2 : var.var3 // ephemeral because of var3 usage
  eg5 = "${var.var3}-${var.var1}" // ephemeral because of var3 usage
  eg6 = local.eg4 // ephemeral because of eg4 is ephemeral
}
```

Once a local is marked as ephemeral, this can be used only in other ephemeral contexts. Check the `User Documentation` section for more details on the allowed contexts.

### Ephemeral resources
Due to the fact ephemeral resources are not stored in the state/plan file, this block is not creating a diff in the OpenTofu's UI.
Instead, OpenTofu will notify the user of opening/renewing/closing an ephemeral resource with messages similar to the following:
```bash
ephemeral.playground_random.password: Opening...
ephemeral.playground_random.password: Opening succeeded after 0s
ephemeral.playground_random.password: Closing...
ephemeral.playground_random.password: Closing succeeded after 0s
```

Ephemeral resources lifecycle is similar with the data blocks:
* Both basic implementations require the same methods (`Metadata` and `Schema`) while datasource is defining `Read` compared with the ephemeral resource that is defining `Open`. When talking about the basic functionality of the ephemeral resources, the `Read` method will behave similarly with the `Read` on a datasource, where it reads the data.
* Also, both blocks support `Configure`, `ConfigValidators` and `ValidateConfig` as extensions of the basic definition.
* Ephemeral resources do support two more operations in contrast with datasources:
  * `Renew`
    * Together with the data returned by the `Open` method call, the provider can also specify a `RenewAt` which will be a specific moment in time when OpenTofu should call the `Renew` method to get an updated information from the ephemeral resource. OpenTofu will have to check for `RenewAt` value anytime it intends to use the value returned by the ephemeral resource.
  * `Close`
    * When an ephemeral resource is having this method defined, it is expecting it to be called in order to release a possible held resource. A good example of this is with a Vault provider that could provide a secret by obtaining a lease, and when the secret is done being used, OpenTofu should call `Close` on that ephemeral resource to instruct on releasing the lease and revoking the secret.

To sum the above details, ephemeral resources are having 1 mandatory method and several optional methods:
* required
  * Schema - will not get in details of this in this RFC since the usage of this is similar with what we are doing for any other data types from a provider
  * Open
* optional
  * Renew
  * Close

#### Basic OpenTofu handling of ephemeral resources
As per an initial analysis, the ephemeral blocks should be handled similarly to a data source block by allowing [ConfigTransformer](https://github.com/opentofu/opentofu/blob/26a77c91560d51f951aa760bdcbeecd93f9ef6b0/internal/tofu/transform_config.go#L100) to generate a NodeAbstractResource. This is needed because ephemeral resources lifecycle needs to follow the ones for resources and data sources where they need to have a graph vertices in order to allow other concepts of OpenTofu to create depedencies on it. 

The gRPC proto schema is already updated in the OpenTofu project and contains the methods and data structures necessary for the epehemeral resources.
In order to make that available to be used, [providers.Interface](https://github.com/opentofu/opentofu/blob/26a77c91560d51f951aa760bdcbeecd93f9ef6b0/internal/providers/provider.go#L109) needs to get the necessary methods and implement those in [GRPCProviderPlugin (V5)](https://github.com/opentofu/opentofu/blob/26a77c91560d51f951aa760bdcbeecd93f9ef6b0/internal/plugin/grpc_provider.go#L31) and [GRPCProviderPlugin (V6)](https://github.com/opentofu/opentofu/blob/26a77c91560d51f951aa760bdcbeecd93f9ef6b0/internal/plugin6/grpc_provider.go#L31).

#### Configuration model
Beside the attributes that are defined by the provider for an ephemeral resource, the following meta-arguments needs to be supported by any ephemeral block:
* lifecycle
* count
* for_each
* depends_on
* provider

#### `Open` method details
When OpenTofu will have to use an ephemeral resource, it needs to call its `Open` method, passing over the config of the ephemeral resource.

The call to the `Open` method will return the following data:
* `Private` that OpenTofu is not going to use in other contexts than calling the provider `Close` or `Renew` optionally defined methods.
* `Result` that will contain the actual ephemeral information. This is what OpenTofu needs to handle to make it available to other ephemeral contexts to reference.
* `RenewAt` being an optional timestamp indicating when OpenTofu will have to call `Renew` method on the provider before using again the data from the `Result`.

Observations:
* In the `Result`, OpenTofu is epecting to find any non-computed given values in the request, otherwise will return an error.
* In the `Result`, the fields marked as computed can be either null or have an actual value. If an unknown if found, OpenTofu will return an error.

> [!NOTE]
>
> If any information in the configuration of an ephemeral resource is unknown during the `plan` phase, OpenTofu needs to defer the provisioning of the resource for the `apply` phase.

#### `Renew` method details
The `Renew` method is called only if the response from `Open` or another `Renew` call is containing a `RenewAt`.
When `RenewAt` is present, OpenTofu, before using the `Result` from the `Open` method response, will check if the current timestamp is at or over `RenewAt` and will call the `Renew` method by providing the previously returned `Private` information.

> [!NOTE]
> 
> `Renew` does not return a *new* information meant to replace the initial `Result` returned by the `Open` call.
> Due to this, `Renew` is only useful for system similar to Vault where the lease can be renewed without generating new data.

#### `Close` method details
When OpenTofu is done using an ephemeral resource, it needs to call its `Close` method to ensure that any remote data associated with the data returned in `OpenResponse.Result` is released and/or cleaned up properly.
The `Renew` request is requiring the latest `Private` data returned by the call to `Open` or `Renew` method.

#### `ConfigValidators` and `ValidateConfig` methods details
There is not much to say here, since this is the same lifecycle that a datasource is having.

#### Testing support
The testing support will be documented later into a different RFC, or as amendment to this one.

### Support in already ephemeral contexts
There are already OpenTofu contexts that are not saved in state/plan file:
* `provider` configuration
* `provisioner` blocks
* `connection` blocks

In all of these, referencing an ephemeral value should work as normal.

### Utilities
#### `terraform.applying`
The `terraform.applying` needs to be introduced to allow the user to check if the current command that is running is `apply` or not.
This is useful when user wants to configure different properties between write operations and read operations.

`terraform.applying` will be set to `true` when `tofu apply` is executed and `false` in any other command.

> [!NOTE]
>
> This keyword is related to the `apply` command and not to the `apply` phase, meaning that when running `tofu apply`, `terraform.applying` will still be `true` also during the `plan` phase of the `apply` command.

This is an ephemeral value that should be handled correctly and ensure that its value or any other value generate from it will not end up in a plan/state file.

#### `ephemeralasnull` function
`ephemeralsnull` function is useful when an object that was built by referencing an ephemeral value wants to be used into a non-ephemeral context.
This is getting a dynamic value and by traversing it, is looking for any ephemeral value and is nullifying it, but it does not nullify any non-ephemeral value within the object.

For example:
```hcl
variable "secret" {
  type = string
  default = "test"
  ephemeral = true
}

locals {
  config = {
    "non-ephemeral": "non-ephemeral-value"
    "ephemeral": var.secret
  }
}

output "test" {
  value = ephemeralasnull(local.config)
}
```

Which after running `tofu apply` should show an output like this:
```hcl
test = {
  "ephemeral" = tostring(null)
  "non-ephemeral" = "non-ephemeral-value"
}
```

This function should work perfectly fine also with a non-ephemeral value.

## Open Questions

Some questions that are also scattered across the RFC:
* Any ideas why the terraform-plugin-framework does not allow write-only SetAttribute, SetNestedAttribute and SetNestedBlock?
  * Based on my tests, MapNestedAttribute is allowed (together with other types).
  * Some info [here](https://github.com/hashicorp/terraform-plugin-framework/pull/1095).
* Considering the early evaluation supported in OpenTofu, could blocks like `provider`, `provisioner` and `connection` be configured with such outputs? Or there is no such thing as "early evaluating a module"? 


## Future Considerations

Website documentation that needs to be updated later:
* write-only - add also some hands-on with generating an ephemeral value and pass it into a write-only attribute
* variables - add an in-depth description of the ephemeral attribute in the variables page
* outputs - add an in-depth description of the ephemeral attribute in the outputs page
