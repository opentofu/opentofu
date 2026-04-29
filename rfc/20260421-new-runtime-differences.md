# Behavior Differences in the New OpenTofu Runtime

In [A new approach to configuration evaluation, planning, and applying](./20251001-eval-plan-apply-architecture.md)
we agreed to try for a new way to implement OpenTofu's core functionality,
learning from our experiences maintaining the current language runtime we
inherited from our predecessor.

During the OpenTofu v1.12 development period we implemented a "walking skeleton"
experimental version of that design to learn whether it would actually work
in practice. We successfully reimplemented the primary functionality of planning
and applying changes to managed resource instances in that new runtime style,
and although we didn't fully implement any other behaviors yet we have explored
them enough to feel confident that they are achievable.

However, if the new approach only led to us reimplementing the same behavior in
a different way it would be hard to justify spending so much time and effort on
it. Although we could perhaps justify it as making it easier to maintain and
implement new functionality in future, we can't be sure that is true until we're
actually doing ongoing maintenence and development on it.

Aside from internal design improvements, the new design is intentionally
arranged to allow us to perform more precise analysis of the configuration so
that our interpretation and execution of it can more closely match the likely
author intent. For example, it finds dependencies between resources using
dynamic analysis rather than static analysis, so given an expression like
`var.predicate ? aws_instance.foo.id : null` we can determine that this only
depends on `aws_instance.foo` when `var.predicate` is true, whereas the old
runtime would assume that dependency is always present because it relies purely
on the static presence of that reference.

We are therefore expecting to make some intentional changes to the current
effective behavior of OpenTofu. This is tricky because authors may written
configurations that rely on the limitations of the current runtime.

This document's purpose is to document some of the behavior differences we're
expecting and discuss the potential consequences of them on existing code. We'll
also discuss potential strategies for minimizing the impact on existing modules
despite these differences.

## General dependency-detection changes

By far the most fundemental difference in the new runtime is that it uses
dynamic analysis to determine the dependencies between resource instances and
provider instances.

As mentioned in the introduction, this means that we'll take into account
dynamic decisions made in expressions containing references. For example:

```hcl
variable "items" {
  type = map(object({
    name  = string
    use_a = optional(bool, false)
  }))
}

resource "example" "a" {
  # ...
}

resource "example" "b" {
  for_each = var.items

  name = each.value.name
  a_id = each.value.use_a ? example.a.id : null
}

resource "example" "c" {
  for_each = var.items

  b_id = example.b[each.key].id
}
```

In the original runtime, dependencies are resolved on a whole-resource basis
and so all instances of `example.b` depend on `example.a`, and all instances
of `example.c` depend on all instances of `example.b`.

In the new runtime with dynamic analysis, we can resolve this more precisely:

- Only instances of `example.b` whose `each.value.use_a` is set depend on
  `example.a`.
- Each instance of `example.c` depends on only one instance of `example.b`,
  correlated by instance key.

As long as the above are truly all of the actual dependencies needed, this is
a superior result because it more accurately reflects the author's intent.

But it _does_ mean that under the new runtime some instances of `example.b`
can begin being created before `example.a` has finished being created, which
is an observable change of behavior that might be consequential if there's
some sort of hidden dependency between them in the remote system. The new
approach requires that the author make that hidden dependency explicit by
e.g. writing a `depends_on` argument.

### `depends_on` behavior changes

The `depends_on` argument in the current language directly exposes the static
analysis behavior as part of the language, so we need to be particularly careful
how we reinterpret it in our new dynamic analysis approach.

Under the new runtime, `depends_on` will be redefined as being a static list
of arbitrary expressions, rather than limited only to HCL static traversals. For
example:

```hcl
  depends_on = [
    # Dynamically-chosen instance key
    aws_instance.example1[each.key],

    # Conditional dependency
    var.predicate ? aws_instance.example2 : null,

    # Reference to just one part of a resource instance
    aws_instance.example3.ami_id,
  ]
```

Internally, OpenTofu would interpret this by evaluating each of the given
expressions to obtain its value. OpenTofu would then behave as if each of
those values had appeared in its entirety somewhere inside the configuration
of the item that the `depends_on` argument belongs to, even though those values
do not _actually_ get used in the configuration.

In particular notice that `aws_instance.example3.ami_id` represents a dependency
on whatever _that attribute in particular_ is derived from, ignoring any
references in any other part of that instance's configuration.

The behavior described in this section is _mostly_ entirely new functionality
that wasn't previously valid at all, but there's one exception: today's OpenTofu
allows `depends_on` to contain a reference like `aws_instance.example4[0]` but
just silently ignores the `[0]` part and treats it as a dependency on _all_
instances of `aws_instance.example4`. The new runtime would instead resolve
this precisely as a dependency only on the zeroth instance.

For `depends_on` in a `module` block, the system behaves as if the values
of all of the listed expressions appeared in every resource instance nested
under the relevant module instances. This preserves the current conservative
behavior that argument causes because we've no way to infer a more precise
intent from this sort of configuration.

### `-target` and `-exclude` behavior changes

The "target" and "exclude" features also effectively expose the current
dependency resolution details as part of the public interface, because operators
can observe which upstream or downstream resource instances get included or
excluded.

The behavior of these features is unfortunately based more on what was
convenient to implement within our predecessor's implementation details rather
than on specific end-user needs, and so folks have found lots of weird and
wonderful ways to use these features that each rely on different aspects of the
behavior. Folks have also discovered various limitations of the current behavior
which they considered to be bugs but which other authors might unfortunately be
relying on for their change workflow to work correctly.

