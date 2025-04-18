
# Module installation in OCI

---

This document is part of the [OCI registries RFC](../20241206-oci-registries.md).

| [« Previous](4-providers.md) | [Up](../20241206-oci-registries.md) | [Next »](6-authentication.md) |

---

In contrast to [providers](4-providers.md), modules already support various different schemes as part of their source addresses. Therefore, modules will be addressable from OCI using the `oci://` prefix in OpenTofu code directly. For example, an author would be able to refer to a repository in OCI registry `example.com` like this:

```hcl
module "foo" {
  source = "oci://example.com/opentofu-vpc-module"
}
```

OpenTofu will attempt to find a tag named `latest` for the `opentofu-vpu-module` repository on the `example.com` OCI registry, and will retrieve the associated artifact as a module package.

## Explicit tag or digest selection

Following the established precedent for [OpenTofu remote module source addresses](https://opentofu.org/docs/language/modules/sources/), the `oci:` scheme will support two optional and mutually-exclusive arguments using URL query string syntax:

* `tag=NAME` specifies a different tag to use, overriding the default of `latest`.
* `digest=DIGEST` directly specifies the digest of the image manifest to select, bypassing the tag namespace altogether.

For example, `oci://example.com/opentofu-vpc-module?tag=1.0.0` selects the artifact whose manifest is associated with the tag `1.0.0`, instead of `latest`.

> [!NOTE]
> Using query string arguments is inconsistent with the typical patterns used in OCI-specific software, which instead uses more concise `@DIGEST` or `:TAG` suffixes.
>
> In the implementation details, OpenTofu module installation is delegated to a third-party library called "go-getter" which has its own opinions about address syntax, and it's from that library that the existing query string convention emerged. Following this approach makes OpenTofu consistent with itself, but at the expense of being slightly inconsistent with other OCI-based software. Trying to follow the OCI conventions closely would go against the grain of go-getter's assumptions -- potentially causing challenges as the system evolves in future -- and would make it harder for OpenTofu users to transfer their knowledge from one source address type to another.
>
> There is additional commentary on this decision under [Modules in package sub-directories](#modules-in-package-sub-directories), below.

## Version-constraint-based selection

OpenTofu currently reserves version-constraint-based module package selection only for [module registry source addresses](https://opentofu.org/docs/language/modules/sources/#module-registry), which are special in that they are handled directly by OpenTofu rather than delegated to the upstream "go-getter" library.

Therefore our first release of these features will rely on direct selection of specific artifact tag names or digests, similar to how OpenTofu's Git support uses Git-style refs to select a Git commit to use.

A future version of OpenTofu might support [version constraints for other source address types](https://github.com/opentofu/opentofu/issues/2495) -- potentially including OCI, Git, and Mercurial -- but that is a broader change to OpenTofu's architecture and outside the scope of this RFC.

> [!NOTE]
> The absense of version-constraint-based selection here is also a notable difference from the functionality we're offering for provider mirrors.
>
> This difference extends the existing precedent that the namespace of providers and their versions is intentionally abstracted to allow multiple installation strategies behind the same syntax configured globally as a concern of the environment where OpenTofu is running, whereas module sources are typically specified directly as physical addresses of specific artifacts unless someone chooses to use a module registry to "virtualize" those physical addresses.
>
> Directly exposing the concepts from the underlying protocol -- tags and digests in this case -- is consistent with OpenTofu's existing design precedent for module sources, such as the ability to select arbitrary Git branches, tags, and commits without any additional abstraction. It will be possible for a registry implementing the OpenTofu Module Registry Protocol to return an `oci:`-schemed source address as the physical location of a module package, although we don't expect that to be a common choice at least for the near future since we understand that the primary motivation for installing modules from OCI registries is to avoid running any additional OpenTofu-specific services. (For more commentary on the interaction between OpenTofu module registries and the new source address syntax, refer to [OCI-based Modules through an OpenTofu Module Registry](#oci-based-modules-through-an-opentofu-module-registry) below.)
>
> We'll use the issue linked in the text above to track interest in introducing the additional abstraction of semver-based selection of non-registry sources as an orthogonal concern in a later release, since that will allow us to approach it holistically as something we offer for all source address types whose underlying protocol has some concept we can use to represent "versions", rather than implementing something OCI-registry-specific.

## Modules in package sub-directories

Although we commonly just casually refer to installing "modules", the unit of module installation is technically the module _package_, which is the term we use for an artifact containing a filesystem tree that has zero or more module directories inside of it. For example, a Git repository _as a whole_ is a module package because `git clone` only supports transferring the entirety of the filesystem tree associated with a particular commit, but a Git repository can potentially contain multiple modules.

An OCI artifact will represent a module package as described in the following section. As usual, OpenTofu expects to find a module in the root directory of an OCI artifact by default, but an author can optionally specify a subdirectory using [the usual subdirectory syntax](https://opentofu.org/docs/language/modules/sources/#modules-in-package-sub-directories):

```hcl
module "foo" {
  source  = "oci://example.com/opentofu-vpc-module//subnets"
}
```

To combine that with the `tag` or `digest` arguments, authors must place the query string after the subdirectory portion: `oci://example.com/opentofu-vpc-module//subnets?tag=1.0.0`.

> [!NOTE]
>
> During the design of this we considered having the source addresses use a more "OCI-native" syntax that uses individual punctuation characters rather than URL query string syntax:
>
> - `oci://example.com/opentofu-vpc-module:TAG-NAME`
> - `oci://example.com/opentofu-vpc-module@DIGEST`
>
> We decided to follow OpenTofu's existing precedent in part because although go-getter would accept both of the above as valid URLs, it would not actually understand the meaning of those `:TAG-NAME` and `@DIGEST` suffixes, and so would treat them as part of the path instead. That would then make them interact with the sub-path syntax differently than every other source address syntax -- the sub-path would need to appear _after_ the tag/digest part, rather than before the query string -- making it harder for authors to transfer what they've learned from one source type to use of other source types.
>
> Making OpenTofu consistent with itself rather than with the typical OCI conventions is a tricky tradeoff, but overall following the existing precedent should make it less likely that any future mechanisms we design that are supposed to work across all source address types will need a special/unique design for the `oci:` scheme, rather than a shared design across all of the source types.

## Artifact layout

Module packages will be represented as a direct ORAS-style image artifact, without a multi-platform image manifest, because module packages are platform-agnostic. The `artifactType` property of the image manifest must be set to `application/vnd.opentofu.modulepkg`.

The image manifest _must_ have exactly one layer with `mediaType` set to `archive/zip`, which refers to a blob containing a `.zip` archive of the module package contents.

The manifest may include other layers with different `mediaType` values, which OpenTofu will ignore. Future versions of OpenTofu might recognize layers with other `mediaType` values.

If needed, operators can publish additional artifacts that refer to the image manifest using the `subject` property in the additional artifact's manifest, making the child artifact discoverable using [the OCI Distribution "referers" API](https://github.com/opencontainers/distribution-spec/blob/main/spec.md#listing-referrers). This is commonly used to attach signatures or metadata such as SBOM documents to an artifact without needing to directly modify the original artifact. OpenTofu will not initially make any use of referring artifacts, but may begin to make use of referrers with specific `artifactType` values in future versions.

## Publishing module packages

The artifact layout described is compatible with [ORAS](https://oras.land/), so operators can use ORAS CLI to push modules:

```
oras push \
    --artifact-type application/vnd.opentofu.modulepkg \
    example.com/opentofu-vpc-module:latest \
    opentofu-vpc-module.zip:archive/zip
```

## OCI-based modules through an OpenTofu Module Registry

The existing [OpenTofu Module Registry Protocol](https://opentofu.org/docs/internals/module-registry-protocol/) works as a façade over arbitrary remote [module source addresses](https://opentofu.org/docs/language/modules/sources/), and so any OpenTofu version that supports OCI-based module installation will also support module registries that respond to [Download Source Code for a Specific Module Version](https://opentofu.org/docs/internals/module-registry-protocol/#download-source-code-for-a-specific-module-version) with a location property using the `oci://` prefix as described above.

For example, a module registry would be allowed to respond to such a request by returning an address like the following:

```json
{"location":"oci://example.com/repository?digest=sha256:1d57d25084effd3fdfd902eca00020b34b1fb020253b84d7dd471301606015ac"}
```

Such a design would use the module registry protocol to hide the implementation detail that the packages are actually coming from an OCI registry, but at the expense of needing to run an additional OpenTofu-specific module registry service. We do not currently anticipate this being a common need, but it is a natural consequence of the existing module registry protocol design.

> [!WARNING]
> OpenTofu Module Registry implementors should note that references to OCI addresses will only work with OpenTofu v1.10 and later. Older versions will reject module source addresses using the `oci:` scheme.

---

| [« Previous](4-providers.md) | [Up](../20241206-oci-registries.md) | [Next »](6-authentication.md) |

---
