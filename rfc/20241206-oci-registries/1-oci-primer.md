# 1. A primer on the OCI protocol

---

This document is part of the [OCI registries RFC](../20241206-oci-registries.md).

| [« Previous](../20241206-oci-registries.md) | [Up](../20241206-oci-registries.md) | [Next »](2-survey-results.md) |

---

OCI registries provide an HTTP interface to access *manifests* and *blobs*. Manifests describe the content in the registry, while blobs are binary data. The specification for the protocol is outlined in the [OCI Distribution Specification](https://github.com/opencontainers/distribution-spec/blob/main/spec.md) The content stored in the OCI registry must follow the [OCI Image Format Specification](https://github.com/opencontainers/image-spec/blob/v1.0.1/spec.md).

It's worth noting that many registry implementations, regarding ghcr.io, don't follow the OCI image format and instead return [Docker Image Manifest](https://distribution.github.io/distribution/spec/manifest-v2-2/) documents. However, the differences are very minor and this document will focus on the OCI specifications. During implementation these differences must be addressed.

> [!TIP]
> There is some flexibility in how the data is stored, which also gave rise to [ORAS](https://oras.land/) (OCI Registry as Storage). For details, see the [ORAS section below](#oras).

> [!WARNING]
> The examples in this document are meant to showcase the protocol only, they are not indicative of how OpenTofu stores data in OCI!

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

Manifests can have two types. An index (media type of `application/vnd.oci.image.index.v1+json` or `application/vnd.docker.distribution.manifest.list.v2+json`) contains a list of image manifests. This is useful when you want to distribute your image for multiple architectures (so-called multi-platform images).

> [!TIP]
> Try it yourself: authenticate with a token as described above, then use the following command:
> ```
> curl -H "Authorization: Bearer djE6b3Blb..." https://ghcr.io/v2/opentofu/opentofu/manifests/1.8.0
> ```
> 
> <details><summary>Result</summary>
>
> ```json
> {
>    "schemaVersion": 2,
>    "mediaType": "application/vnd.docker.distribution.manifest.list.v2+json",
>    "manifests": [
>       {
>          "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
>          "size": 951,
>          "digest": "sha256:105eb6b43b0704093cd48644437934d3eb9200c297756fe4e4d5ed2fccada56c",
>          "platform": {
>             "architecture": "386",
>             "os": "linux"
>          }
>       },
>       {
>          "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
>          "size": 951,
>          "digest": "sha256:26b1e5ed87f80d3b2bb36769d90a448246bd1aa786f57bdc0b8907dc3d2b327f",
>          "platform": {
>             "architecture": "amd64",
>             "os": "linux"
>          }
>       },
>       {
>          "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
>          "size": 951,
>          "digest": "sha256:4ac194402663bf948d022a9b3f79641565fc90cdae476829638f2dfcd4583c77",
>          "platform": {
>             "architecture": "arm64",
>             "os": "linux"
>          }
>       },
>       {
>          "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
>          "size": 951,
>          "digest": "sha256:4ac9d6209f34675c869a63e41110b596e20a679a3c8451c9bd059bbb5a1ba564",
>          "platform": {
>             "architecture": "arm",
>             "os": "linux",
>             "variant": "v7"
>          }
>       }
>    ]
> }
> ```
> </details>

Image manifests (media type of `application/vnd.oci.image.manifest.v1+json` or `application/vnd.docker.distribution.manifest.v2+json`) contain a list layers, each one referencing a blob. These layers are `.tar.gz` files containing the files in the image. The additional metadata is accessible through a separate blob referenced in the `config` section of the manifest.

> [!TIP]
> Try it yourself: authenticate with a token as described above, then use the following command:
> ```
> curl -H "Authorization: Bearer djE6b3Blb..." https://ghcr.io/v2/opentofu/opentofu/manifests/1.8.0-amd64
> ```
> 
> <details><summary>Result</summary>
>
> ```json
> {
>    "schemaVersion": 2,
>    "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
>    "config": {
>       "mediaType": "application/vnd.docker.container.image.v1+json",
>       "size": 2061,
>       "digest": "sha256:f160637911afad6485d75b398c7c62b032f5040e641aff097e3035bcacf697de"
>    },
>    "layers": [
>       {
>          "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
>          "size": 3415640,
>          "digest": "sha256:930bdd4d222e2e63c22bd9e88d29b3c5ddd3d8a9d8fb93cf8324f4e7b9577cfb"
>       },
>       {
>          "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
>          "size": 8158438,
>          "digest": "sha256:27a55bad853afb2cf5f203297db2e5f132a1f9afffce02a20e59284fed62ab4a"
>       },
>       {
>          "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
>          "size": 25145807,
>          "digest": "sha256:22b79cf4f0efedf6423a10f5200cde934aaa8d6c7ece5a45939e87bb5af12e22"
>       }
>    ]
> }
> ```
>
> </details>

## API calls in the "Pull" category

The distribution specification outlines that registries must implement all endpoints in the "Pull" category. This category consists of two endpoints:

- The manifest endpoint is located at `/v2/<name>/manifests/<reference>` and serves manifest documents. These manifests are JSON documents which have a specific content type (e.g. `application/vnd.oci.image.manifest.v1+json`) and the registry is allowed to perform content negotiation based on the `Accept` header the client sends.
- The blob endpoint is located at `/v2/<name>/blobs/<digest>`, containing binary objects based on their digest (checksum). Note that this endpoint may return an HTTP redirect.

> [!TIP]
> Try it yourself: authenticate with a token as described above, then pull a blob from the manifest in the previous example. Observe the `location` header returned in the response, pointing to the actual download location of the blob.
> ```
> curl -v -H "Authorization: Bearer djE6b3Blb..." https://ghcr.io/v2/opentofu/opentofu/blobs/sha256:22b79cf4f0efedf6423a10f5200cde934aaa8d6c7ece5a45939e87bb5af12e22
> ```
> 
> <details><summary>Result</summary>
>
> ```
> > GET /v2/opentofu/opentofu/blobs/sha256:22b79cf4f0efedf6423a10f5200cde934aaa8d6c7ece5a45939e87bb5af12e22 HTTP/2
> > Host: ghcr.io
> > User-Agent: curl/8.5.0
> > Accept: */*
> > Authorization: Bearer djE...
> >
> 
> < HTTP/2 307
> < content-length: 0
> < content-type: application/octet-stream
> < docker-distribution-api-version: registry/2.0
> < location: https://pkg-containers.githubusercontent.com/ghcr1/blobs/sha256:22b79cf4f0efedf6423a10f5200cde934aaa8d6c7ece5a45939e87bb5af12e22?se=2025-02-04T09%3A50%3A00Z&sig=FGZhe9e3oOMEbhebzW49Stfj9J1Cuy77Go77Ob7w8ro%3D&sp=r&spr=https&sr=b&sv=2019-12-12
> < date: Tue, 04 Feb 2025 09:40:56 GMT
> < x-github-request-id: D334:1069C0:704BB:71F3D:67A1E0A8
> <
> ```
> Note: We omitted the TLS output from this example for readability.
> 
> </details>

> [!NOTE]
> The `<name>` part may contain additional `/` characters and must match the regular expression of `[a-z0-9]+((\.|_|__|-+)[a-z0-9]+)*(\/[a-z0-9]+((\.|_|__|-+)[a-z0-9]+)*)*`. This means that a name can consist of an arbitrary amount of path parts, up to the length limit of 255 characters for hostname + name. It is worth noting that many registry implementations place additional restrictions on the name, such as needing to include a project ID, group, namespace, etc. and they may disallow additional path parts beyond that. Therefore, we will have to map the provider addresses to the name in a flexible fashion, configurable for the user.

> [!NOTE]
> A `<reference>` can either be a digest or a tag name. Tag names must follow the regular expression of `[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}`.

> [!NOTE]
> An OCI `<digest>` can take the form of `scheme:value` and use any checksum algorithm. However, the specification states that `sha256` and `sha512` are standardized and compliant registries should support `sha256`.

## API calls in the "Push" category

These API calls are similar to the pull category above, but as the name suggest, are intended for publishing manifests and blobs.

You can do an upload in two ways:

1. First `POST` to the `/v2/<name>/blobs/uploads` endpoint, then `PUT` the blob contents to the URL indicated in the `Location` header from the first response.
2. Immediately `POST` the blob contents to `/v2/<name>/blobs/uploads/?digest=<digest>` indicating a pre-computed digest.

> [!TIP]
> The benefit of the first method is the ability to perform an upload in chunks using `PATCH` requests. See [the specification for details](https://github.com/opencontainers/distribution-spec/blob/main/spec.md#pushing-a-blob-in-chunks).

Once the blobs have been uploaded, you can push the manifest that references them. You can push a manifest by sending a `PUT` request to `/v2/<name>/manifests/<reference>`, where the reference should be the tag name the manifest should appear under.

It is worth noting that manifests can reference other manifests in their `subject` field. You can use this, for example, to sign a manifest and attach the signature to the manifest it signed. You can then use the referrers API described in the next section to query it.

## API calls in the "Content discovery" category

In addition to the "pull" category outlined above, registries may (but do not necessarily have to) implement the API endpoints in the "Content discovery" category. This category is useful when listing provider versions and consists of two endpoints:

- The tag listing endpoint is located at `/v2/<name>/tags/list` and supports additional filtering and pagination parameters. It lists all the tags (a kind of reference) that have manifests.
- The referrer listing endpoint is located at `/v2/<name>/referrers/<digest>` and returns the list of manifests that refer to a specific blob.

> [!WARNING]
> The tag listing endpoint is typically paginated. Client implementations must follow the `Link` header in the response to receive the entire list of tags.

> [!TIP]
> Try it yourself: authenticate with a token as described above, then use the following command:
> ```
> curl -v -H "Authorization: Bearer djE6b3Blb..." https://ghcr.io/v2/opentofu/opentofu/tags/list
> ```
> 
> <details><summary>Results</summary>
>
> ```
> > GET /v2/opentofu/opentofu/tags/list HTTP/2
> > Host: ghcr.io
> > User-Agent: curl/8.5.0
> > Accept: */*
> > Authorization: Bearer djE6b3B...
> >
> 
> < HTTP/2 200
> < content-type: application/json
> < docker-distribution-api-version: registry/2.0
> < link: </v2/opentofu/opentofu/tags/list?last=1.6.0-beta5-386&n=0>; rel="next"
> < date: Tue, 04 Feb 2025 09:47:19 GMT
> < x-github-request-id: D388:2C9949:B77F2:B977F:67A1E226
> <
> ```
> ```json
> {
>   "name": "opentofu/opentofu",
>   "tags": [
>     "1.6.0-alpha1-arm64",
>     "1.6.0-alpha1-amd64",
>     "1.6.0-alpha1-arm",
>     "1.6.0-alpha1-386",
>     "1.6.0-alpha1",
>     "1.6",
>     "1",
>     "latest",
>     "sha256-722da07b0cdf5b6bdf12aff9339f7c274f70552144a0a28c0d2b970c083ffa5c.sig",
>     "sha256-9b9662dbe859f81779d04ba0f8d2b415b09961d5e5435a25e2ba4566873058f0.sig",
>     "sha256-d1aa9cfa30744d52a486b1dd58b7a65dd9d1ae9b79fc9b6ee5d0528ef9cfd54c.sig",
>     "sha256-cd67ab05739a47503450c8d2da4abf89d5418f800d093f43a996de64a0d2fc33.sig",
>     "sha256-28f8c87d6583f570acf7dd33afa6b17888e7f15ae595d74e16cbd4c82921f0d0.sig",
>     "..."
>   ]
> }
> ```
> Note: We omitted the TLS output from this example for readability.
>
> </details>

This category also includes a way to create references between distinct manifests. As an example, you can attach a digital signature of a manifest to an existing manifest by publishing the signature under a separate manifest, but referring to the manifest it signs. You can do this by pushing the new manifest with a `subject` field that references the original manifest.

Clients can query the `/v2/<name>/referrers/<reference>` endpoint to receive a list of manifests that refer to the current manifest in their `subject` field.

> [!TIP]
> The referrers API has been added in the Distribution spec version 1.1. Prominently, Cosign does not appear to use this API to attach signatures. Instead, Cosign creates a tag named after the checksum of the main manifest and suffix it with `.sig`.

## The `_catalog` extension

Although not standardized in the distribution spec, the `/v2/_catalog` endpoint is traditionally used in the [Docker Registry](https://docker-docs.uclv.cu/registry/spec/api/#listing-repositories) as a means to list all images in a registry. However, this endpoint is typically disabled in public registries or only available after authentication.

## ORAS

Everything above refers to the standard container image layout. However, [ORAS](https://oras.land/) describes how artifacts can be stored in a non-standard layout. ORAS today has wide-ranging support.

ORAS uses a different media type to store the artifact in a layer:

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
      "mediaType": "archive/zip",
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
>     terraform-provider-aws_5.84.0_linux_amd64.zip:archive/zip
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
      "mediaType": "archive/zip",
      "digest": "sha256:54b0178fd0fcbd60ce806b2569974694af59faaf0b2c734f703753f1fdfb1f21",
      "size": 146839280,
      "annotations": {
        "org.opencontainers.image.title": "terraform-provider-aws_5.84.0_linux_amd64.zip"
      }
    },
    {
      "mediaType": "archive/zip",
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
> At the time of writing, [ORAS does not support multi-platform images](https://github.com/oras-project/oras/issues/1053).

---

| [« Previous](../20241206-oci-registries.md) | [Up](../20241206-oci-registries.md) | [Next »](2-survey-results.md) |

---
