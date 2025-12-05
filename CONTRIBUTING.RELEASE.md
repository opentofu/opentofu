# OpenTofu release manual

> [!WARNING]
> This manual is intended for OpenTofu maintainers. If you are looking for the normal contribution guide, refer to [`CONTRIBUTING.md`](CONTRIBUTING.md).

This document describes how to publish an OpenTofu release. OpenTofu has three kinds of releases:

- Development snapshots, labeled as "alpha" releases, are built from the `main` branch representing in-progress work toward the next major or minor release series.
- Prereleases, labelled as either "beta" or "rc", are built from a branch representing the release series they are previews of, such as `v1.8`.
- Final releases, which have no special suffix and are intended for routine use, are also built from the corresponding release branch.

We typically produce development snapshots and prereleases only prior to the first final release in each series. For example, `v1.8.0-beta1` comes before `v1.8.0`, but there will not typically be prereleases for subsequent patch releases in a series such as `v1.8.1-beta1`.

--- 

## Naming in this document

- **Alpha** is a development snapshot, representing an arbitrary state of development that might be useful for early testing of new features. These are named as `vX.Y.0-alphaW`, where `X`,`Y` and `W` are numbers, such as `v1.2.0-alpha1`.
- **Beta** is feature-complete for a new release series but still needs further testing to develop confidence about newly-implemented features. This are named as `vX.Y.0-betaW`, where `X`,`Y` and `W` are numbers, such as `v1.2.0-beta1`.
- **RC** is a "release candidate" build that we believe is ready to be re-released as a final release. This are named as `vX.Y.0-rcW`, where `X`,`Y` and `W` are numbers, such as `v1.2.0-rc1`.
- **Final** is a release that we believe is ready for routine use.
    - The first final release in each series is is versioned `vX.Y.0`, where `X` and `Y` are numbers, such as `v1.2.0`. This exactly matches the latest release candidate aside from its compiled-in version number.
    - **Patch releases** are final releases containing only low-risk bug fixes relative to the previous final release in the same series. These are named as `vX.Y.Z` where `X`, `Y` and `Z` are numbers, such as `v1.2.3`.

"Beta" and "RC" are together described as "prereleases", since they have similar release ceremonies and differ only in how release-ready we consider the functionality to be.

> [!WARNING]
> Some tools use the GitHub releases API to determine which releases are available, and may rely on the ordering of that result to decide the latest version rather than actually sorting the available versions by precedence.
> Therefore we try to ensure that after we complete a set of releases the most recent one created was the one with the highest version number. For example, if we were publishing patch releases v1.2.1 and v1.3.1 and the latest release series is v1.4, we would publish v1.2.1 first, then v1.3.1 second, and then finally publish a new v1.4.x release _even if one isn't actually needed_ so that once we are done the latest release will be the v1.4.x release.
> By this strategy, the first release returned by the GitHub API is only "wrong" briefly during our series of releases, but we leave it in a state that these naive tools can still find a suitable "latest release".

---

## Gathering the team for a release

We publish packages and release announcements to a variety of different locations. To fully complete a release you will need access to credentials for all of the following:

- Cloudflare
- PackageCloud
- Snapcraft
- Linkedin
- X

---

## Preparing release announcements

Before you start creating a release, make sure you have the following release anouncements ready to be published:

<details><summary>

### Development snapshot (`vX.Y.0-alphaW`)

</summary>

