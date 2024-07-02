# Add Exclude Flag for Targeted Plan/Apply

Issue: https://github.com/opentofu/opentofu/issues/426

This RFC details a new flag, `-exclude`, which would do the inverse of `-target` - that is, plan/apply all resources except those specified by the exclude flag and their descendants.

Only for use in exceptional circumstances as is the case for `-target`, this makes it a lot easier to plan/apply all changes to an environment except ones with known issues eg where there is a problem with the infrastructure or the committed config for one module/resource, but you still want to apply everything else.

https://github.com/hashicorp/terraform/issues/2253 is one of the most requested features in the Terraform repo


## Proposed Solution

Add a new flag, `-exclude`, to plan and apply. This should allow a plan to be created, or apply to be run, for all resources except those that are "excluded" and their children.

### User Documentation

- Usage example:
`opentf plan -exclude module.something_broken` / `opentf apply -exclude module.something_broken` should be added to --help output for plan/apply.
- Currently, it looks like `-target` is not mentioned in the docs/ directory of the repo.
- `-target` is documented in website/docs/cli/commands/plan.mdx, so we should also document `-exclude` there
- `-target` is *not* documented in website/docs/cli/commands/apply.mdx, but probably should be. It would be nice to add documentation for both `-target` and `-exclude` there

### Technical Approach

Technical summary, easy to understand by someone unfamiliar with the codebase.

- `-exclude` flag added to extendedFlagSet when an operation is provided (same as `-target`)
- Passed through to `OperationRequest`
- `TargetsTransformer.Transform` should remove vertices that are in (or children of items in) `t.Excludes` if `t.Excludes` are provided (just like it currently removes vertices that _aren't_ in `t.Targets`).
- If both targets and excludes are provided, use the most specific path eg:
```-exclude module.entire_environment \
-include module.entire_environment.module.network \
-include module.entire_environment.module.application \
-exclude module.entire_environment.module.application.module.eks
```
should exclude the whole entire_environment module but include the network and application modules within, except the eks module within the application module. This should be the outcome regardless of the order of the flags.
- Return an error when `-target` and `-exclude` point at the exact same resource
- When destroying, if there are dependencies of an excluded resource, the dependencies should also be kept (excluded from change)

https://github.com/opentofu/opentofu/pull/427 this rough proof of concept was previously drafted although there are a few issues
- not code complete
- outdated with merge conflicts
- some unnecessary changes were included
I would likely start from scratch on the latest main branch, and only include the necessary changes.

Potential limitations or impacts on other areas of the codebase: backends are imported from TFE which we can't change. We discussed in the issue that there could be some incompatibility, depending how backends handle `-target` (as we can't force them to support `-exclude`). This doesn't initially look like it will be a problem, but may be worth noting.

### Open Questions

- 

### Future Considerations

- 

## Potential Alternatives

- 
