# OpenTofu OpenVEX Repository

In our initial draft of [Security Advisory Policy for Upstream Dependencies](./20250314-security-patch-policy.md) we initially proposed to publish "zero-severity" security advisories whenever we determined that an upstream advisory did not apply to OpenTofu, so that it would be easy for security teams inside organizations using OpenTofu to find our resolution on any security advisory regardless of whether we determined it affected OpenTofu or not.

We ultimately revised the document due to two constraints of the current security advisory model:

1. The CVE system explicitly deals only in _origin_ advisories, describing problems in the topmost component affected. As a matter of policy it does not accept indirect advisories describing how each advisory affects downstream users, regardless of whether they report that the downstream is affected or unaffected by it.
2. GitHub's security system (which is the primary place we're publishing our advisories) does not accept "zero-severity" advisories even if we have no intention of submitting them upstream as CVEs.

Based on that, our compromise was establish effectively three different possible outcomes depending on our conclusions after investigation:

- If the problem leads to a unique situation within OpenTofu that is not directly addressed in the upstream advisory, we first publish as a GitHub-based advisory and then ask GitHub to submit a CVE on our behalf.

    This particular outcome is rare, because the rule against submitting "proxy advisories" as CVEs is quite strict. Even in some situations where we decided to follow this path, the GitHub security team concluded the advisory was not eligible for CVE submission.

- If OpenTofu is affected by the advisory in essentially the same way as other dependents, we publish a GitHub-based advisory only and don't request that it be submitted as a CVE.

- If we conclude that OpenTofu is not affected by the advisory at all, or that it is affected in a way that is not considered problematic under OpenTofu's threat model, then we share that outcome only in the GitHub issue describing the vulnerability and don't publish an OpenTofu-specific advisory at all.

That last category is unfortunate, because it means any verdict of an advisory being false-positive much harder to discover than the affirmative advisories, which in turn means that folks using security scanners have no easy way to confirm whether the security scanner results are relevant. This inevitably causes folks to ask us about advisories we already reviewed, because they were unable to find our previous conclusions. We must then spend time re-stating our existing conclusions, which has an opportunity cost.

Ideally then we'd like to publish our conclusions in a way that security scanners can automatically consume and incorporate into their results. For this to work at scale, our responses need to be published somewhere that interested parties will expect to find them, and to publish them in a form that automated systems can consume and automatically incorporate into security scanner results.

