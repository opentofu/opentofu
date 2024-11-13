# Provider OCI registry support (stage 1)

Issue: https://github.com/opentofu/opentofu/issues/308

This RFC discusses the possibility of fetching providers from an OCI registry (commonly referred to as a "Docker" registry or container registry) instead of and in addition to using the OpenTofu Provider Registry protocol.

Using the OCI registry protocol carries significant benefits for several groups of OpenTofu users as well as OpenTofu provider authors:

1. Users with an air-gapped setup often have strict compliance requirements. In such an environment, mirroring important providers becomes tricky as every software component typically needs to be approved and audited separately. Anecdotally we know that users in this situation typically rely on the [filesystem mirror](https://opentofu.org/docs/cli/commands/providers/mirror/) rather than running a [network mirror server](https://opentofu.org/docs/internals/provider-network-mirror-protocol/). Adding support for an OCI registry, which organizations with a Kubernetes/OpenShift/Rancher/etc. setup typically already have, would enable organizations with heightened compliance requirements to host provider binaries (mirrors or new ones) themselves without any additional paperwork needing to be done.
2. Several OCI registries have features that the OpenTofu registry does not, such as [CVE and license scanning](https://goharbor.io/docs/2.12.0/administration/vulnerability-scanning/) and robust user interfaces. Instead of developing these tools ourselves, implementing OCI registries would automatically grant users the benefits of these tools and would alleviate us having to implement them for OpenTofu. (This is somewhat contingent on the storage layout we chose.)
3. It simplifies the build processes for providers. Given the right layout, provider authors can now simply use their typical `docker build`/`podman build` and `push` commands to build an entire provider instead of having to wrangle the byzantine goreleaser configuration, which is causing some problems every few months for new provider authors. (We did attempt to provide some guidance for provider authors as part of the [OpenTofu Registry Search project](https://search.opentofu.org/docs/providers) on this matter.)
4. It decentralizes the ecosystem of providers, which is currently very much dependent on OpenTofu and HashiCorp as gatekeepers of the default registries for OpenTofu and Terraform, respectively. For example, currently provider authors are dependent on us to remove a version when a version needs to be revoked or re-released, which can take up to several days over holidays. This can negatively affect their users in urgent cases. An OCI registry would make it possible for a provider author to release their own binaries with the tools that are already in abundance instead of having to create tooling to run a separate registry and set up a domain name. In addition to resolving waiting time issues, a decentralized ecosystem will increase community trust as the OpenTofu team will no longer be the gatekeeper of registry content.

---

## Proposed Solution

With the community goals outlined above, there are two main design goals for this RFC:

1. Provide a viable alternative to the HTTP Network Mirror Protocol by means of an OCI registry. This would serve primarily as a way to mirror public providers (e.g. from the OpenTofu registry) into an air-gapped environment or an in-house distribution system. Users would have to set up a mirror configuration in their project in order for this to work.
2. Provide the ability for users to install providers from public or private OCI registries without additional configuration in their project. While this configuration can also serve as a way to mirror providers, it would make OCI registries first-class citizens in the OpenTofu world and enable users to directly use addresses such as `ghcr.io/opentofu/terraform-provider-aws` in their configuration.

We propose that provider authors or mirror operators should be able to package up existing provider binaries into container images and push them to an OCI registry. OpenTofu should be able to fetch the images from an OCI registry and use it to extract the provider binaries from it, similar to how it extracts the ZIP files from GitHub today.

We discuss more details on a potential configuration format in the [User Documentation](#user-documentation) section.

---

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

---

## User Documentation

### For provider authors / mirror operators

There are two possible approaches for distributing providers.

<details>
<summary>

#### 📂 Standard container tooling approach

</summary>

As a provider author, you may want to package up your provider binary into an OCI image. This involves the following steps:

1. Take all build artifacts and add them to a container image.
2. Upload the container image to an OCI registry.

For example, assuming you have your binary built and wanted to package it up into a container, you could use the following `Dockerfile`:

```Dockerfile
FROM scratch

ADD terraform-provider-aws
ADD README.md
ADD LICENSE
ADD CHANGELOG.md
```

You could now run `docker build -t ghcr.io/opentofu/terraform-provider-aws:1.2.3-amd64 .` followed by `docker push ghcr.io/opentofu/terraform-provider-aws:1.2.3`.

For more advanced use cases, you could enable container image building in your [goreleaser config](https://goreleaser.com/customization/docker/) to enable building multi-arch images.
</details>

<details>
<summary>

#### 📂 ORAS approach

</summary>

As a provider author, you may want to package up your provider binary into an OCI image. You can do this by using the [ORAS command line tool](https://oras.land/docs/installation) using the following commands:

```
oras login -u username -p password ghcr.io
oras push ghcr.io/opentofu/terraform-provider-aws:1.2.3 ./dist:application/octet-stream+tar
```

🛑🛑🛑 TODO: how do you create multi-arch with ORAS? 🛑🛑🛑

</details>

### Mirror configuration

If you would like to use an OCI registry as your OpenTofu mirror, you will be able to do so by specifying a `provider_installation` block with an `oci_mirror` section as follows:

```hcl
provider_installation {
  oci_mirror {
    url = "ghcr.io/{{ .Namespace }}/terraform-provider-{{ .Name }}"
  }
}
```

Note that the `url` field is a Go template to resolve the container image address containing the provider binary. You can use `{{ .Namespace }}` to add the provider namespace (e.g. `hashicorp`) and `{{ .Name }}` to add the provider name (e.g. `aws`) to the container image address.

> [!NOTE]
> OpenTofu will **not** start a container from this image, but instead use the container image as a way to extract the provider binary. Also note that providers are not automatically available in any particular container registry. This feature is intended for advanced use cases.

### The first-class configuration

If you would like to use a provider hosted in an OCI registry, you may do so by invoking a well-known public registry in your `required_providers` block.

```hcl
terraform {
  required_providers {
    aws = {
      source  = "ghcr.io/opentofu/terraform-provider-aws"
      version = "~> 1.0"
    }
  }
}
```

OpenTofu automatically identifies the following registries as OCI registries:

- `docker.io`
- `ghcr.io`
- `containers.pkg.github.com`
- `quay.io`
- `gcr.io`
- `public.ecr.aws`

🛑🛑🛑 TODO: How does a user specify a custom registry? Do we perform some sort of service discovery? Do we distribute a list of known registries? 🛑🛑🛑

## Technical Approach

TBD

## Open Questions

- Do we use ORAS or standard OCI image layouts?
- Do we perform some namespace/name translation to OCI addresses?
- How does a user specify a custom registry?
- Do we perform some sort of service discovery?
- Do we distribute a list of known registries?

TBD

## Future Considerations

TBD

## Potential Alternatives

A number of solutions have been proposed as alternatives, such as being able to download providers directly from GitHub. However, OCI represents a standardized, cross-platform way to obtain provider artifacts and enjoys wide community support.