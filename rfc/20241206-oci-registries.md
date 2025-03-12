# OpenTofu Providers and Modules from OCI Registries

Issue: https://github.com/opentofu/opentofu/issues/308

## Background

OCI registries (also historically known as Docker registries) form the backbone of the container ecosystem. They present an easy way to publish self-contained miniature operating system userland images that users can run without installing additional libraries or tools beyond a container engine. The main feature of an OCI registry is its ability to publish layers containing differential changes, making container image updates very efficient.

However, thanks to the generic architecture of OCI registries, several implementations have popped up that allow users to store arbitrary data in OCI registries beyond container images. For example, [OCI Registry As Storage (ORAS)](https://oras.land/) is such a project.

The OCI registry standardization and the design of OpenTofu's [Provider Registry Protocol](https://opentofu.org/docs/internals/provider-registry-protocol/) and [Module Registry Protocol](https://opentofu.org/docs/internals/module-registry-protocol/) happened around the same time. These protocols followed different design goals. For example, OpenTofu's provider registry protocol concerns itself with artifact signing and decoupling the index and the download part of the registry, whereas OCI Distribution has other priorities such as layer pull efficiency. At the time of writing this RFC OCI artifact signing is still in development and only a partially solved problem, with projects like [sigstore and cosign](https://www.sigstore.dev/) aiming to address the issue.

> [!NOTE]
> We have created a [primer on OCI](20241206-oci-registries/1-oci-primer.md) for this RFC. If you are unfamiliar with the protocol, you may want to read it before reading this RFC.

## Why OCI?

Many users of OpenTofu work in large organizations, sometimes with air-gapped environments. Given the popularity of [Kubernetes](https://kubernetes.io/) in larger organizations, OCI registries are widely available without any additional compliance burden. Cloud providers also offer a range of options, and public container registries such as DockerHub or GitHub Container Registry are also available.

In contrast, running an OpenTofu / Terraform registry requires the setup of [an extra piece of software](https://awesome-opentofu.com/#registry), which incurs additional costs when run publicly and an additional compliance burden when run in an organization.

In addition to the convenience of using existing OCI registry deployments, many registries such as [Harbor](https://goharbor.io/) offer built-in security, license and compliance scanning of container images out of the box, with tools such as [Trivy](https://trivy.dev/) or [Clair](https://clairproject.org/). These security scanners automatically scan container image contents as well as the Go binaries contained in them for possible known vulnerabilities and report them. Furthermore, many of these tools are able to create [Software Bill of Material (SBOM)](https://www.nist.gov/itl/executive-order-14028-improving-nations-cybersecurity/software-security-supply-chains-software-1) artifacts, which is now required by law in many cases. These are features that OpenTofu/Terraform registries do not have today.

## Parts of this RFC

In order to make this RFC easier to read, we have split it into several parts:

1. [A primer on OCI](20241206-oci-registries/1-oci-primer.md)
2. [Survey results](20241206-oci-registries/2-survey-results.md)
3. [Design considerations](20241206-oci-registries/3-design-considerations.md)
4. [Providers in OCI](20241206-oci-registries/4-providers.md)
5. [Modules in OCI](20241206-oci-registries/5-modules.md)
6. [Authentication](20241206-oci-registries/6-authentication.md)
7. [Open questions](20241206-oci-registries/7-open-questions.md)

### Appendices

The following additional sections discuss potential implementation details of the features described in the earlier chapters. These are primarily for core team reference purposes, but potentially interesting to others too.

8. [Authentication-related implementation details](20241206-oci-registries/8-auth-implementation-details.md)
9. [Provider installation implementation details](20241206-oci-registries/9-provider-implementation-details.md)
10. [Module installation implementation details](20241206-oci-registries/10-module-implementation-details.md)

## Potential alternatives

### Better tooling for OpenTofu's existing registry protocols

OCI Registry support resolves a very real pain-point for enterprise users wanting to run a private registry. A potential alternative would be making the use of a private registry easier, or creating a tool that can maintain a private registry purely based on static files. On that note, we could also implement [running the OpenTofu Registry on the same dataset privately](https://github.com/opentofu/registry/issues/1518).

These solutions would also work towards the goal of making the ecosystem fully decentralized.

That being said, neither of these solutions are as convenient as OCI since the infrastructure for this protocol is ubiquitous and cheap.

### OCI Registry proxy using the Provider Network Protocol

It would be possible in principle to write a proxy that acts as a server for the existing [provider network mirror protocol](https://opentofu.org/docs/internals/provider-network-mirror-protocol/) and as a client for the OCI Distribution protocol, thereby performing much the same translation between OpenTofu's provider installer model and OCI but in a separate codebase outside of OpenTofu.

However, this would not meet the goal of supporting OCI Distribution without running any additional server software, and does not have a direct equivalent for module packages since OpenTofu has no "mirror protocol" for those today.

It would also be more challenging to extend our initial implementation to support artifact signing later, because a provider network mirror cannot provide the verified signing keys for a particular provider and, even if it could, the signature structure for OCI artifacts is not the same as the signature structure for today's provider protocol and so it would not be possible to translate an OCI artifact signature into the form expected by today's provider registry protocol without access to the private key of the provider developer.

## Future plans

When it comes to providers, the current RFC mainly addresses OCI as an operator-configured mirror of providers whose origin is an OpenTofu provider registry. This is largely because we do not yet have an artifact signing solution equivalent to the one used with OpenTofu's protocol registry protocol, and so our initial implementation relies on operator configuration as a way to declare a particular OCI registry as trusted to provide correct packages. In a future OpenTofu version we would like to address this shortcoming and enable everyone to use OCI Distribution as a new kind of provider registry without relying on configuration options in the OpenTofu CLI configuration.

Additionally, currently the OCI registry implementation doesn't have the equivalent of the [OpenTofu Registry Search](https://search.opentofu.org/). We expect this will be resolved similar to how Linux packages often publish a supplemental `doc` package, containing the related documentation. This, however, will need tooling in OpenTofu to render and show that documentation. While much of the code can be reused from the [Search source code](https://github.com/opentofu/registry-ui), that project was not written with reuse in mind and will need some additional work to adapt as a general-purpose documentation viewer.
