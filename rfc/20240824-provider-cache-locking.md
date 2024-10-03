# Safe Global Provider Cache Access

Issue: https://github.com/OpenTofu/opentofu/issues/1483

Many CI/CD systems download all providers for every single tofu action run. The global provider cache allows downloaded providers to be saved in between runs and linked into the local provider cache (in .terraform/). Unfortunately, the global provider cache does not have any locking around it and can fail when used in multiple simultaneous runs.

Another common use case is terragrunt. When running `tofu init` on multiple projects simultaneously via the tool, there is a high likely hood of a conflicting write to a provider's directory. This has caused some interesting tooling workarounds on their end, which are not ideal.

Additionally, as we build out true e2e tests for OpenTofu, safe access to the global provider cache can dramatically reduce run times by allowing us to run these concurrently.

## Proposed Solution

A filesystem level lock via native syscall (fcntl flock in POSIX / LockFileEx in Windows) should be added that is safe for cross-process and in some scenarios cross-machine access. It will be best-effort and should rely on standard locking practices (not home grown) and be transparent to the user.

### User Documentation

By setting `TF_PLUGIN_CACHE_DIR` or `plugin_cache_dir` to a directory, tofu will download all required providers to that directory during init. If providers already exist within that directory, the lock file will be checked and the download may be skipped. Once the global cache is populated, providers will be linked into the local provider cache.  This can save significant time/resources across multiple runs/projects.

Example:
```
$ TF_PLUGIN_CACHE_DIR=~/.tofu.d/plugin-cache/ tofu init
Initializing provider plugins...
- Finding latest version of hashicorp/local...
- Installing hashicorp/local v2.5.1...

$ rm .terraform/ -r
$ TF_PLUGIN_CACHE_DIR=~/.tofu.d/plugin-cache/ tofu init
Initializing provider plugins...
- Reusing previous version of hashicorp/local from the dependency lock file
- Using hashicorp/local v2.5.1 from the shared cache directory
```

Any number of tofu instances should be able to be run in parallel and still have safe access to the global cache directory.

#### Scenarios which currently cause failures

Multiple init/plans/apply without a primed cache.

One possibility is that each project downloads and overwrites the same provider files.  The global provider cache scan is not a fast process and should be run as little as possible, it is therefore only run at the start of `tofu init`. ProjectA and ProjectB's inits may be run at the same time.  When this occurs, they will both download the full set of providers that they need and clobber each others files.  This is not ideal, but is only a waste of time/resources.

Another possibility is that one project may already be planning/applying and have a live provider executable running, effectively locking it on disk.  If ProjectA is still downloading providers during init and ProjectB has a much smaller list and is already in plan, ProjectA may attempt to overwrite the provider binary that ProjectB is running and maintaining an execution lock on.


Mixture of missing platform/corrupt lock files with valid lock files.

The provider lock file may have been generated on a system with a different architecture or may have become corrupt due to a variety of reasons. When this happens, the global cache may not match the lock file and force a re-download of the provider. This will cause potentially unexpected downloads in the above scenarios.

### Technical Approach

This proposal hinges on a stable and consistent cross-platform lock. The codebase already contains this in the form of locking local state files.  This code is battle tested and overall quite simple to use.  With some light refactoring, this filesystem locking code can be moved into it's own internal package and used both to lock providers and to lock the local state file.

The existing file locking should be safe on any local filesystems, but should be used with caution on shared volumes such as legacy NFS shares who do not provide strong locking consistency. We will add explicit warnings to the documentation which recommend against using networked filesystems for this use case.

Within the provider installation code, the whole section which inspects and links to the current providers available should be locked at the provider level.  The lock should be done at this granularity specifically for the complicating factors mentioned above.

Additionally, we should make the package installer smarter and able to check the files in the cache against the downloaded version. This prevents a bad cache entry from overwriting a valid entry which may already been in use by another process.

### Open Questions

* Is a filesystem lock syscall safe enough? It is industry standard and battle tested.  Are there any additional caveats that should be mentioned in the docs?
  - Yes. As mentioned above, we should document that networked filesystems are not recommended for this use case.

### Future Considerations

A considerable amount of time is spent scanning the provider cache.  Instead, it should probably be refactored to only read the provider metadata necessary at any given time.  This will offer some significant performance bonuses on large configurations.

## Potential Alternatives

Terragrunt/Gruntwork provides a http provider mirror that can be run locally and maintains a provider cache (archives) external to OpenTofu itself. While this does have some advantages, it still requires each provider to be extracted into the local provider cache folder, taking additional time and space.  It may be a safer alternative to using a networked filesystem between systems.
