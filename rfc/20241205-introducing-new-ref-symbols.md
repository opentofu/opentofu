# Backward-compatible Addition of new Reference Symbols

The evolution of the OpenTofu module language is currently constrained by [the OpenTofu v1.x Compatibility Promises](https://opentofu.org/docs/language/v1-compatibility-promises/), which require that any module that was accepted without errors in any earlier v1.x version must continue to be accepted with equivalent meaning by all future releases in the v1.x series.

Although there are many language changes that we can make without breaking those promises, there are two areas that are particularly challenging:

1. Introducing new top-level "[Named Values](https://opentofu.org/docs/language/expressions/references/)" (also sometimes called "symbols"): the OpenTofu language assumes that any root name not already defined is intended to be a reference to a managed resource.

    For example, `foo.bar` is interpreted as a reference to the object(s) produced by a `resource "foo" "bar"` block. The namespace of managed resource types is an extension point, so if a later version redefined the `foo.` prefix to mean something else then that would be a breaking change.

    Although it is pretty unlikely that there is a provider with a resource type that is literally "foo", we cannot prove that there is no such provider and so to keep our compatibility promises we must assume that such a provider exists.

2. Introducing new "Meta-Arguments", like `count` and `for_each` in resource definitions, in any block type whose normal arguments are defined by something outside of OpenTofu Core.

    For example, `enabled` in a `module` block is understood today as defining an input variable named `enabled` for the child module. If a later version decided that were a meta-argument that selects between either zero or one instances of the module then that would be a breaking change for calls to any existing module with a `variable "enabled"` block in it.

This proposal is related only to the first of these problems. The second would also be good to solve, but the solution proposed here is not sufficient to fully solve that problem. (Some of what's described here could potentially be used as _part of_ a later solution to problem 2, but that remains to be seen.)

## Proposed Solution

The OpenTofu language already includes a _partial_ solution to the first problem: the symbol `resource` is reserved as an alternative way to refer to managed resource blocks that can bypass any already-reserved symbol. For example, if someone were to create a provider with a managed resource type named `path` today then it would be valid to write a `resource "path" "example"` block, but it would not be valid to refer to that block as `path.example`. Instead, the author would need to write `resource.path.example` to force the second segment to be interpreted as a resource type name instead of a reserved prefix.

That solution is not _sufficient_ because forcing someone to retrofit an existing module with `resource.` prefixes in front of any previously-working references would be considered a breaking change by our compatibility promises.

To complete that story for new reserved symbols we might want to add in future, we need a way for a module author to _opt in_ to using the new symbol so that any previously-published modules will continue to treat that new symbol as a reference to the results of a `resource` block.

In earlier work I proposed and made some preparations for [explicit language editions](https://log.martinatkins.me/2021/09/25/future-of-the-terraform-language/#opt-in-language-editions) similar to the Rust ecosystem, where a module would state in a single place which edition of the language it is intending to target and that selection would then opt in to potentially many different new language features at once.

However, it's unlikely that any single feature would ever be important enough _alone_ to justify the effort and complexity introducing a new language edition. I believe we need a lighter-weight mechanism that allows opting into a single feature in isolation, ideally without the need to write anything particularly unusual in a newly-written module, and this so proposal is for a potential pattern we can follow to handle opting in to specific new symbols.

In practice most new top-level symbols are introduced at the same time as a new top-level HCL block type. For example, the `data.` prefix became valid in the same release as it became valid to write a `data` block. Introducing a new top-level block type is _not_ a breaking change, so for any feature that follows this pattern of introducing both at once we can use the presence of at least one block of the new type as implying that the module author intends to use the new top-level symbol in expressions elsewhere in the module.

Just as a motivating example, I'll use the "symbol library" idea previously discussed in [a comment on issue #1962](https://github.com/opentofu/opentofu/issues/1962). That idea is a classic example of something which, as currently proposed, would be blocked by the 1.x Compatibility Promises due to introducing a new top-level symbol:

```hcl
symbols "serverless" {
  source = "./serverless.tfsym.hcl"
}

locals {
  example_thing = symbols.serverless.some_value
}
```

Although it's backward-compatible to introduce a new block named `symbols`, the `symbols.` prefix used in an expression in the `locals` block must continue to be interpreted as a reference to a `resource "symbols" "serverless"` block unless we have some evidence of author intent to use the new feature.

This document proposes that the presence of at least one `symbols` block acts as the opt-in, causing the expression reference parser to understand `symbols.serverless` as a reference to the `symbols "serverless"` block instead. In the unlikely event that the module is also using a provider with a resource type that is literally named "symbols" they would need to refer to such resources with the explicit prefix, like `resource.symbols.example`.

This document _does not_ intend to propose the specific idea of introducing a `symbols` block and a `symbols` reference symbol as shown above. This is instead a proposal for a general design pattern that we could follow for any new language feature that involves references to a new type of block, proposed in a separate RFC here only in the hope of it involving a single technical solution that we can then reuse for many different language features in future. `symbols` is just an example to help with describing the problems and solutions.

## User Experience

Under this proposal, each individual module has a set of "opt-in" flags with one for each distinct feature. For each of those features, a given module is either opted in to that feature or not. Different modules in the same configuration may be opted in to a different subset of the available features, so that it's possible to adopt new features gradually on a module-by-module basis.

The most prominent user experience concern for this proposal is what should happen in the cases where a module is inconsistent: either trying to use a feature that is not opted in, or trying to refer to a resource whose type conflicts with a feature that is opted in without using the `resource.` prefix.

### Using a feature that is not opted-in

This case is relatively unlikely because if there are no new blocks of the new type then there is presumably nothing to refer to using the new symbol. Nonetheless, module authors tend to crib from a variety of different examples when writing modules and so it's possible that someone will copy something from an opted-in module without copying the opt-in block.

The behavior in this case depends on whether there is already a `resource` block in the module whose first label matches the new symbol:

- If so, we mustn't generate any new errors or warnings and so we'll just interpret any references as being resource references with the same rules as today's OpenTofu, and those references will either succeed or fail with unchanged error messages.
- If not, we'll pretend that the feature _is_ opted in and generate an error message with that in mind.

    In the "symbol library" example, an expression like `symbols.serverless` would cause an error reporting that there is no `symbol "serverless"` block, rather than reporting that there is no `resource "serverless" "some_value"` block, since that better matches the most likely user intent and avoids the author having to learn that this feature has any special opt-in behavior.

### Conflicting with a feature that is opted-in

The following module would be worst-case example of such a conflict:

```hcl
resource "symbols" "serverless" {
  # ...
}

symbols "serverless" {
  source = "./serverless.tfsym.hcl"
}

locals {
  example_thing = symbols.serverless.some_value
}
```

This case is particularly troublesome because we have conflicting signals of user intent. Since the `symbols` block is the newer feature, we would assume that the author was intending to refer to the `symbols "serverless"` block. If resolving `symbols.serverless.some_value` in terms of the `symbols` block fails, any error message we generate should be carefully written to be clear that OpenTofu understood this expression as a reference to a `symbols` block to give the reader of the error message a hint about what their mistake might have been.

A less-troublesome case is when the resource and the new block type have differing labels, and so we have some more clues:

```hcl
resource "symbols" "anything_except_serverless" {
  # ...
}

symbols "serverless" {
  source = "./serverless.tfsym.hcl"
}

locals {
  example_thing_1 = symbols.serverless.some_value
  example_thing_2 = symbols.anything_except_serverless.some_value
}
```

Although we could potentially use global information to interpret the `symbols.` prefix differently on each of the local value expressions, subjectively I think that's too confusing and I think it's important that we always interpret this prefix in the same way throughout the scope of a particular module. Therefore in this case the expression for `local.example_thing_2` should generate an error message that directly mentions the `resource.` prefix, so that the author has an opportunity to learn about this hazard and correct it:

```
Error: Reference to undefined symbol library

  on example.tf line 11, in locals:
   11:   example_thing_2 = symbols.anything_except_serverless.some_value

There is no symbols block named "anything_except_serverless" defined in this module.

Did you intend to refer to resource "symbols" "anything_except_serverless"? If so,
use the "resource." prefix:
    resource.symbols.anything_except_serverless.some_value
```

The "Did you intend..." part of this error message would appear only if the module contains a matching `resource` block, to avoid creating confusion for someone who hasn't even considered that there might be a resource type named "symbols" in some provider somewhere.

Note that `symbols.` can only be interpreted as a reference to a symbol library if there is at least one `symbols` block defined elsewhere in the module _or_ if there are no `resource "symbols"` blocks anywhere in the module, so any module written before the inclusion of this feature could not produce this error new message. This error message is also only reachable in the highly unlikely case that the author is trying to use a provider that has a managed resource type literally named "symbols".

## Technical Approach

### Generalizing the idea of "experiments"

The main technical requirement for this proposal is for `package configs` (which contains the static decoding logic for the module language) to provide some new API that reports whether a particular new feature is "opted-in".

We already have some mechanisms for talking about individual features in [`package experiments`](https://pkg.go.dev/github.com/opentofu/opentofu@v1.8.7/internal/experiments), which is currently somewhat vestigial since OpenTofu does not currently make use of the "language experiments" mechanism from its predecessor project.

We can rename that package to `features`, and rename its `Experiment` type to `Feature`, and then use values of that type to represent features in three possible states:

1. **Active Experiment:** Not enabled unless _explicitly_ opted-in with the `experiments` argument in a `terraform` block. Only supported in builds that have experimental features enabled. (At the time of writing, _no_ OpenTofu releases have experimental features enabled; we might choose to change that for nightly builds and/or alpha releases in future.)
2. **Selectable:** No longer selectable in the `experiments` argument, and instead opted in implicitly by using a specific block type somewhere in the module. If the feature was previously an "Active Experiment" then its conclusion message should describe how the new implicit opt-in works, to help module authors migrate away from any earlier explicit experiment opt-in.
3. **Removed:** No longer selectable in the `experiments` argument, and not available in any other way either. This is for experiments that don't have a successful outcome, allowing OpenTofu to return an explicit message that hopefully explains that outcome and any alternative features the author might want to try instead.

State 1 is already possible today and essentially unchanged. States 2 and 3 are split out from the current single "concluded" state, to differentiate between a successful feature that is available opt-in for stable releases vs. a feature whose experiment was unsuccessful and so was removed altogether.

In `package configs` then, `Module.ActiveExperiments` becomes `Module.ActiveFeatures` and acts as a set of _all_ opt-in features that are enabled for that module, regardless of whether they are experimental or not. Feature-specific logic in `package configs` would automatically add "Selectable" features to that set based on the presence of specific block types in the module, so other packages in the codebase do not need to be concerned with how the opt-in is designed.

### Sniffing for Resource Types in a Module

The proposed error messaging behavior also requires being able to ask whether a specific module contains either any managed resource of a specific type, or any specific resource that has a specific type and name.

Although it's technically already possible to do that by grovelling around in [`configs.Module`'s](https://pkg.go.dev/github.com/opentofu/opentofu@v1.8.7/internal/configs#Module) `ManagedResources` field, to avoid spreading those fine details all over we should add two new methods to `configs.Module` that directly answer these questions:

```go
package configs

func (m *Module) HasAnyManagedResourceOfType(typeName string) bool
func (m *Module) HasManagedResource(typeName, name string) bool
```

Internally this would, of course, just consult the `ManagedResources` field as described above, but would keep that logic encapsulated in `package configs` so that we can more easily change it in future if needed.

### Varying Reference Parsing by Enabled Features

The final part of the problem is how to expose all of the new information described in the previous sections to the code that is actually responsible for expression parsing.

The actual reference parser lives in `package addrs`, which is imported by almost every other significant package in the codebase and so it generally cannot depend on other packages. In particular, it cannot import `package configs`, `package tofu`, or `package lang` because all three already make considerable use of the symbols in `package addrs`.

To avoid that problem, we can introduce some indirection in the form of a new interface defined inside `package addrs`:

```go
package addrs

import (
    "github.com/opentofu/opentofu/internal/features"
)

type ParseRefContext interface {
    FeatureEnabled(features.Feature) bool
    HasAnyManagedResourceOfType(typeName string) bool
    HasManagedResource(typeName, name string) bool
}
```

This interface can then be implemented by `configs.Module` to allow `package addrs` to access this functionality without directly referring to any symbols from `package configs` or elsewhere.

The four functions implementing different variations of [reference](https://pkg.go.dev/github.com/opentofu/opentofu@v1.8.7/internal/addrs#Reference) parsing in `package addrs` would then grow to each take a second argument:

```go
package addrs

func ParseRef(traversal hcl.Traversal, refCtx ParseRefContext) (*Reference, tfdiags.Diagnostics)
func ParseRefFromTestingScope(traversal hcl.Traversal, refCtx ParseRefContext) (*Reference, tfdiags.Diagnostics)
func ParseRefStr(str string, refCtx ParseRefContext) (*Reference, tfdiags.Diagnostics)
func ParseRefStrFromTestingScope(str string, refCtx ParseRefContext) (*Reference, tfdiags.Diagnostics)
```

All of these ultimately delegate to the unexported [`addrs.parseRef`](https://github.com/opentofu/opentofu/blob/v1.8.7/internal/addrs/parse_ref.go#L174), which can then make use of the `refCtx` argument to vary its traversal matching based on `ParseRefContext.FeatureEnabled` and its error message generation based on `ParseRefContext.HasAnyManagedResourceOfType` and `ParseRefContext.HasManagedResource`.

### Passing a `ParseRefContext` into `package addrs`

`package tofu` uses [`Evaluator.Scope`](https://github.com/opentofu/opentofu/blob/85dc2615ad1662225ca0cff1ac0dfe818f4ea08d/internal/tofu/evaluate.go#L79) to construct the object that is overall responsible for how references and function calls are resolved in an expression.

This function does in principle have access to all of the information it needs to decide what to pass as `refCtx ParseRefContext` when parsing a reference:

- The root node of the configuration tree, in [`Evaluator.Config`](https://github.com/opentofu/opentofu/blob/85dc2615ad1662225ca0cff1ac0dfe818f4ea08d/internal/tofu/evaluate.go#L41-L42).
- The address of the specific module where expressions are to be evaluated, in the `ModulePath` field of [`evaluationStateData`](https://github.com/opentofu/opentofu/blob/85dc2615ad1662225ca0cff1ac0dfe818f4ea08d/internal/tofu/evaluate.go#L94).

However, `Evaluator.Scope` consumes `evaluationStateData` only indirectly through [`lang.Data`](https://pkg.go.dev/github.com/opentofu/opentofu/internal/lang#Data), so we'll need to add another method to that interface to fill the gap:

```go
package lang

interface Data {
    // ParseRefContext returns the reference-parsing context that should be used
    // when interpreting traversals into addresses that will be evaluated
    // using the other methods of this object.
    ParseRefContext(rootCfgNode *configs.Config) addrs.ParseRefContext

    // (and everything else it already has, unchanged)
}
```

`tofu.evaluationStateData` can then implement this new method as follows:

```go
func (d *evaluationStateData) ParseRefContext(rootCfgNode *configs.Config) addrs.ParseRefContext {
    node := rootCfgNode.DescendentForInstance(d.ModulePath) // (returns another *configs.Config)
    return node.Module // (*configs.Module implements addrs.ParseRefContext)
}
```

Any other implementation of this interface that doesn't have any need to participate in this feature-selection mechanism can safely return a `ParseRefContext` that just always returns `false` from all of its methods, forcing the baseline behavior in all cases. However, `evaluationStateData` is today the main significant implementation of this type, used for all dynamic expression evaluation in an OpenTofu module. We'll need other implementations only if any new opt-in features we add should be allowed in other contexts, such as in early evaluation. These implementations would be similar as long as expressions are being evaluated in the scope of a known module.

## Future Considerations

Any new language feature that involves the creation of a new top-level block type and an associated top-level expression symbol could potentially benefit from this proposal.

In practice it's unlikely that we would take any action on this proposal until we are ready to move forward with at least one such feature, but this is written as an independent proposal in the hope that we can follow this same pattern (and make use of the same internal mechanisms) for many new features of this shape in future.

Existing issues that could benefit this that happen to be open at the time of writing this document include:

- [Add the ability to create custom types](https://github.com/opentofu/opentofu/issues/1962) (the origin of the "symbol library" idea that this document used as an example)
- [Allow provider for_each to refer to data resources, child module output values, etc](https://github.com/opentofu/opentofu/issues/2155) (if we choose to generalize this to the point of treating references to providers as a new kind of normal value with a new `provider.` prefix for references, although it's not clear that this would involve a new top-level block type to use as opt-in so it might not be a good fit for this particular proposal)
- [Ephemeral values - prevent storing sensitive values in state](https://github.com/opentofu/opentofu/issues/1996) (if the final design for this ends up introducing any new block types and references to them; no final design is included in this issue at the time of writing)
- [Interfaces for Environments](https://github.com/opentofu/opentofu/issues/949) (discusses a new block type named `interface` and an `interface.` prefix for referring to it)

### Introducing new meta-arguments

The introduction of this proposal discussed two annoying constraints on the future evolution of the language, but this proposal only aims to solve the first of them. The addition of new meta-arguments, such as a boolean `enabled` argument for any block that currently supports `count` or `for_each`, remains an open question.

The general idea of each module having a set of opted-in language features could potentially apply to meta-arguments too, but there are two different challenges to deal with for that case:

1. What is used as the implicit opt-in to enable one of these features? There isn't typically a new top-level block type added in conjunction with a new meta-argument, so we'd need to find something else to use to represent intent to use a new meta-argument instead of passing it as a normal argument to the external provider/module.
2. Meta-argument handling currently happens too early to react to enabled features in the way this proposal discussed for reference parsing. The functions in `package configs` for decoding `resource`, `data`, `module`, `provider`, and `provisioner` blocks all currently handle the meta-arguments immediately during initial decoding, and so they are decoded while the module is still being analyzed and so we don't yet have a full set of top-level declarations to rely on to infer an opt-in.

The [Language Editions](#language-editions) idea could still potentially be used here, since that is an explicit argument intentionally isolated from the others so that it can potentially be decoded and analyzed before visiting anything else in the module. However, that still presents the same challenge that it's hard for any one feature to justify the ceremony and overhead of introducing a new language edition, and so it'd be better to have some lighter way to opt-in to only a single named feature at first.

If we _can_ find a reasonable way to represent the opt-in for a new meta-argument then the language does already have a way for module authors to continue using a module or provider that was previously using an argument that we've now reserved, though it is somewhat ugly:

```hcl
resource "example" "example" {
  _ {
    # Any argument inside a block of type "_" (literally an underscore)
    # is always treated as a regular argument and never as a meta-argument,
    # and so a module that has opted in to treating "enabled" as a meta-argument
    # can still be compatible with a provider that was expecting an argument
    # of that name at the expense of some yucky syntax.
    enabled = true
  }
}
```

## Potential Alternatives

### Language Editions

The earlier proposal text mentioned the previously-proposed idea of "language editions" and that does remain a potential alternative to this proposal.

The main challenge with language editions is that they are a cross-cutting mechanism where effective use of them ideally requires coordinating a number of changes to all be added together to justify the ceremony of introducing another edition. Despite [the various good design decisions for language editions in Rust](https://doc.rust-lang.org/edition-guide/editions/index.html), each new language edition seems to cause a fair amount of controversy around questions like: is it too soon for another edition? do we have enough improvements queued up to justify the overhead of supporting another edition? When is the scope of the new edition frozen? etc, etc.

We could still potentially use the Language Editions mechanism to bundle opt-ins for a number of new language features together _after the fact_: for those new to our ecosystem, it's likely easier for them to just learn the latest language edition wholesale rather than worrying about the details of each individual opt-in.

In that way _this_ proposal is a sort of bridge to help new features get added which could then, _after the fact_, justify introducing a new language edition to help rally around a new baseline for the language. No individual feature needs to justify the overhead of a new edition just on its own, and instead we can add individual features piecemeal and then decide later whether a new edition is justified.

If we decide to make use of language editions later then we could represent them as packs of [`feature.Feature` values](#generalizing-the-idea-of-experiments) that all get enabled for a module together when a sufficiently-new edition is selected, and so logic throughout the codebase that needs to behave differently depending on the enablement of different features can program against just one API and not need separate support for experimental opts, single-feature opts, and edition-based opts.

### Separate mechanism for each feature

It isn't actually required that we have a single, shared mechanism for representing the status of many different opt-in features. We could instead just let each feature be responsible for its own opt-in mechanism, tailoring the design to only exactly what each new feature needs.

This proposal's goal is to standardize on a specific approach firstly so to keep the internal APIs for communicating about these features relatively straightforward and consistent, and secondly so that module authors can over time develop some expectations for how our feature opt-ins typically behave, particularly if we also later decide to offer other ways to enable/disable them such as language experiments and language editions.
