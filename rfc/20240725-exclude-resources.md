# Exclude Flag for Planning and Applying

Issue: https://github.com/opentofu/opentofu/issues/426

The RFC entails a new flag `-exclude` to be used in planning and applying. The flag's purpose is to be the inverse of the `-target` flag - A targeted plan/apply for every resource that is not excluded

This is solving many problems, like some multi-stage deployment configurations, as well as edge cases where you would just like to skip applying a specific resource temporarily

## Proposed Solution

An `-exclude` flag that could be used in `tofu plan`, `tofu apply` and `tofu destroy`.

This flag will work as an exact inverse of `-target` - when planning, it would act as though we are targeting any resource that is not excluded.

Similarly to how targeted resources also include all of their dependencies in the plan, an excluded resource would mean that all resources dependent on it should be excluded as well

### User Documentation

User should be able to provide one or more excluded resource, via one or multiple `-exclude` flags. For example: `tofu plan -exclude=null_resource.a -exclude=null_resource.b`

An excluded plan - Would exclude any resource given via the `-exclude` flag, and also any resource that is dependent on these resources.

See the following example:

```hcl
# In this example there are 4 null_resources, with the following dependency graph:
# A <---- [B,C] <---- D

resource "null_resource" "a" {
  
}

resource "null_resource" "b" {
  triggers = {
    a = null_resource.a.id
  }
}

resource "null_resource" "c" {
  triggers = {
    a = null_resource.a.id
  }
}

resource "null_resource" "d" {
  triggers = {
    b = null_resource.b.id
    c = null_resource.c.id
  }
}
```

With the above example:
- Running `tofu plan -exclude=null_resource.d` would plan any resource that's not `null_resource.d` (`null_resource.a`, `null_resource.b`, `null_resource.c`)
- Running `tofu plan -exclude=null_resource.a` would create an empty plan, since all resources depend on `null_resource.a`
- Running `tofu plan -exclude=null_resource.b` would exclude both `null_resource.b` and `null_resource.d` which depends on it (so it will plan `null_resource.a` and `null_resource.c`)
- Running `tofu plan -exclude=null_resource.b -exclude=null_resource.c` would exclude `null_resource.b`, `null_resource.c` and also `null_resource.d` which depends on one of them (or in this case - both of them)
- Running `tofu plan -exclude=null_resource.a -exclude=null_resource.b` would create an empty plan, since all resources depend on `null_resource.a`
- Running `tofu plan -exclude=null_resource.e` would create a full plan, since the excluded resource does not exist. This is for parity with `-target`, which creates an empty plan if the target does not exist

When destroying:
- Running `tofu plan -destroy -exclude=null_resource.b` will result in a plan to destroy `null_resource.c` and `null_resource.d`

Note that a resource is dependent on another resource not just by direct resource dependency:

```hcl
locals {
  b = null_resource.a.id
}

resource "null_resource" "a" {
  
}

resource "null_resource" "c" {
  triggers = {
    b = local.b
  }
}
```

In the example above, if you run `tofu plan -target=null_resource.a`, then both `null_resource.a` and `null_resource.c` will be excluded from the plan. `null_resource.c` depends on a local which in itself depends on `null_resource.a`

**Note**: For now, `-exclude` and `-target` flag should not be allowed to be used in conjunction. In the future, we might allow them both to be used in conjunction, with the `-exclude`d resource taking precedence. However, this approach would require a deeper dive into it
**Note 2**: When using the `-target` flag, on an apply from a stored plan file, the flag is completely ignored. So, the behaviour would be the same for the `-exclude` flag

#### Outputs

When planning with an `-exclude` flag, only outputs that rely on **at least one resource** that was not excluded should be recalculated. 

This is the inverted approach to the `-target` flag, for which outputs are only recalculated if all resources that it depends all are targeted

#### Data Sources

Like with the `-target` flag, supplying an `-exclude` flag means that no data sources are refreshed, even if they are technically dependent on resources that are not excluded.

This also means that any dependency on a data source is not considered at all when calculating whether a resource or an output is dependent on a non-excluded resource

#### Cloud

Unlike `-target` flag, which is passed to the cloud backend in remote runs, the `-exclude` flag will not be passed to cloud backends. This is due to a technical limitation, with the cloud client and API calls being managed by `go-tfe`.

### Technical Approach

The technical approach of this should be pretty simple and very similar to how targeted resources work.

Mainly:
- Add `Excludes` alongside `Targets` pretty much anywhere applicable (`Operation`, `NodeAbstractResource`)
- Adapt `GraphNodeTargetable` to also have `SetExcludes`, for dynamic expansion
- In the `TargetTransformer`, remove any excluded resource or resource depending on an excluded resource from the graph

### Open Questions

- Is `-exclude` the correct name for the flag? Maybe `-target-exclude`?

### Future Considerations

## Potential Alternatives

CLI tools or scripts could simulate this. One could get all resources from the state, and then run a plan or apply with `-target` flags for all non-excluded resources (with some adjustments, due to having to deal with dependencies).

However, such alternatives would be slow or inaccurate, and not really suitable for what we're trying to accomplish here.  