In the current runtime these options just cause OpenTofu to naively prune
out parts of the internal dependency graph before walking it. That approach
cannot work for the new runtime because there is no explicit internal dependency
graph: we discover it dynamically _during_ evaluation instead.

For `-exclude`, the new behavior would be to treat any specified resource
instance as forced to be deferred to a future plan/apply round. By the rules
of deferred actions, anything else that _dynamically_ depends on the given
resource instance would also be deferred. Some downstream instances that would
previously have been excluded would no longer be excluded, if we can find no
dynamic reference to the resource instance that was deferred.

`-target` currently remains an unanswered design question, because its behavior
relies on being able to know what's downstream of something before beginning to
evaluate that thing and our dynamic analysis approach cannot do that. At the
time of the original RFC our prototype included a preprocessing step where the
system would first produce a _conservative_ resource instance graph by doing
dynamic analysis in a context where all resource instances have
completely-unknown values, with the intention of using that graph to emulate
the coarse interpretation of `-target` from the current runtime. We ended up
removing that extra pass during walking skeleton development, and we've not
yet planned any alternative way to achieve this.

The dynamic analysis behavior also introduces the possibility of a dependency
being decided based on an ephemeral value and therefore varying between plan
and apply. Depending on how we resolve the question of how to implement
`-target` that might cause the apply phase to rely on something that wasn't
included in the plan. For now we're accepting that we'll generate an error
during the apply phase if that is true, although if we choose to implement
`-target` via an additional conservative resource instance graph then this might
not actually be a problem in practice.

## Data Resource Instance behavior changes

Data resource instances get read during the planning phase when possible, but
are delayed until the apply phase if they appear to use values that won't be
changed in the remote system until the apply phase.

The current runtime has an imprecise rule for deciding this: if there are
any unknown values in the configuration _or_ if any of the direct dependencies
of the data resource already have pending changes.

The new runtime can track more precisely which specific values are expected
to change during the apply phase, even if they are known during the planning
phase. This means that there are some situations that would cause a delayed
read in today's runtime but not in the new runtime.

Consider the following (contrived) example, where "local" is the
`hashicorp/local` provider:

```hcl
resource "local_file" "example" {
  content  = "hello world"
  filename = "${path.root}/hello.txt"
}

data "local_file" "example" {
  filename = local_file.example.filename
}
```

In the current runtime, the reference to `local_file.example` from
`data.local_file.example` is enough for the data resource instance to be delayed
if _any_ change is planned for `local_file.example`.

In the new runtime, the read of `data.local_file.example` would be delayed
only if `local_file.example.filename` in particular were changing, which would
be true during the initial creation of `local_file.example`, but _not_ if
its `content` argument were being updated in-place later because the
data resource instance configuration only refers to the filename, not the
content.

To get the previous behavior, the data resource should declare an explicit
dependency on the content of the file:

```hcl
resource "local_file" "example" {
  content  = "hello world"
  filename = "${path.root}/hello.txt"
}

data "local_file" "example" {
  filename = local_file.example.filename

  depends_on = [
    local_file.example.content,
  ]
}
```

Due to the general rule (described earlier) that the values of each expression
in `depends_on` are treated for dependency resolution just like if they had
appeared somewhere in the configuration of the resource instance, OpenTofu would
then notice when the `content` attribute is changing and delay the data read
until that change has been applied.

## Ephemeral Resource Instance and Provider Instance behavior changes

Ephemeral resource instances and provider instances are both what we consider
to be "ephemeral objects", in the sense that they are opened and closed
separately for each phase and we don't rely on anything about them carrying
forward between plan and apply phases in the same round.

However, because the current runtime relies entirely on conservative static
analysis the effective behavior is that these ephemeral objects get opened
during both the plan and apply phases regardless of whether anything actually
ends up referring to them dynamically.

For example, consider this configuration involving a provisioner and an
ephemeral resource:

```hcl
ephemeral "aws_ssm_parameter" "ssh_key" {
  arn = "arn:example:example"
}

resource "aws_instance" "example" {
  # ...

  connection {
    # ...
    private_key = ephemeral.aws_ssm_parameter.ssh_key.value
  }

  provisioner "remote-exec" {
    # ...
  }
}
```

If the `connection` block is the only reference to
`ephemeral.aws_ssm_parameter.ssh_key` in this module then its result is used
only during the apply phase, because provisioner configurations are not
evaluated (aside from static validation) during the planning phase.

However, our current runtime doesn't understand that and so needlessly fetches
the SSM parameter during the planning phase only to just throw it away again
without using it. This means that the credentials used for planning must have
access to retrieve that secret, which goes against the principle of least
privilege.

In the new runtime we will open ephemeral objects only once we determine
dynamically that they are needed. In the above example, no reference to
`ephemeral.aws_ssm_parameter.ssh_key` would be evaluated during the planning
phase and so it would not be opened at all.

There are some subtle ways in which this could be considered a breaking change
for certain situations:

- If an operator was previously using the same credentials during plan and
  apply and relying on a failure to fetch the key during planning to detect
  insufficient access to apply then that would no longer be effective unless
  they ensure the SSH key gets used somewhere else that would get evaluated
  during planning.

    (This is really just the advantage of not fetching the secret during the
    planning phase reframed as a disadvantage for those who weren't trying
    to separate privilege between the two phases anyway.)
  
- In theory there could be an ephemeral resource type which doesn't produce
  any useful data for downstream use but whose "open" action nonetheless has
  some useful side-effect that something else in the configuration is implicitly
  relying on, particularly if the "close" action doesn't immediately undo that
  side-effect.

    If a module were previously relying on this then it would need to be updated
    to explicitly declare a dependency on the ephemeral resource instance from
    whatever is relying on it, such as by using a `depends_on` argument.
