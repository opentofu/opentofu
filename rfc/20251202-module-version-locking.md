# Module Version Locking

OpenTofu currently lacks a mechanism to lock module versions, similar to how provider versions are locked in `.terraform.lock.hcl`. This creates challenges for reproducibility and security, as module versions can change unexpectedly between runs or across different environments. Users have expressed the need for a feature that allows them to:

1. Lock specific module versions to ensure consistent infrastructure deployments
2. Specify version constraints (e.g., `>= 1.0.0`, `~> 2.1`) for modules
3. Verify module integrity through checksums, similar to provider verification
4. Cache modules efficiently to reduce redundant downloads

Currently, while OpenTofu supports version constraints for modules from registries, these are not locked and can resolve to different versions over time. For git-based and OCI registry modules, users must manually specify exact references (commit SHAs, tags, or digests), which is error-prone and lacks the flexibility of semantic version constraints. This has led to several workarounds in the community, mostly around the `.terraform/modules/modules.json` file.

## Proposed Solution

Introduce a comprehensive module version locking mechanism that extends the existing `.terraform.lock.hcl` file to include module version information, alongside implementing version constraint support for all module source types.

### Specifying Module Version Constraints

Users will be able to specify version constraints for modules in their configuration files, unless the module is local, these shouldn't be versioned.

**For Registry Modules:**
```hcl
module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = ">= 3.0.0, < 4.0.0"
}
```

**For Git Modules:**
```hcl
module "example" {
  source  = "git::https://github.com/example/terraform-module.git"
  version = "~> 1.2"  # Allows patch updates: 1.2.x and will convert to using ref on the URL
                      # It could also allow SHA pinning
}
```

**For OCI Registry Modules:**
```hcl
module "network" {
  source  = "oci://registry.example.com/opentofu/network-module"
  version = ">= 2.0.0, < 3.0.0"  # Will resolve to appropriate tag
}
```

Version constraint syntax follows the same syntax as the provider version constraints.

### Lock File Generation

When running `tofu init`, OpenTofu will resolve module versions based on the specified constraints and record them in `.terraform.lock.hcl`:

```hcl
# .terraform.lock.hcl

provider "registry.opentofu.org/hashicorp/aws" {
  version = "5.31.0"
  # ... existing provider lock information
}

module "vpc" {
  version = "3.19.0"
  source  = "registry.opentofu.org/modules/terraform-aws-modules/vpc/aws"

  constraints = ">= 3.0.0, < 4.0.0"

  hashes = [
    "h1:abcd1234...",
    "zh:efgh5678...",
  ]
}

module "vpc.nested" {
  version = "1.2.5"
  source  = "git::https://github.com/example/terraform-module"

  constraints = "~> 1.2"

  hashes = [
    "h1:ijkl9012...",
  ]
}

module "network" {
  version = "2.3.1"
  source  = "oci://registry.example.com/opentofu/network-module"

  constraints = ">= 2.0.0, < 3.0.0"

  hashes = [
    "h1:xyz789ab...",
  ]
}
```

### Updating Module Versions

To update modules to newer versions within the specified constraints:

```bash
# Update all modules
$ tofu init -upgrade
```

This will:
1. Resolve the latest versions that satisfy the constraints
2. Download the new module versions
3. Update the lock file with new versions and checksums
4. Display which modules were updated

### Verifying Module Integrity

OpenTofu will verify module checksums on every `init`:

```bash
$ tofu init
Initializing modules...
- vpc in terraform-aws-modules/vpc/aws 3.19.0
  Verified checksum matches lock file

Error: Module checksum verification failed

Module "example" has a checksum that doesn't match the lock file.
This may indicate the module has been modified or tampered with.

Expected: h1:ijkl9012...
Got:      h1:mnop3456...

To update the lock file with the new checksum, run:
  tofu init -upgrade
```


**Module Addressing Scheme:**

Modules in the lock file are identified by their call name. This creates a clear mapping between configuration and lock file entries. For nested modules (modules calling other modules), the lock file uses a hierarchical naming scheme:

