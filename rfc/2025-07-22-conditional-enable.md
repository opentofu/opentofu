# Conditional field for enabling resources and modules

## Current status

Issue: https://github.com/opentofu/opentofu/issues/1306

> [!NOTE]
> Every time we refer to *resources* in this RFC, we're talking about [managed resources](https://opentofu.org/docs/language/resources/), [data sources](https://opentofu.org/docs/language/data-sources/) and [ephemeral resources](https://github.com/opentofu/opentofu/issues/2834).

Right now, OpenTofu supports conditional enable/disable of resources by using a workaround with `count`.
This approach brings a few problems, like adding indexes to resources that would be a single instance, making it harder to manage using these indexes.

This RFC proposes a new way to do a cleaner and semantic way by using a new field on the `lifecycle` block called `enabled`:

```
resource "aws_instance" "example" {
  # ...

  lifecycle {
    enabled = var.enable_server
  }
}
```

This is available not only to resources, but to modules too:

```
module "modcall" {
  source = "./mod"

  lifecycle {
    enabled = var.env == "production"
  }
}
```

## Proposed Solution

1. Raise errors if a resource is used while being disabled;
1. Support for conditional enabling on:
  1. Resources - Add a new field on the `lifecycle` block called `enabled`;
  1. Modules - Add a new `lifecycle` block for Modules but only supporting the `enabled` field.


## Technical Approach

### `lifecycle` support on Modules

Lifecycle block is currently not supported on `Modules`, but in our docs we're mentioning to users that it's reserved for future usage:

> OpenTofu does not use the lifecycle argument. However, the lifecycle block is reserved for future versions.

https://opentofu.org/docs/language/modules/syntax/#meta-arguments

It would be the right timing to start to support it, but only for the `enabled` field.

### Migration from `count` to `enabled`

Luckily, this support would be in-place already. When using a `count` for this conditional dependency, if
the resource is changed to use `enabled`, a `move` will be done on the apply phase to remove the index and
turn into a single instance.

```
OpenTofu will perform the following actions:

  # null_resource.example[0] has moved to null_resource.example
    resource "null_resource" "example" {
        id = "8975736315378968412"
    }

Plan: 0 to add, 0 to change, 0 to destroy.
```

### Usage of `enabled` together with `for_each` and `count`.

These three different arguments would behave similarly, but with different semantics.
It could be argued that you want to enable a resource with 4 instances, but on our current code,
you can already change that conditional by setting `count` to 0.

I propose that we do not support them together. You can only use one of them, returning errors if we try to use the others together.

Semantically, this feature would be used for single-instance resources or modules.

### Errors when resource is not enabled

There's some discussion about this [topic](https://github.com/opentofu/opentofu/issues/1306#issuecomment-2398120132), and the better compromise we found is that a enabled 
resource can be `null` when it's disabled, but no errors are going to be shown statically,
like when using `validate` or the language server, in an OpenTofu extension, like VSCode.
Since we need to evaluate the `enabled` expression, these errors are going to be shown dynamically,
when running `plan` or `apply`:

```
│ Error: Attempt to get attribute from null value
│
│   on main.tf line 19, in output "enabled_output":
│   19:     value = null_resource.example.id
│     ├────────────────
│     │ null_resource.example is null
│
│ This value is null, so it does not have any attributes.
```

The available data access patterns for possibly disabled resources are:

- `null_resource.example != null ? null_resource.example.id : "default value"`
- `try(null_resource.example.id, "default_value")`
- `can(null_resource.example.id) ? null_resource.example.id : "default value"`

`try` is the most concise option, but it should be used carefully since it can silently mask
different errors.

Discarded options:

- In a Github thread, someone mentioned about using `?` to deal with disabled resources. Since this would be a HCL change, we discarded that option.

### What happens when it's disabled

Let's suppose we have a created a resource using the `lifecycle -> enabled` field and then we want to disable it.
The behavior is going to be the same as if you wanted to destroy a resource:

```
# aws_instance.demo_vm_2 will be destroyed
# (because enabled is false)
  - resource "aws_instance" "demo_vm_2" {
      - ami                                  = "ami-07df274a488ca9195" -> null
      - arn                                  = "arn:aws:ec2:eu-central-1:532199187081:instance/i-0b63433033e61d818" -> null
      - associate_public_ip_address          = true -> null
      - availability_zone                    = "eu-central-1b" -> null
```

### What types of variables are supported on the conditional

There are a few types of values that cannot be used on this conditional:

1. Unknown values, since they cannot be used until the apply phase
1. Sensitive values
1. Null values
1. Conditionals that do not evaluate to true/bool values

## Future considerations

The `enabled` field was placed in the `lifecycle` block rather than directly on resources/modules to avoid conflicts with existing resources that use "enabled" as an attribute name. A future language edition could promote it to a top-level field. See the discussion [here](https://github.com/opentofu/opentofu/issues/1306#issuecomment-2982113732).
