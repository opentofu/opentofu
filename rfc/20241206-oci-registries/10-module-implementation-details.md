# Module installation implementation details

---

This document is part of the [OCI registries RFC](../20241206-oci-registries.md).

| [« Previous](9-provider-implementation-details.md) | [Up](../20241206-oci-registries.md) |

---

This appendix discusses implementation details related to [installing module packages from OCI registries](5-modules.md).

## OCI Distribution "Getter" for go-getter

OpenTofu currently delegates most of the work of fetching and extracting remote module packages to a third-party upstream library called [go-getter](https://pkg.go.dev/github.com/hashicorp/go-getter). Aside from the special cases of local paths and registry addresses that are handled internally by OpenTofu, all of [OpenTofu's documented module sources](https://opentofu.org/docs/language/modules/sources/) are implemented as some combination of go-getter features.

There are a number of different concepts in go-getter, but the two most relevant to the question of additional module sources are:

- [`Detector`](https://pkg.go.dev/github.com/hashicorp/go-getter#Detector): essentially a preprocessor that takes a raw source address and returns another raw source address, prior to URL parsing.

    This is the mechanism we currently use to implement what we document as the "GitHub" and "Bitbucket" source types: they are really just "detectors" that rewrite certain shorthand address patterns into a fully-qualified source address for the generic "Git" getter. For example, a source address like `github.com/example/example` gets rewritten by the GitHub "detector" into `git::https://github.com/example/example.git`.

    [Detectors can "stack"](https://github.com/opentofu/opentofu/blob/d2ae0b21ede3dddb92914d3c61b5caa3c7f77db0/internal/getmodules/getter.go#L37-L59) -- the output from an earlier detector is fed as the input into a later detector -- but whichever detector runs last for a particular input _must_ produce something in go-getter's "URL-like" syntax, described in the next item. `Detectors` are therefore most commonly used for translating non-URL-like address schemes into URL-like address schemes.

- [`Getter`](https://pkg.go.dev/github.com/hashicorp/go-getter#Getter) does the main work of fetching and extracting a module package. The detector phase must produce a special extended URL syntax which either explicitly specifies or implies a "getter" from [OpenTofu's table of Getters](https://github.com/opentofu/opentofu/blob/d2ae0b21ede3dddb92914d3c61b5caa3c7f77db0/internal/getmodules/getter.go#L79-L87).

    The explicit syntax involves an extra go-getter-specific "scheme" at the front of the address, like the `git::` prefix in the example in the previous point, but if there is no double-colon scheme prefix then go-getter assumes that the URL scheme is also the Getter name, and so e.g. `https://example.com/` would be treated as belonging to the "https" getter.

    After go-getter has decided which Getter to use, it trims off any explicit getter-selection prefix and then parses the remainder of the address using standard URL syntax (as implemented by the Go standard library `net/url` package) and passes it to the `Getter.Get` method, along with the filesystem path to the local directory where the package should be extracted.

Our proposed user experience for modules in OCI registries involves using a new URL scheme `oci:`, without any explicit `Getter` selection prefix. To implement that, we must add a new entry `"oci"` to OpenTofu's table of getters. Our proposed address scheme intentionally follows normal URL syntax otherwise, and so we do not need any special `Detector` for this new source address type.

Upstream go-getter does not currently have a `Getter` for the OCI Distribution protocol. From discussion in pull requests in that repository, it does not seem like the upstream is generally accepting entirely new `Getter` implementations. In particular, [a HashiCorp representative commented](https://github.com/hashicorp/go-getter/pull/517#issuecomment-2666366150) that "It can be hard to get changes through in `go-getter`", in response to a proposal to add a new `Getter` for Azure blob storage.

Therefore we will implement our new `Getter` as an OpenTofu-specific implementation inside `package getmodules` to start. At some later date we could potentially try to upstream it, but that is not a priority for this project and so we do not wish to make that a blocker.

As described in [OCI Client for the Module Package Installer](8-auth-implementation-details.md#oci-client-for-the-module-package-installer), this `Getter` will be written using a dependency-inversion style where it recieves a glue function from `package main` that integrates it with our cross-cutting OCI repository authentication design:

```go
func NewOCIGetter(
    getRegistryClient func(ctx context.Context, registryDomainName, repositoryPath string) (*orasregistry.Registry, error),
) getter.Getter
```

Our initial implementation of this getter will therefore rely on the ORAS-Go library's OCI Distribution client, which provides us with the necessary building-blocks for finding and fetching a module package:

- Resolve a tag into a manifest digest, if the address doesn't specify a digest explicitly using the `digest` argument.
- Fetch a manifest given its digest, which will then in turn tell us the digest of the `.zip` package blob.
- Fetch a blob given its digest, which our `Getter` will use to retrieve the `.zip` file before extracting it into the target directory.

Go-getter also has a further concept [`Decompressor`](https://pkg.go.dev/github.com/hashicorp/go-getter#Decompressor) which in principle allows the use of any one of [a selection of different archive formats](https://github.com/opentofu/opentofu/blob/d2ae0b21ede3dddb92914d3c61b5caa3c7f77db0/internal/getmodules/getter.go#L63-L77) for the final package payload. In practice this is only really used for the HTTP (or "HTTP-like") getters in OpenTofu, to implement [Fetching archives over HTTP](https://opentofu.org/docs/language/modules/sources/#fetching-archives-over-http). Further, the "decompressor" concept is implemented as something independent of any specific getter to be activated as part of the source address syntax, and so it's not a suitable abstraction for situations like OCI Distribution where our getter wants to detect from the manifest which archive format is in use, rather than that being controlled by an independent query string argument.

We wish to keep our OCI image layouts relatively constrained to start, and so we specified that a module package distributed via the OCI Distribution protocol _must_ be a `.zip` archive, and therefore its descriptor must have `mediaType` set to `archive/zip`. To implement zip extraction we'll write our `OCIGetter` to _directly_ instantiate the upstream [`getter.ZipDecompressor`](https://pkg.go.dev/github.com/hashicorp/go-getter#ZipDecompressor) and call it inline as an implementation detail of the final step of extracting the `.zip` archive blob into the target directory. Reusing go-getter's "decompressor" will ensure that our treatment of the `.zip` package will be exactly equivalent to how today's OpenTofu would treat a `.zip` package retrieved using the HTTP getter (etc), which includes some safety features like detecting and rejecting file paths that seem to traverse up out of the target package directory.

> [!NOTE]
> The above signature for `NewOCIGetter` is intended to be illustrative rather than concrete. In practice it may end up taking a single argument that is an implementation of some interface with a method like the function signature shown, but that's a level of design detail we will consider during the implementation phase, with consideration to whether it turns out to be shareable with the similar dependency inversion design we'll be using for the provider installer.

> [!NOTE]
> We could potentially choose to support other blob media types in future versions if we learn of a good reason to do so. If we decide to do that, the most likely design is for `OCIGetter` to have its own table mapping from `mediaType` value to `getter.Decompressor`, along with a priority rule for choosing one blob to use when there are multiple of supported formats.
>
> Our existing table of "decompressors" is keyed by filename suffix rather than by media type -- i.e. `.zip` rather than `archive/zip` -- and so it is not suitable to reuse for matching descriptors in an OCI manifest.
>
> However, this proposal does not intend to constrain those future design details at all. Our initial implementation will ignore any blobs that are not `archive/zip` and will fail with a suitable error message if none use that media type. The hypothetical future project that adds a second supported media type can then propose answers to the relevant questions working within those constraints.

## Package Checksums and Signing

Today's OpenTofu has no general-purpose mechanism for verifying module package checksums, fetching and checking signatures, or "remembering" checksums in the dependency lock file. Although we have heard requests for all of those mechanisms, they are explicitly not in scope for this RFC and should instead be tackled in a cross-cutting way (across all supported source types) in a separate later proposal.

A typical compromise made with today's OpenTofu is to use the "git" getter with a `ref` argument set to a specific Git commit, which therefore provides at least a SHA-1 checksum to verify the resulting content against. SHA-1 is no longer considered cryptographically secure, but the Git community is gradually moving toward using SHA-256, which OpenTofu will automatically benefit from once it becomes more widely deployed.

The OCI Repository source address syntax offers the `digest` argument which provides a similar mechanism: it specifies a checksum of the manifest of the package to be fetched, and the manifest in turn contains a checksum of the final `.zip` blob, and so a source address using that argument (rather than `tag`) can guarantee to succeed installation only if the retrieved manifest actually matches the digest.

> [!NOTE]
> Notwithstanding the earlier remark about treating signature verification as a general concern across all source types, it _is_ potentially possible that a future version of OpenTofu could use [the OCI Distribution referrers API](https://github.com/opencontainers/distribution-spec/blob/main/spec.md#listing-referrers) to automatically discover cryptographic signatures associated with a manifest in an OCI-registry-specific way. However, that is explicitly not in scope for this initial project, and if we decide to pursue it in a later project we should carefully consider the tradeoffs of solving this in an OCI-specific way vs. in a manner that is applicable to other OpenTofu module source types.
>
> Either way, it will require some extensions to our overall module installer API to include some way for the installer to report the outcome of signature verification work so that `tofu init` can display the signing key information in a similar way as we currently do for providers installed from a provider registry. This initial project will not include any changes to the overall module installer API, instead focusing only on providing a new implementation of the existing API.

---

| [« Previous](9-provider-implementation-details.md) | [Up](../20241206-oci-registries.md) |

---
