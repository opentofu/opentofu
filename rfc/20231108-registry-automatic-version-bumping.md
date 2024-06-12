# Automatic Version Bumps of Existing Providers and Modules

Issue: https://github.com/opentofu/opentofu/issues/837 

> [!NOTE]  
> This RFC was originally written by @RLRabinowitz and was ported from the old RFC process. It should not be used as a reference for current RFC best practices.

This proposal is part of the [Homebrew-like Registry Design](https://github.com/opentofu/opentofu/issues/741).

This proposal lays out how existing providers and modules would be updated, adding new versions to existing providers and modules. This proposal is based on scheduled automatic updated, checking if there are any new versions for the providers/modules, and also allows for manual updates for the providers/modules if necessary.

## Proposed Solution

In order to update a provider/module, one would simply need to update the relevant JSON file for the module/provider. Once the JSON file is updated, a process that will eventually the newly-updated provider/module will start. This process will be further explained in a different RFC.

So here I'll explain how the provider/module JSON are going to be updated.

### User Documentation

A module or provider author should not need to perform any action, other than cutting a release.

From @Yantrio:
Right now the intended behavior is as follows for authors of modules/providers:
- To release a **new provider version**, you create a new **github release.**
	- The registry should resolve all semver releases, including pre-releases
- To release a **new module version**, you create a new **git tag.**
	- The registry should resolve all valid semver tags


### Technical Approach

#### Automatic Version Bumps

As new provider and module versions are constantly being published, we'd want the version bump process to be mostly automatic. This RFC takes an approach of a **simple** update process.
- The update process will make a minimal number of GitHub API requests, to avoid hitting Throttling errors
- The update process go over **all** the providers and modules,

Once an hour, via a scheduled GitHub action, the update process will begin. It will go over **all** the existing providers and modules, and check if there are **new versions** for them.

The process will end up rebuilding some of the provider and module JSON files, and pushing those changed files directly to the `main` branch of the registry repository.

##### Providers

We will go by the following heuristic in order to minimize the amount of API calls we make to the GH API (specifically, the "releases" API):

- For every provider, fetch the RSS feed of `https://github.com/<NAMESPACE>/terraform-provider-<PROVIDER>/releases.atom` to get the latest tags of the repository. Then find the last semver tag that has a `v` prefix (so, the first matching element in the RSS feed). This action is very quick, and will most likely not be susceptible to throttling by GitHub
  - The rate limit of that atom page for releases is not clear. When attempting to use it without an `Authorization: Bearer <GITHUB_TOKEN>` header, then you get 429 errors pretty quickly. By adding the Bearer token, those throttling errors are mitigated. I was not able to find those limitations in GitHub docs, but it does not take from Rest API call pool
  - In order to get the tag from the RSS feed, it's best to rely on the `id` field which is in format `tag:github.com,2008:Repository/<REPO_ID>/<TAG_NAME>`. Other fields are not as optimal for resolving the tag name (`link` isn't displayed correctly if tag name has a `+` sign for semver build, `title` might not represent the tag name in some specific scenarios)  
- If the **last created semver tag** is not part of the provider JSON file, then we will rebuild the file from scratch. More details on this in the next section

In this manner, if any new release exists, then we'd find its tag in the RSS feed and attempt to rebuild the provider JSON file.
The only "false-positive" cases we'd attempt to rebuild the JSON file would be cases were the last tag is a pre-release tag for a release, or just a tag that's not ever going to get a release (very unlikely, mainly happens for old unmaintained providers)

###### Using the releases.atom RSS feed vs `git ls-remote`

There are a couple of advantages of getting the tags from the RSS feed, as compared to `git ls-remote` git command

- When using `git ls-remote` we only get the tag names, without any metadata like the time of creation. We could only rely on the `semver` itself to attempt and guess which tag was created when. That's not as good because:
  - We will probably miss new PATCH versions for prior MAJOR/MINOR releases of the provider, and wouldn't notice that we have to rebuild the provider JSON file 
  - We get more "false-positive" cases were we attempt to rebuild the provider JSON file for no reason

