# Design considerations for OCI in OpenTofu

---

This document is part of the [OCI registries RFC](../20241206-oci-registries.md).

| [« Previous](2-survey-results.md) | [Up](../20241206-oci-registries.md) | [Next »](4-providers.md) |

---

## Provider addresses are virtual

Providers in OpenTofu are addressed by a virtual address in the format of `HOSTNAME/NAMESPACE/TYPE`, where `HOSTNAME` defaults to `registry.opentofu.org` and `NAMESPACE` defaults to `hashicorp` for historic reasons. The hostname identifies the so-called "origin registry" of the provider, which is typically the location where the provider author announces new releases.

However, a provider source address is intentionally independent of the actual download URL. By default, OpenTofu contacts the origin registry using [the Remote Service Discovery protocol](https://opentofu.org/docs/internals/remote-service-discovery/) and the [Provider Registry Protocol](https://opentofu.org/docs/internals/provider-registry-protocol/) to find the "official" release package locations. OpenTofu allows an operator to reconfigure the installation strategy for some or all provider source addresses, in which case the hostname in the source address serves only as part of the provider's unique identifier for OpenTofu tracking purposes, while the distribution packages can be discovered and installed from an entirely separate location. For example, OpenTofu uses the source address to "remember" which provider manages each resource in an OpenTofu state snapshot, and so the provider's source address must remain consistent between plan/apply rounds.

OpenTofu modules use this virtual address syntax to declare which providers they need _without_ specifying how they are to be installed. This then allows operators to choose to distribute providers from organization-local mirrors instead of origin registries without needing to modify the source code of the modules themselves. It is for this reason that provider source addresses to not include any part representing the installation protocol, and are therefore _not_ URLs.

In our survey, 64% of respondents have indicated interest in using publicly available providers without using the registry, while 13% of respondents have indicated that they would be interested in publishing a provider in a public OCI registry. A similar number of respondents have indicated that they would like to mirror a provider or create a provider for their internal organization.

To preserve the ability for an operator to unilaterally change the installation method for any provider, we must continue to ensure that no part of the source address syntax forces the use of the OCI Distribution protocol or forces OpenTofu to install any provider from a specific OCI registry. We _could_ potentially allow some domains to have a different _default_ installation behavior using OCI Distribution protocol instead of the OpenTofu Provider Registry protocol, but it must remain possible for the operator to mirror this provider to another location without changing its source address.

Since introducing a new _default_ installation method for certain provider addresses would be profound change, we intend to be cautious: our first release will focus on introducing OCI Registries only as a new kind of provider _mirror_, which operators can opt into with [CLI configuration settings](https://opentofu.org/docs/cli/config/config-file/#provider-installation). For more information, refer to [4-providers.md](Providers).

The OpenTofu Provider Registry Protocol will remain the only available _default_ installation method for now, although we will consider options for using OCI Registries as a new kind of "origin registry" in future releases, once we've learned from the experience of implementing and releasing the new opt-in mirror type.

## OCI layout

A significant consideration is the exact layout of the manifests and blobs for our new artifact types in the OCI registry. As described in [OCI Primer](1-oci-primer.md), OCI registries are very flexible how they store data.

Originally we aimed to represent both providers and modules using the same "differential-tar"-style layout used for traditional container images, including the typical `mediaType` values and blob formats used for container images. This would've allowed constructing OpenTofu artifacts using the same tools commonly used for container images, such as `docker build` with a `Dockerfile`, like this:

```Dockerfile
FROM scratch
ADD * /
LABEL "org.opentofu.artifact-type"="provider"
```

However, after some prototyping we abandoned this approach primarily because it would cause a provider package mirrored in an OCI registry to have a different checksum than its upstream from a traditional OpenTofu provider registry. OpenTofu verifies provider packages against checksums signed by the provider author which are calculated from the `.zip` archive used for distribution, rather than the contents of that archive. These checksums are recorded in the [dependency lock file](https://opentofu.org/docs/language/files/dependency-lock/) to allow for a workflow where, for example, the lock file is originally generated from metadata in the provider's origin registry but then later used to verify copies of those packages in a mirror used in an air-gapped production environment.

Although the OpenTofu dependency lock file format supports checksums generated in a variety of different ways, the mirror-verification workflow works best when all sources can agree on a single checksum scheme to use, because otherwise operators must manually encourage OpenTofu to generate additional checksum schemes (via local checksum calculation) using [`tofu providers lock`](https://opentofu.org/docs/cli/commands/providers/lock/).

An OCI digest of a zip package blob using the `sha256:` scheme is effectively the same as OpenTofu's zip-checksum format, albeit with a slightly different syntax. Therefore we have decided to use the `archive/zip` media type for the blobs in OpenTofu's artifact formats so that OpenTofu can directly compare an OCI-style digest of such a blob with a zip-checksum previously recorded in the dependency lock file, or vice-versa.

Secondarily, reusing the zip format for OpenTofu's artifacts means that we can rely on the existing code in OpenTofu for extracting such archives for use as either provider or module packages, thereby reducing the total amount of code we'll need to maintain and making it considerably less likely that a package distributed via the OCI Distribution protocol will produce a different extracted package on disk to one obtained by other means.

Refer to [Providers](4-providers.md) and [Modules](5-modules.md) for more details on the specific artifact layouts we intend to use.

## Software Bill of Materials (SBOM)

The decision to use a custom image layout as described in the previous section impacts security scanners, which a [large number of our users want to use](2-survey-results.md). We have tested several of the popular security scanners with varying degrees of success.

The popular scanners supporting container images out of the box, such as [Trivy](https://trivy.dev/), only directly support the differential-tar layer format used for container images, and do not currently support custom artifact types. Some tools can support single-image artifacts when they use the container image layout, but do not yet support multi-platform index manifests. Other popular tools, such as TFLint, do not directly support OCI Distribution at all.

However, tools in this space have varying degrees of support for indirect scanning via Software Bill of Materials (SBOM) manifests, which is a growing new design that effectively separates the problem of taking inventory of dependencies from the problem of detecting whether those dependencies have known vulnerabilities. This means that we could provide a means for a provider developer to publish an SBOM for their provider and let the security scanners work with that instead of attempting to construct one themselves by directly inspecting the OCI artifact.

To reduce scope for the initial implementation we do not intend to generate or use SBOM artifacts initially. However, we are considering extending the OpenTofu Provider Registry and Module Registry protocols to be able to return additional links to SBOM artifacts in future, at which point it would become possible to copy those artifacts into an OCI registry along with the packages they relate to, and thus make that information available to any OCI-registry-integrated security scanners that have SBOM support.

At the time of writing we have started discussing possible future SBOM functionality in [Pull Request #2494](https://github.com/opentofu/opentofu/pull/2494). Our initial release will prioritize the fundamentals of hosting provider and module packages in OCI registries _at all_, and so those whose interest in OCI-based distribution is motivated primarily by security scanner integration will need to either wait for a future release or generate and upload SBOMs to their registry separately.

## Artifact signing

While not a question in the survey, several respondents mentioned that artifact signing is an important consideration for them.

Today, OpenTofu providers are signed with GPG keys and [there is an open issue about supporting Sigstore/Cosign](https://github.com/opentofu/opentofu/issues/307), which is blocked on the availability of a stable Go library to do so.

Module packages are currently not signed in OpenTofu at all, which is a separate question to address. This consideration therefore applies primarily to providers.

For providers the OpenTofu Registry, an authority independent of the provider author, serves the role of verifier. If the OpenTofu Registry has verified a GPG key as belonging to an author, OpenTofu accepts it as valid. The same is true for third party registries: the registry holds the public GPG keys and provides them for verification, which is independent of the provider download URL. A malicious upload to the download URL would result in an invalid download unless the attacker can obtain the GPG key.

With OCI, however, there is no default central authority. We could implement publishing the SHA256SUMS file and its signature as part of the manifest, but this would not provide any independent information about which keys are allowed to sign a given provider. Alternatively, users would have to configure their own list of accepted GPG public keys.

There is also still an apparent lack of consensus in the OCI ecosystem about how to generate, publish, discover, and verify signatures. For example, [Cosign uses a tag naming convention](https://docs.sigstore.dev/cosign/signing/signing_with_containers/#signature-location-and-management), while [Notary Project](https://github.com/notaryproject/specifications/blob/main/specs/signature-specification.md#oci-signatures) represents similar information using the OCI "Referrers" mechanism to create a two-way relationship between signature and target artifact.

Since the path forward on signing is currently not clear, we will defer signature verification to a later release. This is consistent with the lack of support for signing in the [Provider Network Mirror Protocol](https://opentofu.org/docs/internals/provider-network-mirror-protocol/), and is justified by mirrors always being explicitly configured by the OpenTofu operator and thus assumed to be trusted by the operator to provide correct provider packages.

Although our first release will not offer any specific solution for provider packages being signed directly by their authors, it will be possible for those who are populating OCI registries to act as provider mirrors or module package sources to generate their _own_ signatures and publish them as additional artifacts that the registry would consider as "referrers" of the main artifact. Such signatures could, for example, represent that a particular person or team within an organization has inspected and approved a dependency for use within that organization, and a separate tool outside of OpenTofu could then scan the repository and draw attention to any artifacts that are not signed in this way. We don't intend to provide any specific tools for this use-case, because it's in support of a process used _alongside_ OpenTofu rather than _within_ OpenTofu, but the ability to represent post-hoc signatures from inside an organization is a general OCI Registry capability that organizations will now be able to benefit from for OpenTofu alongside other OCI registry users, using artifact-format-agnostic tools.

> [!NOTE]
> Although we do not intend to implement any specific design for publishing and verifying provider artifact signatures in the first release, we _have_ tried some prototypes of different strategies to develop confidence that we will be able to successfully retrofit a suitable design later.
>
> The question of which entity serves as "verifier" for the provider signing keys is a policy question that remains to be answered, but if we assume that there will be _some_ reasonable answer to that question then a validly-signed OCI artifact manifest provides equivalent information to the metadata OpenTofu currently obtains from its Provider Registry Protocol: the public key used to sign, and a signature that covers the SHA256 checksums of the provider's official `.zip` packages for a particular version across all platforms.
>
> Therefore a future implementation of verification will be able to integrate into OpenTofu's existing model of provider signature verification with only minimal changes to the protocol-agnostic infrastructure, with most of the work being performed in the OCI-specific "provider source" implementation.

---

| [« Previous](2-survey-results.md) | [Up](../20241206-oci-registries.md) | [Next »](4-providers.md) |

---
