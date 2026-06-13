# Prerelease Provider Version Constraints

Issue: [https://github.com/opentofu/opentofu/issues/1651](https://github.com/opentofu/opentofu/issues/1651)

Currently, prerelease versions (e.g. `1.0.0-beta.1`, `1.0.0-rc.1`) can only be selected by
an exact version constraint (`= 1.0.0-beta.1` or `1.0.0-beta.1`). Inexact operators such as
`>=`, `~>`, `!=` silently exclude prerelease versions even when a prerelease suffix is
included in the constraint boundary. For example, `>= 1.0.0-0` behaves identically to
`>= 1.0.0`: no prerelease versions are ever matched.

This makes it impossible to track a rolling prerelease channel (e.g. always use the latest
`1.0.x` beta) without manually updating an exact pin on every new prerelease.

The version constraint syntax OpenTofu uses is modeled on RubyGems. RubyGems itself has a
well-defined opt-in convention for prerelease matching, but OpenTofu never implemented it.

Issue [#2147](https://github.com/opentofu/opentofu/issues/2147) also touches on prerelease
version quirks, but specifically in the module installer, where two competing parsers create
additional complexity. This RFC addresses only provider version constraints, where the code
path is simpler and the risk of unintended side effects is lower. Module constraints are left
for a follow-up once #2147 is resolved.

## Proposed Solution

Adopt the RubyGems `Dependency#match?` opt-in convention for provider version constraints:
prereleases are included in matching if and only if at least one version boundary in the
constraint set is itself a prerelease. The choice of operator does not affect this decision
(see Open Questions for edge cases around `!=`).

### User Documentation

By default, version constraints only match stable (non-prerelease) releases:

```hcl
version = ">= 1.0.0"   # matches 1.0.0, 1.0.1, 2.0.0, ... (no prereleases)
version = "~> 1.0"     # matches 1.x stable releases only
```

To opt in to prerelease versions, include a prerelease suffix on at least one boundary. The
conventional way to do this is the `-0` suffix, which is the lowest possible prerelease
identifier and therefore matches all prereleases of that version:

```hcl
version = ">= 1.0.0-0"      # matches 1.0.0-alpha, 1.0.0-beta.1, 1.0.0-rc.1, 1.0.0, 1.0.1, ...
version = "~> 1.0.0-0"      # matches prereleases and stable releases in the 1.0.x range
```

Semver ordering applies within prereleases, so more specific lower bounds work as expected:

```hcl
version = ">= 1.0.0-beta.2"  # matches 1.0.0-beta.2, 1.0.0-rc.1, 1.0.0, ... but NOT 1.0.0-beta.1
version = ">= 1.0.0-beta.1"  # matches 1.0.0-beta.1, 1.0.0-beta.2, 1.0.0-rc.1, 1.0.0, ...
```

> **⚠️ Upper bound warning:** Because semver defines prerelease versions as _less than_ their
> corresponding release (e.g. `2.0.0-beta.1 < 2.0.0`), an upper bound without a prerelease
> suffix does **not** exclude prereleases of that version. For example:
>
> ```hcl
> version = ">= 1.0.0-0, < 2.0.0"   # also matches 2.0.0-beta.1, 2.0.0-rc.1, etc.!
> ```
>
> To exclude prereleases of the upper bound version, use a prerelease suffix there too:
>
> ```hcl
> version = ">= 1.0.0-0, < 2.0.0-0"  # matches 1.0.x prereleases and stable, but not 2.0.0-beta.1
> ```
>
> This behavior is consistent with RubyGems, but may be surprising. See Open Questions.

**Behavior change:** This change affects configurations that already use a prerelease boundary
with an inexact operator (e.g. `>= 1.0.0-0`). Such constraints previously silently fell back
to stable-only matching; they will now match prerelease versions as intended. Configurations
using only non-prerelease boundaries (e.g. `>= 1.0.0`, `~> 1.0`) are entirely unaffected.

The opt-in applies to **provider** version constraints only (see scope justification below).
Module and OpenTofu core version constraints are unaffected by this change.

#### Impact on lock files

The opt-in affects all three provider version matching sites:

- **`tofu init` / provider installation** — prerelease versions become candidates when the
  constraint opts in.
- **Lock file verification** (`VerifyDependencySelections`) — a locked prerelease version
  (e.g. `1.0.0-beta.1`) is now considered valid for a constraint like `>= 1.0.0-0`. Previously
  this would error with "locked version does not satisfy constraint", making it impossible to
  lock a prerelease version with an inexact constraint.
- **`tofu providers mirror`** — mirrors prerelease versions when the constraint opts in.

### Technical Approach

Provider version matching is centralized in the `getproviders` package. The key function is
`MeetingConstraints` (a wrapper around `github.com/apparentlymart/go-versions`), which
intersects the exact constraint set with `versions.Released`, thereby stripping all
prereleases.

The fix introduces a helper `MeetingConstraintsForProvider` that inspects the constraint set
and selects the appropriate underlying function:

- If any `SelectionSpec` in the constraint has a non-empty `Boundary.Prerelease` field →
  call `versions.MeetingConstraintsExact`, which performs exact semver comparison including
  prereleases.
- Otherwise → call `versions.MeetingConstraints`, which excludes prereleases (current
  behavior, unchanged).

This mirrors `Gem::Requirement#prerelease?` in RubyGems, which returns true when any version
boundary in the requirement is a prerelease.

The helper is applied consistently across all three provider version matching sites:

| File | Function | Change |
| --- | --- | --- |
| `internal/providercache/installer.go` | `ensureProviderVersionsMightNeed` | replaces direct `MeetingConstraints` call |
| `internal/configs/config.go` | `VerifyDependencySelections` | replaces direct `MeetingConstraints` call |
| `internal/command/providers_mirror.go` | `Run` | replaces direct `MeetingConstraints` call |

The go-versions library author explicitly notes that `MeetingConstraintsExact` is the right
function to use when the caller wants to implement its own prerelease policy:

> "A caller can use this to implement its own specialized handling of pre-release versions
> by applying additional set operations to the result."

#### Scope: providers only

Module version constraints use a different code path
(`internal/initwd/module_install.go`) involving two competing parsers, as documented in
[#2147](https://github.com/opentofu/opentofu/issues/2147). Applying the same fix there
without first resolving those inconsistencies risks introducing new bugs. Limiting this RFC
to providers keeps the change small and reviewable.

OpenTofu core version constraints are resolved at startup against a known set of versions
and do not go through the provider installer; they are unaffected.

**Known limitation:** For `~> X.Y.Z-pre`, the go-versions library computes the upper bound
as `OlderThan(X.Y+1.0)`. Because semver defines prerelease < release, versions like
`X.Y+1.0-beta.1` are numerically less than `X.Y+1.0` and thus fall inside the range. In
RubyGems, the `~>` upper bound uses `v.release < r.bump`, which excludes such versions.
This edge case only arises when using `~>` with a prerelease boundary, which is an unusual
pattern in practice.

### Open Questions

1. **Upper bound behavior:** When a constraint opts in (e.g. `>= 1.0.0-0, < 2.0.0`), the
   upper bound `< 2.0.0` also includes `2.0.0-beta.1` because semver defines
   `2.0.0-beta.1 < 2.0.0`. This is consistent with RubyGems but likely surprising to users
   who expect `< 2.0.0` to mean "strictly below the 2.0.0 stable release". Should we
   deviate from RubyGems here and apply `Released` filtering to upper-bound constraints even
   when the lower bound opts in? Or is documenting the `< 2.0.0-0` pattern sufficient?

2. **`!=` with prerelease boundary:** A constraint like `!= 1.0.0-bad` opts in globally,
   enabling all other prereleases — likely not the intent. Should `!=` be excluded from the
   opt-in rule, or is it acceptable as documented behavior?

3. Should OpenTofu emit a **warning** when a prerelease-opting-in constraint is used, to
   make the behavior change visible to users upgrading from older versions?

4. The `~> X.Y.Z-pre` upper bound edge case (see Technical Approach): is the divergence from
   RubyGems acceptable, or should it be addressed?

5. Should the opt-in eventually be extended to **module** version constraints as a follow-up
   to resolving #2147?

### Future Considerations

- Extending the same opt-in behavior to module version constraints once issue #2147 (module
  installer prerelease quirks) is resolved.
- Extending the opt-in to `required_version` (OpenTofu core version constraints), allowing
  users to test against prerelease OpenTofu builds using the same convention (e.g.
  `required_version = ">= 1.12.0-0"`). This uses a different code path from providers and
  would be a separate change.
- A consistent deprecation or warning path for existing configs that used `>= X.Y.Z-pre`
  without expecting prerelease matching.

## Potential Alternatives

**Always include prereleases for all inexact operators (no opt-in).**
Simpler, but breaks all existing configurations that rely on `>=` excluding prereleases. Not
viable without a major version bump.

**Introduce a new explicit syntax (e.g. `>= 1.0.0 prerelease`).**
No ambiguity with existing configs, but invents new syntax incompatible with RubyGems and
unfamiliar to users. Adds parser complexity.

**Apply prerelease opt-in only to the lower bound.**
When a constraint opts in, apply `MeetingConstraintsExact` only for lower-bound operators
(`>=`, `>`), while keeping `Released` filtering for upper-bound operators (`<`, `<=`). This
would make `>= 1.0.0-0, < 2.0.0` exclude `2.0.0-beta.1` as most users would expect.
However, this diverges from RubyGems, requires more complex per-operator logic, and may
produce surprising results for other operator combinations.

**Do nothing / document the limitation.**
Preserves backward compatibility entirely, but leaves users with no path to track prerelease
channels without manual exact pinning. Issue #1651 shows real demand for this.