However, the RSS feed of releases.atom is not documented well, and its behaviour might change in the future. For example, there's no documentation regarding its limitations and quotas, or regarding using a bearer token.

###### How to update a provider JSON file

We overwrite the entire content of `versions` in the provider JSON file. We will only keep the `repository` as-is, if it exists

- Use GH API to fetch all releases, not including draft or prerelease releases. Each release is a provider version
- `version` - Will be the release's tag, with the prefix `v` removed
- `protocols` - Will be taken from the `*_manifest.json` artifact of the release. Otherwise, it would default to `["5.0"]`
- `shasums_url` - Will be the download URL of the `*_SHA256SUMS` artifact
- `shasums_signature_url` - Will be the download URL of the `*_SHA256SUMS.sig` artifact
- `targets` - A target will be created per each release artifact in format `*_<OS>_<ARCH>.zip`
  - `os` and `arch` will be taken from the file name of the release artifact
  - `filename` and `download_url` will be taken from the release artifact's information
  - `shasum` can be taken from the `_SHA256SUMS` file, cross-referenced with the current release artifact

##### Modules

For modules, we will simply list all tags, using `git ls-remote`, and pick the semver tags (with or without a `v` prefix). 
Whether there's any difference compared to the existing JSON file or not, the process will build the JSON file fresh from the discovered semver tags. Building the file is extremely fast, and building it fresh would make sure that any new version (even new patch versions for a prior major release) would be added to the JSON file

###### How to update a module JSON file

Very simple. Each `version` should simply be a semver tag

##### Handling GH API throttling

This approach is very simple, and makes sure the update process is fast and not so prone to GH API throttling. We will only call the GH API if the **latest semver tag** of a provider does not exist, and not GH API calls will be made at all for the modules
- In [my personal PoC](https://github.com/RLRabinowitz/rlrabinowitz.github.io), which contains ~3,000 providers, the average amount of API calls is around 120. [The limit for API calls in GH actions is 1,000](https://docs.github.com/en/actions/learn-github-actions/usage-limits-billing-and-administration#usage-limits)
  - A small portion of those are actual version bumps, or providers with pre-release tags that are long due for a release
  - Most of those API calls are for providers that are not truly supported anymore, and have not gotten a new release in years, but for some reason have a newer tag that was never released. Those only happen with providers that are very rarely used today
- If we'd like to be more on the safe side here, there are some enhancements here that could be adopted in the future to lower risk of hitting GH API throttling. For example - running the update process on a chunk of the providers/modules, where for each hour a different chunk is updated

##### Manual Version Bumps with Automated Approval and Merge

We would allow manually update a provider/module's JSON files, via a PR. This would not be necessary in most cases, as the automatic version bump process should add new versions once an hour for all modules and providers. However, it might be necessary, for example, if a provider's already released artifacts have been changed and re-uploaded.

In this case, one could open a Pull Request to the registry repository, with the necessary changes. The core maintainers of OpenTofu would manually go over the Pull Request, and decide whether the change is OK and legitimate and can be put into the registry

##### Justification and Tradeoffs

This approach tries making the automatic version bumps for providers and modules as simple as possible, without the need to start working around GH API throttling issues and errors. The update process is pretty quick, and re-running it (if necessary) should be very easy

However, as the registry scales with more providers, this approach might not scale well if more providers are added that have "stranded" semver tags that are never going to be actually released. If there are more providers with such tags, this would mean that the update process would make more GH API calls

This is probably not much of a concern, as this is a rare case, and we can work around it if we'd like (remove those providers from the auto-update process, for example)

### Open Questions


### Future Considerations

From @Yantrio:
For pre-releases: We have to bear in mind that some providers are already heavily using Github Release pre-releases and their users expect these pre-releases to be consumed. We've hit this issue with our current registry implementation here: 
https://github.com/kbst/terraform-provider-kustomization/issues/240

We should be careful here when discussing this though. This is with regards to Github Releases being marked as pre-release, and not the semver version having a suffix to mark it as a pre-release.

## Potential Alternatives

