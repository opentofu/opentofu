# 2. Design considerations for OCI in OpenTofu

---

This document is part of the [OCI registries RFC](../20241206-oci-registries.md).

| [« Previous](2-survey-results.md) | [Up](../20241206-oci-registries.md) | [Next »](4-registry-changes.md) |

---

## Provider addresses are virtual

Currently, providers in OpenTofu are addressed by a virtual address in the format of `HOSTNAME/NAMESPACE/TYPE`, where `HOSTNAME` defaults to `registry.opentofu.org` and `NAMESPACE` defaults to `hashicorp` for historic reasons.

However, this address is independent of the actual download URL, which can be anywhere. When OpenTofu needs to download a provider (unless configured with a mirror), it consults the [Remote Service Discovery endpoint](https://opentofu.org/docs/internals/remote-service-discovery/) on the provided hostname and then uses the [Provider Registry Protocol](https://opentofu.org/docs/internals/provider-registry-protocol/) to obtain the correct download URL.

It is worth noting that, in contrast to modules, provider addresses have no protocol part, such as `oci://`.

In our survey, 64% of respondents have indicated interest in using publicly available providers without using the registry. In parallel, 13% of respondents have indicated that they would be interested in publishing a provider in a public OCI registry.

In parallel, a similar number of respondents have indicated that they would like to mirror a provider or create a provider for their internal organization.

Changing the provider address understanding would result in numerous changes in the OpenTofu and other codebases, and affect third party tools rely on this understanding. It would also affect the state file format, as well as the JSON output.

Alternatively, we could implement adding a set of well-known registries, such as `ghcr.io`, to a list of host names that should be treated as OCI registries rather than OpenTofu registries. This, however, opens up several questions, for example:

1. How do we convert a virtual provider address into an OCI address? Some OCI registries need various prefixes, etc.
2. What do we do if a host exposes both an OpenTofu and an OCI registry? Which one do we prefer?
3. How does such a change affect third party tools?

Since adding support for public providers via OCI would be a profound change, we opt to proceed with caution. We will implement using OCI for providers in a private setting first. In other words, we will treat OCI for providers the same way as the Provider Mirror Protocol and you will need to add [a CLI configuration option](https://opentofu.org/docs/cli/config/config-file/#provider-installation) to your `.tofurc` file, which we explore in detail in the [providers section of this RFC](providers.md).

## OCI layout

The most important consideration for us was the layout of the data in the OCI registry. As described [in the primer](1-oci-primer.md), OCI registries are very flexible how they store data.

Originally, we considered that we could store both providers and modules in a traditional container image layout. This would have allowed you to use your existing Docker/Podman tooling to publish their images by creating a `Dockerfile`/`Containerfile` with the following contents:

```Dockerfile
FROM scratch
ADD * /
```

However, there are several key reasons for deciding against this layout:

1. **It breaks provider checksums**<br />OpenTofu today records the checksums of all providers it sees in the `.terraform.lock.hcl` file. These checksums can have two formats: the `h1` (Go-specific directory hash) and the `zh` (ZIP hash) format. While the former would be suitable for hashing container images, hashes today are almost universally the of the latter format. Computing a SHA256 hash over a ZIP file is much simpler than extracting its contents and using a [Go-specific hashing format over the files](https://pkg.go.dev/golang.org/x/mod/sumdb/dirhash). Provider authors publish their checksums in the SHA256SUMS file when releasing providers and sign the checksums file with their GPG key. Fortunately for us, OCI registries also use SHA256 checksums as blob identifiers, so storing the ZIP file in a blob in OCI will guarantee that the checksum doesn't change even when switching from the OpenTofu Registry to a mirrored OCI registry. In contrast, a container image-like layout would mean you have to run `tofu providers lock` to update the checksums in your `.terraform.lock.hcl` and your lock file would now be exclusive to your OCI mirror.
2. **Supporting layers adds complexity**<br />OpenTofu today contains over 300.000 lines of code. Supporting the diff-tar layer format adds complexity to the codebase and increases the resource consumption of the download. Simply downloading a ZIP-blob allows us to reuse much of the code already in place in OpenTofu today.

Therefore, we have decided to use a layout that does not indicate a container image and follows the [ORAS](https://oras.land) conventions with added multi-arch support. See the [Providers](5-providers.md) and [Modules](6-modules.md) documents for details on the respective artifact layouts.

> [!NOTE]
> In contrast to providers, modules currently have a protocol option. This allows us to integrate them safely without breaking existing tooling. However, to indicate possible OCI layout changes in the future, we will use the protocol prefix of `oci+zip://`.

## Software Bill of Materials (SBOM)

The decision to use a custom image layout as described above profoundly impacts security scanners, which a [large number of our users want to use](2-survey-results.md). We have tested several of the popular security scanners with varying degrees of success.

The popular scanners supporting container images out of the box, such as [Trivy](https://trivy.dev/), only support the default container image-like layout and do not support custom artifact types, such as the one uploaded with [ORAS](https://oras.land/). Other popular tools, such as TFLint, do not support container images at all and need to be configured manually.

However, we have also discovered that many of these tools [already support various SBOM formats](https://trivy.dev/latest/docs/target/sbom/). This gives us the option to let security scanners work on SBOM files instead of scanning a container image and the binary contained therein.

We, therefore, intend to start supporting the publication of SBOM artifacts in the SPDX (JSON) and CycloneDX (JSON and XML) format in the existing OpenTofu Registry by the provider author, and we intend the mirroring tool for OCI registries to mirror these artifacts correctly. Since this file will be included in the `SHA256SUMS` file, the contents of this file will also be signed with the provider GPG key.

Additionally, we intend to provide a hook in the tool that mirrors or builds OpenTofu provider and module OCI artifacts that allows for the creation of an SBOM artifact, which the tool will then publish in the OCI registry.

Since modules are more flexible in their publication mechanism than providers, for modules we will consider files included in the root folder of module distributions.

For both purposes, OpenTofu will consider SBOM-specific file names in the provider release or root folder of the module as an SBOM attestation. However, OpenTofu will not validate the correctness of these artifacts and will simply accept them as such.

> [!NOTE]
> Using SBOMs instead of scanning the contents offers the same features for most use cases. Security scanners for providers rely on the Go-specific symbols in the provider binary for their CVE and license reporting, which are easy to fake. Only virus scanners that analyze the binary itself would be able to detect malicious binaries.

## Artifact signing

While not a question in the survey, several respondents have taken the time to express that artifact signing is an important consideration to them.

Today, OpenTofu providers are signed with GPG keys and [there is an open issue about supporting Sigstore/Cosign](https://github.com/opentofu/opentofu/issues/307), which is blocked on the availability of a stable Go library to do so.

Modules are currently not signed in OpenTofu, which is a separate question to address. This consideration will, therefore, only address providers.

For providers, the OpenTofu Registry, an authority independent of the provider author, serves the role of verifier. If the OpenTofu Registry has verified a GPG key as belonging to an author, OpenTofu accepts it as valid. The same is true for third party registries: the registry holds the public GPG keys and provides them for verification, which is independent of the provider download URL. A malicious upload to the download URL would result in an invalid download unless the attacker can obtain the GPG key.

With OCI, however, there is no such central authority. We could implement publishing the SHA256SUMS file and its signature as part of the manifest, but this would not provide any additional benefit as OpenTofu would have to accept any GPG signature as valid. Alternatively, users would have to set up a list of valid GPG public keys, adding the burden of key management to the user.

Alternatively, we could also support Sigstore/Cosign for providers as well, but this is blocked on the availability of a stable Go library and is also [tricky to run in an air-gapped environment](https://blog.sigstore.dev/sigstore-bring-your-own-stuf-with-tuf-40febfd2badd/), which is something that over 30% of respondents indicated running.

One of the main goals of supporting OCI is to ease the maintenance burden, not add to it. This is also something that many respondents indicated in their responses when we asked about the reasons for wanting OCI. Running a Sigstore infrastructure or performing manual key management is contrary to this goal.

Since the path forward on signing is currently not clear, we will defer signing to a later release. This is in line with the lack of support for signing in the [Provider Network Mirror Protocol](https://opentofu.org/docs/internals/provider-network-mirror-protocol/).

## Authentication

---

This document is part of the [OCI registries RFC](../20241206-oci-registries.md).

| [« Previous](2-survey-results.md) | [Up](../20241206-oci-registries.md) | [Next »](4-registry-changes.md) |

---
