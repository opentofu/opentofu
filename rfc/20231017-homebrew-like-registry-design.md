# Homebrew-like Registry Design 

Issue: https://github.com/opentofu/opentofu/issues/741 

> [!NOTE]  
> This RFC was originally written by @RLRabinowitz and was ported from the old RFC process. It should not be used as a reference for current RFC best practices.

This is a proposal for the stable registry design.

**Disclaimer: This is not a proposal by the core team, it's my personal proposal for this.**

## Proposed Solution

The design is loosely based on Homebrew's approach to providing Formulae to anyone. Homebrew manages the Formulae under one centralize repository ([homebrew-core](https://github.com/Homebrew/homebrew-core)). New Formulae are created via PRs. Adding new versions to a Formula requires a PR as well, and it can be created using `brew bump-formula-pr`

The design here is similar - Have a centralized repository containing index files for every provider and module. The index files contain version info for the provider, and other necessary metadata. Creating a provider/module would simply mean to create such an index file for the provider/module. Adding a new version would require a PR, but we could add some means to simplify and automate the process.

The requirements are available in this issue [#258](https://github.com/opentofu/opentofu/issues/258)

### User Documentation

N/A

### Technical Approach

The design consists of:

- **A github repository**, managing the "source of truth" of the Registry's providers/modules and their versions
  - Updates are mainly using pull requests to add new providers/modules, or to add new versions to existing ones. Adds automation to simplify the process and to not force all provider/module authors to constantly publish versions manually
- **A serving layer** - Serves the provider/module versions and metadata 
- **A documentation website layer** - WIP

#### The provider/module github repository

A repository containing a file per module and per provider. The file is a JSON file, containing all necessary information for OpenTofu to get the provider/modules versions and artifacts

##### The provider/module JSON file

Will be of the following format:

```
{
  "keys" ["<PUBLIC_KEY_1>", "<PUBLIC_KEY_2>"],
  "versions": [{
    "version": "v3.0.352",
    "protocols": ["4.2", "5.0"],
    "shasum_url": "https://example.com/download/v3.0.352/SHASUM",
    "shasum_sig_url": "https://example.com/download/v3.0.352/SHASUM.sig",

    "targets": [{
      "os": "darwin",
      "arch": "amd64",
      "download_url": "https://example.com/download/v3.0.352/my-provider-darwin-amd64.zip",
      "file_name": "my-provider-darwin-amd64.zip",
      "shasum": "1"
    }, 
    {
      "os": "darwin",
      "arch": "arm64",
      "download_url": "https://example.com/download/v3.0.352/my-provider-darwin-arm64.zip",
      "file_name": "my-provider-darwin-amd64.zip",
      "shasum": "2"
    }]
  }]
}
```

##### Adding a provider/module to the registry

Simply create a PR adding the JSON file for your provider/module, with all the versions and necessary metadata. After the PR is approved and merged, the file will be part of the main branch, and will later be served as part of the serving layer
We could also provide a script that could be used to generate such a file for your provider/module repository, and even create the PR with the change

##### Adding a new version to an existing provider/module

Creating a PR adding the new version to the existing JSON file of the provider/module. We could add a bunch of methods to help automate the process, either to help out contributors with the version bump, or by completely automating the PR creation process

###### Auto-Approval of PRs

We could automatically approve and merge PRs of existing providers/modules. We could rely on the release in GitHub being the source-of-truth
We can simply auto-approve if the keys have not been changed in the PR, and the artifacts' signature match the public keys (But even that is not a must)

###### `tofu bump-provider-pr`

Similarly to brew's [brew bump-formula-pr](https://docs.brew.sh/How-To-Open-a-Homebrew-Pull-Request#submit-a-new-version-of-an-existing-formula), we could create a command that updates the JSON based on your repository, forks the OpenTofu Registry repository and creates a PR updating it

###### A GitHub Action for use in the provider/module repository

We could create a GH action that creates the PR for you. Authors of provider/module repositories could add that GHA to their release workflow, and immediately publish an OpenTofu version

###### Periodic refresh of versions of existing providers/modules

Since we probably can't count on all provider/module authors to always bump the versions by themselves, we might need an automation around bumping the versions ourselves.
The automation should take GitHub API rate limits into account, and attempt to not hit those limits (or deal with them in some manner)

We run a cron GHA that runs `git ls-remote --tags` for all providers and modules, and compares them to the versions in the JSON files
For modules - Look for vX.X.X and X.X.X tags. Any tag that does not exist in the JSON files should be added - Use GH API to fetch the release and create the file
For providers - Look for vX.X.X tags. We can't solely rely on tags, not all tags have releases. So, in order to avoid making unnecessary GH API calls to those releaseless tags every time the GHA runs, we can do the following:
- Add a cached file in the GHA that lists how many times we've attempted to find a release for a tag, and it wasn't there. Once it goes over a specific threshold - Don't try using it again
- For every tag you find, before checking if it has a release, check the cached file. If we checked that tag too many times, don't check again. Otherwise - check if release exists
- If the release doesn't exist, update the number of attempts for that tag by 1
- The cached file should be enough, as losing it should not cause too many GH API calls (hopefully), but we can switch to having it stored in a more persistent manner (S3 or some DB) 

After the GHA is done, it'll create a PR adding those changes (or simply merge to main directly)

**An alternative option** is to not rely on tags, and attempt to only use the GH API for releases when checking for updates. This would require first-class handling of GH rate limits, similarly to as suggested in [the following RFC](https://github.com/opentofu/opentofu/issues/724) 

###### Initial filling of the registry's JSON files for existing providers/modules

We'd need to have the registry fill up with JSON file for all existing providers/modules. For that, we can run a GHA similar to the mentioned above to fill all of those, if we provide it with a set of providers and modules we'd want to populate.
It might take some time, as we'll probably get throttled by GitHub API along the way, but eventually all necessary JSON files will be created

#### The serving layer

For the serving layer, we have a couple of options

##### Option 1: GitHub Pages

- Easy to set up, simply serving the JSON files as they are
- Would require changes in the CLI - Instead of (or in addition to) supporting v1, we'd support this new JSON file format that has all the info we need to install providers/modules
- We shouldn't have any issues with bandwidth limitations, as GH Pages only has a [soft limit of 100GB per month](https://docs.github.com/en/pages/getting-started-with-github-pages/about-github-pages#usage-limits), but its 1GB hosting limitation might be a problem for expansion in the future, as we add more providers and modules to the repository
- ~~For this approach **we wouldn't support v1 API in the registry anymore** - meaning that legacy terraform and other tooling relying on the API won't be able to use the OpenTofu Registry~~ We could still support v1 API for the registry (minus the `X-Terraform-Get` header for module download) by rendering on a different branch that would contain the files in the necessary format for the v1 API (similarly to the mapping mentioned in Option 2)

##### Option 2: CloudFront over replicated S3

- Similarly to the serving layer in [the following RFC](https://github.com/opentofu/opentofu/issues/724), this approach will have high-availability and expandability
- This would allow us to set up the hosted files in a different manner than how they are stored kept in the repository
  - Would require a pipeline after merge to main, to map the change to the format of the hosted files, and upload them to S3
  - Would allow us to keep the v1 API if we'd like (would require a small change, having the module download API return the URL in the body and not in the header. CLI would be adjusted as well to support both)

### Open Questions

### Future Considerations

#### The documentation layer - WIP

Currently, this RFC assumes the documentation website would be a separate system
That system could use the data hosted in the registry to generate persistent data and APIs that could used for documentation. Then, the documentation website could be a similar effort to that mentioned in other RFCs, like https://github.com/opentofu/opentofu/issues/724 or https://github.com/opentofu/opentofu/issues/722

## Potential Alternatives

See issues linked in [#258](https://github.com/opentofu/opentofu/issues/258)
