# A primer on the OCI protocol

---

This document is part of the [OCI registries RFC](../20241206-oci-registries.md).

| [« Previous](../20241206-oci-registries.md) | [Up](../20241206-oci-registries.md) | [Next »](2-survey-results.md) |

---

OCI registries provide an HTTP interface to access *manifests* and *blobs*. Manifests are metadata describing the content in the registry, while blobs are the actual data. The specification for the protocol is outlined in the [OCI Distribution Specification](https://github.com/opencontainers/distribution-spec/blob/main/spec.md). The content stored in the OCI registry must follow the [OCI Image Format Specification](https://github.com/opencontainers/image-spec/blob/v1.0.1/spec.md).

It's worth noting that many registry implementations don't yet follow the OCI image format and instead return [Docker Image Manifest](https://distribution.github.io/distribution/spec/manifest-v2-2/) documents. However, the differences are very minor and this document will focus on the OCI specifications. During implementation these differences must be addressed.

> [!TIP]
> There is some flexibility in how the data is stored, which also gave rise to [ORAS](https://oras.land/) (OCI Registry as Storage). For details, see the [ORAS section below](#oras).

> [!WARNING]
> The examples in this document are meant to illustrate the plain OCI Distribution protocol only. They are not part of the proposal for how we intend to capture OpenTofu-specific artifacts (providers or modules) in OCI registries.

## Authentication

Although registries don't necessarily need authentication, many public registries (including `ghcr.io` and Docker Hub) require an "anonymous" token even to access public images. When accessing an endpoint, the registry server might send a `WWW-Authenticate` header field, indicating that authentication is needed.

For example, accessing `https://ghcr.io/v2/opentofu/opentofu/tags/list` will return the following header field:

```
www-authenticate: Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="repository:opentofu/opentofu:pull"
```

It is worth noting that accessing the base endpoint of `/v2/` will not yield a valid scope on `ghcr.io` and should not be used for authentication. The `realm` field in the `WWW-Authenticate` indicates the endpoint to use for authentication, and so we can perform the authentication by performing a `GET` request to exchange (optional) credentials for a temporary bearer token:

```shell
curl -u user:password 'https://ghcr.io/token?service=ghcr.io&scope=repository:opentofu/opentofu:pull'
```

If successful, the registry server's response will contain a temporary bearer token we can use for subsequent requests:

```json
{"token":"djE6b3Blb..."}
```

> [!TIP]
> Try it yourself:
> ```
> curl -v 'https://ghcr.io/token?service=ghcr.io&scope=repository:opentofu/opentofu:pull'
> ```

### Index vs. image manifests

Manifests can be of two different types. An index manifest (media type `application/vnd.oci.image.index.v1+json` or `application/vnd.docker.distribution.manifest.list.v2+json`) contains a list of image manifests and platform-selection information associated with each one. This is needed when distributing separate artifacts for each target platform.

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

Image manifests (media type `application/vnd.oci.image.manifest.v1+json` or `application/vnd.docker.distribution.manifest.v2+json`) contain a list of "layers", each of which refers to a blob. In traditional container images, the layers are `.tar.gz` archives representing the files in the root filesystem. Some additional metadata is accessible through a separate blob specified in the `config` section of the manifest.

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

The distribution specification requires that registries must implement all endpoints in the "Pull" category. This category contains two endpoints:

- The manifest endpoint is located at `/v2/<name>/manifests/<reference>` and serves manifest documents. These manifests are JSON documents which have a specific content type (e.g. `application/vnd.oci.image.manifest.v1+json`) and the registry is allowed to perform content negotiation based on the `Accept` header the client sends.
- The blob endpoint is located at `/v2/<name>/blobs/<digest>`, containing binary objects based on their digest (checksum). Note that this endpoint may return an HTTP redirect.

The `<name>` part may contain additional `/` characters and must match the regular expression of `[a-z0-9]+((\.|_|__|-+)[a-z0-9]+)*(\/[a-z0-9]+((\.|_|__|-+)[a-z0-9]+)*)*`. This means that a name can consist of an arbitrary amount of path parts, up to the length limit of 255 characters for hostname + name. It is worth noting that many registry implementations place additional restrictions on the name, such as needing to include a project ID, group, namespace, etc. and they may disallow additional path parts beyond that.

A `<reference>` can either be a digest or a tag name. Tag names must follow the regular expression of `[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}`.

An OCI `<digest>` can take the form of `scheme:value` and use any checksum algorithm. However, the specification states that `sha256` and `sha512` are standardized and compliant registries should support `sha256`.

> [!NOTE]
> Unfortunately the term "reference" is overloaded with two meanings in the OCI ecosystem, and sometimes refers to the entire address of a tag/digest in a specific repository in a specific registry.
>
> For example, `latest` is a local reference specifying only a tag name but leaving the repository address implied, but `example.com/foo/bar/baz:latest` is a fully-qualified reference that refers to the `latest` tag in the `foo/bar/baz` repository on the `example.com` registry. In this chapter we will use the local meaning of "reference" as described above, but documentation for other software in this ecosystem sometimes uses the other meaning.

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

## API calls in the "Push" category

These API calls are similar to the pull category above but, as the name suggests, are intended for publishing manifests and blobs.

You can upload a new artifact in two ways:

1. First `POST` to the `/v2/<name>/blobs/uploads` endpoint, then `PUT` the blob contents to the URL indicated in the `Location` header from the first response.
2. Immediately `POST` the blob contents to `/v2/<name>/blobs/uploads/?digest=<digest>` indicating a pre-computed digest.

> [!TIP]
> The benefit of the first method is the ability to perform an upload in chunks using `PATCH` requests. Refer to [Pushing a Blob in Chunks](https://github.com/opencontainers/distribution-spec/blob/main/spec.md#pushing-a-blob-in-chunks) for more details.

Once the blobs have been uploaded, you can push the manifest(s) that refer to them. You can push a manifest by sending a `PUT` request to `/v2/<name>/manifests/<reference>`, where the reference should be the tag name the manifest should appear under.

## API calls in the "Content discovery" category

In addition to the "pull" category outlined above, registries may optionally implement the API endpoints in the "Content discovery" category. This category includes two additional endpoints:

- The tag listing endpoint is located at `/v2/<name>/tags/list` and supports additional filtering and pagination parameters. It lists all the tags in the selected repository that have associated manifests.

    This is useful, for example, to answer the question "which versions of this artifact are available?" in order to implement semantic-versioning-based matching or other similar non-exact selection techniques.
- The referrer listing endpoint is located at `/v2/<name>/referrers/<digest>` and returns the list of manifests that refer to a specific blob.

    A manifest can optionally include a `subject` property which refers to another manifest, effectively creating a tree of artifacts where the `subject` property indicates the parent of the current manifest. The referrer listing endpoint then describes the opposite relationship, returning a list of all of the child manifests that refer to the given parent manifest.

    Uses of this are still emerging in the ecosystem at the time of writing, but typically it's used for artifacts that serve as post-hoc attestations or other metadata about the parent, such as presenting a signature for the parent manifest that might have been generated by someone other than the parent artifact's author. However, it's notable that some signing mechanisms -- including Cosign -- expect signatures presented as specially-named tags rather than using this API.

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


## The `_catalog` extension

Although not standardized in the distribution spec, the `/v2/_catalog` endpoint is traditionally used in the [Docker Registry](https://docker-docs.uclv.cu/registry/spec/api/#listing-repositories) as a means to list all images in a registry. However, this endpoint is typically disabled in public registries or only available after authentication.

## ORAS

Everything above refers to the standard container image layout. However, [ORAS](https://oras.land/) describes how artifacts can be stored in a non-standard layout. ORAS today has wide-ranging support.

ORAS relies on the `mediaType` property of a layer descriptor to differentiate different kinds of layers, beyond the "differential-tar" format expected for container images. For example, it's possible to declare a layer of type `archive/zip` instead of `application/vnd.docker.image.rootfs.diff.tar.gzip`:

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

When pushing multiple files to a single tag with ORAS, each file is represented as a separate layer:

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

Traditional container engines were, of course, not built to expect these different layer and config formats and so [they may return "interesting" errors when encountering such manifests](https://oras.land/docs/how_to_guides/manifest_config#docker-behaviors).

It is worth noting that trying to pull an ORAS image with traditional containerization software may result in unexpected errors [as documented here](https://oras.land/docs/how_to_guides/manifest_config#docker-behaviors). ORAS defaults to using a fixed config blob representing an empty JSON object, `{}`, whose media type is `application/vnd.oci.empty.v1+json` and whose digest is always `sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a`. ORAS-aware software understands this convention and skips fetching the known-empty configuration blob altogether, but traditional container engines such as Docker will return errors such as "invalid rootfs in image configuration" if someone tries to use them with such a non-container-image artifact.

> [!NOTE]
> At the time of writing [ORAS does not support multi-platform images](https://github.com/oras-project/oras/issues/1053). However, it is possible to push externally-generated manifests directly using the `oras manifest` subcommand.

---

| [« Previous](../20241206-oci-registries.md) | [Up](../20241206-oci-registries.md) | [Next »](2-survey-results.md) |

---
