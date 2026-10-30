# Module Version Locking

Issue: https://github.com/opentofu/opentofu/issues/586

Related:
- https://github.com/opentofu/opentofu/issues/2495 - version constraints for git/OCI modules
- https://github.com/opentofu/opentofu/issues/1942 - module SHA pinning
- https://github.com/opentofu/opentofu/issues/1199 - module caching (`TF_MODULE_CACHE_DIR`)
- https://github.com/opentofu/opentofu/pull/2892 - "registry in a file" (`.opentofu.deps.hcl`)
- https://github.com/opentofu/opentofu/pull/3252 - module package metadata and installation specs (prerequisite)
- https://github.com/hashicorp/terraform/issues/29503 - upstream request

## Problem Statement

Provider versions are locked in `.terraform.lock.hcl` with pinned versions and checksums. Modules have no equivalent mechanism. This means:

- The same configuration can resolve different module versions across machines and CI runs.
- There is no checksum verification to detect tampering or corruption.
- Modules are downloaded once and copied when installing.

The community has worked around this using `.terraform/modules/modules.json`, manual ref pinning, and wrapper scripts, but none of these provide the guarantees that provider locking does.

## Proposed Solution

Extend `.terraform.lock.hcl` to include module entries with pinned versions and checksums for registry modules.

## Prerequisites

### Module Immutability

Modules can write files into `path.module` at runtime, making checksum re-verification unreliable. This RFC depends on [PR #3252](https://github.com/opentofu/opentofu/pull/3252) (Module Installation Specifications) to provide the immutability mechanism. That RFC introduces `module-package.meta.hcl`, which allows declaring modules as read-only. Only modules marked as read-only are eligible for version locking.

Immutability can be declared from either side:

- **Module author**: ships a `module-package.meta.hcl` in their module package declaring it as read-only. This benefits all consumers automatically. See [Module Author section in PR #3252](https://github.com/opentofu/opentofu/pull/3252).
- **Root module consumer** (takes precedence): creates a `module-package.meta.hcl` in their project root, marking any consumed module as read-only - even if the upstream author hasn't declared it. Users don't have to wait for upstream authors. See [Project Author section in PR #3252](https://github.com/opentofu/opentofu/pull/3252).

## Design

### Version Constraints

Version constraints use the same syntax as providers. Local modules are not versioned.

**Registry modules** already support this:
```hcl
module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = ">= 3.0.0, < 4.0.0"
}
```

### Lock File Format

Running `tofu init` resolves module versions and records them in `.terraform.lock.hcl` alongside existing provider entries:

```hcl
provider "registry.opentofu.org/hashicorp/aws" {
  version = "5.31.0"
  # ... existing provider lock information
}

module "registry.opentofu.org/terraform-aws-modules/vpc/aws" {
  version     = "3.19.0"
  constraints = ">= 3.0.0, < 4.0.0"

  hashes = [
    "h1:abcd1234...",
    "zh:efgh5678...",
  ]
}
```

Modules are identified by their fully qualified source address. The lock file always stores the full registry form (e.g., `registry.opentofu.org/terraform-aws-modules/vpc/aws`), even when the user writes the short form (`terraform-aws-modules/vpc/aws`) in their configuration. OpenTofu normalizes the source to the full form during `tofu init`, matching how providers are stored in the lock file (e.g., `registry.opentofu.org/hashicorp/aws`).

This means:

- Multiple module calls using the same source and resolving to the same version share one lock entry.
- Renaming a module block does not invalidate the lock.

### Checksums

Checksums use the same algorithms as providers:

- **`h1:`** (directory hash) - SHA256 of file paths and contents using Go's `dirhash.Hash1`, encoded as base64. Verifies the extracted module directory.
- **`zh:`** (zip hash) - SHA256 of the raw archive served by the registry, encoded as hex. Verifies the downloaded archive before extraction.

Registry modules store one `h1:` and one `zh:` hash per entry, with no platform variants. The registry serves the same zip archive to all platforms, and extraction preserves bytes exactly. Per-platform hashing (as providers use for platform-specific binaries) only matters if we add version locking for git modules later, since git's `core.autocrlf` can convert LF to CRLF during checkout on Windows, producing different `h1:` hashes for the same commit on different platforms.

Both hashes are computed during `tofu init` and stored in the lock file. In the future, the registry could also compute and serve hashes when a module is submitted or updated. This is out of scope for this RFC.


## User Workflows

### First-time Locking

On `tofu init`, if no lock file exists or the lock file has no module entries, OpenTofu resolves versions for all registry modules marked as read-only and adds them to `.terraform.lock.hcl`.

### Updating Modules

```bash
# Update all modules
$ tofu init -upgrade
```

Resolves the latest versions within constraints, downloads them, and updates the lock file with new versions and checksums.

### Constraint Changes

If a user changes a version constraint (e.g., `">= 3.0"` to `">= 5.0"`) and the locked version no longer satisfies it, `tofu init` errors and prompts the user to run `tofu init -upgrade`.

### Verifying Integrity

OpenTofu verifies checksums on every `init`.

Successful verification:
```
$ tofu init
Initializing modules...
- vpc in terraform-aws-modules/vpc/aws 3.19.0
  Verified checksum matches lock file
```

Failed verification:
```
$ tofu init
Initializing modules...

Error: Module checksum verification failed

Module "terraform-aws-modules/vpc/aws" has a checksum that doesn't match the lock file.
This may indicate the module has been modified or tampered with.

Expected: h1:ijkl9012...
Got:      h1:mnop3456...

To update the lock file with the new checksum, run:
  tofu init -upgrade
```

## Future Considerations

1. **Module caching ([#1199](https://github.com/opentofu/opentofu/issues/1199))**:  A lock file with checksums is a prerequisite for a safe shared module cache. Implement `TF_MODULE_CACHE_DIR` for shared module storage across projects, possibly following something related to [Terragrunt's CAS approach](https://terragrunt.gruntwork.io/docs/features/cas/).

2. **Registry-in-a-file integration ([#2892](https://github.com/opentofu/opentofu/pull/2892))**:  introduces `.opentofu.deps.hcl` for mapping logical module addresses to physical sources with version resolution. Modules installed through that mechanism could require immutability from the beginning, simplifying the locking story for that pathway.

3. **Version constraints for git and OCI sources ([#2495](https://github.com/opentofu/opentofu/issues/2495)).**: We decided to put those off for this RFC, but at some point integrating them with the module locking workflow would give us better version constraint resolution for all types.

## Open Questions

1. **Warning on unlocked modules.** Right now, modules without `module-package.meta.hcl` are silently skipped during locking. A warning during `tofu init` would let users know they have modules without integrity guarantees. On the other hand, early on most modules won't have adopted `module-package.meta.hcl`, so the warning could get noisy fast.

2. **Duplicate labels for same source with different versions.** If the same module source is used with different version constraints that resolve to different versions (e.g., `~> 3.0` and `~> 5.0`), each needs its own lock entry. The existing provider lock parser rejects duplicate labels. Two options: (a) the module lock parser diverges from provider logic and allows duplicate labels with different versions, or (b) encode the version in the label, e.g. `module "registry.opentofu.org/terraform-aws-modules/vpc/aws:3.19.0"`.

3. **Auto-pruning stale lock entries.** When a module call is removed from configuration, should its lock entry be pruned on the next `tofu init`?

4. **`tofu modules lock` command.** Providers have `tofu providers lock` to pre-populate hashes for providers. Should we implement a `tofu modules lock` equivalent?
