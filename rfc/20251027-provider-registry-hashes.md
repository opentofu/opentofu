# Extending the Registry v1 protocol with additional provider hashes

Related Issues:
* [Request: More efficient lock file updates](https://github.com/opentofu/opentofu/issues/1442)
* [tofu providers lock command takes too much time](https://github.com/opentofu/opentofu/issues/2563)
* [Expose Additional Provider + Provider Locking Functionality Programmatically](https://github.com/opentofu/opentofu/issues/3058)
* [Consider usage of TF_PLUGIN_CACHE_DIR without dependency lock files](https://github.com/opentofu/opentofu/issues/3094)

Locking providers is currently a necessary, though tedious process, and is a stumbling block for many OpenTofu users.  Tofu locks providers to ensure that the provider installed has not been tampered with or adulterated either in transport or after installation.

Locking is done with two different hashing mechanisms, `zh` and `h1`.

## Zip Hash (zh)

Zip Hashing is the older mechanism which consists of a sha256sum calculation of the entire provider release bundle (zip). In the tofu registry, this hash is provided in multiple ways and can be signed with a GPG key for added security.

This hash only protects the provider during initial download and extraction during the installation process, but is useless to check the provider unzipping/installation.

## Hash Scheme 1 (h1)

Hash Scheme 1 is a newer mechanism which uses go's sumdb Hash1 algorithm and DirHash function to provide a snapshot of what is expected post-installation.  This ensures that any modification to the provider post-installation is detected and reported.


Pain Points:

* Tofu `init` records a zh entry for each platform, but only records a h1 entry for the current platform
  - Note: this is for the registry path, the http mirror path is subtly different
  - When running `tofu init` on a system with a different 

* Tofu `providers lock` can be given multiple platforms and will update the lockfile for each
  - This requires downloading the provider release bundle for each platform and generating a h1 hash (tedious and slow)
  - This is a non-obvious step when applying a plan on a different architecture

* External tools would like to manage the provider lock file
  - Re-implementing the download/calculation of providers in multiple projects/languages is not ideal


Technical limitations:
* Provider authors only publish ZipHash (sha256sum) data in their releases, not h1 hashes.
* The registry only reports ZipHashes due to the above

## Proposed Solution

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
* If the registry serves h1 hashes, we calculate the hash for the downloaded provider release bundle and compare it to the corresponding registry h1 hash
  - Report that this h1 has been verified locally
* If the registry serves h1 hashes, we treat them as trusted given the above checks
  - Report that all of these h1's are valid for this provider and available for use in the dependency lock file

This means that on `tofu init`, all applicable zh and h1 hashes will be stored in the dependency lock file.

### User Documentation

Currently, the recommended workflow with OpenTofu is to run `tofu init`, run `tofu providers lock -platform=... -platform=...` for every platform you might interact with, and commit the lockfile.

In practice, many users just run `tofu init` and [are confused/annoyed](https://github.com/opentofu/opentofu/issues/1442) when the plan they produce does not run correctly in their CI environment (different architecture).

With these proposed changes, that stumbling block goes away for 99% of users.  `tofu init` will produce a fully functional lockfile across all supported platforms (depending on provider releases), no second step required.

For users that want to provider additional safety guarantees, they can also use `tofu providers lock`, which will keep the same functionality that it has today.  It will have the added benefit that it will also compare the
h1 registry data to the downloaded archive and ensure no local corruption has occurred during the bundle extraction.

### Technical Approach

The largest technical hurdle with this approach is providing the h1 hashes in the registry v1 api response data. In practice, this means that we need to calculate this every time a new provider release is added to the registry, as well as backfilling the existing release data.

Calculating the h1 and zh values for a given provider binary is quite simple and relies entirely on standard library calls, a dozen lines of code.  Amending the registry codebase to support this and writing a backfill tool is not terribly difficult, I have prototyped that in [this commit](https://github.com/opentofu/registry/commit/243e1e8a42ce8f2234064a7ce7e9d24213e28fd6) for reference and to gather performance metrics.

#### Performance implications

Right now, we scan every single provider the registry knows about every 15 minutes and update the registry data if there are new version.  This process (Bump Versions Github Action) takes ~4 minutes on average and we want to keep it in that same ballpark so we can maintain our bump rate even as the registry grows.  For most providers this does not any significant time, the entirety of the k8s provider (70 releases) took under 4 minutes to backfill.  3.5 min to download interleaved with 56s hashing, corresponding to under 4s added per "medium" sized provider release.  The AWS provider is much larger and therefore slower, but in practice we can still bump a single release with hashing in under 20s.

We have also run the Bump Versions action in a different Github repo for a week with the patch linked above.  It has not seen any dramatic slowdowns compared to the main repository.

The only scenario in which this cause a hiccup in bump times would be if a massive provider like AWS with thousands of release files was added via the standard submission pipeline.  If we are concerned about that, we could modify the "Submit Provider" command to generate a more complete release file instead of relying on the bump process to pick it up later.

#### API Format

As with any API design, there are multiple ways to accomplish the same thing.  In practice, our requirements are fairly clear given the above discussion.

OpenTofu fetches the json data at `${registryBaseURL}/${providerNamespace}/${providerType}/${providerVersion}/download/${OS}/${ARCH}`.  This includes everything that is necessary to download and check a release bundle.  We propose adding the following optional field to this structure:
```json
  "hashes": {
    "zh:hashvalue": []{ "h1:hashvalue" }
  }
```

This describes what additional hashes are valid for a provider release bundle, given an (ideally signed) zip hash.  It also leaves room to add additional types of hashes in the future.

We could also go with a simpler approach and simply list the hashes as a flat []string:
```json
  "hashes": {
    "zh:hashvalue",
    "h1:hashvalue",
    ...
  }
```


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

Given that the Version Bump process will be running concurrently with this process, we should consider overwriting any changes to the metadata files while this process runs. This is reasonably safe since the registry does not run with "delete" enabled by default.  Any metadata that is pushed will not be removed, even if the metadata file is temporarily reverted before it's re-bumped.

### Open Questions

* Should this RFC have additional review by third parties given the potential security implications?
* Which proposed API format should we implement?
* Should we run the backfill on a large rented machine (faster) or do it in github actions (slower, better traceability)
* Should we bundle backfill commits (a-z0-9) or have a commit per-provider metadata file?
* Do we want to space out the backfill to prevent overloading the API generation process (sync to R2) ?

### Future Considerations

We may want to consider extending the trust model of `tofu providers mirror` and how we accept hashes from provider mirrors. Mirrors report their own hashes, though we only report that the single matching hash should be added to the lockfile.  We may want to add a mode/setting that says "Yes, I really trust this internal provider mirror and will happily accept it's hashes, as long as the checksum of the bundle I downloaded matches at least one".  This would help out some of the larger organizations relying on this functionality avoid the class of problem described in this RFC, but for mirrors.

## Potential Alternatives

* Improving the performance of `tofu providers lock`
  - The majority of time spent in that command is downloading large providers, the checksums are fast
  - In practice, Github caps how quickly you can download provider releases and there are no good ways around that that we know of
  - This was explored in https://github.com/opentofu/opentofu/pull/2731, with throttling occasionally actually causing more harm than good
  - Running this second command is still a stumbling block for many users

* Have `tofu init` download and h1 checksum all the provider release platforms
  - If the previous alternative was fast, this could be a viable option

* Treat h1 hashes as optional when applying a saved plan, as long as zh's exist and are valid
  - This is technically possible, but could weaken our safety guarantees in subtle and problematic ways
