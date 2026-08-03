# Explicit type conversions and refinements for unknown values

A fundamental part of OpenTofu's workflow is the idea of generating a plan and
reviewing it (either manually, or with the help of tools) before actually taking
the proposed actions during the applying phase.

An important tradeoff of that model is that during the planning phase there are
often types or values that a provider cannot predict until a related action has
already been taken, such as a surrogate unique identifier chosen by the remote
API only once an object is actually being created.

OpenTofu's programming model is designed to, as much as possible, tolerate those
unknown types and values and still predict as well as possible what the final
value might be. However, unknown information during the planning phase can
prevent OpenTofu from detecting certain problems until the applying phase, and a
few language features currently cannot tolerate unknown values at all because
certain information is needed to produce any sort of useful plan.

This document proposes adding some new built-in functions to the OpenTofu
language to allow module authors to optionally give OpenTofu additional hints
about the range of values that ought to be possible in a particular location,
either to avoid an unknown type or value appearing in a location where the
OpenTofu language disallows that, or just to make the generated plan more
complete so that it's easier to review either manually or using automatic review
tools.

This RFC was primarily motivated by [opentofu#2630](https://github.com/opentofu/opentofu/issues/2630)
which originally proposed just the `convert` function described later, but it
includes some additional functionality intended to reduce the impact of (though
not to completely solve) the following issues related to handling of unknown
values:

- [opentofu#1685](https://github.com/opentofu/opentofu/issues/1685): Some providers fail in a confusing way when their configuration contains unknown values
- [opentofu#2322](https://github.com/opentofu/opentofu/issues/2322): OpenTofu can't plan when `count`, `for_each`, or `enabled` is an unknown value
- [opentofu#2464](https://github.com/opentofu/opentofu/issues/2464): impossible to plan providers based on re-read data sources
- [opentofu#3533](https://github.com/opentofu/opentofu/issues/3533): `yamldecode` causes resource attribute used in count meta-argument to be unknown during plan

Hopefully other future proposals will solve these more comprehensively by making
OpenTofu better tolerate unknown values in these contexts, but in the meantime
this proposal gives authors an opportunity to increase the amount of available
context so that there are fewer unknown values that could potentially cause
these problems.

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
    are unknown themselves, such as how OpenTofu's `jsondecode` function can't
    predict anything about its result if the given JSON string is unknown
    because the string content represent describes both a type and a value.

2. Unknown value of a known type: this is perhaps the most common situation
   involving unknown values, because most unknown values originate in attributes
   exported by a provider where the provider's schema tells OpenTofu what type
   it can expect.

    When dealing with values in this category, OpenTofu can at least typically
    perform some plan-time typechecking even if it can't predict anything else.
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
    of its proposed change.

Overall then, the later in this list a particular value sits the more likely
that any derived values will be known and the more information OpenTofu can
potentially provide in the plan UI or in the JSON description of the plan.

The provider plugin protocol allows providers themselves to produce "refined"
values, and so in principle e.g. the `hashicorp/aws` provider's `aws_subnet`
resource type could return an `id` attribute whose value is an unknown string
that is definitely not null and definitely has the prefix `subnet-`, which
would then allow OpenTofu to return `true` for an expression like
`aws_subnet.example.id != null` even when the id hasn't been decided yet.

Unfortunately, although the plugin protocol already has support for returning
unknown values with refinements
[an issue about supporting that in the plugin framework](https://github.com/hashicorp/terraform-plugin-framework/issues/869)
has been languishing since late 2023 and there is also no support in the legacy
plugin SDK, and so typical providers written in terms of either of those
libraries cannot actually participate in this part of the protocol, and so
most values returned by providers are not "refined" at all and so expressions
derived from those values are less known than they ought to be.

There are also various situations elsewhere in the OpenTofu language where
an unknown value arises beyond the direct result of a provider. For example,
passing an unknown value to `yamldecode` causes it to return an unknown value
of an unknown type because it's possible to express various different types and
values in YAML syntax.

It would be helpful to allow authors of shared modules to represent what they
know or expect from the providers or functions they are interacting with so that
OpenTofu can perform more complete checking of those assumptions during the
planning phase, and so that users of the module can run into fewer situations
where an unknown value prevents successful planning.

## Proposed Solution

This document proposes adding a number of new built-in ("core") functions to
the OpenTofu language, giving module authors direct access to the language's
type conversion and value refinement features so they can optionally tell
OpenTofu more information about the values used within and returned by a module.

### `convert` function for type conversions

OpenTofu already has a set of functions that act as _partial_ type hints
primarily to allow constructing values of types for which there is no built-in
construction syntax.

For example:

- `tolist([1, 2, 3])` constructs a value of a tuple type using the tuple
  constructor syntax and then asks OpenTofu to convert it to a list type,
  automatically inferring the element type as `number`.
- `tomap({ a = "foo", b = "bar" })` constructs a value of an object type using
  the object constructor syntax and then asks OpenTofu to convert it to a map
  type, automatically inferring the element type as `string`.
- `tonumber(x)` allows authors to force applying the same conversions that
  OpenTofu makes when a string is used somewhere that a number is expected,
  such as converting `"1"` to `1` or returning an error if `x` is not of a
  type that can be converted to a number.

The subset of these functions that can convert to various types of a particular
kind -- `tolist`, `toset`, and `tomap` -- provide no way to explicitly state
which element type is expected, and so they produce incomplete type information
when used with a zero-length tuple/object. There's also no function for
converting a value to a specific object or tuple type.

The new `convert` function would allow any type conversion that would be
allowed for input variables in a module to be performed inline as part of an
expression, giving full type information.

This function has a special signature where its second argument is interpreted
as a type constraint with exactly the same syntax and capabilities as `type` in
a `variable` block, instead of as a value.

For example:

```hcl
locals {
  example = convert(
    yamldecode(maybe_unknown),
    object({
      name = string
    }),
  )
}
```

Because just about any value supported by OpenTofu can be described in YAML,
passing an unknown string to `yamldecode` produces an unknown value of an
unknown type. The example above then tells OpenTofu to assume that an unknown
result will be convertable to the specified object type. If `maybe_unknown`
is known during the planning phase then OpenTofu will check the requested
conversion immediately, but if it's unknown then OpenTofu will assume the
conversion will succeed during the applying phase and then check anything that's
derived from this result under the assumption that it's of the specified object
type.

In this particular case, that means that if an expression elsewhere in the
module refers to `local.example.name` then OpenTofu will assume that it's a
string, and if there's an incorrect expression like `local.example.nome` (a typo)
then OpenTofu will immediately report it during the planning phase, instead of
only once `maybe_unknown` becomes known.

### `assume...` functions for refinements

The `assume...` family of functions complement `convert` by allowing module
authors to directly specify their expectations about a value beyond its type,
in ways that OpenTofu can translate into cty refinements for downstream analysis.

These functions all work by stating something that is expected to be true for
a given value, and then OpenTofu will check that assumption once the given value
is known enough to be able to confirm it and then either return the final value
or raise an error if the assumption is not satisfied.

These functions all follow OpenTofu's broad rule that the applying phase will
either do what the planning phase proposed or will return an error explaining
why not. In return for a module author making some additional promises to
OpenTofu (which would fail during the applying phase if they don't hold), any
expressions derived from the results of these functions are more likely to
identify downstream problems during the planning phase.

This document proposes the following functions, most of which directly match
a kind of refinement that `cty` supports:

- `assumeequal(got, assumed)` tells OpenTofu to assume that `got` will be
  equal to `assumed`, where `assumed` is required to be wholly-known.

    In practice this means that it checks whether `got` equals `assumed` once
    `got` is known, returning an error if not. In any case where it succeeds
    it returns `assumed` verbatim.

    `got` is implicitly converted to match the type of `assumed` before
    comparison, reporting an error if that's not possible. This automatic
    conversion is useful to allow `got` to have a wholly-unknown or
    partially-unknown type, and is a reasonable implicit behavior because
    no two values of different types can possibly be equal anyway.

    This is the strongest of all of the assume functions. Successful use of it
    requires there to be enough context available elsewhere in the module to
    completely predict the final value, such as predicting an "ARN" value for
    an AWS object based on a particular service's documented ARN syntax:

    ```hcl
    output "role_arn" {
      value = assumeequal(
        aws_iam_role.example.arn,
        provider::aws::arn_build(
          data.aws_partition.current.id,
          "iam",
          "", # Roles are global objects, so no region specified
          aws_caller_identity.current.account_id,
          "role/${aws_iam_role.example.name}",
        ),
      )
    }
    ```

    Such an approach could be useful if the resulting ARN will be incorporated
    into an IAM policy document, since otherwise the entire policy document
    source code would be unknown. Using this technique instead of just
    hard-coding the expected value means that OpenTofu will verify during the
    apply phase that the assumption was correct, avoiding potentially-confusing
    behavior if not, and that the locally-constructed value will be treated as
    having the same dependencies as the dynamic value returned from the provider.

    Although this function ultimately returns `assumed` and discards `got`, it
    will copy any marks from any part of `got` to the corresponding part of
    `assumed` before returning, so that dynamic provenance information is
    preserved. For example, this means that if `got` is derived from a
    deprecated value then uses of the corresponding part of `assumed` will also
    generate the relevant deprecation warnings. Since the two values are required
    to be equal for a successful result, there is no ambiguity in how to merge
    the marks from `got` and `assumed` before returning.

    There are two practical exceptions though: "sensitive" and "ephemeral" marks
    from `got` are just completely dropped unless equivalent marks are present
    in `assumed`. For example, if `got` contains an ephemeral value that's
    equal to a non-ephemeral part of `assumed` then it's fine to use the
    non-ephemeral value in contexts where ephemeral values are not allowed,
    because OpenTofu can assume that `assumed` will not change between the
    planning and applying phases regardless of what `got` ends up set to,
    and the call during the applying phase will return an error if the two
    values end up not matching at that point and so an unexpected ephemeral
    value would not "leak out". If `assumed` itself contains sensitive or
    ephemeral marks then those are still preserved into the result.

- `assumenotnull(v)` tells OpenTofu to assume that the result will definitely
  not be null, so that `result == null` and `result != null` comparisons can
  return known values.

    An unknown value can have this refinement only if its type is known, so
    this function returns an error if `v` has an unknown type. The error will
    suggest using the `convert` function, or some other means of type
    conversion, to also specify which type to assume.

    Using this function in combination with other `assume...` functions may
    cause OpenTofu to make additional deductions. For example, if
    `assumelistlength` specifies the same value for minimum and maximum length
    and the value is also assumed to not be null then the result will be
    automatically be promoted to a known list of the given length whose values
    are all unknown, instead of an unknown list whose length is unknown.

    Since the other `assume...` functions have implicit type conversion behavior
    built into them, using other functions to build the argument to
    `assumenotnull` avoids the need for an explicit call to `convert`:

    ```hcl
    assumenotnull(
      assumestringprefix(local.subnet_id, "subnet-"),
    )
    ```

    `assumestringprefix` implies that the result is a string and therefore
    `assumenotnull` receives a string value even when `local.subnet_id` does
    not yet have a known type.

- `assumestringprefix(s, prefix)` converts `s` to a string in the same way as
  `tostring` does and then returns it with the additional refinement that it
  begins with the string given in `prefix`.

    ```hcl
    assumestringprefix(aws_subnet.example.id, "subnet-")
    ```

    Unknown strings refined in this way can return a known `true` for a test
    like `result != ""`, and the `startswith` function can return a known result
    if the refined prefix is at least as long as the prefixed being tested which
    can be useful for checking input variable validation rules against unknown
    strings during the planning phase.

- The `assumelistlength...` family of functions convert the given value to a
  list in the same way as `tolist` does and then return it with the additional
  assumption that its length is between the specified bounds.

    - `assumelistlength(l, min, max)` specifies both bounds
    - `assumelistlengthmin(l, min)` specifies only the lower bound
    - `assumelistlengthmax(l, max)` specifies only the upper bound

    Providing a nonzero minimum length means that `length(result) != 0` can
    return `true` even when the actual length is unknown.

- The `assumesetlength...` family of functions are similar to `assumelistlength...`
  but for sets, beginning with the same conversion as `toset` performs.

- The `assumemaplength...` family of functions are similar to `assumelistlength...`
  but for maps, beginning with the same conversion as `tomap` performs.

The proposed functions all follow the established naming convention in OpenTofu
where built-in functions have words all in lowercase without any underscores
delimiting words, which unfortunately makes their names quite "clunky". However,
being consistent seems more important because it's annoying to have multiple
conventions and force authors to constantly refer to the documentation to find
out which convention is used for each function.

## Implementation Details

### Ambiguity of primitive type symbols in the second argument to `convert`

The use of type expressions in the second argument of `convert` runs into a
tricky problem with how OpenTofu resolves references between objects using
static analysis.

Consider the following example:

```hcl
convert(local.example, string)
```

The second argument `string` is a valid type expression, but the initial static
analysis pass to find references doesn't have enough information to treat that
argument in a special way and so in today's OpenTofu it would misunderstand
that argument as a reference to a _value_ symbol `string`. Since there is no
reserved prefix of that name, OpenTofu would recognize it as a reference to
a resource of type `string` and report it as invalid because a reference to
a resource must always have at least two parts.

```
╷
│ Error: Invalid reference
│
│ A reference to a resource type must be followed by at least one attribute
│ access, specifying the resource name.
╵
```

Thankfully, the fact that a standalone symbol like `string`, `number`, `bool`,
or `any` without a subsequent attribute access is never a valid resource
reference also suggests a solution: we can change OpenTofu's reference parser
to treat these specific symbol names differently so that a standalone reference
is treated as a successful reference to a different type of referenceable
address, instead of returning an error. The codepath that populates
`hcl.EvalContext` based on the reference analysis would then silently skip
addresses of that special type, so that there would be no matching symbol in
the HCL evaluation context.

When used correctly as part of `convert` that outcome would work fine because
the conversion function would not attempt to resolve its second argument as
a value expression anyway. If one of these special symbols appears in another
location where value expressions are expected, HCL would attempt to resolve
it in the symbol table and would return its own error message for the missing
symbol:

```
╷
│ Error: Unknown variable
│ 
│ There is no variable named "string".
╵
```

This reveals HCL's different meaning of the word "variable" that we
normally hide behind OpenTofu's custom error messages, which is not ideal but
is an acceptable compromise to make this feature work.

(The new language runtime discussed in
[A new approach to configuration evaluation, planning, and applying](./20251001-eval-plan-apply-architecture.md)
will also need its own variation of this workaround, but we'll deal with that
outside of this RFC because the new runtime is still under active development
anyway, and so implementing `convert` in that context is not urgent.)

### Initial implementations of the `assume...` functions

The author of this RFC previously implemented a set of functions with names
matching the `assume...` functions in this proposal as part of the provider
plugin [`apparentlymart/assume`](https://github.com/apparentlymart/terraform-provider-assume).

The overall meaning and base functionality of the functions in that provider
matches the functions in this proposal, giving confidence that all of the
described behaviors are possible. The author is willing to relicense those
implementations under MPL 2.0 to use as a starting point for the implementations
in OpenTofu, if desired.

The functions in the provider actually have a few small, minor differences
compared to what is proposed above, and so further work would be needed to build
on those implementations. For example, provider-defined functions cannot handle
cty "marks" precisely because the provider plugin wire protocol cannot represent
them, but the built-in implementations should be written to preserve marks
exactly as they appear in the input value unless stated otherwise in the
descriptions above.

### Metadata and language server considerations

OpenTofu currently offers a command `tofu metadata functions -json` which
describes all of the built-in functions in a machine-readable JSON format. This
output is intended for use by external tools, such as the OpenTofu language
server (tofu-ls) which uses it for code completion and function signature
feedback.

The `"type"` property used to describe the expected type for a function argument
is currently just the default JSON representation of a `cty.Type` provided by
the upstream cty library, which doesn't have any way to represent type
expressions because they rely on the HCL-level concept of "custom decoded"
function arguments.

Therefore we'll modify the implementation of `tofu metadata functions` to
have a special case for when a function parameter is defined as accepting the
special capsule type we'll be using to trigger the custom decoding mechanism
in HCL, and arrange for it to produce the JSON serialization `"type"`. This
means then that the JSON description of the `convert` function will be:

```json
{
  "return_type": "dynamic",
  "parameters": [
    {
      "name": "value",
      "type": "dynamic"
    },
    {
      "name": "type",
      "type": "type"
    }
  ]
}
```

The OpenTofu langauge server should then react to this parameter type by using
the same code completion behavior that would be used for the `type` argument
in a `variable` block, since the expected content in those positions is
identical.

Note that introducing a new value for the `"type"` property means that consuming
code that is decoding this format by using `cty.Type.UnmarshalJSON`
(or equivalent) will encounter an error, because cty itself does not recognize
that type. It will return the error `invalid primitive type name "type"`.

The OpenTofu documentation has never actually made promises about what can
appear as the value of that property, so we have not guaranteed that it would
always be something that `cty` alone could parse. The change in this section
means that any software intending to parse the functions metadata JSON output
will be required to use OpenTofu-specific code to do it, rather than using
the cty implementation directly. As part of implementing this change we will
update the documentation for this JSON output format to describe the
representation of the `"type"` property in more detail, including a note that
consumers should be prepared to deal with unrecognized values that might be
added in future versions of OpenTofu.

## Future Considerations

The following sections describe some future possibilities related to the
features described in this proposal which are nonetheless intentionally left
for later proposals to limit the size of this one.

### Explicit type constraints in `output` blocks

This proposal makes it possible to declare an explicitly-typed output value
by using the `convert` function in its value expression, like this:

```hcl
output "example" {
  value = convert(local.example, map(string))
}
```

This RFC could therefore be considered as a solution to
[opentofu#2831](https://github.com/opentofu/opentofu/issues/2831), although
relying on dynamic evaluation for the type conversion means that it would be
challenging for static-analysis-based module documentation tools to reliably
extract type information.

Therefore we may still consider adding an explicit `type` argument to `output`
blocks in a future proposal, making it easier for documentation tools to
consume the type by static analysis just as they would for `variable` blocks:

```hcl
output "example" {
  type  = map(string)
  value = local.example
}
```

That new syntax would be arguably redundant with the `convert` function, and so
a proposal for it will presumably justify the redundancy by showing evidence
that specific documentation tools intend to use the declarative syntax.

### Refinement information in the plan diff UI

Currently OpenTofu treats specific types and unknown value refinements as
internal details used for analysis and validation, rather than something exposed
directly in the user interface.

For example:

- The type of a value shown in the plan UI is often at least partially implied
  by presentation syntax rather than stated explicitly.
- Unknown values are shown as just `(known after apply)` without any information
  about their refinements.

If we make these mechanisms more directly accessible to module authors then we
may find that they'd prefer OpenTofu to give explicit feedback in the plan
UI about these so that the author can use it for manual testing during
development.

For example, an unknown value with a string prefix refinement could be rendered
as if it were a string template including the prefix as a literal part:

```
  + vpc_id = "vpc-${(known after apply)}"
```

It's not so clear how we would present other kinds of refinements concisely in
the plan output. For example, there is no readily-available syntax for
representing "definitely not null".

Changes to the plan UI are explicitly out of scope for this proposal, but a
future RFC could potentially propose exposing additional detail in the plan
UI if we can find a way to do it without making the output overwhelming or
confusing to those who are not yet aware of these more complex concepts.

### Refinement information in the JSON plan description

As with the plan UI in the previous section, OpenTofu also does not currently
expose details about unknown value refinements in the machine-readable JSON
output describing a plan.

A future RFC could propose to expose some or all of the refinement information
so that it can be used by other software used alongside OpenTofu, such as
a "policy as code" tool that may be able to detect a policy violation based
on a refined unknown value even though the full final value is not known yet.

Changes to the JSON plan output are explicitly out of scope for this proposal,
because that warrants some careful design of its own to find the best tradeoff
in what to represent and how to represent it for practical use by external
software.

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

  # NOT CURRENTLY VALID: This is hypothetical syntax for inline assumptions
  # about provider-selected attribute values.
  id = assumed(notnull(stringprefix("vpc-")))
}
```

During the planning phase then OpenTofu could apply additional refinements
directly to the `aws_vpc.example.id` result _and_ show the assumptions directly
as part of the plan for this resource as described in the previous two sections:

```
  # aws_vpc.example will be created
  + resource "aws_vpc" "example" {
     + cidr_block = "192.168.0.0/24"
     + id         = "vpc-${(known after apply)}"
     (...etc...)
  }
```

OpenTofu could then check that the assumptions were correct after the provider
returns the final state of this new resource instance, returning an error
similar to a postcondition failure if not.

Although this idea is potentially useful and has a number of desirable
qualities, it seems to require defining another little microsyntax for
describing the assumptions (since the value we're making assumptions about
is implicit in the attribute name, rather than an explicit argument as with the
proposed functions) and would be a lot more intrusive into OpenTofu's existing
implementation of plan and apply.

Therefore this proposal focuses on the function-based approach to start, though
we might consider something like the above later if it seems warranted based
on our experiences with the functions from this proposal.

### Custom Type Aliases

Another proposal [Symbol Libraries](https://github.com/opentofu/opentofu/pull/4052)
is currently under discussion, and it is considering a means for authors to
define custom-named aliases for types so that they can be reused in multiple
places without copy-paste.

Accepting both of these proposals implies that whatever extensions that proposal
makes to the `type` argument in a `variable` block should also be available in
the second argument to `convert`. Exactly how that would be achieved is left
to the other proposal to define, since it'll presumably be similar to that
proposal's implementation strategy for custom type aliases in input variable
declarations.
