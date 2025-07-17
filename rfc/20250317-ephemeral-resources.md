# Ephemeral resources, variables, outputs, locals and write-only arguments

Issue: https://github.com/opentofu/opentofu/issues/1996

Right now, OpenTofu information for resources and outputs are written to state and plan files as it is. This is presenting a security risk
as some information from the stored objects can contain sensitive bits that can become visible to whoever is having access to the state or plan files.

To provide a better solution for the aforementioned situation, OpenTofu introduces the concept of "ephemerality" which is meant to make use of the already existing functionality in terraform-plugin-framework.
Any new feature under this new concept should provide ways to skip values from being written to the state and plan files.

This new concept is going to offer another way to tackle the aforementioned issue, adding one more option in OpenTofu to choose for securing the plan and state files.
Here are the other existing options, providing different levels of safety:
* sensitive marked outputs and variables
  * The values marked this way ensures only that the information is sanitized from the user interface, but these are still stored in plaintext in the state and plan files.
* state encryption (recommended way)
  * This is meant to provide in transit and at rest encryption of the state and plan files, regardless of what providers/modules offer.
  * By using this you don't need to choose what to store and what not, but everything is safely stored.
  * This is also preventing state tempering and [privilege escalation](https://www.plerion.com/blog/hacking-terraform-state-for-privilege-escalation).

## Proposed Solution
Two new concepts will be introduced:
* `ephemeral` resources
* `resource`'s `write-only` attributes

Several existing features will have to be able to be updated with the new functionality:
* variables
* outputs
* locals
* providers
* provisioners
* `connection` block

> [!NOTE]
>
> In order to provide a similar and familiar UX for the users, the proposal in this RFC is heavily inspired by the Terraform public documentation and available blog posts on the matter.

In the attempt to provide to the reader an in-depth understanding of the ephemerality implications in OpenTofu,
this section will try to explain the functional approach of the new concept in each existing feature.

### User Documentation
#### Write-only attributes
This is a new concept that allows any existing `resource` to define attributes in its schema that can be only written without the ability to retrieve the value afterwards.

By not being readable, this also means that an attribute configured by a provider this way should not be written to the state or plan file either.
Therefore, these attributes are suitable for configuring specific resources with sensitive data, like passwords, access keys, etc.

A write-only attribute can accept an ephemeral or a non-ephemeral value, even though it's recommended to use ephemeral values for such attributes.

Because these attributes are not written to the plan file, updating a write-only attribute is getting a bit trickier.
Provider implementations do generally include also a "version" argument linked to the write-only one.
For example having a write-only argument called `secret`, providers should also include
a non-write-only argument called `secret_version`. Every time the user wants to update the value of `secret`, it needs to change the value of `secret_version` to trigger a change.
The provider implementation is responsible with handling this particular case: because the version attribute is stored also in the state, the provider needs to compare the value from the state with the one from the configuration and in case it differs, it will trigger the update of the `secret` attribute.

At the time of writing this RFC, write-only attributes are supported by a low number of providers and resources.
Having the `aws_db_instance` as one of those, here is an example on how to use the write-only attributes:
```hcl
resource "aws_db_instance" "example" {
  // ...
  password_wo         = "your-initial-password"
  password_wo_version = 1
  // ...
}
```
By updating **only** the `password_wo`, on the `tofu apply`, the password should not be updated.
To do so, the `password_wo_version` needs to be incremented too:
```hcl
resource "aws_db_instance" "example" {
  // ...
  password_wo         = "new-password"
  password_wo_version = 2
  // ...
}
```

As seen in this particular change of the [terraform-plugin-framework](https://github.com/hashicorp/terraform-plugin-framework/commit/ecd80f67daed0b92b243ae59bb1ee2077f8077c7), the write-only attribute cannot be configured for [set attributes](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/attributes/set), [set nested attributes](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/attributes/set-nested), and [set nested blocks](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/blocks/set-nested).

Write-only attributes cannot generate a plan diff.
This is because the prior state does not contain a value that OpenTofu can use to compare the new value against and
also the value provider returns during planning of a write-only argument should always be null.
This means that there could be inconsistencies between plan and apply for the write-only arguments.

#### Variables
Any `variable` block can be marked as ephemeral.
```hcl
variable "ephemeral_var" {
  type      = string
  ephemeral = true
}
```
OpenTofu should allow usage of these variables only in other ephemeral contexts:
* write-only arguments
* other ephemeral variables
* ephemeral outputs
* local values
* ephemeral resources
* provisioner blocks
* connection blocks
* provider configuration

Usage in any other place should raise an error:
```hcl
│ Error: Invalid use of an ephemeral value
│
│   with playground_secret.store_secret,
│   on main.tf line 30, in resource "playground_secret" "store_secret":
│   30:   secret_name = var.password
│
│ "secret_name" cannot accept an ephemeral value because it is not a write-only attribute, meaning it will be written to the state.
╵
```

For being able to use ephemeral variables, the module's authors need to mark those as so.
If OpenTofu finds an ephemeral value given to a non-ephemeral variable in a module call, an error will be shown:
```hcl
│ Error: Invalid usage of ephemeral value
│
│   on main.tf line 21, in module "secret_management":
│   21:   secret_map     = var.secrets
│
│ Variable `secret_map` is not marked as ephemeral. Therefore, it cannot reference an ephemeral value. In case this is actually wanted, you can add the following attribute to its declaration:
│   ephemeral = true
```


OpenTofu should not store ephemeral variable(s) in plan files.
If a plan is generated from a configuration that is having at least one ephemeral variable,
when the plan file will be applied, the value(s) for the ephemeral variable(s) needs to be provided again.

#### Outputs
Most `output` blocks can be configured as ephemeral.

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
* locals
* `provisioner` block
* `connection` block

An `output` block from a root module cannot be marked as ephemeral.
This limitation is natural since ephemeral outputs are meant to be skipped from the state file.
Therefore, there is no use for such a defined output block in a root module.
When encountering an ephemeral output in a root module, an error similar to this one should be shown:
```hcl
│ Error: Unallowed ephemeral output
│
│   on main.tf line 36:
│   36: output "write_only_out" {
│
│ Root module is not allowed to have ephemeral outputs
```

Ephemeral outputs are useful when a child module returns sensitive data,
allowing the caller to use the value of that output in other ephemeral contexts.
When using outputs in non-ephemeral contexts, OpenTofu should show an error similar to the following:
```hcl
│ Error: Invalid use of an ephemeral value
│
│   with aws_secretsmanager_secret_version.store_from_ephemeral_output,
│   on main.tf line 31, in resource "aws_secretsmanager_secret_version" "store_from_ephemeral_output":
│   31:   secret_string = module.secret_management.secrets
│
│ "secret_string" cannot accept an ephemeral value because it is not a write-only attribute, meaning it will be written to the state.
╵
```

Any output that wants to use an ephemeral value must also be marked as ephemeral.
Otherwise, it needs to show an error:
```hcl
│ Error: Output not marked as ephemeral
│
│   on mod/main.tf line 33, in output "password":
│   33:   value = reference.to.ephemeral.value
│
│ In order to allow this output to store ephemeral values add `ephemeral = true` attribute to it.
```
> [!NOTE]
>
> It needs to be said that the last error will be raised only when a non-ephemeral output references an ephemeral value.
> However, an ephemeral marked output needs to be allowed to reference a non-ephemeral value.

#### Locals
Local values are automatically marked as ephemeral if any of the value that is used to compute the local is already an ephemeral one.

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
* `provider` blocks configuration
* `connection` and `provisioner` blocks

#### Ephemeral resource
In contrast with the write-only arguments where only specifically tagged attributes are not stored in the state/plan file, `ephemeral` resources must not be stored in the state file and have only a reference stored in the plan file.

The ephemeral blocks are behaving similar to `data`, where it reads the indicated resource and once it's done with it, is going to close it.

Ephemeral resources can be referenced only in specific contexts:
* other ephemeral resources
* ephemeral variables
* ephemeral outputs
* locals
* to configure `provider` blocks
* in `provisioner` and `connection` blocks
* in write-only arguments

For example, you can have an ephemeral resource that is retrieving the password from a secret manager, password that can be passed later into a write-only attribute of another normal `resource`.
To do so, the flow of an ephemeral resource should look similar to the following:
* Requests the information from the provider.
* It is passed into the evaluation context, which will be used to evaluate expressions referencing it.
  * It does not store the value in the plan or state file.
* When accessing the value, OpenTofu will have to check for the presence of the `RenewAt`:
  * If the timestamp is having a value and the current timestamp is at or over the timestamp indicated by `RenewAt`, call the provider `Renew` method.
* When a reference is found to an ephemeral resource, OpenTofu should double check that the attribute referencing it is allowed to do so.
  * See the contexts above where an ephemeral resource can be referenced.
* At the end, the ephemeral resource needs to be closed. OpenTofu should call `Close` on the provider for all opened ephemeral resources.

Besides the attributes in a schema of an ephemeral resource, the block should also support the meta-arguments existing in OpenTofu:
* `depends_on`
* `count`
* `for_each`
* `provider`
* `lifecycle`

The only `lifecycle` content that ephemerals should support are `precondition` and `postcondition`. In case OpenTofu will find other known attributes of the `lifecycle` block, it should show an error similar to the following:
```
│ Error: Invalid lifecycle configuration for ephemeral resource
│                                                           
│   on ../mod/main.tf line 44, in ephemeral "aws_secretsmanager_secret_version" "secret_retrieval":                     
│   44:     create_before_destroy = true                    
│                                                           
│ The lifecycle argument "create_before_destroy" cannot be used in ephemeral resources. This is meant 
│ to be used strictly in "resource" blocks.
```

The meta-arguments `provisioner` and `connection` should not be supported.

#### Providers
`provider` block is ephemeral by nature, meaning that the configuration of this is never stored into state/plan file.

Therefore, this block should be configurable by using ephemeral values.

#### `provisioner` block
As `provisioner` information is not stored into the plan/state file, this can reference ephemeral values like ephemeral variables, outputs, locals and values from ephemeral resources.

Whenever doing so, the output of the provisioner execution should be suppressed:
```
(local-exec): (output suppressed due to ephemeral value in config)
```
#### `connection` block
When the `connection` block is configured, this should be allowed to use ephemeral values from variables, outputs, locals and values from ephemeral resources.

### Example of how the new changes will work together
To better understand how all of this should work in OpenTofu, let's take a look at a comprehensive example.
#### Configuration
<details>
<summary>./mod/main.tf</summary>

```hcl
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "6.0.0-beta1"
    }
  }
}

variable "secret_map" {
  type = map(string)
  default = {}
  ephemeral = true # (1)
}

variable "secret_version" { # (2)
  type    = number
  default = 1
}

variable "secret_manager_arn" {
  type    = string
  default = ""
}

resource "aws_secretsmanager_secret" "manager" {
  count = var.secret_version > 1 ? 1 : 0
  name  = "ephemeral-rfc-example"
}

resource "aws_secretsmanager_secret_version" "secret_creation" {
  count                    = var.secret_version > 1 ? 1 : 0
  secret_id                = aws_secretsmanager_secret.manager[0].arn
  secret_string_wo = jsonencode(var.secret_map) # (3)
  secret_string_wo_version = var.secret_version
}

ephemeral "aws_secretsmanager_secret_version" "secret_retrieval" { # (4)
  count     = var.secret_version > 1 ? 1 : 0
  secret_id = aws_secretsmanager_secret.manager[0].arn
  depends_on = [
    aws_secretsmanager_secret_version.secret_creation
  ]
}

ephemeral "aws_secretsmanager_secret_version" "secret_retrieval_direct" {
  count     = var.secret_version > 1 ? 0 : 1
  secret_id = var.secret_manager_arn
}

output "secrets" {
  value = "${var.secret_version > 1 ?
    jsondecode(ephemeral.aws_secretsmanager_secret_version.secret_retrieval[0].secret_string) :
    jsondecode(ephemeral.aws_secretsmanager_secret_version.secret_retrieval_direct[0].secret_string)}"
  ephemeral = true # (5)
}

output "secret_manager_arn" {
  value = var.secret_version > 1 ? aws_secretsmanager_secret.manager[0].arn : null
}
```
This module can be used for two separate operations:
1. When `secret_version` is having a value greater than 1, it will add the given `secret` into a secret manager.
2. When `secret_version` is not given, it will use the `secret_manager_arn` to read the secret by using an ephemeral resource.

Details:
- (1) Variables should be able to be marked as ephemeral. By doing so, those should be able to be used only in ephemeral contexts.
- (2) Version field that is going together with the actual write-only argument to be able to update the value of it. To upgrade the secret, the version field needs to be updated, otherwise OpenTofu should generate no diff for it.
- (3) `aws_secretsmanager_secret_version.secret_creation.secret_string_wo` is the write-only attribute that is receiving the `secret` variable which is ephemeral (even though a write-only argument can use also a non-ephemeral value).
- (4) Using ephemeral resource to retrieve the secret. Maybe looks a little bit weird, because right above we are having the resource of the same type that is looking like it should be able to be used to get the secret. In reality, because that `resource` is using the write-only attribute `secret_string_wo` to store the information, that field is going to be null when referenced.
- (5) Module output that is referencing an ephemeral value, it needs to be marked as ephemeral too. Otherwise, OpenTofu should generate an error.
  - This is similar to the behavior that is already present for `sensitive` values.
</details>

<details>
<summary>./store/main.tf</summary>

```hcl
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "6.0.0-beta1"
    }
  }
}
provider "aws" {
  alias = "secrets-read-write"
}

variable "access_key" {
  type      = string
  ephemeral = false # (1)
}

variable "secret_key" {
  type      = string
  ephemeral = false
}

locals {
  secrets = {
    "access_key" : var.access_key,
    "secret_key" : var.secret_key
  }
}

module "secret_management" {
  providers = {
    aws : aws.secrets-read-write
  }
  source         = "../mod"
  secret_map     = local.secrets
  secret_version = 2
}

output "secret_manager_arn" {
  value = module.secret_management.secret_manager_arn
}
```
This file is a configuration used to manage the secrets from the secret manager and just 
outputs the ARN of the secret manager to be used later.

Details:
- (1) The variable that is going to be used in an ephemeral variable, is not required to be ephemeral. The value can also be a hardcoded value without being ephemeral.
</details>

<details>
<summary>./read/main.tf</summary>

```hcl
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "6.0.0-beta1"
    }
  }
}
provider "aws" {
  alias = "read-secrets"
}

variable "secret_manager_arn" {
  type = string
}

module "secret_management" { # (1)
  providers = {
    aws : aws.read-secrets
  }
  source             = "../mod"
  secret_manager_arn = var.secret_manager_arn
}

provider "aws" {
  alias      = "dev-access"
  access_key = module.secret_management.secrets["access_key"] # (2)
  secret_key = module.secret_management.secrets["secret_key"]
}

resource "aws_ssm_parameter" "store_ephemeral_in_write_only" { # (3)
  provider         = aws.dev-access
  name             = "parameter_from_ephemeral_value"
  type             = "SecureString"
  value_wo = jsonencode(module.secret_management.secrets) # (4)
  value_wo_version = 1

  provisioner "local-exec" {
    when    = create
    command = "echo non-ephemeral value: ${aws_ssm_parameter.store_ephemeral_in_write_only.arn}"
  }

  # provisioner "local-exec" { # (5)
  #   when    = create
  #   command = "echo write-only value: ${aws_ssm_parameter.store_ephemeral_in_write_only.value_wo}"
  # }

  provisioner "local-exec" {
    when    = create
    command = "echo ephemeral value from module: #${jsonencode(module.secret_management.secrets)}#" # (6)
  }
}
```
This configuration is using the same module to retrieve the secret by using an ephemeral resource and 
is using it to create a new resource, passing the ephemeral value into a write-only attribute.

Details:
- (1) Calling the module that we defined previously just to retrieve the secret by using an ephemeral value.
- (2) Use the secret from the module to configure the `aws.dev-access` provider.
- (3) Here we used `aws_ssm_parameter` which can be configured with write-only arguments.
- (4) Referencing a module ephemeral output to ensure that the ephemeral information is passed correctly between two modules.
- (5) This `provisioner` block is commented out because interpolation of null values is not allowed in OpenTofu. Reminder: a write-only argument will always be returned as null from the provider even when the configuration is actually having a value.
- (6) A provisioner that is referencing an ephemeral value (module output) should have its output suppressed. Details in [Write-only arguments under Technical Approach](#write-only-arguments)
</details>

#### CLI Output

<details>
<summary>Applying `store` configuration</summary>

```
module.secret_management.ephemeral.aws_secretsmanager_secret_version.secret_retrieval[0]: Configuration unknown, deferring...
# ^^^ (1)
 
OpenTofu used the selected providers to generate the following execution plan. Resource actions are indicated with
the following symbols:
  + create

OpenTofu will perform the following actions:

  # module.secret_management.aws_secretsmanager_secret.manager[0] will be created
  + resource "aws_secretsmanager_secret" "manager" {
      + name                           = "ephemeral-rfc-example"
      # ...
    }

  # module.secret_management.aws_secretsmanager_secret_version.secret_creation[0] will be created
  + resource "aws_secretsmanager_secret_version" "secret_creation" {
      + secret_string_wo         = (write-only attribute)
      + secret_string_wo_version = 2
      # ...
    }
# ^^^ (2)
Plan: 2 to add, 0 to change, 0 to destroy.

Changes to Outputs:
  + secret_manager_arn = (known after apply)

Do you want to perform these actions?
  OpenTofu will perform the actions described above.
  Only 'yes' will be accepted to approve.

  Enter a value: yes

module.secret_management.aws_secretsmanager_secret.manager[0]: Creating...
module.secret_management.aws_secretsmanager_secret.manager[0]: Creation complete after 0s [id=arn:aws:secretsmanager:AWS_REGION:ACC_ID:secret:ephemeral-rfc-example-WGZP9D]
module.secret_management.aws_secretsmanager_secret_version.secret_creation[0]: Creating...
module.secret_management.aws_secretsmanager_secret_version.secret_creation[0]: Creation complete after 0s [id=arn:aws:secretsmanager:AWS_REGION:ACC_ID:secret:ephemeral-rfc-example-WGZP9D]
module.secret_management.ephemeral.aws_secretsmanager_secret_version.secret_retrieval[0]: Opening... # <--- (3)
module.secret_management.ephemeral.aws_secretsmanager_secret_version.secret_retrieval[0]: Opening complete after 0s
module.secret_management.ephemeral.aws_secretsmanager_secret_version.secret_retrieval[0]: Closing... # <--- (4)
module.secret_management.ephemeral.aws_secretsmanager_secret_version.secret_retrieval[0]: Closing complete after 0s

Apply complete! Resources: 2 added, 0 changed, 0 destroyed.

```

This is an output that would be visible when running `tofu apply` by using `store/main.tf`.

Details:
- (1) If an ephemeral block is referencing any unknown value, the opening is deferred for later, when the value will be known.
- (2) As can be seen, the ephemeral resources are not shown in the list of changes. The only mention of those is in the actual action logs where we can see that it opens and closing those.
- (3) This should be visible in the action logs, while an ephemeral resource will be opened.
- (4) This should be visible in the action logs, while an ephemeral resource will be closed.
</details>

<details>
<summary>Applying `read` configuration</summary>

```
module.secret_management.ephemeral.aws_secretsmanager_secret_version.secret_retrieval_direct[0]: Opening...
module.secret_management.ephemeral.aws_secretsmanager_secret_version.secret_retrieval_direct[0]: Opening complete after 0s
module.secret_management.ephemeral.aws_secretsmanager_secret_version.secret_retrieval_direct[0]: Closing...
module.secret_management.ephemeral.aws_secretsmanager_secret_version.secret_retrieval_direct[0]: Closing complete after 0s
# ^^^ (1)
OpenTofu used the selected providers to generate the following execution plan. Resource actions are indicated with the
following symbols:
  + create

OpenTofu will perform the following actions:

  # aws_ssm_parameter.store_ephemeral_in_write_only will be created
  + resource "aws_ssm_parameter" "store_ephemeral_in_write_only" {
      + name             = "parameter_from_ephemeral_value"
      + type             = "SecureString"
      + value            = (sensitive value)
      + value_wo         = (write-only attribute) # <--- (2)
      + value_wo_version = 1
      # ...
    }

Plan: 1 to add, 0 to change, 0 to destroy.

Do you want to perform these actions?
  OpenTofu will perform the actions described above.
  Only 'yes' will be accepted to approve.

  Enter a value: yes

module.secret_management.ephemeral.aws_secretsmanager_secret_version.secret_retrieval_direct[0]: Opening...
module.secret_management.ephemeral.aws_secretsmanager_secret_version.secret_retrieval_direct[0]: Opening complete after 0s
aws_ssm_parameter.store_ephemeral_in_write_only: Creating...
aws_ssm_parameter.store_ephemeral_in_write_only: Provisioning with 'local-exec'...
aws_ssm_parameter.store_ephemeral_in_write_only (local-exec): Executing: ["/bin/sh" "-c" "echo non-ephemeral value: arn:aws:ssm:AWS_REGION:ACC_ID:parameter/parameter_from_ephemeral_value"]
aws_ssm_parameter.store_ephemeral_in_write_only (local-exec): non-ephemeral value: arn:aws:ssm:AWS_REGION:ACC_ID:parameter/parameter_from_ephemeral_value
aws_ssm_parameter.store_ephemeral_in_write_only: Provisioning with 'local-exec'...
aws_ssm_parameter.store_ephemeral_in_write_only (local-exec): (output suppressed due to ephemeral value in config) # <--- (3)
aws_ssm_parameter.store_ephemeral_in_write_only (local-exec): (output suppressed due to ephemeral value in config)
aws_ssm_parameter.store_ephemeral_in_write_only: Creation complete after 0s [id=parameter_from_ephemeral_value]
module.secret_management.ephemeral.aws_secretsmanager_secret_version.secret_retrieval_direct[0]: Closing...
module.secret_management.ephemeral.aws_secretsmanager_secret_version.secret_retrieval_direct[0]: Closing complete after 0s
```

This is an output that would be visible when running `tofu apply` by using `read/main.tf`.

Details:
- (1) Because the only resource that this configuration is going to create is referencing an ephemeral resource from the module, the ephemeral resource is accessed during plan phase too.
- (2) Write-only arguments are not going to be shown in the UI either.
- (3) This is how a provisioner output should look like when an ephemeral value is used in the expression.

</details>

## Technical Approach
> [!NOTE]
>
> Any rule ending in `If any found, an error will be raised.` is having an error defined in the [User Documentation](#user-documentation) section.

In this section, as in the "Proposed Solution" section, we'll go over each concept, but this time with a more technical focus.

### Write-only arguments
Most of the write-only arguments logic is already in the [provider-framework](https://github.com/hashicorp/terraform-plugin-framework):
* [Initial implementation](https://github.com/hashicorp/terraform-plugin-framework/pull/1044)
* [Sets comparison enhancement](https://github.com/hashicorp/terraform-plugin-framework/pull/1064)
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
> Write-only attributes should be presented in the OpenTofu's UI as `(write-only attribute)` instead of the actual value.

### Variables

For enabling ephemeral variables, these are the basic steps that need to be taken:
* Update config to support the `ephemeral` attribute.
* Mark the variables with a new mark and ensure that the marks are propagated correctly.
  * Recently, [2503](https://github.com/opentofu/opentofu/pull/2503) has been merged. This changed the way OpenTofu is handling marks. Before 2503, there was only one mark (sensitive), so there were places that treated a marked value as a sensitive one, but when a new mark (deprecated) has been introduced, we had to fix the way OpenTofu considers sensitive values.
* Based on the marks, ensure that the variable cannot be used in other contexts than the ephemeral ones (see the [User Documentation](#user-documentation) section for more details on where this is allowed). If any found, an error will be raised.
* Check the state of [#1998](https://github.com/opentofu/opentofu/pull/1998). If that is merged, in the changes where variables from plan are verified against the configuration ones, we also need to add a validation on the ephemerality of variables. If the variable is marked as ephemeral, then the plan value is allowed (expected) to be missing.
* Ensure that when the prompt is shown for an ephemeral variable, there is indication of that:
  ```hcl
  var.password (ephemeral)
     Enter a value:
  ```
* If a module has an ephemeral variable declared, that variable can get values from any source, even another non-ephemeral variable.
* A variable not marked as ephemeral should not be able to reference an ephemeral value. A non-ephemeral variable will not become ephemeral when referencing an ephemeral value. If any found, an error will be raised.

We should use boolean marks, as no additional information is required to be carried. When introducing the marks for these, extra care should be taken in *all* the places marks are handled and ensure that the existing implementation around marks is not affected.

> [!NOTE]
>
> When adding the mark for ephemeral, ensure that there are unit tests to confirm that multiple mark types can work together:
> * When an ephemeral marked value is marked also with deprecated/sensitive, all marks are present on the value.
> * When an ephemeral marked value is unmarked for some operations, the other marks are still carried over.

### Outputs

For enabling ephemeral outputs, these are the basic steps that need to be taken:
* Update config to support the `ephemeral` attribute.
* Mark the outputs with a new mark and ensure that the marks are propagated correctly.
  * We should use boolean marks, as no additional information is required to be carried. When introducing the marks for these, extra care should be taken in *all* the places marks are handled and ensure that the existing implementation around marks is not affected.
* Based on the marks, ensure that the output cannot be used in other contexts than the ephemeral ones (see the [User Documentation](#user-documentation) section for more details on where this is allowed). If any found, an error will be raised.

> [!INFO]
>
> For an example on how to properly introduce a new mark in the outputs, you can check the [PR](https://github.com/opentofu/opentofu/pull/2633) for the deprecated outputs.

Strict rules:
* A root module cannot define ephemeral outputs. If any found, an error will be raised.
* Any output that wants to use an ephemeral value must also be marked as ephemeral. If any found, an error will be raised.
* Any output referencing an ephemeral value needs to be marked as ephemeral too. If any found, an error will be raised.
* Any output from a root module that is referencing a write-only attribute needs to be marked as sensitive. If any found, an error will be raised.
* Any output marked as ephemeral should be able to reference a non-ephemeral value.

Considering the rules above, root modules cannot have any ephemeral outputs defined.

### Locals
Any `local` declaration should be marked as ephemeral if in the expression that initialises it an ephemeral value is used:
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

Once a local is marked as ephemeral, this can be used only in other ephemeral contexts. Check the `Proposed Solution` section for more details on the allowed contexts.

### Ephemeral resources
Due to the fact ephemeral resources are not stored in the state, this block is not creating a diff in the OpenTofu's UI.
Instead, OpenTofu should notify the user of opening/renewing/closing an ephemeral resource with messages similar to the following:
```bash
ephemeral.playground_random.password: Opening...
ephemeral.playground_random.password: Opening succeeded after 0s
ephemeral.playground_random.password: Closing...
ephemeral.playground_random.password: Closing succeeded after 0s
```

Methods that an ephemeral resource should/could have:
* Required:
  * Open - should be called on the provider to read the information of the indicated resource.
  * Metadata - returns the type name of the ephemeral resource.
  * Schema - should be called on the provider to get the schema defined for the ephemeral resource.
* Optional:
  * Renew - if the response from `Open` contains a valid `RenewAt`, OpenTofu should call this method in order to instruct the provider to renew any possible remote information related to the secret returned from the `Open` call.
  * Close - should be called on the provider to clean any possible remote information related to the secret returned in the response from `Open`.
  * ValidateConfig - validates the configuration provided for the ephemeral resource.

Ephemeral resources lifecycle is similar to the data blocks:
* Both basic implementations require the same methods (`Metadata` and `Schema`) while the datasource defines `Read` compared with the ephemeral resource defining `Open`. When talking about the basic functionality of the ephemeral resources, the `Open` method should behave similarly to the `Read` on a datasource, where it asks the provider for the data associated with that particular ephemeral resource.
* Both also include `ValidateConfig` as extension of the basic definition.
* Ephemeral resources do support two more operations in contrast with datasources:
  * `Renew`
    * Together with the data returned by the `Open` method call, the provider can also specify a `RenewAt` which will be a specific moment in time when OpenTofu should call the `Renew` method to trigger an update on the remote information related with the secret returned from the `Open` call. OpenTofu will have to check for `RenewAt` value anytime it intends to use the value returned by the ephemeral resource.
  * `Close`
    * When an ephemeral resource is having this method defined, OpenTofu should call it in order to release a possible held resource before the `provider.Close` is called. A good example of this is with a Vault/OpenBao provider that could provide a secret by obtaining a lease, and when the secret is done being used, OpenTofu should call `Close` on that ephemeral resource to instruct on releasing the lease and revoking the secret.

#### OpenTofu handling of ephemeral resources
As per an initial analysis, the ephemeral blocks should be handled similarly to a data source block by allowing [ConfigTransformer](https://github.com/opentofu/opentofu/blob/26a77c91560d51f951aa760bdcbeecd93f9ef6b0/internal/tofu/transform_config.go#L100) to generate a NodeAbstractResource. This is needed because ephemeral resources lifecycle needs to follow the ones for resources and data sources where they need to have a graph vertices in order to allow other concepts of OpenTofu to create depedencies on it.

The gRPC proto schema is already updated in the OpenTofu project and contains the methods and data structures necessary for the epehemeral resources.
In order to make that available to be used, [providers.Interface](https://github.com/opentofu/opentofu/blob/26a77c91560d51f951aa760bdcbeecd93f9ef6b0/internal/providers/provider.go#L109) needs to get the necessary methods and implement those in [GRPCProviderPlugin (V5)](https://github.com/opentofu/opentofu/blob/26a77c91560d51f951aa760bdcbeecd93f9ef6b0/internal/plugin/grpc_provider.go#L31) and [GRPCProviderPlugin (V6)](https://github.com/opentofu/opentofu/blob/26a77c91560d51f951aa760bdcbeecd93f9ef6b0/internal/plugin6/grpc_provider.go#L31).

#### Configuration model
Beside the attributes that are defined by the provider for an ephemeral resource, the following meta-arguments needs to be supported by any ephemeral block:
* lifecycle
  * The only attributes supported by the `lifecycle` in `ephemeral` blocks context are `precondition` and `postcondition`. If any found, an error will be raised.
* count
* for_each
* depends_on
* provider

#### `Open` method details
When OpenTofu will have to use an ephemeral resource, it needs to call its `Open` method, passing over the config of the ephemeral resource.

The call to the `Open` method will return the following data:
* `Private` that OpenTofu is not going to use in other contexts than calling the provider `Close` or `Renew` optionally defined methods.
* `Result` will contain the actual ephemeral information. This is what OpenTofu needs to handle to make it available to other ephemeral contexts to reference.
* `RenewAt` timestamp indicating when OpenTofu should call `Renew` method on the provider before using the data from the `Result`.

Observations:
* In the `Result`, OpenTofu is expecting to find any non-computed given values in the request, otherwise should return an error.
* In the `Result`, the fields marked as computed can be either null or have an actual value. If an unknown if found, OpenTofu should return an error.

> [!NOTE]
>
> If any information in the configuration of an ephemeral resource is unknown during the `plan` phase, OpenTofu should defer the provisioning of the resource for the `apply` phase. This means that inconsistency can occur between the plan and the apply phase.

#### `Renew` method details
The `Renew` method is called only if the response from `Open` or another `Renew` call is containing a valid `RenewAt` value.
When `RenewAt` is present, OpenTofu, before using the `Result` from the `Open` method response, should check if the current timestamp is at or over `RenewAt` and should call the `Renew` method by providing the previously returned `Private` information, that could be from the `Open` call or a previous `Renew` call.

> [!NOTE]
>
> `Renew` does not return a *new* information meant to replace the initial `Result` returned by the `Open` call.
> Due to this, `Renew` is only useful for systems where an entity can be renewed without generating new data.

#### `Close` method details
Right before closing the provider, all the ephemeral resources that were open during the operation should be cleaned up. This means that OpenTofu needs to call `Close` on every opened ephemeral resource to ensure that any remote data associated with the data returned in `OpenResponse.Result` is released and/or cleaned up properly.

#### `ConfigValidators` and `ValidateConfig` methods details
There is not much to say here, since this is the same lifecycle that a datasource is having.

#### Checks stored in state
In case of the checks that an ephemeral resource can be configured with, the behavior of those should not be affected, meaning that even for the ephemeral resources, the results of blocks like `precondition` and `postcondition` should be stored in the state.

Having a configuration like the following:
```hcl
ephemeral "playground_secret" "test" {
  ...
  lifecycle {
    precondition {
      condition     = 1 == 1
      error_message = "your message here"
    }
  }
}
```
in the state file we should see the following:
```json
{
  "...": "...",
  "check_results": [
    {
      "object_kind": "resource",
      "config_addr": "ephemeral.playground_secret.test",
      "status": "pass",
      "objects": [
        {
          "object_addr": "ephemeral.playground_secret.test",
          "status": "pass"
        }
      ]
    }
  ]
}
```

#### Plan file
During the implementation of the ephemeral resources execution, we had to make a decision on how we can include ephemeral resources
in the execution graph for the `apply` phase, especially when `tofu` is executed with a plan file.
There were two immediate options available:
* Add a new graph transformer (or enhance the expansion one from the apply graph steps) to create a subgraph responsible with the creation of concrete nodes for the ephemeral resources in the configuration.
* Use the already planned ephemeral resources and store only a stub in the changes to enable the already existing `DiffTransformer` to create concrete graph nodes based on the changelist.

We went with the second to ensure that the flow is consistent between `tofu apply -auto-aprove` and `tofu plan -out planfile && tofu apply planfile`.

The information from the ephemeral resources that is stored in the plan file is limited to only the address of the resource and the action (ie: OPEN).
Anything else related to this type of changes, are trimmed out and must never be stored in the plan file.

> [!NOTE] 
> 
> For more details please refer to [#2985](https://github.com/opentofu/opentofu/pull/2985).
### Testing support
Due to the scope size this RFC is covering, the testing support will be documented later into a different RFC, or as amendment to this one.

### Support in existing ephemeral contexts
There are already OpenTofu contexts that are not saved in state/plan file:
* `provider` configuration
* `provisioner` blocks
* `connection` blocks

In all of these, referencing an ephemeral value should work as normal.

### Utilities
#### `tofu.applying`
The `tofu.applying` needs to be introduced to allow the user to check if the current command that is running is `apply` or not.
This is useful when the user wants to configure different properties between write operations and read operations.

`tofu.applying` should be set to `true` when `tofu apply` is executed and `false` in any other command.

> [!NOTE]
>
> This keyword is related to the `apply` command and not to the `apply` phase, meaning that when
> running `tofu apply`, `terraform.applying` should still be `true` also during the `plan` phase of 
> the `apply` command. This is `true` also when running a destroy operation.

This is an ephemeral value that should be handled accordingly, meaning that its value or any other value generated 
from it will not end up in a plan/state file.

> [!NOTE]
> 
> For feature parity, the same functionality under `tofu.applying` should be available under `terraform.applying` too.

#### `ephemeralasnull` function
`ephemeralasnull` function is useful when an object built by referencing an ephemeral value wants to be used into a non-ephemeral context.
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

This function should also work perfectly fine with a non-ephemeral value.

> [!NOTE]
>
> When we encounter an output in the root module that is referencing an ephemeral value, we could recommend to use `ephemeralasnull` to be able to store that information in the state.
> This would be a warning that will come together with the error diagnostic discussed in the ephemeral outputs section.

## Open Questions

Some questions that are also scattered across the RFC:
* Any ideas why the terraform-plugin-framework does not allow write-only SetAttribute, SetNestedAttribute and SetNestedBlock?
  * Based on my tests, MapNestedAttribute is allowed (together with other types).
  * Some info [here](https://github.com/hashicorp/terraform-plugin-framework/pull/1095).
* Considering the early evaluation supported in OpenTofu, could blocks like `provider`, `provisioner` and `connection` be configured with such outputs? Or there is no such thing as "early evaluating a module"?
* Considering that the `check` blocks can have a `data` block to be used for the assertions, should we consider adding also the `ephemeral` blocks support inside of the `check` blocks? Or should we have this as a possible future feature?


## Future Considerations

Website documentation that needs to be updated later:
* write-only - add also some hands-on with generating an ephemeral value and pass it into a write-only attribute
* variables - add an in-depth description of the ephemeral attribute in the variables page
* outputs - add an in-depth description of the ephemeral attribute in the outputs page
