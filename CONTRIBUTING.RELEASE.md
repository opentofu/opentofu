# OpenTofu release manual

> [!WARNING]
> This manual is intended for OpenTofu core and fork maintainers. If you are looking for the normal contribution guide, see [this file](CONTRIBUTING.md).

This manual describes how to create an OpenTofu release. OpenTofu has two kinds of releases. Alpha releases are created
from the `main` branch, while we split off a version (e.g. `v1.8`) branch before creating a `beta`, `rc` or `stable`
release.

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

### Alpha version (`X.Y.0-alphaW`)

</summary>

- Feature Preview video for upcoming flagship features (see https://www.youtube.com/@opentofu for examples)
- "Help us test..." blog post (see https://opentofu.org/blog/ for examples)
- Community Slack announcement
- Linkedin and X posts

</details>

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

## Tagging the release

Now that you have the files up to date, do the following:

1. On your computer, make sure you have checked out the correct branch:
   * `main` for `alpha` releases
   * `vX.Y` for any other releases (assuming you are releasing version `X.Y.Z`)
2. Make sure the branch is up-to-date by running `git pull`
3. Create the correct tag: `git tag -m "X.Y.Z" vX.Y.Z` (assuming you are releasing version `X.Y.Z`) 
   * If you have a GPG key, consider adding the `-s` option to create a GPG-signed tag
4. Push the tag: `git push vX.Y.Z`

---

## Creating the release

Now comes the big step, creating the actual release.

1. Head on over to the [Actions tab](https://github.com/opentofu/opentofu/actions) on the main repository
2. Select the `release` workflow on the left side
3. Click the `Run workflow` button, which opens a popup menu
4. Select the correct branch:
   * For `alpha` releases, select the `main` branch
   * For all other releases, select the appropriate version branch
5. Enter the correct git tag name: `vX.Y.Z`
6. If you are releasing the latest `X.Y` version, check the `Release as latest?` option.
7. If you are releasing an `alpha`, `beta` or `rc` version, check the `Release as prerelease?` option.
8. Click the `Run workflow` button.

Now the release process will commence and create a *draft* release on GitHub. If you did not check the prerelease option, it will also publish to Snapcraft and PackageCloud.

---

## Publishing the GitHub release

The release process takes about 30 minutes. When it is complete, head over to the [Releases section](https://github.com/opentofu/opentofu/releases) of the main repository and find the new draft release. Change the following settings

- Edit the text (see the examples below).
- Check `Set as a pre-release` if you are releasing an alpha, beta, or release candidate.
- Check `Set as the latest release` if you are releasing a stable or point release for the latest major version. Do not check this checkbox if you are releasing a point release for an older major version.
- Check `Create a discussion for this release` if you are releasing a stable (`X.Y.0`) version.
- Click `Publish release`

<details><summary>

### Alpha, beta, or release candidate

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

## Updating `get.opentofu.org`

In order for the installer script to work, you will need to update the https://get.opentofu.org/tofu/api.json file. You can do this by logging in to Cloudflare and go to the [`opentofu-get` project in Cloudflare Pages](https://dash.cloudflare.com/84161f72ecc1f0274ab2fa7241f64249/pages/view/opentofu-get). Here click the three dots on the latest production deployment and click `Retry deployment`.

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

- Alpha: Community Slack, Linkedin, X, Blog, YouTube
- Beta: Community Slack, Linkedin, X, Blog
- Stable: Community Slack, Linkedin, X, Blog
- Point release: Community Slack
