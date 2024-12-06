# Provider Package Installation from OCI Registries

Issue: [#308](https://github.com/opentofu/opentofu/issues/308)

The associated issue represents support for using container image repositories in registries as defined by [The OpenContainers Distribution specification v1.0.0](https://specs.opencontainers.org/distribution-spec/?v=v1.0.0) as an alternative supported installation source location for OpenTofu module and provider packages.

Module and provider installation in OpenTofu have enough differences that we intend to treat most of the relevant details as separate RFCs for each case, and this particular document is focused on the provider-package-specific details. It relies on some other proposals that describe some details that are common across both provider and module package installation.

Today's OpenTofu supports provider installation by a number of different OpenTofu-specific strategies:

- Direct installation from a provider's origin registry: The default behavior is to attempt to use the hostname given in the provider's source address (which defaults to `registry.opentofu.org` if not specified) with [the OpenTofu Provider Registry Protocol](https://opentofu.org/docs/internals/provider-registry-protocol/).

    This makes a good default behavior because it requires no special configuration on the part of the user. The hostname in the provider source address gives us enough information to self-configure the registry client using [OpenTofu's remote service discovery protocol](https://opentofu.org/docs/internals/remote-service-discovery/).

    However, it's not sufficient for all cases because it assumes that the client machine running OpenTofu can make a direct network connection to a server running at whatever hostname the provider source address belongs to. This is sometimes either technically impossible (for air-gapped systems) or prohibited by local policy despite being technically possible.

    This is the only case where the hostname in the provider source address is used as a location to contact. In all other cases we use hostnames only to delegate the management of the top-level of our provider source address namespace to the domain name system.

- Installing from a mirror server configured by the user: users can use [a `provider_installation` block in their CLI configuration](https://opentofu.org/docs/cli/config/config-file/#explicit-installation-method-configuration) to tell OpenTofu to install some or all providers from a different server implementing [OpenTofu's Provider Mirror Protocol](https://opentofu.org/docs/internals/provider-network-mirror-protocol/).

    The key difference between the Registry protocol and the Mirror protocol is that the latter includes the hostname from the provider source address as part of its requests, and so a single mirror can host packages belonging to many different hostnames at once, whereas a registry can only host packages belonging to its own hostname.

- Treating a directory in the local filesystem as a mirror: users can use [a `provider_installation` block in their CLI configuration](https://opentofu.org/docs/cli/config/config-file/#explicit-installation-method-configuration) to tell OpenTofu to install some or all providers from a directory in their local filesystem, containing copies of provider packages following one of two documented layouts.

    This is similar to a network mirror but avoids the need to run an additional network service, as long as the mirrored packages are only needed on one system or they can be copied onto every system where they will be needed.

The introduction of OCI Distribution repositories as a new package source is intended to meet similar goals as for the "direct" and "network mirror" strategies, but to achieve those goals by reusing existing registry infrastructure already deployed for the distribution of Docker-style container images. This means that those who are already invested in the OCI ecosystem can make further use of services they have already purchased or deployed, and that provider developers can potentially use existing container image hosting infrastructure as the primary distribution vehicle for their own providers, instead of relying on the OpenTofu-maintained registry or on some other OpenTofu-specific registry service.

## Proposed Solution

This proposal includes three different variations of using OCI registry infrastructure for provider package distribution, which will be described in later sections.

What they all have in common is:
- Some means for translating an OpenTofu provider source address, such as `registry.opentofu.org/apparentlymart/assume`, into an OCI distribution repository address, which varies between the three methods.
- A convention for describing provider release versions as OCI distribution "tags" whose names follow a specified syntax.
- A convention for mapping OpenTofu platform identifiers like `linux_amd64` onto the common conventions for multi-platform container image manifests.
- A convention for the expected manifest and layer archive formats used to describe a single provider package, giving the filesystem contents needed to run a single version of a provider on a single platform.
- A way to provide OpenTofu with any credentials needed to interact with the registry that hosts the selected OCI repository.

Some of these concerns have considerable overlap with the separate goal of installing _module packages_ from OCI registries, and so this proposal builds on two upstream proposals that are shared between those two problems:

- [OpenTofu packages in OCI Registries](./20241206-packages-in-oci-registries.md): Describes the general conventions OpenTofu follows in interpreting the content of an OCI distribution repository as an OpenTofu-specific package format. This proposal specifies a specific application of those conventions to provider packages.
- [OCI Distribution Registry Authentication Configuration](./20241206-oci-registry-auth-config.md): Describes a cross-cutting approach to configuring OpenTofu to authenticate to OCI registries. This proposal relies on the preconfigured OCI Distribution client produced by that proposal, expecting that it will encapsulate all of the registry-authentication-related concerns, and so we will not discuss registry authentication any further in this document.

### Provider Packages as OCI distribution objects

Later sections will describe some different ways OpenTofu can translate provider source addresses into OCI distribution repository addresses, but a key goal of this proposal is for all of them to share a single convention for how that repository is used once selected, and that single convention is the focus of this section.

Provider packages specialize [our general OCI repository conventions](./20241206-packages-in-oci-registries.md) in the following ways:

- Each repository tag that is to be treated as a provider version must refer to a _multi-platform_ manifest, which describes in turn one child manifest for each target platform the provider supports where each describes a single container image.
- For the leaf manifests describing individual versions for individual platforms, the `org.opentofu.package-type` label **must** be set to `provider_binary`, so that OpenTofu can unambigously reject attempts to install generic container images or incompatible kinds of OpenTofu-specific packages with a clear error message.
- The union of all layers of each leaf image must represent a filesystem directory with the same content as an OpenTofu provider package distributed by other means. In particular, for a provider whose source address is `example.com/namespace/name` the resulting directory must contain an executable whose name begins with the prefix `terraform-provider-name`, which must be marked in the appropriate way to make it executable on the target platform. (e.g. executable permission on Unix-like platforms, or a `.exe` filename suffix for Windows)

    If the same provider release is also being distributed under the same source address via other installation methods then the set of file paths in the resulting directory and the content of each file must exactly match the content of the packages delivered by the other methods, because OpenTofu will expect them all to match the same package checksums recorded in the dependency lock file.

Those producing provider packages as container images may use any tools they wish as long as they produce manifests and layers consistent with OpenTofu's expectations.

One possible approach is to use Docker CLI's "buildx" plugin to build multiple container images with a surrounding multi-platform manifest. If the provider's build process already produces `./dist/PLATFORM` directories containing the content of each platform's release package then the following `Dockerfile` would cause `docker buildx` to gather them all beneath a multi-platform manifest:

```dockerfile
FROM scratch
LABEL org.opentofu.package-type="provider_binary"
ARG TARGETPLATFORM
ARG BUILDPLATFORM
ARG TARGETOS
ARG TARGETARCH
COPY ./dist/${TARGETOS}_${TARGETARCH}/* /
```

The following command would then build a selection of images and generate the multi-platform manifest for a specified set of platforms:

```shell
docker buildx build \
  --platform linux/amd64,darwin/amd64,windows/amd64,windows/arm64,linux/arm64,darwin/arm64 \
  --tag example.com/namespace/opentofu-provider-name:1.0.0
```

This would then build a multi-platform manifest suitable to be pushed to an OCI repository with the address `example.com/namespace/opentofu-provider-name`, under the tag `1.0.0`, using `docker push` as normal.

This particular strategy assumes that the developer already used Go's support for cross-compilation, or some other non-Docker-specific strategy, to build native executables for each of the specified platforms into the subdirectories of `dist`. Since the `Dockerfile` only uses `COPY`, it's reusable without any virtualization or emulation to build packages for any supported platform regardless of what platform the container runtime used for building is running on.

### "Magic" inference of an OCI repository address

The first of the three supported approaches for translating an OpenTofu provider source address into an OCI repository address is a "magic" mapping strategy based on a special reserved hostname suffix.

The OpenTofu project will acquire a short, mnemonic domain to reserve for this mechanism. This RFC does not yet propose a specific domain, so for now we'll use `oci.opentofu.org` as a placeholder. Although that name would technically work, it's likely too long for ergonomic use and so should be replaced in a later version of this RFC once we've already purchased the intended domain.

As a special case in the provider installation behavior, any provider belonging to a hostname that ends with the suffix `.oci.opentofu.org` will skip the normal service discovery process and will instead use a hard-coded internal mapping as follows:

1. Trim the `.oci.opentofu.org` suffix from the hostname to obtain the OCI registry domain name.
2. Use the "namespace" portion of the provider source address, followed by the fixed delimiter `/opentofu-provider-`, and then the "type" portion of the provider source address as the repository name under the selected registry domain name.

For example, the provider source address `example.com.oci.opentofu.org/foo/bar` would automatically resolve to the OCI repository address `example.com/foo/opentofu-provider-bar`.

This particular scheme is intended for anyone who wants to use a pre-existing OCI registry to host packages for a provider without needing to set up any special OpenTofu-specific additions aside from following the designated naming scheme for the registry name. The specified repository naming scheme intentionally uses only two segments while using a fixed prefix for maximum compatibility with the naming restrictions of commonly-available public registry implementations while minimizing conflicts with non-OpenTofu-provider-related uses of those registries.

### OCI Registry as an implementation detail of "direct" installation

The second of the three supported approaches for translating an OpenTofu provider source address into an OCI repository address uses [OpenTofu's existing service discovery mechanism](https://opentofu.org/docs/internals/remote-service-discovery/) to allow the deployment of something that behaves much like a normal OpenTofu provider registry, but uses the OCI distribution protocol instead of the OpenTofu-specific registry protocol _as a hidden implementation detail_. This compromise requires a small amount of OpenTofu-specific protocol implementation alongside a generic OCI registry, in return for hiding the implementation detail that an OCI registry is being used.

The owner of a hostname can deploy a service discovery document which announces the new service identifier `oci-providers.v1`, with the associated value being a template for constructing an OCI repository address from the "namespace" and "type" portions of the provider source address using [RFC 6570 URI Template](https://www.rfc-editor.org/rfc/rfc6570) (level 1) syntax, which in practice means that `{namespace}` and `{type}` get substituted for the values from the provider source address:

```json
{
    "oci-providers.v1": "example.com/opentofu-providers/{namespace}/{type}"
}
```

If the service discovery document for `example.net` included the definition shown above, then the provider source address `example.net/foo/bar` would be translated to the OCI Distribution repository address `example.com/opentofu-providers/foo/bar`.

This mechanism is essentially a more explicit generalization of the "magic" method from the previous section, allowing the owner of a hostname to customize exactly how providers belonging to that hostname are to be mapped onto OCI repository addresses.

Note that there is no requirement that the provider source address hostname match the OCI registry domain name: it's possible for someone to project any hostname they own onto repositories in an OCI Distribution registry run by someone else if they want to. For example, the owner of `example.org` could announce the following in its service discovery document:

```json
{
    "oci-providers.v1": "ghcr.io/example-org-otf-providers/{namespace}-{type}"
}
```

...and then they can rely on this third-party OCI registry implementation as an implementation detail, while using generic source addresses like `example.org/foo/bar` (mapping to `ghcr.io/example-org-otf-providers/foo-bar`) in all of their OpenTofu modules to reduce te lock-in to any specific OCI registry vendor.

### OCI Registry as a new kind of "mirror"

The final of the three supported approaches for translating an OpenTofu provider source address into an OCI repository address is a new third kind of "mirror" that a user can write into their local CLI configuration:

```hcl
provider_installation {
  oci_mirror {
    # repository_template is an HCL-style template that can substitute the
    # hostname, namespace, and type portions of the provider source address
    # to describe how to translate to an OCI repository address.
    repository_template = "example.com/opentofu-providers/${hostname}/${namespace}/${type}"
  }
}
```

Provider _mirrors_ are configured and controlled by the local user of OpenTofu rather than by the owner of the hostname in the source address, and so this mechanism is particularly useful when someone cannot (or must not) rely on external registry services for _any_ provider they need to use. Instead then, they would copy the packages for all of their desired providers into an OCI registry they control, using a methodical naming scheme that can be described as a template like the above, and OpenTofu would then use this mirror for all providers regardless of which hostname their source address belongs to.

In the above example, a module depending on `example.net/foo/bar` would cause OpenTofu to attempt installation from the OCI Distribution repository at `example.com/opentofu-providers/example.net/foo/bar`, bypassing any attempt to contact `example.net` over the network.

As with the other installation methods, it's possible to designate specific provider source addresses, entire namespaces, or entire hostnames as included or excluded from an `oci_mirror` block. This then allows more complicated rulesets where some providers are mirrored and others are not, or different subsets of providers are mirrored in different locations. For example:

```hcl
provider_installation {
  oci_mirror {
    repository_template = "example.com/examplenet-mirror/${namespace}-${type}"
    include             = ["example.net/*/*"]
  }
  oci_mirror {
    repository_template = "example.com/exampleorg-mirror/${namespace}-${type}"
    include             = ["example.org/*/*"]
  }
  direct {
    exclude = ["example.net/*/*", "example.org/*/*"]
  }
}
```

Under this configuration:

1. `example.net/foo/bar` would be installed from the OCI distribution repository at `example.com/examplenet-mirror/foo-bar`
2. `example.org/foo/bar` would be installed from the OCI distribution repository at `example.com/exampleorg-mirror/foo-bar`
3. `registry.opentofu.org/opentofu/lua` would be installed directly from `registry.opentofu.org`'s provider registry implementation.

The `repository_template` given in each `oci_mirror` block must include a reference to any component of the provider source address that is wildcarded in the `include` argument. In the most recent example above, the templates did not need to include `${hostname}` because both blocks force only a single specific hostname in the `include` argument, and so no other hostname could reach that installation method.

### Package checksums and signing

OpenTofu tracks in its [dependency lock file](https://opentofu.org/docs/language/files/dependency-lock/) the single selected version of each previously-installed provider, and a set of checksums that are considered acceptable for that version of that provider.

Additionally, whenever possible OpenTofu retrieves a GPG signature and associated public key for a provider (with both decided by its origin registry) and uses that signature to verify that the fetched provider package matches one of the checksums that was covered by the GPG signature. If that verification succeeds, OpenTofu assumes that all other checksums covered by the same signature are also trusted, and so records _all_ of them in the dependency lock file for future use. This mechanism is important to ensure that the dependency lock file ends up recording checksums for _all_ platform-specific packages offered for that provider version, even though OpenTofu only downloads and verifies the package for the current platform.

OpenTofu's existing provider registry protocol always uses `.zip` archives as provider packages and requires the provider developer's signature to cover a document containing SHA256 checksums of all of the `.zip` archives for a particular version. These translate into `zh:`-schemed checksums in the dependency lock file, but since those checksums can only be verified against a not-yet-unpacked `.zip` archive OpenTofu also generates a more general `h1:` checksum based on the _contents_ of the package it downloaded, which it can then use to verify already-unpacked provider packages in the local filesystem.

The established conventions for OCI manifest signing rely on the fact that OCI repositories use content-addressable storage and so signing a manifest containing the digests of other manifests or layer archives is sufficient to sign those manifest and layer archives themselves. However, much as with the signed `.zip` checksums used in the existing registry protocol, _these_ signed checksums are only useful to verify artifacts retrieved from an OCI repository and so OpenTofu will again need to also retain `h1:` checksums of the final content of the package after installation, calculated locally.

A new checksum scheme `ch:` will be used to track any container manifest digests mentioned in a multi-platform manifest that is covered by a supported signature. OpenTofu will record signed `ch:` checksums in the dependency lock file in a similar way to `zh:` checksums from a native OpenTofu Provider Registry, requiring that any future package retrieved for that provider version match at least one of the recorded checksums so that a previously-trusted signed provider cannot be compromised by just deleting its signing metadata.

The `ch:` checksum scheme is followed by a manifest digest using the same syntax as in OCI distribution manifests. For example, `ch:sha256:bc66513901b81f6cf8b2cf66d7414daa874f8f717626febe8591775232fedd0f`.

**TODO:** Define what constitutes a "supported signature". We are currently evaluating existing designs used in the OCI ecosystem, including Cohost and Docker Content Trust.

### Technical Approach

Since all three of the possible mapping methods described above result in the address of an OCI Distribution repository that is expected to follow the same conventions for its contents, we can support all three with only a single client implementation.

**TODO:** Write out the rest of this, once we've achieved consensus on the requirements and user experience parts of the proposal. For now, refer to [the prototype implementation](https://github.com/opentofu/opentofu/pull/2170).

### Open Questions

#### Provider Source Addresses with Unsupported Unicode Characters

OpenTofu's provider source address syntax allows a wide variety of Unicode characters in all three components, following the [RFC 3491 "Nameprep"](https://datatracker.ietf.org/doc/rfc3491/) rules.

However, the OCI Distribution specification has a considerably more restrictive allowed character set for repository names: it supports only ASCII letters and digits along with a small set of punctuation.

In principle then, there are some valid OpenTofu provider source addresses that cannot be translated mechanically to valid OCI Distribution repository addresses via simple template substitution alone. A provider source address that, for example, has a Japanese alphabet character in its "type" portion would be projected into a syntactically-invalid OCI repository address.

Our initial prototyped assumed that in practice non-ASCII characters in these addresses are very rare, and so just returns an error message whenever this situation arises:

```
requested provider address example.com/foo/ほげ contains characters that
are not valid in an OCI distribution repository name, so this provider
cannot be installed from an OCI repository as
ghcr.io/examplecom-otf-providers/foo-ほげ
```

Of course, we cannot see into every organization to know whether they have in-house providers that are named with non-ASCII characters, and the fact that the OpenTofu project works primarily in English means that we are less likely to hear from those whose typical working language is not English.

If we learn in future that supporting non-ASCII characters in provider source addresses installed from OCI registries is important, we could potentially force a specific scheme for automatically transforming those names into ones that are compatible with the OCI repository name requirements, such as applying a "[Punycode](https://en.wikipedia.org/wiki/Punycode)-like" encoding to them before rendering them into the template.

However, Punycode in particular is not generally human-readable and so translation strategies like this often require some UI support to automatically transcode the data back into human-readable form for display. Any OpenTofu-specific mapping strategy we might invent is unlikely to be handled automatically by the UI associated with any general-purpose OCI registry implementation.

## Potential Alternatives

### "Magic" inference of OCI repository addresses for well-known OCI registries

In ["Magic" inference of an OCI repository address](#magic-inference-of-an-oci-repository-address) we discussed reserving an OpenTofu-project-owned domain name for special treatment in triggering an automatic mapping to OCI repository addresses without any special configuration outside of the source address.

That compromise is intended to allow use of existing unmodified OCI registries while remaining consistent with the idea that we use ownership of hostnames in the domain name system to delegate management of our top-level namespace.

There are some hostnames that are currently well-known to be OCI registries, including:

- `docker.io`
- `ghcr.io`
- `containers.pkg.github.com`
- `quay.io`
- `gcr.io`
- `public.ecr.aws`

We could potentially also treat these specific hostnames as special cases, forcing them to be transformed in a similar way as our reserved domain suffix.

However, that would not be appropriate because these hostnames do not belong to domains that our project owns and controls. The owners of these domain names are ultimately responsible for deciding their meaning, and so they should remain free to offer OpenTofu-specific services or not on hostnames under their domains as they see fit.

Using an OpenTofu-project-owned domain for the "magic" mapping gives a single fixed string that someone can search for to learn more about how it works, and is also more explicit that this mapping is something special that OpenTofu is providing rather than something offered directly by the vendor that owns the underlying registry. This will therefore hopefully reduce the risk of owners of these registries being bothered by bug reports and similar related to OpenTofu's treatment of those domains.

If the owner of a domain offering an OCI registry decides that they'd like their domain alone to be usable as part of OpenTofu provider source addresses, they can publish an OpenTofu service discovery document as described in [OCI Registry as an implementation detail of "direct" installation](#oci-registry-as-an-implementation-detail-of-direct-installation), using content like the following if they _only_ want to offer provider registry services and they want to follow the same naming scheme that our "magic" inference would have followed:

```json
{
    "oci-providers.v1": "ghcr.io/{namespace}/opentofu-provider-{type}"
}
```

If a static document like the above were placed at `https://ghcr.io/.well-known/terraform.jon` then OpenTofu would translate a source address like `ghcr.io/foo/bar` into the OCI repository address `ghcr.io/foo/opentofu-provider-bar` and then attempt installation from that repository.

This compromise leaves the owners of these domains in control of what services are being provided in their name, while still providing a zero-configuration-required approach for using these (or any other) registries for OpenTofu provider hosting even if they do not choose to support it directly.
