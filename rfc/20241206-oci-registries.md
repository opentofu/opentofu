# OpenTofu Providers and Modules from OCI Registries

Issue: https://github.com/opentofu/opentofu/issues/308

## Background

OCI registries (also historically known as Docker registries) form the backbone of the container ecosystem. They present an easy way to publish self-contained miniature operating system userland images that users can run without installing additional libraries or tools beyond a container engine. The main feature of an OCI registry is its ability to publish layers containing differential changes, making container image updates very efficient.

However, thanks to the generic architecture of OCI registries, several implementations have popped up that allow users to store arbitrary data in OCI registries beyond container images. For example, [OCI Registry As Storage (ORAS)](https://oras.land/) is such a project.

The OCI registry standardization and the implementation of the OpenTofu/Terraform Registry protocols (the latter used by OpenTofu) happened around the same time (see [here for providers](https://opentofu.org/docs/internals/provider-registry-protocol/) and [here for modules](https://opentofu.org/docs/internals/module-registry-protocol/)). These protocols followed different design goals. For example, the provider registry protocol concerns itself with artifact signing and decoupling the index and the download part of the registry, whereas OCI is concerned, for example, with layer pull efficiency. To this day OCI image signing is still in development and only a partially solved problem, with projects like [sigstore/cosign](https://www.sigstore.dev/) attempting to take a stab the issue.

> [!NOTE]
> We have created a [primer on OCI](20241206-oci-registries/1-oci-primer.md) for this RFC. If you are unfamiliar with the protocol, you may want to read it before reading this RFC.

## Why OCI?

Many users of OpenTofu work in large organizations, partially in air-gapped environments. Given the popularity of [Kubernetes](https://kubernetes.io/) in larger organizations, OCI registries are widely available without any additional compliance burden. Cloud providers also offer a range of options, and public container registries such as the Docker Hub of ghcr.io are also at users' disposal.

In contrast, running an OpenTofu / Terraform registry requires the setup of an [extra piece of software](https://awesome-opentofu.com/#registry), which incurs additional costs when run publicly and an additional compliance burden when run in an organization.

In addition to the simplicity of using existing OCI registry deployments, many registries, such as [Harbor](https://goharbor.io/), offer built-in security, license and compliance scanning of container images out of the box with tools such as [Trivy](https://trivy.dev/) or [Clair](https://clairproject.org/). These security scanners automatically scan container image contents as well as the Go binaries contained in them for possible known vulnerabilities and report them. Furthermore, many of these tools are able to creates [Software Bill of Material (SBOM)](https://www.nist.gov/itl/executive-order-14028-improving-nations-cybersecurity/software-security-supply-chains-software-1) artifacts, which is now required by law in many cases. These are features that OpenTofu/Terraform registries do not have today.

## Parts of this RFC

In order to make this RFC easier to read, we have split it into several parts:

1. [A primer on OCI](20241206-oci-registries/1-oci-primer.md)
2. [Survey results](20241206-oci-registries/2-survey-results.md)
3. [Design considerations](20241206-oci-registries/3-design-considerations.md)
4. [Changes to the existing registry](20241206-oci-registries/4-registry-changes.md)
5. [Providers in OCI](20241206-oci-registries/5-providers.md)
6. [Modules in OCI](20241206-oci-registries/6-modules.md)
7. [Authentication](20241206-oci-registries/7-authentication.md)
8. [Open questions](20241206-oci-registries/8-open-questions.md)

### Appendices

The following additional sections discuss potential implementation details of the features described in the earlier chapters. These are primarily for core team reference purposes, but potentially interesting to others too.

9. [Authentication-related implementation details](20241206-oci-registries/9-auth-implementation-details.md)
10. [Provider installation implementation details](20241206-oci-registries/10-provider-implementation-details.md)
11. [Module installation implementation details](20241206-oci-registries/11-module-implementation-details.md)

## Potential alternatives

OCI resolves a very real pain-point for enterprise users wanting to run a private registry. A potential alternative would be, of course, making the use of a private registry easier, or creating a tool that can maintain a private registry purely based on static files. On that note, we could also implement running the OpenTofu Registry on the same dataset privately, which has been [documented in this issue](https://github.com/opentofu/registry/issues/1518).

These solutions would also work towards the goal of making the ecosystem fully decentralized.

That being said, neither of these solutions are as convenient as OCI since the infrastructure for this protocol is ubiquitous and cheap. 

## Future plans