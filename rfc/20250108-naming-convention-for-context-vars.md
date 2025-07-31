# Naming convention for internal variables representing "contexts"

There are currently various different ideas in OpenTofu's internals that are named using the noun "context", with the most prominent being:

- `tofu.Context` representing an instance of the overall language runtime with a given set of plugins, that callers can ask to perform plan/apply/etc operations against.
- `tofu.EvalContext` as an implementation detail of the language runtime that carries various global mutable state used in the implementation of different graph node types.
- `hcl.EvalContext` for specifying which variables and functions should be available when evaluating an HCL expression.

Following typical Go naming idiom, instances of these types in various parts of the codebase have tended to be stored or passed in variables named `ctx`, which means that the current meaning of `ctx` varies considerably depending on which part of the codebase you are reading, and various ad-hoc naming conventions have emerged for dealing with situtions where several of these need to be present in the same scope.

The upstream Go project later introduced a new ecosystem-wide idiom in [`context.Context`](https://pkg.go.dev/context#Context), which is also conventionally passed in arguments named `ctx`. We'd like to gradually adopt that idiom throughout OpenTofu so that we can interoperate better with third-party implementations of cross-cutting concerns like logging and tracing, but those additions then tend to cause even more conflict in what `ctx` might mean in any particular function.

This is a lightweight proposal aiming only to settle on a new, and less ambiguous naming convention that we would follow in new code and hopefully also gradually retrofit into old code.

## Proposed Solution

Adopt the following as conventional names for variables of each type throughout the OpenTofu codebase:

* `context.Context` as `ctx` (following the Go-ecosystem-wide naming convention)
* `tofu.Context` as `tofuCtx`
* `tofu.EvalContext` as `evalCtx`
* `hcl.EvalContext` as `hclCtx`

Aside from `tofuCtx`, these specific variable names are proposed because they each already have at least some precedent in the codebase from situations where there were multiple different context types in scope at the same time. The `evalCtx` name in particular was adopted recently as part of [#2161](https://github.com/opentofu/opentofu/pull/2161), while the others have been in longer use. Standardizing on these previously-informal conventions means that some of our code is already following the conventions, and so we are less likely to encounter confusing conflicts between existing and new usages. `tofuCtx` has no existing precedent -- existing code uses `tfCtx` -- but the number of instances is small enough that the risk of confusion is low.

These plain names are appropriate to use whenever there is only one variable of a type in scope and there is no further meaning to communicate beyond what type it has. In some situations multiple contexts of the same type are in scope, which warrants additional prefixes to clarify the relationships between them. For example, functions that deal with the transition from static objects from the configuration to dynamic instances declared by `count`/`for_each`/etc will tend to have `globalEvalCtx` and `moduleEvalCtx`, both of type `tofu.EvalContext` but with the former being unscoped and the latter scoped to a particular module instance path. The general rule then is to add the modifier word to the front of the name but to retain the above naming conventions for the type-specifying suffix.

### User Documentation

This is an internal change that does not impact the user experience in any way.

### Technical Approach

Because this proposal is primarily about naming conventions, its main vehicle for "implementation" is for us to take care to use the new names for new code we write and to oppportunistically update existing code to follow the new conventions in any situation were we're already making substantial changes to it. This proposal does not call for proactively making sweeping changes across the whole codebase, because such changes tend to make Git history harder to follow and make it harder to backport bug fixes to older release branches.

In principle it would be possible to use a linting tool to raise errors if any new variables named `ctx` had a type other than `context.Context`, to reinforce that we're intending to follow the Go-ecosystem-wide naming convention for that in a similar way as we avoid using the name `err` for variables of types that don't implement `error`. However, our existing linter aggregator `golangci-lint` does not currently include any linter that be configured to enforce rules like that, and at this time it doesn't seem worth the effort to implement such a linter ourselves since the core team can take care to use the new names in new work and can give feedback about naming on community PRs when needed. We could revisit this decision later if we find that in practice we are spending substantial time giving that kind of feedback.

### Future Considerations

#### Future work on `context.Context` plumbing in the language runtime

The earlier work in [#2161](https://github.com/opentofu/opentofu/pull/2161) arranged for a chain of `context.Context` to pass all the way into the language runtime, stopping just short of changing the various graph-node-related interfaces that would therefore require changing many implementations all at once.

In any future work that changes the signature of `tofu.GraphNodeExecutable.Execute` or `tofu.GraphNodeDynamicExpandable.DynamicExpand` in a way that requires updating all callers, we should take that opportunity to change their signatures so that there is an additional first argument `ctx context.Context` and so the existing `tofu.EvalContext` argument is named `evalCtx` in all implementations. That would then go a long way to establishing this new naming convention as the prominent one, despite there still being some stragglers left in leaf functions called by `Execute` that we would need to clean up separately.

At the time of writing this proposal there is [a prototype of entirely removing `GraphNodeDynamicExpandable`](https://github.com/opentofu/opentofu/pull/2285) in favor of inlining the "dynamic expansion" logic inside `GraphNodeExecutable.Execute`. If we decide to move ahead with that as a real project, that project could be a good place to absorb the cost of changing the `GraphNodeExecutable.Execute` signature since that project is likely to cause changes to most existing implementations of that interface for other reasons anyway.

#### `tofu.EvalContext` ought not to be exported

`tofu.EvalContext` is intended as an implementation detail of the language runtime and is exported mainly just because early code in this codebase was written by those who preferred to export symbols "just in case" they'd be useful in future work, whereas the API boundaries between subsystems emerged later as understanding of and experience with the system grew.

At some future point we would ideally rename this type and its implementations to have an unexported name, at which point the naming convention of `evalCtx` would apply only to code within `package tofu`. The other conventions would remain cross-cutting. To increase the likelihood of that being viable before too long, we should avoid introducing any new uses of `tofu.EvalContext` outside of `package tofu` even though that is currently technically possible.

#### Other "context" objects we might add in future

This proposal has focused on the four main "context" types that we currently have in broad use. "Context" is a relatively intuitive name for objects that carry miscellaneous cross-cutting information about the ambient situation where some work is being performed, and so future work might propose new types whose primary noun is "context".

In that event, the each new proposal should include a conventional name to use for a variable of that type in similar vein to the conventions in this proposal. Wherever possible, new proposals should avoid using this highly-overloaded noun and choose something more specific instead, but not to the point where the proposed new name becomes highly contrived or unintuitive.

## Potential Alternatives

Since the name `ctx` is most commonly used for variables of type `tofu.EvalContext` today, for historical reasons, it might be more pragmatic to keep using the short name for the contexts of that type and to adopt a different name for the cross-cutting `context.Context` values. We could also potentially choose to follow some different rules only for `package tofu`, while following the broader Go ecosystem conventions in other packages where `tofu.EvalContext` is not used.

However, that would cause our naming conventions to deviate significantly from the prevailing Go community idiom, making it less likely that new contributors would follow our conventions on their first attempt and so increasing the likelihood of at least one review feedback round-trip.

We could also continue with the current situation where naming is decided ad-hoc in each local situation separately. This has admittedly worked out okay in most situations so far, but does tend to cause confusion for folks working on broader changes that involve reviewing and/or modifying many different parts of the codebase at once.
