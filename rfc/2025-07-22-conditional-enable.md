# Conditional field for enabling resources and modules

## Current status

Issue: https://github.com/opentofu/opentofu/issues/1306

> [!NOTE]
> Every time we refer to *resources* in this RFC, we're talking about [managed resources](https://opentofu.org/docs/language/resources/), [data sources](https://opentofu.org/docs/language/data-sources/) and [ephemeral resources](https://github.com/opentofu/opentofu/issues/2834).

Right now, OpenTofu supports conditional enable/disable of resources by using a workaround with `count`.
This approach brings a few problems, such as adding indexes to resources that would be single instances, making it harder to manage using these indexes.

This RFC proposes a new way to do this in a cleaner and more semantic manner by using a new field on the `lifecycle` block called `enabled`:

```
resource "aws_instance" "example" {
  # ...

  lifecycle {
    enabled = var.enable_server
  }
}
```

This is available not only for resources, but for modules too:

```
module "modcall" {
  source = "./mod"

  lifecycle {
    enabled = var.env == "production"
  }
}
```

## Proposed Solution

1. Raise errors if a resource is used while being disabled and the dependent resources are not disabled too;
2. Support for conditional enabling on:
    1. Resources - Add a new field on the `lifecycle` block called `enabled`;
    2. Modules - Add a new `lifecycle` block for modules but only supporting the new `enabled` field.


## Technical Approach

### `lifecycle` support on Modules

The lifecycle block is currently not supported on `module` block, but in our docs we mention to users that it's reserved for future usage:

> OpenTofu does not use the lifecycle argument. However, the lifecycle block is reserved for future versions.

https://opentofu.org/docs/language/modules/syntax/#meta-arguments

It would be the right timing to start supporting it, but only for the `enabled` field.

## Migration from existing resources

### Migration from `count` to `enabled`

As mentioned before, users were adding support for enabling/disabling resources by using `count = var.enabled ? 1 : 0` and this is the most common case where they want to use an `enabled` field.
Luckily, this support is [in-place already](https://github.com/opentofu/opentofu/pull/3066#discussion_r2231518392). When using a `count` for this conditional dependency, if
the resource is changed to use `enabled`, an implicit `move` will be done on the apply phase to remove the index and
turn it into a single instance.

```
OpenTofu will perform the following actions:

  # null_resource.example[0] has moved to null_resource.example
    resource "null_resource" "example" {
        id = "8975736315378968412"
    }

Plan: 0 to add, 0 to change, 0 to destroy.
```

### Migration from `for_each` to `enabled`

Unlike `count`, implicit moves are not handled by OpenTofu when migrating from `for_each`. This means that the user should explicitly
tell OpenTofu what needs to be moved:

```
moved {
  from = aws_s3_bucket.example["for_each_key"]  # name of the key being used by for_each
  to = aws_s3_bucket.example
}
```

So a complete example of that support would be:

```
# For_each before:

locals {
  buckets = ["bucket-1", "bucket-2"]
}

moved {
  from = aws_s3_bucket.example["bucket-1"]  # name of the key being used by for_each
  to = aws_s3_bucket.example
}

resource "aws_s3_bucket" "example" {
  for_each = var.enable ? toset(local.buckets) : []
  bucket = each.key
}

# To:
resource "aws_s3_bucket" "example" {
  lifecycle {
    enabled = var.enable
  }
}
```

It doesn't make a lot of sense in the example above to move from `for_each` to `enabled` if more than one resource is being used. `bucket-2` in the example above would be removed. This path of migration
should be used *only* if a single instance is being used and for_each is supporting that conditional.

### Usage of `enabled` together with `for_each` and `count`.

These three different arguments would behave similarly, but with different semantics.
It could be argued that the user wants to enable a resource with 4 instances, but in our current code,
that conditional can be changed to `count=0`.

The proposal is that we do not support them together. Only one of them can be used, returning errors if we try to use the others together.

Semantically, this feature would be used for single-instance resources or modules.

### Errors when resource is not enabled

There's some discussion about this [topic](https://github.com/opentofu/opentofu/issues/1306#issuecomment-2398120132), and the better compromise we found is that an enabled 
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

The only available data access patterns for possibly disabled resources are:

- `null_resource.example != null ? null_resource.example.id : "default value"`
- `try(null_resource.example.id, "default_value")`
- `can(null_resource.example.id) ? null_resource.example.id : "default value"`

Notice this can be extended by using custom provider functions. Let's say if someone would like to
create a function that add a warning instead of raising an error when trying to access the attribute,
they could do:

- `provider::custom::warning_enabled(null_resource.example.id, "default value")

For now, we will be offering the three options above. 
`try` is the most concise option, but it should be used carefully since it can silently mask
different errors not caused by null values access. `can` and `!= null` are more verbose but 
are more reliable on doing what you're trying to do.

Discarded options:

- In a GitHub thread, someone mentioned using `?` to deal with disabled resources. Since this would be a HCL change, we discarded that option.

### What happens when it's disabled

Let's suppose we have created a resource using the `lifecycle -> enabled` field and then we want to disable it.
The behavior is going to be the same as if a resource is being destroyed:

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

There are a few types of values that cannot be used in this conditional:

1. Unknown values, since they cannot be used until the apply phase
2. Sensitive values
3. Null values
4. Conditionals that do not evaluate to true/bool values
5. Ephemeral values

## Future considerations

The `enabled` field was placed in the `lifecycle` block rather than directly on resources/modules to avoid conflicts with existing resources that use "enabled" as an attribute name. A future language edition could promote it to a top-level field. See the discussion [here](https://github.com/opentofu/opentofu/issues/1306#issuecomment-2982113732).