[OpenVEX](https://github.com/openvex/spec) specifies a mechanism for publishing machine-readable information about whether and how already-published security advisories apply to already-published release artifacts. This document proposes that the OpenTofu project maintain its own OpenVEX repository as our primary central location for publishing our conclusions about upstream security advisories, and also encourage those reports to be mirrored into the various third-party "VEX hubs" that act as aggregators of such data.

This proposal therefore modifies [Security Advisory Policy for Upstream Dependencies](./20250314-security-patch-policy.md) with a different policy for how we share our analysis results. If this proposal is accepted then part of its implementation would be to update [our `SECURITY.md` document](https://github.com/opentofu/opentofu/blob/main/SECURITY.md) to mention that we have OpenVEX data available, which would then be a single central place to find all of our advisory investigation outcomes instead of it being scattered across various different GitHub issues and GitHub Security Advisories.

## OpenVEX Repository in a GitHub Repository

To minimize the amount of additional infrastructure we need to maintain and new workflow we need to adopt, we'll keep our OpenVEX documents in a new GitHub repository at `opentofu/vex`. Using a separate decidated repository means that we can easily update it independently of changes in the main development repositories, without conflicting with each main repository's own release procedures and tagging schemes. This repository will be a "living document" whose `main` branch is constantly updated and never contains any relevant Git tags.

This repository will contain a separate OpenVEX JSON document for each component that we release security-supported binary packages for. Initially this will cover only the main OpenTofu CLI releases, in a file called `opentofu.vex.json`. If we add other security-supported components in future, they should follow the same `component-name.vex.json` naming scheme, and the same general structure.

The `opentofu.vex.json` file contains a JSON document conforming to the OpenVEX JSON-LD schema, which looks like this:

```json
{
  "@context": "https://openvex.dev/ns/v0.2.0",
  "@id": "tag:opentofu.org,2026:vex#opentofu",
  "author": "OpenTofu Project",
  "timestamp": "2026-09-21T00:00:00Z",
  "version": 1,
  "statements": [
    {
      "vulnerability": {
        "name": "CVE-2026-39883",
        "aliases": [
          "GO-2026-5426",
          "GHSA-hfvc-g4fc-pqhx"
        ]
      },
      "products": [
        {"@id": "pkg:generic/opentofu.org/opentofu@1.12.6"}
      ],
      "status": "not_affected",
      "justification": "vulnerable_code_not_in_execute_path",
      "impact_statement": "OpenTofu does not use the affected function resource.WithHostID."
    }
  ]
}
```

All of the properties above are defined in the OpenVEX v0.1 specification as of commit `61b5f885d0f4`. The OpenTofu VEX repository will use them as follows:

- `"@context"`: Required by the spec to have this specific value, identifying this object as using the OpenVEX JSON-LD schema.
- `"@id"`: An arbitrary URL used to uniquely identify this JSON-LD object. General form is `tag:opentofu.org,2026:vex#` followed by the same component name used in the document filename.
- `"author"`: Always `"OpenTofu Project"` to reflect that these are official project announcements.
- `"timestamp"`: Specified to be the time when this document began existing, and so we'll set it to just an arbitrary time close to when we first establish the repository. (This timestamp is _not_ required to change each time the document is updated.)
- `"version"`: An integer that increments each time the document is updated.
- `"statements"`: An ever-growing list of vulnerability conclusion statements:
    - `"vulnerability"`: identifies which vulnerability this statement is referring to. Where possible we would list the CVE ID as the primary name, and both the Go vulnerability database identifier and the GitHub Security Advisory identifier as aliases.
    - `"products"`: uses "product URL" syntax to identify which product is affected.

        This establishes for the first time the OpenTofu Project's `pkg:generic/opentofu.org/*` PURL namespace, whose members again match the component names used in our OpenVEX document filenames. We will list all of the versions that we reviewed during our analysis, which should typically be at least the latest version of each active release series. This generic PURL namespace avoids tethering our identifier to GitHub and avoids misrepresenting OpenTofu as being a Go library, rather than a standalone application that just happens to be written in Go.
    - `"status"`: machine-readable representation of our conclusion about whether the releases we identified in `"products"` are affected by the vulnerability.

        At the time of authoring this RFC the OpenVEX specification only allows for specifying whether or not OpenTofu is affected, but [the OpenVEX community is  discussing severity modifications](https://github.com/openvex/spec/issues/31) and if accepted later we would adopt that extension to describe situations where OpenTofu is affected by an advisory in a less severe way than the upstream maintainers classified it.
    - `"justification"`: Whenever `"status"` is `"not_affected"` we would use one of the broad predefined justifications to give more information about why that is true.
    - `"impact_statement"`: A human-readable elaboration of `"justification"` summarizing why we reached that conclusion.
    - `"action_statement"`: Whenever `"status"` is `"affected"`, a human-readable summary of the action we're recommending, such as `"Upgrade to OpenTofu v1.13.4."`.

At the time of this proposal we do not expect the list of statements to grow large enough to overwhelm representing them all in a single JSON document per component. Each component gets its own document because we will typically evaluate each one separately and so may publish conclusions about the same advisory in different components at different times.

## Discovery of our OpenVEX Repository

For ease of incorporating our OpenVEX metadata into third-party security scanning software and VEX hubs, we will implement [the AquaSecurity VEX Repository Specification](https://github.com/aquasecurity/vex-repo-spec/blob/b795dc2403cc825956b6bb197c55734c1728f761/README.md). Although this is not universally supported across all tool, it seems to be a de-facto standard for now and is straightforward to support without running any additional infrastructure.

In practice this means publishing a small static discovery document on our website and including an additional index file in the `opentofu/vex` repository.

At `https://opentofu.org/.well-known/vex-repository.json we will publish the following JSON document describing how to obtain the content of the GitHub repository:

```json
{
  "name": "OpenTofu VEX Repository",
  "description": "Security advisory impact statements from the OpenTofu project",
  "versions": [
    {
      "spec_version": "0.1",
      "locations": [
        {"url": "https://github.com/opentofu/vex/archive/refs/heads/main.zip//vex-main"},
      ],
      "update_inverval": "168h"
    }
  ]
}
```

The URL specified in the discovery document refers to the GitHub-provided endpoint that returns a `.zip` archive of the current repository content on the `main` branch, meaning that we can treat this discovery document as a static artifact on the website which will not need to be updated again unless we switch away from using this GitHub repository to host our VEX content.

In the root of the `opentofu/vex` repository we will place an `index.json` document which initially lists only the `opentofu` VEX document:

```json
{
  "updated_at": "2026-09-21T00:00:00Z",
  "packages": [
    {
      "id": "pkg:generic/opentofu.org/opentofu",
      "location": "opentofu.vex.json"
    }
  ]
}
```

## Use in Third-party Security Scanners

Support for OpenVEX in third-party security scanners, as with just about everything related to security scanning, is ever-changing as the different security scanner vendors maintain a sort of semi-cooperative/semi-competitive posture relative to one another. This document proposes that OpenTofu host its own repository in order to be vendor-agnostic while following available public specifications, with the assumption that vendors either already support or can be encouraged by their customers to support these specifications.

Since the technology ecosystem around OpenVEX is primarily oriented around libraries rather than end-applications, typical scanner implementations rely on heuristic support for different packaging ecosystems (container image vs. NPM package vs. Maven package, etc) and will tend to require custom configuration to understand that an arbitrary `opentofu` executable corresponds to PURL `pkg:generic/opentofu.org/opentofu@VERSION`. The details of this will differ between tools so those who employ security scanners would need to discuss with their vendors how best to configure them to scan OpenTofu with awareness of our OpenVEX repository.

Just as an overview to give an idea of what this might look like, Aqua Security's Trivy allows specifying Vex repositories in a YAML configuration file:

```yaml
repositories:
  - name: OpenTofu
    url: https://opentofu.org
    enabled: true
```

Some other tools don't support the repository protocol but can still be configured to use directly-specified OpenVEX JSON documents during scanning. Folks using these tools would therefore likely require some additional setup to pre-fetch the contents of our repository.

This proposal intentionally has a more limited scope of just establishing a machine-readable alternative to our current natural language advisory analysis conclusions, leaving it up to those wishing to rely on this information to decide how best to incorporate it. Future proposals may extend this mechanism based on how folks react to this initial data availability. Refer to [Future Considerations](#future-considerations) below for more information.

## Maintaining the VEX metadata

We'll maintain the data in the `opentofu/vex` repository using a typical GitHub pull request workflow.

However, the VEX repository structure encourages placing many unrelated statements together in a single file, which is convenient for consumers but awkward for maintainers since concurrent PRs targeting the same file are highly likely to conflict with each other.

Therefore instead of directly maintaining the `opentofu.vex.json` file (and any other `.vex.json` files we include in future) we'll maintain a more human-editing-friendly source data tree under a directory named after the component name, with a separate YAML file per statement, giving repository contents like the following:

```
index.json
opentofu.vex.json
opentofu/
  CVE-2026-39883.yaml
  CVE-2026-32952.yaml
  ...
```

The YAML files under the component directory will follow the same schema as the JSON objects under `"statements"` in the `.vex.json` files, but written using YAML syntax instead of JSON syntax for easier human editing and the ability to use comments to record additional maintainer-oriented context when appropriate.

The names of these files are arbitrary but should conventionally follow what was specified as `"name"` under `"vulnerability`" in the document. In some cases we may have multiple statements for the same vulnerability in order to describe different statuses for different versions, in which case we'll use names like `CVE-2026-39883-1.12.yaml` and `CVE-2026-39883-1.13.yaml` that include whatever additional version context is needed to make for a unique filename that's understandable to human maintainers.

We will then write a small, standalone Go program under a `tools/generate` directory in this repository which generates the single `opentofu.vex.json` based on all of the YAML files under `opentofu/`. The `"version"` property is set to the number of YAML files in each directory to ensure that it increases each time we add a new statement without us having to manually maintain a version number. This will be listed as a Go tool so it can be run as `go tool generate`.

The repository will also include a GitHub Actions-based Pull Request check which runs `go tool generate` and fails if it generates something different to what's in the pull request's code tree.

Unfortunately because the pull requests will still include updates to the central `opentofu.vex.json` they are still likely to become conflicted if we have multiple open concurrently, but such conflicts can be resolved relatively easily by just rebasing and running `go tool generate` again, thereby overwriting the conflicted `opentofu.vex.json` with a new version suitable to submit. In practice we tend to discover new vulnerabilties in batches based on our weekly `govulncheck` run and so it's likely that a single person would just be submitting a selection of statements all at once describing the results of analyzing that batch, and so we should not frequently encounter conflicts.

## Future Considerations

This proposal is intentionally focused quite narrowly on just establishing a place where we can publish VEX information in a machine-readable format, but as noted above the initial form of this will require explicit configuration that varies depending on which security scanner product someone is using.

The following sections describe some additional work we could do in future to ease this, but none of these are intended to be part of this proposal.

### SBOMs, Cosign, Rekor, etc

To make security scanning more likely to "just work", we could use [Sigstore](https://www.sigstore.dev/)'s infrastructure to publish an attestation of the SBOM ("Software Bill of Materials") associated with each OpenTofu release, including the specific Package URL that identifies it for correlation with the OpenVEX statements this RFC proposes.

This would then allow suitably-equipped security scanners that are broadly scanning over an entire filesystem to take a checksum of the executable, determine that it's an OpenTofu release, and automatically discover its SBOM. The SBOM document would identify it as being (for example) `pkg:generic/opentofu.org/opentofu@1.14.3`, which the security scanner can then correlate with the information in our OpenVEX repository. The SBOM could also include a direct link to our OpenVEX URL so that a suitably-configured security scanner can automatically discover it and suggest using it.

For our Docker images we could go further and embed the signed SBOM information directly inside the image.

All of this is out of scope for this first proposal because it requires significant changes to our release process, including an assortment of new release-time dependencies we'd need to evaluate carefully. It's still useful to publish machine-readable information about our conclusions about security advisories without this (at the expense of requiring more manual configuration on the end-user's part) and we can do _that_ part without making any changes to the main OpenTofu release process.

(Some aspects of this were previously discussed in [PR #2494](https://github.com/opentofu/opentofu/pull/2494), which may be useful background information if someone wants to propose this anew in future.)

### Metadata embedded in the `opentofu` executable

Our executables automatically get various metadata embedded in them by the Go toolchain, but that information primarily describes which Go modules OpenTofu depends on, and in particular doesn't currently allow capturing a specific Package URL (PURL) to use for metadata lookups.

There is an emerging standard for embedding metadata into Linux ELF executables in a language-agnostic way, as described in [package-notes](https://github.com/systemd/package-notes). This involves adding an extra "note" section to the ELF executable which security scanners can then extract to get a JSON document containing relevant metadata. For example:

```json
{
  "type": "generic",
  "name": "opentofu",
  "version": "1.14.3",
  "purl": "pkg:generic/opentofu.org/opentofu@1.14.3"
}
```

Some security scanners already support this convention, and others have indicated intention to support it. For a security scanner configured to trust this metadata, it could therefore automatically learn which PURL to use when matching the statements in our OpenVEX repository.

This mechanism is, of course, Linux-specific. It remains to be seen if similar conventions will emerge for embedding metadata in the executable formats used by other platforms, but Linux is widely used and so a Linux-only solution like this would still be quite impactful.

The Go toolchain does not currently have any support for inserting this metadata into the executables it generates, and so doing so would require some post-processing of the executable such as using the `objcopy` utility from GNU binutils. This is out of scope for this proposal because it requires changes to the release process, although this particular change is less invasive than SBOM/Sigstore support would be.

### OpenVEX Statements For Other Components

This document only proposes that we publish OpenVEX statements for the main OpenTofu CLI releases.

We could potentially grow this in future to cover other software we ship as locally-run executables, such as `tofu-ls`. If we do so, we'd continue the established naming scheme using a file called `tofu-ls.vex.json`, the PURL `pkg:generic/opentofu.org/tofu-ls`, etc.
