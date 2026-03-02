# Improving interactions with provider locking

Related Issues:
* [Request: More efficient lock file updates](https://github.com/opentofu/opentofu/issues/1442)
* [tofu providers lock command takes too much time](https://github.com/opentofu/opentofu/issues/2563)
* [Expose Additional Provider + Provider Locking Functionality Programmatically](https://github.com/opentofu/opentofu/issues/3058)
* [Consider usage of TF_PLUGIN_CACHE_DIR without dependency lock files](https://github.com/opentofu/opentofu/issues/3094)

Locking providers is currently a necessary, and usually painless process for the standard use case. There exist several advanced settings which alter the provider installation behavior enough to cause speed-bumps in this workflow.

## Background

OpenTofu locks providers to ensure that the provider installed has not been tampered with or adulterated either in transport or after installation. Locking is done with two different hashing mechanisms, `zh` and `h1`.

### Zip Hash (zh)

Zip Hashing is the older mechanism which consists of a sha256sum calculation of the entire provider release bundle (zip). In the tofu registry, this hash is provided in multiple ways and can be signed with a GPG key for added security.

This hash only protects the provider during initial download and extraction during the installation process, but is useless to check the unzipped/installed provider.

### Hash Scheme 1 (h1)

Hash Scheme 1 is a newer mechanism which uses go's sumdb Hash1 algorithm and DirHash function to provide a snapshot of what is expected post-installation.  This ensures that any modification to the provider post-installation is detected and reported.

## Normal Workflow

The population of the .terraform.lock.hcl file is managed by `tofu init`. The registry is queried for acceptable hashes for the providers present in the configuration, as well as for the corresponding download links. Init then downloads and extracts the providers to `.terraform/providers`. It records all zh's in the registry's response, as well as the single h1 hash for the current platform.

On the developer's machine, subsequent calls to `tofu` in the project directory will use the lockfile to ensure the locally installed providers have not been tampered with.

When running tofu in automation (Github actions, GitLab workflows, TACOs, etc...), `tofu init` will re-download the providers from the registry and will require at least one ZH to match. If the architecture does not match, a new h1 hash will be appended to the lockfile for each provider.

In automation, subsequent calls to `tofu` in the project directory will use the lockfile to ensure the locally installed providers have not been tampered with.

## Advanced Workflows

### Read only lockfile in automation

As mentioned above: If the architecture between a developer's machine and the automation system does not match, a new h1 entry will be added to the lockfile as the provider is installed. For security reasons, some automation systems consider the lockfile read-only and will interpret this change to the lockfile as a blocking issue and refuse to continue. This is enforced via `tofu init -lockfile=readonly`.

This is usually fixed by running `tofu providers lock` on the developer's machine (sometimes as a git-hook) with additional architectures listed and committing the resulting lock file.

### Utilizing a Network or Filesystem Mirror (zip)

For efficiency, many people rely on a internal Network or Filesystem mirror for providers. This usually takes the form of an internal service or pre-built cache.

When utilizing a network mirror, we only fetch the download link and zh for the current platform to store in the lockfile.  If this is during a developer's first `tofu init` or `tofu init -upgrade`, the lockfile will not contain anything other than the zh and h1 for the developer's platform and will fail on systems with a different architecture. Again, this can be addressed with `tofu providers lock`.

### Utilizing a Filesystem Mirror (extracted)

Filesystem mirrors can also be configured to contain only the pre-extracted providers. This has some significant performance advantages, but again is a stumbling block when multiple architectures are at play.

A developer may have run init on a standard machine, populating all of the zh entries and a single h1 entry for a different platform. An automation system with a filesystem mirror would not have any matching hash entries and would therefore fail to init.

The global plugin cache (TF_PLUGIN_CACHE_DIR) suffers from the same issue.

The only way to work around this is to allow TF_PLUGIN_CACHE_MAY_BREAK_DEPENDENCY_LOCK_FILE (potentially dangerous) or to use `tofu providers lock` on every provider upgrade/installation.

### Pain Points

* `tofu providers lock` can be given multiple platforms and will update the lockfile for each
  - This requires downloading the provider release bundle for each platform and generating a h1 hash (tedious and slow)
  - This is a non-obvious step when applying a plan on a different architecture and serves as a stumbling block

* External tools would like to manage the provider lock file
  - Re-implementing the download/calculation of providers in multiple projects/languages is not ideal

Technical limitations:
* Provider authors only publish ZipHash (sha256sum) data in their releases, not h1 hashes.
* The registry only reports ZipHashes due to the above


## Proposed Solution (registry)

Given that we maintain the [registry code and data](https://github.com/opentofu/registry) in the OpenTofu org, we can add the additional h1 hashes to the registry metadata (more notes on this below) and extend our api responses to include that data. Once the registry is able to serve this new information, OpenTofu could optionally trust this new source of hashes.

The current registry trust chain is as follows for a given provider release bundle:
* Ensure that one of the GPG keys in the registry `"signing_keys"` signs the list of zh hashes linked to via `"shasums_url"` and `"shasums_signature_url"`
  - Report that all of these zh's are valid for this provider and available for use in the dependency lock file
  - Note: OpenTofu treats this step as OPTIONAL by default, given that we had to re-build the registry from scratch with no data due to the terraform registry policy
    - There are options available to force this check for strict environments
* Ensure that the zh listed in the registry `"shasum"` exists within the list of signing keys given by `"shasums_url"`, which has been verified in the previous step
* Ensure that the zh listed in the registry `"shasum"` matches the calculated value for the downloaded provider release bundle
 - Report that this zh has been verified locally

We propose amending that trust chain:
* If the registry serves h1 hashes, we calculate the hash for the downloaded (zh verified above) provider release bundle and compare it to the corresponding registry h1 hash
  - Report that this h1 has been verified locally
  - Report that all of these h1's are valid for this provider and available for use in the dependency lock file

This means that on `tofu init`, all applicable zh and h1 hashes will be stored in the dependency lock file (assuming all validation passes).

### User Documentation

If you are:
* Populating the provider lockfile from a system without any non-standard provider configuration
  * ex: normal developer laptop
* Running tofu on other platforms with any of the following non-standard provider configurations
  * read-only lockfile
  * provider mirrors
  * TF_PLUGIN_CACHE_DIR
  * ex: automated on another system / orchestrator

Your current workflow will involve some form of `tofu providers lock -platform=... -platform=...` for every platform you might interact with, and committing the lockfile. Alternatively, you have disabled provider locking with `tofu init -upgrade` or similar (not recommended).

In practice, many users just run `tofu init` and [are confused/annoyed](https://github.com/opentofu/opentofu/issues/1442) when the plan they produce does not run correctly on other environments (different architecture).

With these proposed changes, that stumbling block goes away for this common scenario. `tofu init` will produce a fully functional lockfile across all supported platforms (depending on provider releases), no second step required.

For users that want to provider additional safety guarantees, they can also use `tofu providers lock`, which will keep the same functionality that it has today.  It will have the added benefit that it will also compare the
h1 registry data to the downloaded archive and ensure no local corruption has occurred during the bundle extraction.

### Technical Approach

The largest technical hurdle with this approach is providing the h1 hashes in the registry v1 api response data. In practice, this means that we need to calculate this every time a new provider release is added to the registry, as well as back-filling the existing release data.

Calculating the h1 and zh values for a given provider binary is quite simple and relies entirely on standard library calls, a dozen lines of code.  Amending the registry codebase to support this and writing a backfill tool is not terribly difficult, I have prototyped that in [this commit](https://github.com/opentofu/registry/commit/243e1e8a42ce8f2234064a7ce7e9d24213e28fd6) for reference and to gather performance metrics.

#### Performance implications

Right now, we scan every single provider the registry knows about every 15 minutes and update the registry data if there are new version.  This process (Bump Versions Github Action) takes ~4 minutes on average and we want to keep it in that same ballpark so we can maintain our bump rate even as the registry grows.  For most providers this does not add any significant time, the entirety of the k8s provider (70 releases) took under 4 minutes to backfill.  3.5 min to download interleaved with 56s hashing, corresponding to under 4s added per "medium" sized provider release.  The AWS provider is much larger and therefore slower, but in practice we can still bump a single release with hashing in under 20s.

We have also run the Bump Versions action in a different Github repo for a week with the patch linked above.  It has not seen any dramatic slowdowns compared to the main repository.

The only scenario in which this cause a hiccup in bump times would be if a massive provider like AWS with thousands of release files was added via the standard submission pipeline.  If we are concerned about that, we could modify the "Submit Provider" command to generate a more complete release file instead of relying on the bump process to pick it up later.

#### API Format

As with any API design, there are multiple ways to accomplish the same thing.  In practice, our requirements are fairly clear given the above discussion.

OpenTofu fetches the json data at `${registryBaseURL}/${providerNamespace}/${providerType}/${providerVersion}/download/${OS}/${ARCH}`.  This includes everything that is necessary to download and check a release bundle.  We propose adding the following optional field to this structure:
```json
{
  "packages": {
    "${platform}": {
      "hashes": [
        "zh:${hashvalue}",
        "h1:${hashvalue}"
      ],
      "package_size": ${calculated_size_bytes},
    }
  }
}
```

This describes what additional hashes are valid for a provider release bundle across all platforms.  It also leaves room to add additional types of hashes in the future.

By providing the "package_size" field, we also can reduce the likelihood of hash collision attacks.

#### Backfill procedure

Once Version Bump process above has been patched to support calculating this field for new versions, we can begin the backport process.

This process should:
* List all provider metadata files in the registry.
* For each provider metadata file:
  - For each version without h1 data:
    - For each platform:
      - Download the release bundle for this platform
      - Calculate the shasum and ensure it matches the existing entry in the registry metadata file
      - Calculate the h1 hash and add it to the registry metadata file

### Open Questions

* Should we run the backfill on a large rented machine (faster) or do it in github actions (slower, better traceability)
* Should we bundle backfill commits (a-z0-9) or have a commit per-provider metadata file?
* Do we want to space out the backfill to prevent overloading the API generation process (sync to R2) ?

### Future Considerations

## Potential Alternatives

* Improving the performance of `tofu providers lock`
  - The majority of time spent in that command is downloading large providers, the checksums are fast
  - In practice, Github caps how quickly you can download provider releases and there are no good ways around that that we know of
  - This was explored in https://github.com/opentofu/opentofu/pull/2731, with throttling occasionally actually causing more harm than good
  - Running this second command is still a stumbling block for many users

* Have `tofu init` download and h1 checksum all the provider release platforms
  - If the previous alternative was fast, this could be a viable option


## Proposed Solution (mirrors)

The above solution works well for scenarios where the main source of truth is the OpenTofu registry. However, there are environments in which the lockfile is entirely generated from provider mirrors.

Today, OpenTofu only trusts a single locally verified hash (platform specific) from provider mirrors. This means that the lockfile generated in this scenario is incredibly sparse and subject to the speed-bumps described above.

Given that provider mirrors usually are part of a secure and managed internal environment, we should add a config option to the .tofurc file to trust all platform hashes reported by mirrors *if* we have successfully downloaded and verified the package for the current platform.

The other tweak we would likely need to make is extending `tofu providers mirror` to record the zh's along with the already present h1 hashes.

## Proposed Solution (registry is down, but cache is valid)

Although not common, we do occasionally encounter issues with our registry host ([#3508](https://github.com/opentofu/opentofu/issues/3508)). Users who have TF_PLUGIN_CACHE_DIR setup have found that their cache can not be used without the registry, even if the lockfile is fully populated.

We propose the following changes to the provider package installer (from @apparentlymart):
>
    Before we make any request to the registry, we'd check whether the dependency lock file selection corresponds to a package already in the global cache directory. If so, we just install from that cache directory and don't interact with the registry or change the checksums in the dependency lock file at all, and the rest of the steps are skipped.
>
    If the cache contents could not be immediately verified with the hashes in the dependency lock file then OpenTofu would make the registry API request, add new hashes to the lock file as appropriate, and re-validate the cache contents against the hashes returned from the registry.
>
    If this succeeds then we can skip fetching the package itself from the registry and just install from the cache. Between this and the previous step we will use the cache if either the lock file or the registry can provide a hash that matches the cache directory.
>
    If none of the hashes we can find in either the lock file or the registry match the cache directory then we fall back to fetching the package from the URL the registry indicated, just as we would if the cache directory were not present at all.

This also has the benefit of reducing the number of calls to the OpenTofu registry when `tofu init` is run massively in parallel, i.e Terragrunt and other automation tools.
