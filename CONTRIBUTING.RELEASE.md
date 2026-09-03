# OpenTofu release manual

> [!WARNING]
> This manual is intended for OpenTofu core and fork maintainers. If you are looking for the normal contribution guide, see [this file](CONTRIBUTING.md).

This manual describes how to create an OpenTofu release. OpenTofu has two kinds of releases. Nightly releases are created
from the `main` branch, while we split off a version (e.g. `v1.8`) branch before creating a 'beta', `rc` or `stable`
release.

--- 

## Naming in this document

- **Nightly** is a unstable preview of the code as it sits on main.  It is built once per day and is versioned as `X.Y.0-dev`, where `X` and `Y` are numbers, such as `1.2.0-dev`. 
- **Beta** is a semi-stable preview release. This is versioned `X.Y.0-betaW`, where `X`,`Y` and `W` are numbers, such as `1.2.0-beta1`.
- **RC** is a release candidate which does not have new features over a beta. This is versioned `X.Y.0-rcW`, where `X`,`Y` and `W` are numbers, such as `1.2.0-rc1`.
- **Stable** is a release that has no new features and bug fixes over an RC. This is versioned `X.Y.0`, where `X` and `Y` are numbers, such as `1.2.0`.
- **Point release** is a release that contains bugfixes only on top of a stable release. This is versioned `X.Y.Z` where `X`, `Y` and `Z` are numbers, such as `1.2.3`.

> [!WARNING]
> Many tools depend on the release order on GitHub to determine the latest version. When creating a point release, make sure to release the oldest version first, then follow by the newer versions. Do not release an older point release without also releasing the newer versions or tooling _will_ break.

---

## Gathering the team for a release

To create a release, make sure you have people on standby with the following credentials:

- Cloudflare
- PackageCloud
- Snapcraft
- Linkedin
- X

---

## Preparing public relations collaterals for a release

Before you start creating a release, make sure you have the following marketing collaterals ready to be published:

<details><summary>

### Beta (`X.Y.0-betaW`)

</summary>