- "Help us test..." blog post, similar to [Help us test OpenTofu 1.10.0-alpha1](https://opentofu.org/blog/help-us-test-opentofu-1-10-0-alpha1/).
- Community Slack announcement
- Linkedin and X posts

(We are currently considering adopting automated scheduled development snapshots instead, such as nightly builds, in which case we are likely to stop publishing explicit announcements for development snapshots. However, we currently create them manually and announce them in a similar way as prereleases.)

</details>

<details><summary>

### Prereleases (`vX.Y.0-betaW` or `vX.Y.0-rcW`)

</summary>

The _first_ prerelease (labelled "beta1")  is accompanied by a
"Get ready for..." blog post, similar to
[Get ready for OpenTofu Beta 1.9.0](https://opentofu.org/blog/opentofu-1-9-0-beta1/).

For _all_ prereleases, we also publish:

- Community Slack announcement
- Linkedin and X posts

beta2 and onwards should typically include only bugfixes relative to beta1,
and so we don't publish a new blog post for each one.

</details>

<details><summary>

### First final release in new series (`vX.Y.0`)

</summary>

- Release blog post with the feature and community highlights since the last release, similar to [OpenTofu 1.9.0 is available ...](https://opentofu.org/blog/opentofu-1-9-0/).
- Community Slack announcement
- Linkedin and X posts

</details>

<details><summary>

### Subsequent patch releases in an existing series (`vX.Y.Z`)

</summary>

- Community Slack announcement

Patch releases should include only bugfixes relative to their predecessors in
the same series, and so we don't publish a new blog post for each one.

</details>

---

## Preparing the repository for a release

Before you can create a release, you need to make sure the following files are up to date:

- [CHANGELOG.md](CHANGELOG.md) (Note: do not remove the `(unreleased)` string from the version number before the stable release.)
- [version/VERSION](version/VERSION)

These changes should be the final PR merged before the release.

---

## Tagging the release

Now that you have the files up to date, do the following:

1. On your computer, make sure you have checked out the correct branch:
   * `main` for "alpha" releases
   * `vX.Y` for any other releases (assuming you are releasing `vX.Y.Z`)
2. Make sure the branch is up-to-date by running `git pull`
3. Create the correct tag: `git tag -m "vX.Y.Z" vX.Y.Z` (assuming you are releasing `vX.Y.Z`)
4. Push the tag: `git push origin vX.Y.Z`

---

## Creating the release

Now comes the big step, creating the actual release.

1. Head on over to [the opentofu/opentofu repository's "Actions"](https://github.com/opentofu/opentofu/actions)
2. Select the `release` workflow on the left side
3. Click the `Run workflow` button, which opens a popup menu
4. Select the correct branch:
   * For "alpha" releases, select the `main` branch
   * For all other releases, select the appropriate release branch
5. Enter the correct git tag name: `vX.Y.Z`
6. If you are releasing the latest `vX.Y` version, check the `Release as latest?` option.
7. If you are releasing a development snapshot or prerelease version, check the `Release as prerelease?` option.
8. Click the `Run workflow` button.

Now the release process will commence and create a *draft* release on GitHub. If you did not check the prerelease option, it will also publish to Snapcraft and PackageCloud.

---

## Publishing the GitHub release

The release process takes about 30 minutes. When it is complete, head over to the [Releases section](https://github.com/opentofu/opentofu/releases) of the main repository and find the new draft release. Change the following settings:

- Edit the text (see the examples below).
- Check `Set as a pre-release` if you are releasing a development snapshot or prerelease version.
- Check `Set as the latest release` if you are releasing a final release for the latest release series. Do not check this checkbox if you are releasing a point release for an older major version.
- Check `Create a discussion for this release` if you are releasing the first final release of a new series (`vX.Y.0`).
- Click `Publish release`.

<details><summary>

### Development snapshots and prereleases

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

### First final release in new series (`vX.Y.0`)

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

For all the features, refer to [the detailed changelog](https://github.com/opentofu/opentofu/blob/v1.8.0/CHANGELOG.md).

For full details, refer to [the full diff](https://github.com/opentofu/opentofu/compare/v1.7..v1.8.0).
```

</details>

<details><summary>

### Patch release (`X.Y.Z`)

</summary>

For patch releases, simply copy the relevant section from the [`CHANGELOG.md`](CHANGELOG.md) file.

</details>

---

## Updating `get.opentofu.org`

In order for the installer script to work, you will need to update the https://get.opentofu.org/tofu/api.json file. You can do this by logging in to Cloudflare and go to the [`opentofu-get` project in Cloudflare Pages](https://dash.cloudflare.com/84161f72ecc1f0274ab2fa7241f64249/pages/view/opentofu-get). Here click the three dots on the latest production deployment and click `Retry deployment`.

---

## Updating the website/documentation

For final releases (_not_ development snapshots and prereleases) you will need to update [the opentofu.org repository](https://github.com/opentofu/opentofu.org) to make the new documentation available.

Before you begin, make sure that all submodules are up to date by running:

```
git submodule init
git submodule update
```

> [!WARNING]
> If you are using Windows, make sure your system supports symlinks by enabling developer mode and enabling symlinks in git.

<details><summary>

### First release in a new series (`vX.Y.0`)

</summary>

1. Add a submodule for the new release to the website repository:
   ```
   git submodule add -b vX.Y https://github.com/opentofu/opentofu opentofu-repo/vX.Y
   ```
2. After you have done this, open the [`docusaurus.config.ts`](https://github.com/opentofu/opentofu.org/blob/main/docusaurus.config.ts) file and `presets` section.
3. Here, locate the previous latest version:
   ```
   "vX.Y-1": {
     label: "X.Y-1.x",
     path: "",
   },
   ```
   Change it to:
   ```
   "vX.Y-1": {
     label: "X.Y-1.x",
     path: "vX.Y-1",
     banner: "none",
   },
   ```
4. Now add the new version you are releasing:
   ```
   "vX.Y": {
     label: "X.Y.x",
     path: "",
   },
   ```
5. After this is set, change the `lastVersion` option to refer to the newly-added version.
6. Locate any version that is no longer supported and remove the following line to add a deprecation warning:
   ```
     banner: "none",
   ```
7. Finally, locate the `navbar` option and `Docs` dropdown to reflect the new version list. It should look something like this:
   ```
   items: [
      {
        label: "vX.Y.x (current)",
        href: "/docs/"
      },
      {
        label: "vX.Y-1.x",
        href: "/docs/vX.Y-1/"
      },
      // ...
      {
        label: "Development",
        href: "/docs/main/"
      },
    ],
   ```

</details>

<details><summary>

### Patch release (`X.Y.Z`)

</summary>

For a patch release, you only need to make sure that the submodules for the supported versions are up to date. You can do this by running the following script:

```bash
cd opentofu-repo
for ver in $(ls); do
  cd "${ver}"
  git pull origin "${ver}" 
  cd ..
  git add "${ver}"
done
```

Now you can commit your changes and open a pull request.

> **Note:** You can safely run the script above anytime you need to update the documentation independently of a release. It's ok for the website to have minor doc fixes that are not in line with OpenTofu releases.

</details>

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

- Development snapshots: Community Slack, Linkedin, X, Blog, YouTube.
- Prereleases: Community Slack, Linkedin, X. For the first beta, also the Blog.
- First final release in new series: Community Slack, Linkedin, X, Blog.
- Patch release: Community Slack.

## Post-release Cleanup

Once all of the above is complete, the release is finished as far as end-users are concerned.

However, we have some other tasks to perform shortly after the release that support our ongoing development work.

### First prerelease in a new series

Our first prerelease for each release series marks that series being feature-complete, and so before we merge any more PRs we must create the new release branch, named after the first two segments of the release version number: `vX.Y`.

Creating the new branch also implicitly marks that `main` is now tracking development for the _next_ release series. Any bugfixes that are relevant to the prereleases should typically be merged first into `main` and then backported to the release branch.

Reset the changelog on `main` so that it contains only a section for the release series that is now seeing new feature development, and so that the list of links to prior release series includes a link to the changelog on the release branch just created.

### First release in a new series

[.github/workflows/govulncheck.yml](The `govulncheck.yml` GitHub Actions workflow) contains a list of the currently-supported versions of OpenTofu, for which we will generate GitHub issues for newly-published third party security advisories.

    Add the branch name for the release branch of the new current release series, and remove any versions that are no longer supported.
