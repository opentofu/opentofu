# Functions for Hinting About Unknown Values

A fundamental part of OpenTofu's workflow is the idea of generating a plan and
reviewing it (either manually, or with the help of tools) before actually
taking the proposed actions durnig the apply phase.

An important tradeoff of that model is that during the planning phase there are
often values that a provider cannot predict until a related action has already
been taken, such as a surrogate unique identifier chosen by the remote API
only once an object is actually being created.

OpenTofu's programming model is designed to, as much as possible, tolerate those
unknown values and still predict as well as possible what the final value might
be. However, there are a few parts of the language that cannot currently
tolerate unknown values at all because certain information is needed to produce
any sort of useful plan, and although we do hope to reduce the number of those
situations over time it's still typically better for OpenTofu to have more
information during planning when possible, if only to give a human reviewer as
much information as possible to decide if the plan matches what they intended.

This document proposes adding some new built-in functions to the OpenTofu
language to allow module authors to optionally give OpenTofu additional hints
about the range of values that ought to be possible in a particular location,
either to avoid an unknown value appearing in a location where the OpenTofu
language disallows that, or just to make the generated plan more complete so
that it's easier to review either manually or using automatic review tools.

This is intended as a contribution to (though not necessarily a full solution
for) the following issues:
- [opentofu/opentofu#1685](https://github.com/opentofu/opentofu/issues/1685)
- [opentofu/opentofu#2322](https://github.com/opentofu/opentofu/issues/2322)
- [opentofu/opentofu#2464](https://github.com/opentofu/opentofu/issues/2464)
- [opentofu/opentofu#2630](https://github.com/opentofu/opentofu/issues/2630)
- [opentofu/opentofu#3533](https://github.com/opentofu/opentofu/issues/3533)

## Background

The concept of "unknown values" in OpenTofu actually comes from
[`cty`](https://github.com/zclconf/go-cty), which is the upstream library used
for the type system underlying the OpenTofu language.

For values in that type system, there are several different levels of
"known-ness", in the hope that an application like OpenTofu can track differing
amounts of detail depending on context:

1. Unknown value of unknown type: the worst case where we know absolutely nothing
   about the value: it could be of any type, and it could be null.

    This situation can arise in the results of functions that decide their
    result type based on the values given as arguments when those values
    are unknown themselevs, such as how OpenTofu's `jsondecode` function can't
    predict anything about its result if the given JSON string is unknown
    because the string content represent describes both a type and a value.

2. Unknown value of a known type: this is perhaps the most common situation
   involving unknown values, because most unknown values originate in attributes
   exported by a provider where the provider's schema tells OpenTofu what type
   it can expect.

    When dealing with values of this type, OpenTofu can at least typically
    perform some plan-type typechecking even if it can't predict anything else.
    For example, passing an unknown string to a function that expects a list
    will fail even though OpenTofu doesn't know the actual string value yet.

3. Unknown value of a known type, with "refinements":
   [refinements](https://github.com/zclconf/go-cty/blob/main/docs/refinements.md)
   are a `cty` concept for tracking additional partial information about an
   unknown value even when the exact value isn't known yet, which means that
   certain operations on those unknown values can return known results even
   when the input is unknown.

    For example, one possible refinement is that an unknown value is definitely
    not null, in which case `value != null` can return `true` even if `value`
    is otherwise unknown. Similarly, an unknown value of a string type can
    have an optional "known prefix", such as representing that AWS subnet ids
    always start with `subnet-` even if we don't know the digits that follow,
    and so `value != ""` and `startswith(value, "ami-")` can return known
    values even though we don't know the entire string value yet.

4. Partially-known values: for collection and structural types (lists, objects,
   etc) it's possible for the top-level value to be known but for one of its
   nested values to be in any of the previous states.

    For example, if `value` is unknown then `[value]` constructs a
    partially-known tuple value, because the top-level tuple is known even
    though its first element is unknown.

    In this case OpenTofu can typically produce known results for operations
    that only work shallowly. For example, OpenTofu can determine the length
    of a known list even if one or more of its elements is unknown.

5. Wholly-known values: the most ideal case is that OpenTofu already knows
   exactly what value should appear in a particular position during the apply
   phase.

    During the apply phase _all_ values are wholly-known, but during the
    planning phase OpenTofu only knows values that are written directly in
    the configuration or that a provider is able to predict exactly as part
    of its planned change.

Overall then, the later in this list a particular value sits the more likely
that any derived values will be known and the more information OpenTofu can
potentially provide in the plan UI or in the JSON description of the plan.

When "refinements" were introduced into OpenTofu's predecessor the end goal had
been for providers themselves to be able to produce refined values, so that
e.g. the `hashicorp/aws` provider's `aws_subnet` resource type could set its
`id` attribute to an unknown string that's definitely not null and definitely
has the prefix `subnet-`, and other similar rules that provider developers can
infer from the documentation of their underlying API.

Unfortunately, although the plugin protocol already has support for returning
unknown values with refinements
[an issue about supporting that in the plugin framework](https://github.com/hashicorp/terraform-plugin-framework/issues/869)
has been languishing since late 2023 and there is also no support in the legacy
plugin SDK, and so typical providers written in terms of either of those
libraries cannot actually participate in this part of the protocol, and so
most values returned by providers are not "refined" at all and so expressions
derived from those values are less known than they ought to be.

As an interim workaround, the third-party provider
[`apparentlymart/assume`](https://github.com/apparentlymart/terraform-provider-assume/)
(written by the author of this RFC) offers a small set of functions that allow
module authors to introduce their own refinements in place of the ones that
providers would ideally report for themselves. For example:

```hcl
output "subnet_id" {
  value = provider::assume::notnull(
    provider::assume::stringprefix(
        "subnet-",
        aws_subnet.example.id,
    ),
  )
}
```

These two extra function calls would tell OpenTofu that it should assume that
the result of this output value is definitely not null and definitely starts
with `subnet-`. If the final known value turns out not to match the given
assumptions then these functions will raise an error during the apply phase,
but if they were correct assumptions then the final value known value will
pass through for normal use.

OpenTofu can also make its own assumptions about its built-in functions with
similar effect. For example, any value that at least has a known type will
be returned by [`coalesce`](https://opentofu.org/docs/v1.11/language/functions/coalesce/)
as "definitely not null" because that function skips over any null values in
its arguments _by definition_.

Though a combination of relying on the implicit assumptions about built-in
functions and calling the more explicit functions in `apparentlymart/assume`
it's often possible to turn "unknown value of known type" into
"unknown value of known type _with refinements_", but it's annoying to have
to depend on a third-party provider for such low-level functionality that is
a fundamental part of the type system, and relying on the implicit refinements
caused by other functions can make modules brittle under maintenence if an
expression is inadvertently changed in a way that makes those implicit
refinements no longer effective.

## Proposed Solution

This document proposes a number of changes that aim for a better compromise in
situations where a module author can compensate for missing refinements in
providers:

- Built-in versions of the functions from the `apparentlymart/assume` provider,
  so that explicit refinements can be used without bringing in an external
  dependency.

    This allows module authors to transform "unknown value of known type" into
    "unknown value of known type _with refinements_", or possibly even into
    "partially-known value" if the refinements are strong enough.

    (Avoiding an additional provider dependency is particular desirable for
    shared modules, because organizations often have relatively strict policies
    about which providers they may use in their production environments to
    minimize the scope of security audits.)

- A new function for converting values to match arbitrary type constraints,
  thus generalizing the existing partial solution with the `tostring`, `tolist`,
  etc functions.

    This allows module authors to transform "unknown value of unknown type"
    into "unknown value of known type". Just adding type information at all can
    be helpful to let OpenTofu perform typechecking of unknown values, but
    having a known type is also a prerequisite for adding refinements because
    refinements are type-specific.

- Changes to the OpenTofu's plan description JSON format to expose additional
  details about unknown values where available, so that consumers of that
  format such as policy-enforcement or automatic approval tools can optionally
  make use of that additional information.

- Changes to OpenTofu's plan diff UI to expose _some_ additional information
  about unknown values, curated to just a subset that seems unlikely to cause
  confusion for readers who are not completely familiar with concepts like
  refinements.

The following sections describe each of these items in more detail.

### `assume...` functions

The "assume" family of functions all declare that OpenTofu should assume certain
details about an unknown value, and then check those assumptions once the
values become known in the apply phase. All except `assumeequal` correspond
directly to an existing refinement type supported by `cty`, applying the
corresponding refinement to their argument or returning an error if the given
value is not consistent with the refinement.

- `assumenotnull(v)` tells OpenTofu to assume that the result will definitely not
  be null, so that `== null` and `!= null` comparisons will return known values.

    Using this in combination with one or more of the other functions may cause
    other operations on the result to return known values too.

- `assumestringprefix(s, prefix)` returns `s` (a string) with the additional
  assumption that it begins with the string given in `prefix`.

    ```hcl
    assumestringprefix(aws_subnet.example.id, "subnet-")
    ```

    Unknown strings with such a refinement can return a known `true` for
    a test like `s != ""`, and the `startswith` function can return a known
    result if the refined prefix is at least as long as the prefix passed
    to that function which can be useful for checking some input variable
    validation rules against unknown strings during the planning phase.

- The `assumelistlength` family of functions tell OpenTofu to assume that
  a list has a length within specified bounds:

    - `assumelistlength(l, min, max)`
    - `assumelistlengthmin(l, min)`
    - `assumelistlengthmax(l, max)`

    Providing a nonzero minimum length means that `length(l) != 0` can return
    `true` even if the actual value isn't known.

    If refinements on list length combine in such a way that the minimum and
    maximum bounds are equal then OpenTofu will automatically promote the
    result to a partially-known list whose elements are unknown, so that
    `length(l)` can return a known value.

- The `assumesetlength` family of functions are similar to `assumelistlength`
  but for sets.

- The `assumemaplength` family of functions are similar to `assumelistlength`
  but for maps.

- `assumeequal(got, assumed)` tells OpenTofu to assume that `got` will be
  equal to `assumed`, where `assumed` is required to be wholly-known.

    In practice this means that it checks whether `got` equals `assumed` once
    `got` is known, and returns an error if not. In any case where it succeeds
    it just returns `assumed`, verbatim. `got` is also implicitly converted
    to match the type of `assumed` so that it's possible to use unknown values
    of unknown type or partially-known values with unknown-typed parts.

    This is the strongest of all of the assume functions and so probably the
    one that's least useful in practical situations. Successful use of it would
    require there to be enough context available elsewhere in the module to
    completely predict the final value, such as predicting an "ARN" for an
    AWS object based on a particular service's documented ARN syntax:

    ```hcl
    output "role_arn" {
      value = assumeequal(
        aws_iam_role.example.arn,
        provider::aws::arn_build(
          data.aws_partition.current.id,
          "iam",
          "", // Roles are global objects, so no region specified
          aws_caller_identity.current.account_id,
          "role/${aws_iam_role.example.name}",
        ),
      )
    }
    ```

    Such an approach could be useful if the resulting ARN will be incorporated
    into an IAM policy document, since otherwise OpenTofu would not be able
    to include the IAM policy source code in the plan output.

All of these functions are already available as part of the
`apparentlymart/assume` provider. Their author and sole copyright holder (also
the author of this RFC) is happy to contribute those implementations to the
OpenTofu project under MPL-2.0.

The proposed functions all follow the established naming convention in OpenTofu
where built-in functions have words all in lowercase without any underscores
delimiting words, which unfortunately makes their names quite clunky. However,
being consistent seems more important because it's annoying to have multiple
conventions and force authors to constantly refer to the documentation to find
out which convention is used for each function.

### `convert` function

Because "refinements" are type-specific, OpenTofu must at least know the type
of an unknown value before it can track refinements for it.

The `convert` function allows performing any type conversion that would be
allowed for input variables in a module to be performed inline as part of an
expression, and one notable use for that is to create something that can have
additional refinements applied to it:

```hcl
assumenotnull(
  convert(
    yamldecode(maybe_unknown),
    object({
      name = string
    }),
  ),
)
```

Because just about any value supported by OpenTofu can be described in YAML,
passing an unknown string to `yamldecode` produces an unknown value of an
unknown type. The example above then tells OpenTofu to assume that unknown
result will be convertable to the specified object type, and then to assume
that the object value will not be null. Without that type conversion the
`assumenotnull` call would be ineffective because an unknown-type value cannot
have refinements.

...

TODO: notes about how this would be implemented:

- need to change OpenTofu's reference analyzer to understand that the second
  argument of `convert` needs to be treated as a type expression instead of
  a value expression.
- however, type expressions can contain value expressions when specifying the
  default value for an optional attribute in an object type, and so the
  reference analyzer still needs to find and report those.

### JSON plan description changes

...

### Plan diff UI changes

...

## Future Considerations

### Inline value assumptions for resource attributes

This proposal intentionally focuses on adding normal functions because that's
something we can do in a relatively-isolated way without significant changes
to the OpenTofu runtime implementation: the runtime already supports type
conversions and refinements via `cty`, so the proposed functions are just making
those existing behaviors directly usable by module authors.

However, a key limitation of that approach is that assumptions can be applied
only to values produced by expressions in the module itself, and not directly
to values produced by a provider. Since the values produced by a provider are
the most directly accessible in OpenTofu's plan UI and JSON export of plans,
that's an annoying limitation.

We could potentially address that by extending the language to allow authors
to write their assumptions about a resource's _provider-selected_ attribute
values directly inside the resource configuration.

For example:

```hcl
resource "aws_vpc" "example" {
  cidr_block = "192.168.0.0/24"

  # INVALID: This is hypothetical syntax for inline assumptions about
  # provider-selected attribute values, which is not currently implemented.
  id = assumed(notnull(stringprefix("vpc-")))
}
```

During the planning phase then OpenTofu could apply additional refinements
directly to the `aws_vpc.example.id` result _and_ show the assumptions directly
as part of the plan for this resource:

```
  # aws_vpc.example will be created
  + resource "aws_vpc" "example" {
     + cidr_block = "192.168.0.0/24"
     + id         = "vpc-${(known after apply)}"
     (...etc...)
  }
```

OpenTofu could then check that the assumptions were correct after the provider
returns the final state of this new resource instance, returning an error if
not.

Although this idea is potentially useful and has a number of desirable
qualities, it seems to require defining another little microsyntax for
describing the assumptions (since the value we're making assumptions about
is implicit in the attribute name, rather than an explicit argument as with the
proposed functions) and would be a lot more intrusive into OpenTofu's existing
implementation of plan and apply.

Therefore this proposal focuses on the function-based approach to start, though
we might consider something like the above later if it seems warranted based
on our experiences with the functions from this proposal.
