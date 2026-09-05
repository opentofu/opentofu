# Module Dependency Cooldown

Issue: https://github.com/opentofu/opentofu/issues/3990

Right now, `tofu init` will happily install a module version the moment it's published, even if it was published seconds ago. This proposes letting operators configure a minimum age a module version must reach (based on when OpenTofu's registry first saw it) before `tofu init` is allowed to select it. If someone compromises a maintainer account or publishes a typosquatted module, this gives the community a window to notice and react before anyone's `tofu init -upgrade` can pick it up.

This mirrors a pattern already used elsewhere: [Go added the same idea to `go get`](https://github.com/golang/go/issues/76485) for the same reason, and OpenTofu's own [`provider_installation`](https://opentofu.org/docs/cli/config/config-file/#provider-installation) block already has an `include`/`exclude` pattern this proposal reuses directly.

This design has been validated by implementing it against the real OpenTofu codebase and testing it end-to-end against the live public registry. This RFC reflects what that work actually confirmed, not just a plan, see [Technical Approach](#technical-approach) for what's proven versus what's still open.

## Proposed Solution

Add a new `module_installation` block to the CLI configuration, alongside the existing `provider_installation` block. When configured, `tofu init` filters out any module version that hasn't existed for at least `cooldown_period` before considering it for constraint resolution.

### User Documentation

```hcl
module_installation {
  direct {
    cooldown_period = "7d"
    include = ["registry.opentofu.org/*/*/*"]
  }
}
```

- `cooldown_period` : minimum age a version must have before it's eligible for selection. Accepts a number followed by `d`, `h`, `m`, or `s` (e.g. `"7d"`, `"72h"`).
- `include` / `exclude` : which registries this applies to, using the same glob pattern style as `provider_installation`'s `include`/`exclude`. Note module patterns have **four** segments (`host/namespace/name/target-system`), not three like provider patterns, module registry addresses carry an extra "target system" component (e.g. `aws`, `google`) that provider addresses don't.
- No `module_installation` block at all means no change in behavior this is fully opt-in.
- A registry not matched by `include` is simply not subject to cooldown; it's not blocked or restricted in any other way. This is deliberate: OpenTofu can only vouch for the publish-date honesty of `registry.opentofu.org` by default. Third-party registries can be opted in explicitly by an operator who trusts them.

If every version that would satisfy a module's `version` constraint is currently in cooldown, `tofu init` fails, naming the version that would have matched, when it was discovered, and when it clears:

```
Error: No available version of module "module.vpc" satisfies constraint "~> 2.1.0"

The following version(s) would satisfy the constraint but have not yet
cleared the configured dependency cooldown period:

  - 2.1.0 (discovered 2026-08-04, clears cooldown 2026-08-11)
```

Cooldown is checked once, at the moment `tofu init` selects a version to write into the dependency lock file. It is not re-checked on later `plan`/`apply` runs against an already-locked selection. a version that already cleared cooldown stays selected until the operator runs `tofu init -upgrade`.

### Technical Approach

The filtering happens in `internal/initwd`, at the point where `tofu init` walks a registry module's available versions and picks the best one satisfying the `version` constraint. Each candidate version is checked against the configured cooldown before being allowed to match, using the same logic that already governs constraint matching a version in cooldown is treated exactly as if it didn't exist yet, the same way a version that fails the semver constraint is already skipped today.

The timestamp used is the registry's own record of when it first observed the version, not a Git tag or commit date (both of which can be backdated or moved after the fact by whoever controls the repository).

**This data already exists.** The two things this proposal originally assumed were still open turned out to already be shipped:

- The public OpenTofu module registry already returns a `discovered` timestamp per version in its API response ([registry PR #4113](https://github.com/opentofu/registry/pull/4113)).
- For registry-delegated Git modules, the registry already resolves a version's tag to a specific commit hash before handing it to OpenTofu, so a tag moved after the fact doesn't silently change what gets installed for an already-cleared version. This is a working piece of module version immutability, already live, ahead of the broader immutability work discussed in [#4071](https://github.com/opentofu/opentofu/pull/4071).

This means module cooldown doesn't need to wait on #4071 . It can be built against what's already shipped.

**What was confirmed by implementing this**, against the real `opentofu/opentofu` repository:

- `module_installation` config parsing, following the same pattern `provider_installation` already uses.
- The registry's `discovered` field read into the CLI's module version metadata.
- A new module-address pattern matcher for `include`/`exclude`. This turned out to be genuinely new work, not reuse: provider addresses and module addresses have a different number of segments (three vs. four), so the existing provider-pattern-matching code couldn't be shared directly.
- CLI configuration threaded from the top-level command down to where version selection actually happens. This plumbing didn't exist before and needed to be added across several files.
- The filtering logic itself, inserted into the existing version-selection loop.

**Real-world test:** with an intentionally extreme cooldown period configured, `tofu init` against a real public module correctly refused to install any version. With the same configuration removed, the identical module installed normally. Same module, same machine, only the configuration changed.

**What this doesn't yet cover:**

- The cooldown-specific error message shown above is designed but not yet implemented today a blocked install just produces the existing generic "no version satisfies constraint" message, with nothing distinguishing "blocked by cooldown" from any other cause.
- Only tested as an all-or-nothing block (every version cooled down at once). Not yet tested: a cooldown window where some versions clear and others don't, confirming the newest *eligible* version is selected correctly.
- The `-from-module` init flow (a separate, older code path) doesn't currently have CLI configuration available to it at all, so cooldown won't apply there without separate follow-up work.

**Limitations:**

- Applies only to modules resolved through a module registry. Direct source addresses (`git::`, `http::`, etc. used without going through a registry) have no version list to filter and are unaffected.
- Applies only to the `direct` installation method, since that's the only module installation method that exists today (see [#4071](https://github.com/opentofu/opentofu/pull/4071) for discussion of possible future module mirrors).

### Open Questions

- **Should a locked selection ever be re-checked?** As proposed, cooldown is a one-time gate at selection time; a version stays selected indefinitely once locked, even if something later called into question whether it should have cleared. Is that the right tradeoff, or should something revisit locked selections periodically?
- **Should there be an explicit bypass** for someone who understands the risk and wants to override cooldown for a single run? Current lean is no, an operator who needs this can lower `cooldown_period` in configuration instead, keeping the mechanism simple.
- **Backfill for versions that predate `discovered` tracking.** The registry already defaults these to a fixed historical date outside any reasonable cooldown window; this RFC should state that explicitly as the expected behavior rather than leaving it implicit.

### Future Considerations

- If OpenTofu later adds module installation methods beyond `direct` (mirrors, similar to provider mirrors), cooldown configuration would need to extend to those methods too.
- The `-from-module` gap noted above is a natural, small follow-up once the core mechanism lands.
- If a future RFC adds broader module version pinning/immutability to the dependency lock file, cooldown enforcement could potentially move to lock-time rather than purely install-time, which may change some of the tradeoffs in the first open question above.

## Potential Alternatives

**Enforce cooldown at the registry level instead of the client.** The registry could simply omit versions younger than some fixed age from its API response. This would protect everyone by default with no client-side change at all, but removes per-operator configurability (some organizations may want a longer or shorter window than others, or none at all) and doesn't help with third-party or self-hosted registries, which this RFC's `include`/`exclude` approach does support.
