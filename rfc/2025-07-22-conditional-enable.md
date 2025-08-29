# Conditional field for enabling resources and modules

## Current status

Issue: https://github.com/opentofu/opentofu/issues/1306

> [!NOTE]
> Every time we refer to *resources* in this RFC, we're talking about [managed resources](https://opentofu.org/docs/language/resources/), [data sources](https://opentofu.org/docs/language/data-sources/) and [ephemeral resources](https://github.com/opentofu/opentofu/issues/2834).

Right now, OpenTofu supports conditional enable/disable of resources by using `count` as a workaround:

```
variable "enabled" {
  default = true
}

resource "aws_instance" "example" {
  count = var.enabled ? 1 : 0
  # ...
}
```

This approach brings a few problems, such as adding indexes to resources that would be single instances, making it harder to manage using these indexes.

This RFC proposes a new way to do this in a cleaner and more semantic manner by using a new field on the `lifecycle` block called `enabled`:

```
resource "aws_instance" "example" {
  # ...

  lifecycle {
    enabled = var.enabled
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

1. Support for conditional enabling on:
    1. Resources - Add a new field on the `lifecycle` block called `enabled`;
    2. Modules - Add a new `lifecycle` block for modules but only supporting the new `enabled` field.
2. By default, this field is enabled if not otherwise specified.
3. Raise errors if a resource is used while being disabled and the dependent resources are not disabled too;

## Technical Approach

### `lifecycle` support on Modules

The lifecycle block is currently not supported on `module` block, but in our docs we mention to users that it's reserved for future usage:

> OpenTofu does not use the lifecycle argument. However, the lifecycle block is reserved for future versions.

https://github.com/opentofu/opentofu/blob/90c000ea047c3fca96cd9932f2d03e3cab652e96/website/docs/language/modules/syntax.mdx?plain=1#L133

This proposal could be the right time to start supporting this `lifecycle` block, but only to add the `enabled` field.

## Migration from existing resources

### Migration from `count` to `enabled`

As mentioned before, users were adding support for enabling/disabling resources by using `count = var.enabled ? 1 : 0`
and this is the most common case where they want to use an `enabled` field.

To support moving between resources where the `count = 0` workaround was used to the new `enabled` approach,
OpenTofu will automatically [support an implicit move of the resource out of the box with no changes needed](https://github.com/opentofu/opentofu/pull/3066#discussion_r2231518392). An implicit `move` will be done on the apply phase to remove the 
index and turn it into a single instance.

```
OpenTofu will perform the following actions:

  # null_resource.example[0] has moved to null_resource.example
    resource "null_resource" "example" {
        id = "8975736315378968412"
    }

Plan: 0 to add, 0 to change, 0 to destroy.
```

This behavior only exists for resources at the moment. As part of the RFC, support will be written for implied moves within modules.
Until then, this can be done with:

```
moved {
  from = module.enabled_module[0]
  to   = module.enabled_module
}
```

### Migration from `for_each` to `enabled`

Unlike `count`, OpenTofu doesn't handle implicit moves when migrating from `for_each`. Users must explicitly tell OpenTofu what needs to be moved:

```
moved {
  from = aws_s3_bucket.example["for_each_key"]  # name of the key being used by for_each
  to = aws_s3_bucket.example
}
```

Here's a complete example of this migration:

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

Moving from `for_each` to `enabled` doesn't make sense if more than one resource is being used. In the example above, `bucket-2` would be removed.
This migration path should be used only when a single instance is being used and `for_each` is supporting that conditional.

### Errors when resource is not enabled

There's been discussion about this [topic](https://github.com/opentofu/opentofu/issues/1306#issuecomment-2398120132), and
the best compromise we found is that a disabled resource can be `null`, but no errors will
be shown statically, such as when using `tofu validate` or the `tofu-ls` language server.
Since we need to evaluate the `enabled` expression, these errors will be shown dynamically
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

This can be extended using custom provider-defined functions. For example, if someone wanted to create a function that adds a warning instead of raising an error when trying to access the attribute,
they could do:

- `provider::custom::warning_enabled(null_resource.example.id, "default value")`

For now, we offer the three options above. 
`try` is the most concise option, but it should be used carefully since it can silently mask errors not caused by null value access. `can` and `!= null` are more verbose but more reliable for doing what you're trying to do.

Discarded options:

- In a previous GitHub discussion on another project, there was a request to support the optional attribute operator (?.), as seen in expressions like `aws_s3_bucket.example?.bucket`. Since this operator is a new HCL2 language feature that's not straightforward to implement and is essentially syntactic sugar for the system proposed in this RFC, we believe this discussion should be postponed until the foundational feature is in place.

### What happens when it's disabled

Suppose we have created a resource using the `enabled` field and then we want to disable it.
The behavior will be the same as if a resource is being destroyed:

```
# aws_instance.demo_vm_2 will be destroyed
# (because enabled is false)
  - resource "aws_instance" "demo_vm_2" {
      - ami                                  = "ami-07df274a488ca9195" -> null
      - arn                                  = "arn:aws:ec2:eu-central-1:532199187081:instance/i-0b63433033e61d818" -> null
      - associate_public_ip_address          = true -> null
      - availability_zone                    = "eu-central-1b" -> null
```

### Usage of `enabled` together with `for_each` and `count`.

These three arguments behave similarly but with different semantics:

- `enabled` is used for single-instance resources or modules.
- `count` creates multiple instances of a resource or module, differentiating them with integer indices.
- `for_each` creates multiple instances with added flexibility through access to `each.key` and `each.value`.

One might argue that `count` and `enabled` can be used together, but this would create confusion. The data access patterns for these resources are different:

- `count` and `for_each` resources can be accessed as: `aws_s3_bucket.example[*].bucket`;
- `enabled` resources can be accessed as: `null_resource.example != null ? null_resource.example.id : "default value"`; 

If both fields were enabled together, how would a user decide which access pattern to use? If `enabled` is `true` and `count` is greater than zero,
`aws_s3_bucket.example[*].bucket` could be used, but if `enabled` is `false`, the expression would break due to accessing null fields. The same applies to 
`for_each` resources.
To avoid this confusion, we prefer to allow only one of these fields to be used at a time. `enabled` is designed for single-instance resources or modules.

### What types of variables are supported on the conditional

Several types of values cannot be used in this conditional:

1. Unknown values, since they cannot be used until the apply phase
2. Sensitive values
3. Null values
4. Conditionals that do not evaluate to true/bool values
5. Ephemeral values

## Future considerations

- The `enabled` field was placed in the `lifecycle` block rather than directly on resources/modules to avoid conflicts with
existing resources that use "enabled" as an attribute name. A future language edition could promote it to a top-level field.
See the discussion [here](https://github.com/opentofu/opentofu/issues/1306#issuecomment-2982113732).
- We may want to support at some point the combination of `enabled` field with `count` and `for_each`. In this current iteration, we need to see how users will use the feature, but for now, consider [this section](#usage-of-enabled-together-with-for_each-and-count).