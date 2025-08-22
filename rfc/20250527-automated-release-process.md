# Automated Release Process

[Our current release process](https://github.com/opentofu/opentofu/blob/867be99bf11810d48168894e0955a6d71e1c175e/CONTRIBUTING.RELEASE.md) is long, with lots of manual steps and requires access to an assortment of third-party systems.

This RFC proposes partial automation of the release process, along with some changes to the process to make it easier to automate.

## Periodic Development Snapshots

We currently produce "alpha" releases that somewhat-arbitrarily represent snapshots in the development process of features in a new release series, in the hope of getting early feedback from those who are particularly interested in the new features before they are feature complete.

The planning and communication overhead of manually-curated releases is not really justified given the typical (low) level of engagement we see with alpha releases, so we will remove the human element entirely by producing development snapshots on a regular schedule based on whatever happens to be on the `main` branch at the time.

Other similar projects produce new snapshots roughly every 24 hours, traditionally called "nightlies" though of course there is no time of day that is nighttime _everywhere_. We will instead refer to these as "alpha" releases, both to continue with something like our existing naming scheme and because "alpha" sorts before "beta" in a naive sort.

Releasing every 24 hours means that there will now be far more of these releases than before, so to make it easier to keep track of which commit each snapshot was built from we'll incorporate the latest commit's timestamp and commit id into the version numbers, in place of the current incrementing integers, producing version numbers like: `v1.11.0-alpha20200907205600-7b23bcd95eed`. If someone opens a GitHub issue based on such a release we will then be able to easily find the corresponding commit without having to litter our repository with hundreds of Git tags.

We expect that development snapshots will be used primarily by those who are particularly interested in testing a specific feature that's currently under development, and that the engineers working on that feature will share links to specific snapshots part of GitHub issue comments when requesting such tests, and so development snapshots do not need to be published in a highly-discoverable location such as our GitHub Releases feed. Instead, our automated snapshot process will publish them to a system such as Amazon S3 or Cloudflare R2 to be exposed as if a static website.

The automated snapshot process will retain the latest 30 snapshots, discarding any older ones as part of the publishing process. This is a compromise to keep specific snapshots available long enough that we can use them to reproduce bug reports while avoiding unbounded storage costs.

If there are no new commits since the most recent development snapshot then no new snapshot will be created. The automated snapshot process recognizes this situation by calculating the version number corresponding to the latest commit on the `main` branch and comparing it with the version number of the most recent snapshot.

The automated snapshot process will be implemented initially as a [scheduled GitHub Action](https://docs.github.com/en/actions/writing-workflows/choosing-when-your-workflow-runs/events-that-trigger-workflows#schedule). Because OpenTofu has contributors from all around the world, there is no obvious "quiet time" to schedule this process for, so we will arbitrarily pick an hour and minute for the job to run each day, avoiding likely-high-volume times like midnight UTC or the zeroth minute of any hour.

## Curated Releases

For the sake of this document, "Curated Releases" broadly means all of the kinds of releases that happen only when the team decides they are ready, rather than happening automatically on a schedule. That currently includes:

- Prereleases: "beta" and "rc" releases representing feature-complete code that is ready for testing but not necessarily yet ready for final release.
- Final releases: releases intended for routine use, including both the first release in a new series (e.g. v1.11.0) and subsequent patch releases in an existing series (e.g. v1.11.5).

These two categories differ mainly in how we communicate to the community about them, with most of the release steps being the same. However, there are some notable differences to deal with:
- We don't publish prereleases to our Debian package ("apt") or RPM package repositories.
- Prereleases must be marked as such in the GitHub Release entries we create.
- Only new patch releases in the currently-active release series must be marked as "the latest release" in the GitHub Release entries we create.
- We only "Create a discussion for this release" for the first final release in each series.

The release process for Curated Releases begins by opening a new Pull Request in the opentofu/opentofu repository which:
- Sets the `VERSION` file to contain the version number we intend to release, if it doesn't already reflect that.
- Includes the final edits to the changelog that will be locked in as part of the release tag.
- Is targeting the branch that the release will be published from. (`main` for the first prerelease of a series, or the series' own branch otherwise.)
- Is labeled with the "release" label.

Creating a new PR with the "release" label, adding that label after it was already created, or pushing a new commit to a PR that already has that label triggers a special GitHub Actions workflow to prepare the release. That workflow performs the following steps:

1. Verifies that the content of the latest commit on the pull request is consistent, failing quickly if not:
    - `VERSION` contains a valid number that makes sense for the PR's target branch.
    - The first section of `CHANGELOG.md` has a heading that matches the basename of the version number from `VERSION` (without any prerelease suffix).
    - Either there is no existing GitHub Release record matching the content of `VERSION`, or there is one but it's marked as a draft.
2. Produces all of the artifacts for the release, including the main `.zip` archives, all of the derived artifacts like RPM packages, and signed artifact checksums.
3. Builds the end-to-end test driver executable for each of our "tier 1" platforms (exact selection TBD elsewhere) and then runs the tests against the artifacts we built in the previous step.
4. Assuming that all of the previous steps succeeded without detecting any problems, creates or updates a draft GitHub release with all of the artifacts attached to it.
    - If creating a new GitHub release, rather than updating a pre-existing draft, it also pre-populates the release notes with the relevant section of `CHANGELOG.md` as a starting point.
5. Posts a comment to the Pull Request announcing that all of the above has completed and linking to the draft GitHub Release.

At this point the release captain (a member of the maintainers team) can inspect the draft GitHub Release and amend the release notes if necessary. If anything went wrong in the initial workflow they can push new commits to the Pull Request to correct the problems, which causes the preparation workflow above to re-run.

Once the draft GitHub Release seems ready, the release captain merges the pull request which then triggers another GitHub Actions workflow to finalize the release. That workflow publishes the draft release, thereby causing [the `release`: `published` event](https://docs.github.com/en/actions/writing-workflows/choosing-when-your-workflow-runs/events-that-trigger-workflows#release) to be triggered, which in turn triggers other workflows for copying subsets of the release artifacts into other locations. For example:

- Copying the `.deb` and `.rpm` artifacts into their respective repositories.
- Pushing the container image artifacts to suitable repositories.
- Updating `get.opentofu.org` to include the latest release.
- If the target branch of the PR was `main`, and therefore this is the first prerelease of a new series, push the new Git branch for that series initially referring to the same commit as the release's tag.

These follow-on steps are modeled as separate workflows that have both a `release` trigger and a [`workflow_dispatch` trigger](https://docs.github.com/en/actions/writing-workflows/choosing-when-your-workflow-runs/events-that-trigger-workflows#workflow_dispatch) so that we can trigger them manually and separately if something goes catastrophically wrong with the initial automatic trigger. When triggered manually these workflows take a GitHub Release name (i.e. a version number) as an input, and copy the relevant artifacts from that release, failing if there is no such release or if it isn't yet marked as published.

## Avoiding Commits in Other Repositories

Our current manual release process includes making manual edits to various repositories that must all be carefully coordinated and timed to ensure that everything updates in the correct sequence. For an automated release process this is considerably more bothersome, because it requires cross-repository commit access and causes the process to have steps that are difficult to repeat robustly in the event of failure.

Instead, we'll change the strategy we use for each of those cases.

### On `get.opentofu.org`

We don't actually create a new Git commit for this repository on each release, and instead it pulls data from GitHub releases.

As a mechanism to support the strategies described in the following sections, we will update the `get.opentofu.org` generation script to generate an additional API endpoint that summarizes the disposition of each release series that isn't yet end-of-life. For example:

```json
{
  "series": {
    "1.10": {
      "latest": "1.10.0-beta2",
      "status": "beta"
    },
    "1.9": {
      "latest": "1.9.2",
      "status": "active"
    },
    "1.8": {
      "latest": "1.8.5",
      "status": "active"
    },
    "1.7": {
      "latest": "1.7.9",
      "status": "active"
    }
  }
}
```

The code that generates this endpoint should implement our end-of-life policy as executable code. At the time of writing this RFC that policy is that the three series with the greatest numbers that have at least one final release are considered "active". We have also discussed possibly adding a minimum amount of time that a release series is supported, and if we adopt such a policy in future then the logic would use the date of the earliest final release in each series to implement that rule.

The generator will implement the additional rule that any series with at least one prerelease has its status set to either "beta" or "rc" depending on the type of the latest prerelease.

Although this RFC proposes this API primarily for our internal use as described in the following sections, it also represents a first-party equivalent to [the OpenTofu entry on `endoflife.date`](https://endoflife.date/opentofu), updated automatically by us shortly after each new release is published. If we decide that we want to offer a more complete equivalent to that data then we could potentially choose to also include the greatest-numbered series that is now considered end of life, listed as status "end-of-life", but that is outside the scope of this proposal and mentioned only to suggest that the clients of this API described in the following section should be resilient to additional values of "status" that are not relevant to their needs.

### The `opentofu.org` website

Currently the website draws versioned documentation from manually-configured git submodules in its own repository, which therefore need to be updated for each new release.

Instead, we will add a script to the opentofu/opentofu.org repository that requests the list of release series from our `get.opentofu.org` API. It will then construct the set of Git submodules dynamically so that it includes all of the series reported by that API response, along with an always-present entry for the `main` branch.

It will then automatically generate the [`docusaurus.config.ts`](https://github.com/opentofu/opentofu.org/blob/e6ba51daa3b5533abcefd25dbf1e4f471092dcd8/docusaurus.config.ts) file (or, more likely, a separate file that is included into it) so that all of the submodules that were just set up are visible to Docusaurus and ensures that they are all listed in the "Docs" drop-down menu in the navigation bar as follows:

- The greatest release series that has status "active" has "(current)" written after it, and appears first.
- All other series of status "active" appear next, with no suffix at all.
- Each series of status "beta" or "rc" appears next, with either "(beta)" or "(rc)" written after it depending on the status.
- Finally, the entry for the `main` branch is listed, with a synthetic series number taken from the `VERSION` file currently on that branch followed by "(alpha)", if and only if that series number is different from all of those already listed. If it would match another release then it's skipped altogether.

    (That special exception deals with the awkward period between publishing the first prerelease in a new series and updating the `VERSION` file on the `main` branch to reflect what that branch is now representing.)

This script should run both as part of the automatic publishing process for the website and as part of the development environment setup process.

Generating the per-version documentation links dynamically based on `get.opentofu.org` both avoids any need for us to commit to this repository each time we make a release and allows us to configure Docusaurus to use a consistent URL prefix for a particular release series throughout its lifecycle, so that we can create durable links to the docs for features under development even before there's a final release available. The naked `/docs/` prefix, without a version number after it, is then treated as an alias for whichever series has the "(current)" suffix, with the generated pages including `<link rel="canonical" ...>` metadata elements that refer to the durable versioned URL.

### Our `govulncheck` GitHub Actions workflow

In the opentofu/opentofu repository we have a GitHub Actions workflow that runs on a schedule and checks whether any of our currently-supported releases have dependencies that are associated with vulnerabilities in the [Go Vulnerability Database](https://vuln.go.dev/), using [`govulncheck`](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck). That workflow currently has a hard-coded list of branches to check which we must manually update when we publish the first final release of a series.

We will change this workflow to use the new `get.opentofu.org` release series API instead, so that it'll react automatically to any new releases without us needing to edit the workflow directly.

## GitHub Actions setup details

GitHub Actions workflows live inside normal GitHub repositories, but placing our release workflows inside the main opentofu/opentofu repository would mean that we'd need to maintain them separately for each release branch.

Instead, we will place only small stub workflows in the release branches that use [Reusable Workflows](https://docs.github.com/en/actions/sharing-automations/reusing-workflows) to trigger the "real" workflows managed in a separate `opentofu/release` repository. This means that when we update them we will update them only on that repository's `main` branch and yet the changes will immediately take effect for all release branches in opentofu/opentofu.

The opentofu/opentofu repository should have its branch protection rules set so that a failure of the special release preparation workflow prevents merging a pull request, to help avoid mistakenly publishing a broken release. Repository administrators would be able to override this rule in exceptional cases if needed.

The GitHub Actions job steps that build artifacts for a release should be reusable between both our periodic development snapshot process and our curated release process, since they both need to perform similar work. To reduce the compute and storage overhead for development snapshots we'll configure the workflow to produce only the primary distribution archives (the `.zip` files) in that case, reserving the derived artifacts like deb/rpm packages and container images only for curated builds. Prerelease curated builds should still include the full set of artifacts even though we skip publishing them to secondary sources, since that will help us to notice early if the build process for those has become broken.

## Other release steps not in scope

This proposal notably does not include anything related to release announcements, such as blog posts and social media posts.

Although we could eventually automate some of those to some degree, that would expose the release automation to considerably more fiddly API credentials and potential failures, and we currently tend to introduce some "human touch" into our release announcements that would be harder to maintain with a fully-automated announcement workflow.

A future proposal could suggest ways to incorporate some announcements into the release workflow, such as including a machine-recognizable fragment of the GitHub Releases text that gets automatically copied into social media posts. However, that's intentionally not included here because this rework of our release workflow already represents a considerable amount of work and release announcements seem like a good place to draw the line for our first round of automation.
