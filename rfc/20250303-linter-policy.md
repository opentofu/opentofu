# OpenTofu Codebase Linting Policy

Early in the life of the OpenTofu project the team adopted a large [golangci-lint](https://golangci-lint.run/) configuration containing a rather arbitrary assortment of linter configurations inherited from another project. Because OpenTofu's predecessor project did not use these lint rules, there was an insurmountable number of lint failures on existing code and so we've been working under a compromise where the lint rules are only applied to new code added after the adoption of that linter configuration.

In the meantime we've got considerable experience working under these lint rules and have encountered some friction:

- Code maintenence work often gets bogged down in resolving previously-quieted lint failures, making that work take more time and adding risk in cases where fixing the lint rule requires non-trivial refactoring.
- Certain specific linters are not really compatible with the Go style conventions used in the existing codebase, causing new code to need to be written differently than existing code in order to pass linters. In practice we've typically preferred to add `nolint` comments instead in such cases, because the lint rules in question are more subjective style preferences than truly detecting problems with the code.
- Our lint configuration includes a large number of linters with overlapping scope, meaning that certain problems are detected by multiple different linters but with different details, and so making them _all_ happy at once can be challenging.
- The runtime of `golangci-lint` in our configuration is quite long on the hardware some of our developers use, making it frustrating to iterate when addressing lint failures. This also contributes to the excessive runtime of our so-called "Quick Checks" for pull requests.

## Proposed Solution

The golangci-lint project has already curated a default set of linters that it uses when no special configuration is present. Our linting policy should change to use only those linters enabled by default, with their default settings.

We currently use golangci-lint v1.64.5, which means that our initial set of linters would be the following:

- `errcheck`: Detects situations where a function returns a value of type `error` and the caller does not use that error result.
- `gosimple`: Makes various suggestions for preferring simpler forms over more complex equivalents, such as using [the built-in `copy` function](https://pkg.go.dev/builtin#copy) instead of a hand-written loop over the source slice.
- `govet`: Performs similar checks to [`go vet`](https://pkg.go.dev/cmd/vet), thereby reinforcing some checks that the Go toolchain already makes when we run tests.
- `ineffassign`: Detects when a statement assigns a value to a variable but the subsequent code makes no further use of that variable.
- `staticcheck`: Detects various different mistakes through static analysis, such as passing a nil `context.Context` to a function.
- `unused`: Detects various kinds of ineffectual constructs such as assigning a value to a field in an object that is not subsequently used.

If we update golangci-lint in future then we would default to immediately adopting its updated defaults, and so update any existing code that fails under the new linters as part of the same PR that upgrades golangci-lint. We are assuming that the maintainers of golangci-lint are curating their default set of linters to maintain a good compromise of coverage vs. false positives based on feedback from a broad set of downstream projects, and so they will continue to curate a good set of defaults in future releases.

### Excluding legacy files and packages

We have a small number of files and packages that exist only to preserve backward-compatibility with older versions of OpenTofu or its predecessor project, where we prefer to leave the existing code completely unchanged since it is not frequently used and so if we break it during refactoring we probably won't hear about it for a long time.

We will therefore use a small `.golangci.yml` that excludes those files and packages:

```yaml
issues:
  exclude-files:
    # We have a few patterns that are excluded from linting completely because
    # they contain effectively-frozen code that we're preserving for backward
    # compatibility, where changes would be risky and that risk isn't warranted
    # since we don't expect to be doing any significant maintenence on these.
    - "^internal/ipaddr/"
    - "^internal/legacy/"
    - "^internal/states/statefile/version\\d+_upgrade\\.go$"
```

### Updating existing code

Under these new default settings, with the legacy code excluded as per the above configuration, there are approximately 80 lines of code in the current codebase that fail at least one of the lint rules.

That is a much smaller number than fail our current larger set of linters, and so we will proactively fix them all prior to adopting this new policy and thus we can remove our exception for older code and create a new lint-free baseline for future work. We have a list of the current failures which we'll place into this RFC's tracking issue if it's accepted, and then use that as a checklist to know when we're ready to apply the lint rules to the entire codebase.

We will also review all of the currently-present `nolint` comments and remove any that are no longer needed with our new reduced configuration, to avoid confusion for future maintainers about which linters are included in our policy. Coincidentally there are also roughly 80 `nolint` comments in the codebase for us to review, although not all of them will necessarily be removed as part of this work if they relate to a linter that will remain enabled and the risk of correcting it seems too high.

## Future Considerations

This proposal calls for us to rely entirely on the golangci-lint project's default linter settings, since that represents a broad and durable policy decision that avoids the need to debate and negotiate each individual linter and its fine settings.

Over time we may notice during code review that certain mistakes happen often, or that certain mistakes have particularly costly consequences, and if so that would represent new information that could justify changing the broad policy described in this document. In particular, we may consider certain OpenTofu-specific lints that the golangci-lint project would consider to be out of scope, such as something similar to `errcheck` which also understands the OpenTofu-specific usage conventions of `tfdiags.Diagnostics`.

When making such decisions we will weigh the benefit in reduced risk against the likely maintenence cost of the custom linter or lint configuration and the changes it would require to existing code. In particular, we will prefer to try to meet our needs by small configuration changes of linters we are already using rather than introducing entirely new linters, or developing our own golangci-lint plugins. One individual making a particular mistake is unlikely to justify a change to our linting policy unless that mistake both had significant consequences and that mistake seems likely to be repeated by someone else.

## Complexity Linting Disabled

Notably, none of the "complexity-related" linters previously discussed in [A Pragmatic Approach to Linting for Code Complexity](2024113-pragmatic-complexity-linting.md) are included in golangci-lint's default set, and so accepting this proposal effectively implies cancelling our work on that earlier RFC.

This does not mean that we shall not continue working to make existing code easier to understand and maintain, but only that we will not rely on broad qualitative analysis by technology to achieve that. Instead, we will encourage team members to comment during the code review process if they find certain code hard to follow due to its structure or complexity, and we'll agree as a team to take that kind of feedback seriously and make a good-faith effort to address it.

At the time of the previous RFC discussion a significant number of OpenTofu core team members spoke in favor of using the methodology in that proposal to proactively address all of the existing complexity lint failures, but after accepting that RFC there was insufficient motivation to actually follow through on the project, suggesting that in practice the cost of using these linters exceeds their value.

Although that project would be effectively cancelled by accepting this proposal, team members would still be encouraged to refactor particularly-egregious examples when time allows, potentially using the list of problems from [that project's tracking issue](https://github.com/opentofu/opentofu/issues/2325) as a starting point.

## Potential Alternatives

### Selectively disable or reconfigure existing linters

This proposal was made in response to various ongoing small discussions about the cost/benefit tradeoff of individual linters in the comments a variety of different pull requests. The most recent occurrance, which directly motivated writing this proposal now, was [a proposal to disable a specific linter called `mnd`](https://github.com/opentofu/opentofu/pull/2553#discussion_r1976443515) (which appears to stand for "magic number detector") which at the time of writing we've now included in 20 different `nolint` comments throughout the codebase, many of which are annotated with comments indicating varying degrees of frustration such as "This check is stupid".

A less extreme version of this proposal would be to start with our current linter configuration and gradually disable or reconfigure individual existing linters that seem to frustrate us more than help us. If we took this strategy, it seems likely that the `mnd` linter would be the first to be disabled.

This proposal instead takes the opposite approach of starting with a minimal set of linters in their default configurations, with the option of growing that set cautiously over time if we become aware of new categories of frequent/costly mistake. This approach also makes it more feasible for us to rework all existing lint failures so that we can move forward with a zero-lint baseline for future work.

### Change nothing

Although many of us have been frustrated with one or more of the linters at _some_ point, the costs described at the start of this proposal are not huge and we _could_ probably live with them and continue in our current mode of placating the linters where possible and adding `nolint` comments otherwise.

However, based on our experience so far it does seem like we would end up constantly revisiting this decision intermittently forever, and making repeated tradeoffs about just how much change to existing code we're willing to tolerate as a side-effect of other work.