```hcl
module "vpc.nested" {
  version = "1.2.5"
  source  = "git::https://github.com/example/terraform-module"

  constraints = "~> 1.2"

  hashes = [
    "h1:ijkl9012...",
  ]
}
```

### 2. Module Installer Enhancement

The module installer (`internal/initwd`) will be modified to:

**For Registry Modules:**
- Query the registry API for available versions
- Resolve version constraints to specific versions
- Download module archives
- Compute checksums of module contents

**For Git Modules:**
- Parse git repository URLs and refs
- Support version constraints by:
  - Using `git ls-remote --tags` to fetch tags efficiently (no full clone needed)
  - Filtering tags that match strict semantic versioning patterns
- Compute checksums of the checked-out module directory (excluding `.git`)
- Cache cloned repositories indexed by URL hash (not raw URL to prevent credential leaks)

**For OCI Registry Modules:**
- Parse OCI registry URLs and image references
- Support version constraints by:
  - Querying the OCI registry API for available tags
  - Filtering tags that match semantic versioning patterns
  - Resolving constraints against available tags
  - Using the `tag` query parameter to specify resolved version (e.g., `?tag=2.3.1`)
  - Supporting `digest` parameter for immutable references when needed
- Download module package archives from OCI registry
- Compute checksums of the extracted module contents
- Use OCI credentials from standard locations (Docker config, credential helpers)
- Cache downloaded artifacts indexed by registry/repository/digest

**For Local Modules:**
- No version locking or caching
- Continue current behavior of direct path reference

### 3. Version Constraint Resolution

**Conflict Resolution:**

If different modules require incompatible versions of the same transitive dependency, OpenTofu will:
1. Fail fast with a clear error message
2. Display the conflicting constraints and their sources
3. Suggest resolution strategies (e.g., updating constraints)

Example error:
```
Error: Conflicting module version requirements

Module "app" requires module "common" version >= 2.0.0
Module "utils" requires module "common" version < 2.0.0

No version of module "common" satisfies both constraints.

To resolve, update the version constraints in:
  - modules/app/main.tf
  - modules/utils/main.tf
```

### 4. Checksum Calculation

Module checksums will be calculated as:

**For Registry Modules:**
- Hash the downloaded module in the `.terraform/modules` folder
- Support multiple hash formats for cross-platform verification

**For Git and other VCS Modules:**
- Walk the module directory tree
- Hash file contents in deterministic order (sorted by path)
- Exclude version control directories (`.git`, `.svn`, etc.)

**For OCI Registry Modules:**
- Extract the module archive from the OCI artifact layer
- Walk the extracted module directory tree
- Hash file contents in deterministic order (sorted by path)
- Use the same hashing algorithm as other module types for consistency
- Optionally validate against OCI artifact digest for additional integrity verification

### Migration Guide

**For Existing Projects:**

When upgrading to OpenTofu with module locking support:

1. **First `tofu init` after upgrade:**
   ```bash
   $ tofu init
   Initializing modules...
   - vpc in terraform-aws-modules/vpc/aws 3.19.0

   OpenTofu has created a lock file .terraform.lock.hcl to record module versions.
   Include this file in your version control repository.
   ```

2. **Lock file is automatically generated** with currently resolved versions

3. **No breaking changes** to existing workflows - version constraints in config continue to work

**Backward Compatibility:**

- Lock files without module entries remain valid
- OpenTofu reads existing provider-only lock files
- New lock files can include both providers and modules
- No changes required to existing configurations

### Future Considerations

1. Add support for caching modules: https://github.com/opentofu/opentofu/issues/1199 maybe using a similar approach to Terragrunt https://terragrunt.gruntwork.io/docs/features/cas/

2. **How should OpenTofu handle git modules without semantic version tags?**
   - Option A: Require semantic version tags for version constraints
   - Option B: Fall back to commit SHA when no version tags exist
   - Option C: Support branch names as "versions" (less precise)
   - **Decision: Option A + B** - Require semantic version tags for version constraints. If version constraint is specified but no matching tags exist, fail with clear error message. Allow SHA/branch refs without version constraints.
   - **Implementation:** Use strict semver regex pattern. Cache tag lists locally to handle remote deletions.