- "Get ready for..." blog post (see https://opentofu.org/blog/ for examples)
- Community Slack announcement
- Linkedin and X posts

</details>

<details><summary>

### Release Candidate (`X.Y.0-rcW`)

</summary>

- Community Slack announcement
- Linkedin and X posts

</details>

<details><summary>

### Stable release (`X.Y.0`)

</summary>

- Release blog post with the feature and community highlights since the last release (see https://opentofu.org/blog/ for examples)
- Community Slack announcement
- Linkedin and X posts

</details>

<details><summary>

### Point release (`X.Y.Z`)

</summary>

- Community Slack announcement

</details>

---

## Preparing the repository for a release

Before you can create a release, you need to make sure the following files are up to date:

- [CHANGELOG.md](CHANGELOG.md) (Note: do not remove the `(unreleased)` string from the version number before the stable release.)
- [version/VERSION](version/VERSION)

Ideally, make sure these changes go in as the last PR before the release.

---

## Double check proper traceability of the included changes

For better traceability, backported changes should link one of the following:
* the link to the PR that added the change in the `main` branch
* the link to the issue that the change fixes

Ideally, all the backports should be done through backporting PRs, whose description should include one (or both) of the links above.

If the backport is done through pushing directly to the targeted version branch, the commit message should include one (or both) of the links above.

> [!WARNING]
Right before issuing the release, ensure that all of the included changes follow the rules above.
If not, a comment should be added, to either the PR or the issue, with the commit that actually backported the change.
>
> By doing so, it will help with the forward tracing (ie: from the issue to the commit) but will not provide back tracing (from the commit back to the issue).
Therefore, this is a last resort, in cases where neither of the recommended options above cannot be done anymore (aka it might require rebasing on a branch where it is not allowed).


## Tagging the release

Now that you have the files up to date, do the following:

1. On your computer, make sure you have checked out the correct branch:
   * `vX.Y`, assuming you are releasing version `X.Y.Z`
2. Make sure the branch is up-to-date by running `git pull`
3. Create the correct tag: `git tag -m "X.Y.Z" vX.Y.Z` (assuming you are releasing version `X.Y.Z`)
   * If you have a GPG key, consider adding the `-s` option to create a GPG-signed tag
4. Push the tag: `git push origin vX.Y.Z`

---

## Creating the release

Now comes the big step, creating the actual release.

1. Head on over to the [Actions tab](https://github.com/opentofu/opentofu/actions) on the main repository
2. Select the `release` workflow on the left side
3. Click the `Run workflow` button, which opens a popup menu
4. Select the correct branch:
   * For `beta` releases, select the `main` branch
   * For all other releases, select the appropriate version branch
5. Enter the correct git tag name: `vX.Y.Z`
6. If you are releasing the latest `X.Y` version, check the `Release as latest?` option.
7. If you are releasing a `beta` or `rc` version, check the `Release as prerelease?` option.
8. Click the `Run workflow` button.

Now the release process will commence and create a *draft* release on GitHub. If you did not check the prerelease option, it will also publish to Snapcraft and PackageCloud.

---

## Publishing the GitHub release

The release process takes about 30 minutes. When it is complete, head over to the [Releases section](https://github.com/opentofu/opentofu/releases) of the main repository and find the new draft release. Change the following settings

- Edit the text (see the examples below).
- Check `Set as a pre-release` if you are releasing a beta or release candidate.
- Check `Set as the latest release` if you are releasing a stable or point release for the latest major version. Do not check this checkbox if you are releasing a point release for an older major version.
- Check `Create a discussion for this release` if you are releasing a stable (`X.Y.0`) version.
- Click `Publish release`

<details><summary>

### Beta, or release candidate

</summary>

Create a text highlighting how users can test the new features, for example:

```markdown
⚠️ Do not use this release for production workloads! ⚠️

It's time for the first prerelease of the 1.9.0 version! This includes a lot of major and minor new features, as well as a ton of community contributions!

The highlights are:

* **`for_each` in provider configuration blocks:** An alternate (aka "aliased") provider configuration can now have multiple dynamically-chosen instances using the `for_each` argument:

    ```hcl
    provider "aws" {
      alias    = "by_region"
      for_each = var.aws_regions

      region = each.key
    }
    ```

    Each instance of a resource can also potentially select a different instance of the associated provider configuration, making it easier to declare infrastructure that ought to be duplicated for each region.
```

</details>

<details><summary>

### Stable release (`X.Y.0`)

</summary>

Create a more elaborate text explaining the flagship features of this release, ideally linking to the blog post and/or video for the release, for example:

```markdown
We're proud to announce that OpenTofu 1.8.0 is now officially out! 🎉

## What's New?
* Early variable/locals evaluation
* Provider mocking in `tofu test`
* Resource overrides in `tofu test`
* Override files for OpenTofu: keeping compatibility
* Deprecation: `use_legacy_workflow` has been removed from the S3 backend-backend

See the launch post on our blog: https://opentofu.org/blog/opentofu-1-8-0/

For all the features, see the [detailed changelog](https://github.com/opentofu/opentofu/blob/v1.8.0/CHANGELOG.md).

You can find the full diff [here](https://github.com/opentofu/opentofu/compare/v1.7..v1.8.0).
```

</details>

<details><summary>

### Point release (`X.Y.Z`)

</summary>

For point releases, simply copy the section from the [CHANGELOG.md](CHANGELOG.md) file.

</details>

---

## Check the Post-release Workflow

If you have published a version that is now considered to be the latest stable release, this should automatically trigger a "Post-release" GitHub actions workflow.

Verify that this worked as expected by checking [the run history for the "Post-release" workflow](https://github.com/opentofu/opentofu/actions/workflows/post-release.yml). There should be a run named after the version you just published and it should complete successfully.

You can safely re-run this workflow on failure as long as the release whose job you are re-running is still considered to be the latest release.

---

## Updating the website/documentation

All releases require some sort of update to the [opentofu.org](https://github.com/opentofu/opentofu.org) repository, but the details depend on what kind of release you are publishing.

Before you begin, make sure that all submodules are initialized:

```
git submodule init
```

> [!WARNING]
> If you are using Windows, make sure your system supports symlinks by enabling developer mode and enabling symlinks in git.

The following summarizes what we need to do for each kind of release:

- Beta release for new series (e.g. v1.8.0-beta1): [Introduce new release series](#website-new-release-series).
- Subsequent beta release or release candidate for new series (e.g. v1.8.0-beta2 or v1.8.0-rc1): [Git submodule update](#website-submodule-update)
- Final release for new series (e.g. v1.8.0): [Change the default version](#website-change-default-version).
- Patch release in existing series (e.g. v1.8.1): [Git submodule update](#website-submodule-update)

The following subsections describe the process for each of these types of changes.

<details><summary>

### <span id="website-new-release-series">Introduce new release series</span>

</summary>

When we publish the first beta release for a new release series (e.g. v1.8.0-beta1) we establish the configuration and git submodule for that series' maintenence branch, with it initially listed as "(beta)" in the navigation. This is so that folks participating in prerelease testing can more easily find the relevant documentation for new features.

Before performing these steps, the maintenence branch (e.g. `v1.8`) must've been created in the main `opentofu/opentofu` repository, initially referring to the same commit as the `v1.8.0-beta1` tag.

The following steps all take place in a work tree for the `opentofu/opentofu.org` repository:

1. Add the new release series to the array of strings in `versions.json` in the root of the repository.
2. Add a submodule for the new release to the website repository:
   ```shell
   git submodule add -b v1.8 https://github.com/opentofu/opentofu opentofu-repo/v1.8
   ```
3. Open `docusaurus.config.ts` in your text editor and find the `presets` property, which contains a nested `versions` property containing an object for each release series, like this:
   ```js
      // ...
      "v1.7": {
         label: "1.7.x",
         path: "v1.7",
         banner: "none",
      },
      current: {
         label: "1.7.x",
         path: "",
         banner: "none",
      },
      main: {
         label: "Development",
         path: "main",
         banner: "unreleased",
         noIndex: true,
      },
      // ...
   ```

   Just before the `current` property, introduce a property for the new series that's currently in beta, using the "unreleased" banner so that its pages will contain a warning about it being an unreleased version, and including "(beta)" in its label for now:

   ```js
      "v1.8": {
         label: "1.8.x (beta)",
         path: "v1.8",
         banner: "unreleased",
      },
   ```
4. Still in `docusaurus.config.ts`, find the `navbar` property which includes an item whose label is "Docs" and whose nested items describe the navigation links for different versions, like this:
   ```js
      // ...
      {
         label: "v1.7.x",
         href: "/docs/v1.7/",
      },
      {
         label: "Development",
         href: "/docs/main/",
      },
      // ...
   ```

   Add a new element to the end of the list just before the one labelled "Development" which matches the entry added to `presets` in the previous step:

   ```js
      {
         label: "v1.8.x (beta)",
         href: "/docs/v1.8/",
      },
   ```

   (We'll move this new series to the top of the menu later once it becomes stable, but prerelease versions are always listed last.)
5. Create a new file in the `versioned_sidebars` directory:
   ```shell
   cp versioned_sidebars/version-v1.7-sidebars.json versioned_sidebars/version-v1.8-sidebars.json
   ```

   These files are typically identical between versions because they just tell Docusaurus to generate the navigation automatically based on the documentation files detected in the submodule directory.
6. Create a symlink in the `versioned_docs` directory to help Docusaurus find the submodule for the new version:
   ```shell
   ln -s ../opentofu-repo/v1.8/website/docs versioned_docs/version-v1.8
   ```

After following these instructions you should be able to launch the local dev server for the website by running `docker compose up --build` and find a working link for the new release series near the bottom of the dropdown menu under "Docs" in the top navigation bar.

If everything looks good then commit all of these changes and open a PR in the `opentofu.org` repository.

</details>

<details><summary>

### <span id="website-submodule-update">Git submodule update</span>

</summary>

The website build process uses the commit currently selected by each of the submodules as the source of documentation for an OpenTofu release series.

Each time we publish a new release in a series (including a new beta or release candidate during the prerelease period) we need to update the corresponding submodule in the `opentofu/opentofu.org` repository to match.

For example, if you're releasing v1.8.1:

1. Change directory into the submodule for the relevant series:
   ```shell
   cd opentofu-repo/v1.8
   ```
2. Fetch the newly-created tag and switch to it:
   ```shell
   git fetch origin v1.8.1
   git checkout v1.8.1
   ```
3. We don't currently have any automation for keeping the "Development" section updated, so while you're here anyway it's a good opportunity to update that to whatever is currently the latest commit on the `main` branch:
   ```shell
   cd ../main
   git pull --rebase
   ```
4. Return to the root directory and commit these changes:
   ```shell
   cd ../..
   git add opentofu-repo/v1.8
   git commit
   ```

Use `docker compose up --build` to start the dev server for the website and check that the "Docs" submenu in the to navigation still contains the relevant release series and that it's still possible to navigate to its documentation. If successful, submit these changes in a PR to the `opentofu.org` repository.

</details>

<details><summary>

### <span id="website-change-default-version">Change the default version</span>

</summary>

Once the prerelease period for a new release series is over and we're ready to publish its first stable release (e.g. v1.8.0) we need to adjust some of the configuration that we would've previously added when publishing v1.8.0-beta1 so that this series is now treated as the default version and no longer identified as being unreleased.

The following steps all take place in a work tree for the `opentofu/opentofu.org` repository:

1. Open `docusaurus.config.ts` in your text editor and find the `presets` property, which contains a nested `versions` property containing an object for each release series, including the beta entry for the newly-stable series:
   ```js
      // ...
      "v1.7": {
         label: "1.7.x",
         path: "v1.7",
         banner: "none",
      },
      "v1.8": {
         label: "1.8.x (beta)",
         path: "v1.8",
         banner: "unreleased",
      },
      current: {
         label: "1.7.x",
         path: "",
         banner: "none",
      },
      main: {
         label: "Development",
         path: "main",
         banner: "unreleased",
         noIndex: true,
      },
      // ...
   ```

   First identify any previous series whose security support period has ended, and change their `banner` entries from `"none"` to `"unmaintained"`. (Updating this only when we publish a new release means that we'll lag behind slightly in marking earlier series as being unmaintained, which we accept on the assumption that a new release series should typically arrive less than two months after an existing one has reached end-of-life.)

   Then remove the " (beta)" suffix from the newly-stable series' label, change the `banner` property to `"none"`, and change the `current` entry to refer to the new series:

   ```js
      // ...
      "v1.7": {
         label: "1.7.x",
         path: "v1.7",
         banner: "none",
      },
      "v1.8": {
         label: "1.8.x",
         path: "v1.8",
         banner: "none",
      },
      current: {
         label: "1.8.x",
         path: "",
         banner: "none",
      },
      main: {
         label: "Development",
         path: "main",
         banner: "unreleased",
         noIndex: true,
      },
      // ...
   ```
2. Still in `docusaurus.config.ts`, find the `navbar` property which includes an item whose label is "Docs" and whose nested items describe the navigation links for different versions, like this:
   ```js
   // ...
   {
      label: "v1.6.x",
      href: "/docs/v1.6/",
   },
   {
      label: "v1.8.x (beta)",
      href: "/docs/v1.8/",
   },
   {
      label: "Development",
      href: "/docs/main/",
   },
   // ...
   ```

   First delete the objects for any series whose `banner` was changed to `"unmantained"` in the previous step. We preserve the old version doc pages to avoid breaking incoming links, but we only list the currently-supported series in the main navigation bar to prevent it from being cluttered with a growing set of end-of-life releases.

   Then move the entry that was previously added for the new series (`v1.8.x (beta)` in this example) to the top of the list and remove the ` (beta)` suffix from its label.
3. Follow the [Git submodule update](#website-submodule-update) steps to change the submodule to refer to the same commit as the `v1.8.0` tag in the `opentofu/opentofu` repository, but don't commit the change yet until finishing this set of steps.
4. Recreate the `docs` symlink in the root of the repository so that it refers to the submodule directory for the newly-stable release series:
   ```shell
   rm docs
   ln -s opentofu-repo/v1.8/website/docs docs
   ```

After following these instructions you should be able to launch the local dev server for the website by running `docker compose up --build` and find that the link for the new release series is the first item in the dropdown menu under "Docs" in the top navigation bar. Following that link should lead to the documentation for the new series, without any alert boxes warning that it is unreleased or unmaintained.

If everything looks good then commit all of these changes and open a PR in the `opentofu.org` repository.

</details>

---

## GitHub milestones

If you are publishing a _stable_ release (not a beta or release candidate), update the GitHub milestones in the `opentofu/opentofu` repository as follows:

* Close the milestone whose name matches the release you've just published.
* Create a milestone for the next patch release in the same series, if it doesn't already exist. For example, if you just closed a "v1.8.0" milestone then create "v1.8.1`.

   **Exception:** If you've just published what is expected to be the final patch release in its series (because the end-of-life date is imminent or already passed) then don't create a new milestone in that series.

For the first release in a new series we use the same milestone continuously for its beta releases, release candidates, and final release, so we should _not_ close e.g. the v1.8.0 milestone when v1.8.0-beta1 is released.

The milestone for the first release in a new series (e.g. v1.9.0 relative to v1.8.0) will typically have been created at some point during the previous development period as a place to drop any issues that we decide to no longer treat as release blockers, but if not then we should create the v1.9.0 milestone when closing the v1.8.0 milestone, in addition to creating the v1.8.1 milestone as described above.

---

## Updating govulncheck github workflow

In [.github/workflows/govulncheck.yml](.github/workflows/govulncheck.yml), there is a matrix with the actively maintained versions of OpenTofu.

During each release:

- If starting a new release series, add its maintenence branch to the matrix.
- If any earlier series has reached end-of-life since the previous release, remove its maintenence branch from the list.

---

## Testing the release

Make sure you have a Linux box with Snapcraft installed and download the installer shell script from `https://get.opentofu.org/install-opentofu.sh`.

Now test the following 3 installation methods to make sure all distribution points are up to date.

1. Snapcraft (stable and point releases only):
   * `sudo snap install opentofu --classic`
   * `tofu --version`
   * `sudo snap uninstall opentofu`
2. Deb (stable and point releases only)
   * `./install-opentofu.sh --install-method deb`
   * `tofu --version`
   * `apt remove --purge tofu`
3. Standalone:
   * `./install-opentofu.sh --install-method standalone --opentofu-version X.Y.Z`
   * `/usr/local/bin/tofu --version`
   * `sudo rm -rf /opt/opentofu /usr/local/bin/tofu`

---

## Posting the announcement

Once you are happy that the release works, post the announcements to the following places:

- Beta: Community Slack, Linkedin, X, Blog
- Stable: Community Slack, Linkedin, X, Blog
- Point release: Community Slack
