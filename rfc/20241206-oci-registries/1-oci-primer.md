# 1. A primer on the OCI protocol

---

This document is part of the [OCI registries RFC](../20241206-oci-registries.md).

| [« Previous](../20241206-oci-registries.md) | [Up](../20241206-oci-registries.md) | [Next »](2-survey-results.md) |

---

OCI registries provide an HTTP interface to access *manifests* and *blobs*. Manifests describe the content in the registry, while blobs are binary data. The specification for the protocol is outlined in the [OCI Distribution Specification](https://github.com/opencontainers/distribution-spec/blob/main/spec.md) The content stored in the OCI registry must follow the [OCI Image Format Specification](https://github.com/opencontainers/image-spec/blob/v1.0.1/spec.md).

It's worth noting that many registry implementations, regarding ghcr.io, don't follow the OCI image format and instead return [Docker Image Manifest](https://distribution.github.io/distribution/spec/manifest-v2-2/) documents. However, the differences are very minor and this document will focus on the OCI specifications. During implementation these differences must be addressed.

> [!TIP]
> There is some flexibility in how the data is stored, which also gave rise to [ORAS](https://oras.land/) (OCI Registry as Storage). For details, see the [ORAS section below](#oras).

## Authentication

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

## API calls in the "Pull" category

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
> A `<reference>` can either be a digest or a tag name. Tag names must follow the regular expression of `[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}`.

> [!NOTE]
> An OCI `<digest>` can take the form of `scheme:value` and use any checksum algorithm. However, the specification states that `sha256` and `sha512` are standardized and compliant registries should support `sha256`.

## API calls in the "Content discovery" category

In addition to the "pull" category outlined above, registries may (but do not necessarily have to) implement the API endpoints in the "Content discovery" category. This category is useful when listing provider versions and consists of two endpoints:

- The tag listing endpoint is located at `/v2/<name>/tags/list` and supports additional filtering and pagination parameters. It lists all the tags (a kind of reference) that have manifests.
- The referrer listing endpoint is located at `/v2/<name>/referrers/<digest>` and returns the list of manifests that refer to a specific blob.

> [!TIP]
> Try it yourself: authenticate with a token as described above, then use the following command:
> ```
> curl -H "Authorization: Bearer djE6b3Blb..." https://ghcr.io/v2/opentofu/opentofu/tags/list
> ```

## The `_catalog` extension

Although not standardized in the distribution spec, the `/v2/_catalog` endpoint is traditionally used in the [Docker Registry](https://docker-docs.uclv.cu/registry/spec/api/#listing-repositories) as a means to list all images in a registry. However, this endpoint is typically disabled in public registries or only available after authentication.

## ORAS

Everything above refers to the standard container image layout. However, [ORAS](https://oras.land/) describes how artifacts can be stored in a non-standard layout. ORAS today has wide-ranging support.

ORAS "abuses" the OCI system to store artifacts of an arbitrary type instead of storing it as a differential tar layer.

```json
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "artifactType": "archive/zip+opentofu-provider",
  "config": {
    "mediaType": "application/vnd.oci.empty.v1+json",
    "digest": "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a",
    "size": 2,
    "data": "e30="
  },
  "layers": [
    {
      "mediaType": "archive/zip+opentofu-provider",
      "digest": "sha256:54b0178fd0fcbd60ce806b2569974694af59faaf0b2c734f703753f1fdfb1f21",
      "size": 146839280,
      "annotations": {
        "org.opencontainers.image.title": "terraform-provider-aws_5.84.0_linux_amd64.zip"
      }
    }
  ],
  "annotations": {
    "org.opencontainers.image.created": "2025-02-03T11:47:34Z"
  }
}
```

> [!TIP]
> Try it yourself: install ORAS and a local registry, download a provider and then run:
> ```
> oras push \
>     --artifact-type application/vnd.opentofu.provider \ 
>     localhost:5000/oras:latest \
>     terraform-provider-aws_5.84.0_linux_amd64.zip:archive/zip+opentofu-provider
> ```
> You can now list the manifest:
> ```
> oras manifest fetch localhost:5000/oras:latest --pretty
> ```

When pushing multiple files with ORAS, each file is stored in a separate layer:

```json
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "artifactType": "application/vnd.opentofu.provider",
  "config": {
    "mediaType": "application/vnd.oci.empty.v1+json",
    "digest": "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a",
    "size": 2,
    "data": "e30="
  },
  "layers": [
    {
      "mediaType": "archive/zip+opentofu-provider",
      "digest": "sha256:54b0178fd0fcbd60ce806b2569974694af59faaf0b2c734f703753f1fdfb1f21",
      "size": 146839280,
      "annotations": {
        "org.opencontainers.image.title": "terraform-provider-aws_5.84.0_linux_amd64.zip"
      }
    },
    {
      "mediaType": "archive/zip+opentofu-provider",
      "digest": "sha256:aa50fb3769355eeddfec7614bae674d0841c3b0b771e5183ac2db4dfc04b9423",
      "size": 132401007,
      "annotations": {
        "org.opencontainers.image.title": "terraform-provider-aws_5.84.0_linux_arm64.zip"
      }
    }
  ],
  "annotations": {
    "org.opencontainers.image.created": "2025-02-03T12:00:46Z"
  }
}
```

ORAS can also customize the config manifest using the `--config` option. This will result in the following manifest:

```json
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "artifactType": "application/vnd.opentofu.provider",
  "config": {
    "mediaType": "text/plain+SHA256SUMS",
    "digest": "sha256:2bc757edf7a4532ebe70d994963dd51532fd9907e27a95ffd57763b5795170e0",
    "size": 1122
  },
  "layers": []
}
```

It is worth noting that trying to pull an ORAS image with traditional containerization software will result in unexpected errors [as documented here](https://oras.land/docs/how_to_guides/manifest_config#docker-behaviors).

> [!NOTE]
> At the time of writing, [ORAS does not support multi-arch images](https://github.com/oras-project/oras/issues/1053).

---

This document is part of the [OCI registries RFC](../20241206-oci-registries.md).

| [« Previous](../20241206-oci-registries.md) | [Up](../20241206-oci-registries.md) | [Next »](2-survey-results.md) |

---
