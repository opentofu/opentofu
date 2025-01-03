# Dependency Packages from OCI Registries

Issue: https://github.com/opentofu/opentofu/issues/308

This RFC discusses the possibility of fetching both module and provider packages from a OCI registries (commonly referred to as "Docker" registries or container registries) in addition to using OpenTofu's own registry protocols.

Using the OCI registry protocol carries significant benefits for several groups of OpenTofu users as well as OpenTofu module and provider authors:

1. Users with an air-gapped setup often have strict compliance requirements. In such an environment, mirroring important dependencies becomes tricky as every software component typically needs to be approved and audited separately.

    Anecdotally we know that users in this situation with _providers_ typically rely on the [filesystem mirror](https://opentofu.org/docs/cli/commands/providers/mirror/) rather than running a [network mirror server](https://opentofu.org/docs/internals/provider-network-mirror-protocol/). Adding support for an OCI registry, which organizations with a Kubernetes/OpenShift/Rancher/etc setup typically already have, would enable organizations with heightened compliance requirements to host provider binaries (mirrors or new ones) themselves without any additional paperwork needing to be done.

    OpenTofu currently offers [various other options](https://opentofu.org/docs/language/modules/sources/) for module installation, so the benefits of OCI registry support for this cohort is not so significant, but allowing use of a single (probably-already-deployed) registry for _all_ external dependencies leads to a simpler infrastructure deployment overall.
2. Several OCI registries have features that the OpenTofu registry does not, such as [CVE and license scanning](https://goharbor.io/docs/2.12.0/administration/vulnerability-scanning/) and robust user interfaces.

    Instead of developing these tools ourselves, implementing an OCI Distribution client would automatically grant users the benefits of these tools and would alleviate us having to implement them for OpenTofu, as long as we choose a storage layout that is sufficiently similar to that used by other software.
3. It simplifies the packaging processes for provider developers.

    Given the right layout, provider authors can now simply use their typical `docker build`/`podman build` and `push` commands to build an entire provider instead of having to wrangle the byzantine goreleaser configuration, which is causing some problems every few months for new provider authors. (We did attempt to provide some guidance for provider authors as part of the [OpenTofu Registry Search project](https://search.opentofu.org/docs/providers) on this matter.)
4. It decentralizes the ecosystem of providers.

    Although it is technically possible for any third-party to run an OpenTofu-compatible module and/or provider registry today, OpenTofu uses its own specialized protocols for these and so in practice the OpenTofu project runs the only significantly-used implementation of these protocols at `registry.opentofu.org`.

    OCI registry support would make it possible for a provider author to release their own binaries with services that are already available in abundance for container image distribution, instead of having to create tooling to run a separate registry. In addition to resolving waiting time issues, a decentralized ecosystem would increase community trust as the OpenTofu project staff will no longer be an effective gatekeeper of registry content.

This document is part of a family of proposals for various integration points with OCI Registries in OpenTofu. This document focused on the cross-cutting problem of what metadata, manifest and layer content structure OpenTofu will expect for packages distributed through OCI registries, which is a general problem across both module and provider packages.

The following related RFCs build on _this_ RFC with the specific details of module and provider package support respectively:

- (**TODO:** Link to "Module Package Installation from OCI Registries" once it exists)
- [Provider Package Installation from OCI Registries](20241206-provider-install-from-oci-registry.md)

This document is also a sibling of another cross-cutting proposal: [OCI Distribution Registry Authentication Configuration](20241206-oci-registry-auth-config.md).

## A primer on the OCI protocol

OCI registries provide an HTTP interface to access *manifests* and *blobs*. Manifests describe the content in the registry, while blobs are binary data. The specification for the protocol is outlined in the [OCI Distribution Specification](https://github.com/opencontainers/distribution-spec/blob/main/spec.md) The content stored in the OCI registry must follow the [OCI Image Format Specification](https://github.com/opencontainers/image-spec/blob/v1.0.1/spec.md).

It's worth noting that many registry implementations, regarding ghcr.io, don't follow the OCI image format and instead return [Docker Image Manifest](https://distribution.github.io/distribution/spec/manifest-v2-2/) documents. However, the differences are very minor and this document will focus on the OCI specifications. During implementation these differences must be addressed.

> [!NOTE]
> There is some flexibility in how the data is stored, which also gave rise to [ORAS](https://oras.land/) (OCI Registry as Storage). However, ORAS artifacts require specialized client tools to work with.

### Authentication

Although registries don't necessarily need authentication, many public registries, such as `ghcr.io` and the Docker Hub require an anonymous token even to access public images. When accessing an endpoint, a client may receive a `WWW-Authenticate` header, indicating that authentication must be performed.

For example, accessing `https://ghcr.io/v2/opentofu/opentofu/tags/list` will return the following header:

```
www-authenticate: Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="repository:opentofu/opentofu:pull"
```

It is worth noting that accessing the base endpoint of `/v2/` will not yield a valid scope on `ghcr.io` and should not be used for authentication. The `realm` field in the `WWW-Authenticate` indicates the endpoint to use for authentication. We can perform the authentication by performing a simple get request:

```
curl -u user:password 'https://ghcr.io/token?service=ghcr.io&scope=repository:opentofu/opentofu:pull'
```

The username/password part is optional for public registries and the response will contain a token we can use for authentication:

```
{"token":"djE6b3Blb..."}
```

> [!TIP]
> Try it yourself:
> ```
> curl -v 'https://ghcr.io/token?service=ghcr.io&scope=repository:opentofu/opentofu:pull'
> ```

### Index vs. image manifest

Manifests can have two types. An index (media type of `application/vnd.oci.image.index.v1+json` or `application/vnd.docker.distribution.manifest.list.v2+json`) contains a list of image manifests. This is useful when you want to distribute your image for multiple architectures (so-called multi-arch images).

> [!TIP]
> Try it yourself: authenticate with a token as described above, then use the following command:
> ```
> curl -H "Authorization: Bearer djE6b3Blb..." https://ghcr.io/v2/opentofu/opentofu/manifests/1.8.0
> ```

Image manifests (media type of `application/vnd.oci.image.manifest.v1+json` or `application/vnd.docker.distribution.manifest.v2+json`) contain a list layers, each one referencing a blob. These layers are `.tar.gz` files containing the files in the image. The additional metadata is accessible through a separate blob referenced in the `config` section of the manifest. 

> [!TIP]
> Try it yourself: authenticate with a token as described above, then use the following command:
> ```
> curl -H "Authorization: Bearer djE6b3Blb..." https://ghcr.io/v2/opentofu/opentofu/manifests/1.8.0-amd64
> ```

### API calls in the "Pull" category

The distribution specification outlines that registries must implement all endpoints in the "Pull" category. This category consists of two endpoints:

- The manifest endpoint is located at `/v2/<name>/manifests/<reference>` and serves manifest documents. These manifests are JSON documents which have a specific content type (e.g. `application/vnd.oci.image.manifest.v1+json`) and the registry is allowed to perform content negotiation based on the `Accept` header the client sends.
- The blob endpoint is located at `/v2/<name>/blobs/<digest>`, containing binary objects based on their digest (checksum). Note that this endpoint may return an HTTP redirect.

> [!TIP]
> Try it yourself: authenticate with a token as described above, then use the following command:
> ```
> curl -H "Authorization: Bearer djE6b3Blb..." https://ghcr.io/v2/opentofu/opentofu/manifests/1.8.0
> ```

> [!NOTE]
> The `<name>` part may contain additional `/` characters and must match the regular expression of `[a-z0-9]+((\.|_|__|-+)[a-z0-9]+)*(\/[a-z0-9]+((\.|_|__|-+)[a-z0-9]+)*)*`. This means that a name can consist of an arbitrary amount of path parts, up to the length limit of 255 characters for hostname + name. It is worth noting that many registry implementations place additional restrictions on the name, such as needing to include a project ID, group, namespace, etc. and they may disallow additional path parts beyond that. Therefore, we will have to map the provider addresses to the name in a flexible fashion, configurable for the user.

> [!NOTE]
> A `<reference>` can either be a digest or a tag name. They must follow the regular expression of `[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}`.

> [!NOTE]
> An OCI `<digest>` can take the form of `scheme:value` and use any checksum algorithm. However, the specification states that `sha256` and `sha512` are standardized and compliant registries should support `sha256`.

### API calls in the "Content discovery" category

In addition to the "pull" category outlined above, registries may (but do not necessarily have to) implement the API endpoints in the "Content discovery" category. This category is useful when listing provider versions and consists of two endpoints:

- The tag listing endpoint is located at `/v2/<name>/tags/list` and supports additional filtering and pagination parameters. It lists all the tags (a kind of reference) that have manifests.
- The referrer listing endpoint is located at `/v2/<name>/referrers/<digest>` and returns the list of manifests that refer to a specific blob.

> [!TIP]
> Try it yourself: authenticate with a token as described above, then use the following command:
> ```
> curl -H "Authorization: Bearer djE6b3Blb..." https://ghcr.io/v2/opentofu/opentofu/tags/list
> ```

### The `_catalog` extension

Although not standardized in the distribution spec, the `/v2/_catalog` endpoint is traditionally used in the [Docker Registry](https://docker-docs.uclv.cu/registry/spec/api/#listing-repositories) as a means to list all images in a registry. However, this endpoint is typically disabled in public registries or only available after authentication.

## Conventions for OpenTofu Packages in OCI Registries

OpenTofu will follow some common conventions across both module and provider packages, so that similar tools and processes and be used for both kinds of artifact. If we introduce any other kind of external dependency artifact in future it should ideally follow similar conventions.

The module-package-specific and provider-package-specific RFCs both describe ways for a user to specify a source address that can be translated into an OCI Distribution repository address. OpenTofu will then interact with that repository assuming the conventions described below. Some parts of the following description include choses to be made by either the module-package-specific or provider-package-specific RFC as appropriate.

### Package versions as tags

OpenTofu uses [Semantic Versioning 2.0.0](https://semver.org/) notation to describe package versions. For provider packages such version numbers are mandatory. For module packages they are used only when a module address is being resolved through a module registry.

Whenever OpenTofu is required to enumerate a set of available versions from an OCI Distribution repository, it will retrieve all of the _tags_ currently available in the repository. It will attempt to parse each tag name as a version number using semantic versioning notation, and silently ignore any tag names for which parsing fails.

The set of version numbers found by _successfully_ parsing tag names is the set of available versions.

Interpreting package versions as tags is only needed when using an OCI repository only indirectly through language features designed for OpenTofu registries. An explicitly-OCI-specific mechanism, such as a new [module source address](https://opentofu.org/docs/language/modules/sources/) syntax dedicated to OCI repository addresses, should accept arbitrary tag names and match them exactly without requiring that they conform to any syntax beyond what is required by the OCI Distribution specification itself.

### Image manifest structure

OpenTofu expects any selected tag to refer to either a multi-platform _index_ manifest (for provider packages) or a single _image_ manifest (for module packages), as described in [Index vs. Image Manifest](#index-vs-image-manifest) above.

Any OpenTofu-specific image manifest **must** include a label named `org.opentofu.package-type`, whose value represents a specific package type as defined in one of the downstream RFCs. OpenTofu will then use this label (or the absense thereof) to return helpful error messages:

- If the label is not present at all, OpenTofu reports generically that this image is "not intended for OpenTofu".
- If the label is present but has the wrong value, OpenTofu may offer a more specific error message. For example, if the label suggests that the image is intended to be a module when OpenTofu was trying to install a provider, OpenTofu might report "this package is an OpenTofu module, not a provider".

### Filesystem layer contents

Each leaf image manifest refers to a series of "layers", each of which is a "tar" stream. OpenTofu extracts each layer in turn into the same directory. The final directory contents after this process represent the content of the package.

The root directory of the filesystem represented by all of the layers is the root of the OpenTofu package. For example, a provider package requires the provider plugin executable to be in its root directory, and so that executable must be directly at the root of one of the filesystem layers for OpenTofu to accept the result as valid provider package content. (Refer to the module-specific and provider-specific RFCs for more detail on what contents are expected for each package type.)

This structure means that it's possible to use a typical `Dockerfile`-based workflow to build a valid image by starting from the empty "scratch" base image and adding each file that should be included in the package:

```dockerfile
FROM scratch
LABEL org.opentofu.package-type="modules"
ADD *.tf
ADD *.tofu
```

```shell
docker build -t ghcr.io/example/opentofu-something:1.0.0 .
docker push ghcr.io/example/opentofu-something:1.0.0
```

Refer to the module-package-specific and provider-package-specific RFCs for more details on what filesystem layout is expected for each value of `org.opentofu.package-type`.

## Potential Alternatives

### ORAS Manifest Conventions

[OCI Registry As Storate](https://oras.land/), or _ORAS_, is an effort to define a more general set of conventions for distributing various different kinds of artifacts through OCI registries. At the time of writing, the ORAS documentation provides the following motivation:

> For a long time (pretty much since the beginning), people have been using/abusing OCI Registries to store non-container things. For example, you could upload a video to Docker Hub by just stuffing the video file into a layer in a Docker image (don't do this).
>
> The [OCI Artifacts](https://github.com/opencontainers/artifacts) project is an attempt to define an opinionated way to leverage OCI Registries for arbitrary artifacts without masquerading them as container images.
>
> Specifically, [OCI Image Manifests](https://github.com/opencontainers/image-spec/blob/main/manifest.md) have a required field known as `config.mediaType`. According to the [guidelines](https://github.com/opencontainers/artifacts/blob/main/artifact-authors.md) provided by OCI Artifacts, this field provides the ability to differentiate between various types of artifacts.
>
> Artifacts stored in an OCI Registry using this method are referred to herein as **OCI Artifacts**.

The ORAS project aims to provide generalized tools for working with arbitrary _OCI Artifacts_, rather than "masquerading them as container images".

This RFC is effectively proposing to "use/abuse" OCI Registries in just the way that the ORAS motivation bemoans.

We have proposed to use a container-image-shaped structure because in practice that is an effective way to distribute arbitrary filesystem trees using the registry software already deployed, built with existing container-image-focused tools that are well understood by at least some members of our community. We've chosen to use an OpenTofu-specific label, `org.opentofu.package-type`, to distinguish OpenTofu-specific images from typical container images or other image types.

We could potentially follow the OCI Artifacts guidelines by replacing our `org.opentofu.package-type` label with some new `artifactType` value, such as `application/vnd.opentofu.provider-pkg.v1+json` and `application/vnd.opentofu.module-pkg.v1+json`. OpenTofu could otherwise follow the existing index manifest and image manifest formats largely unchanged, as long as all of the registries that our users want to use are willing to store image manifests with the additional `artifactType` field.

Unfortunately, the current ORAS CLI commands are at a somewhat lower level of abstraction than tools like `docker build`, requiring some somewhat-arcane knowledge of specific manifest formats. There does not appear to be any direct analog to `docker build` that can take a high-level description of a filesystem layout and some metadata and automatically construct suitable image and index manifests describing that content.

It appears then that if we were to use the OCI Artifacts conventions we would need to offer our own OpenTofu-format-aware tools for building module and provider package artifacts. We would prefer to make use of the ecosystem's existing build tools both due to the opportunity cost of maintaining our own and due to the familiarity of tools like `docker build` among many in our community.

We do note that this means that it will be possible for someone to accidentally attempt to use an OpenTofu module or provider package as a container image in a container runtime, which will of course not work correctly because our package structure is not consistent with the typical expectations of a Linux userspace. Perhaps in future we could encourage (but not require) those creating images for OpenTofu to use a manifest structure that Docker (and similar) would reject as invalid but OpenTofu would accept, but we expect that authors would follow those recommendations only if the intended result were easy to achieve with commonly-used image build tools.
