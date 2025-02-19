
# Module installation in OCI

---

This document is part of the [OCI registries RFC](../20241206-oci-registries.md).

| [« Previous](4-providers.md) | [Up](../20241206-oci-registries.md) | [Next »](6-authentication.md) |

---

In contrast to [providers](4-providers.md), modules already support schemes as part of their addresses. Therefore, modules will be addressable from OCI using the `oci://` prefix in OpenTofu code directly. For example, you will be able to address an AWS ECR registry like this:

```hcl
module "foo" {
  source = "oci://example.com/opentofu-vpc-module"
}
```

OpenTofu will attempt to find a tag named `latest` for the `opentofu-vpu-module` repository on the `example.com` OCI registry, and will retrieve the associated artifact as a module package.

## Explicit tag or digest selection

Following the established precedent for [OpenTofu remote module source addresses](https://opentofu.org/docs/language/modules/sources/), the `oci:` scheme will support two optional query string arguments, which are mutually-exclusive:

* `tag=NAME` specifies a different tag to use, overriding the default of `latest`.
* `digest=DIGEST` directly specifies the digest of the image manifest to select, bypassing the tag namespace altogether.

For example, `oci://example.com/opentofu-vpc-module?tag=1.0.0` selects the artifact whose manifest is associated with the tag `1.0.0`, instead of `latest`.

> [!NOTE]
> Using query string arguments is inconsistent with the typical patterns used in OCI-specific software, which instead uses more concise `@DIGEST` or `:TAG` suffixes.
>
> In the implementation details, OpenTofu module installation is delegated to a third-party library called "go-getter" which has its own opinions about address syntax, and it's from that library that the existing query string convention emerged. Following this approach makes OpenTofu consistent with itself, but at the expense of being slightly inconsistent with other OCI-based software. Trying to follow the OCI conventions closely would go against the grain of go-getter's assumptions -- potentially causing challenges as the system evolves in future -- and would make it harder for OpenTofu users to transfer their knowledge from one source address type to another.

## Version-constraint-based selection

OpenTofu currently reserves version-constraint-based module package selection only for [module registry source addresses](https://opentofu.org/docs/language/modules/sources/#module-registry), which are special in that they are handled directly by OpenTofu rather than delegated to the upstream "go-getter" library.

Therefore our first release of these features will rely on direct selection of specific artifact tag names or digests, similar to how OpenTofu's Git support uses Git-style refs to select a Git commit to use.

A future version of OpenTofu might support [version constraints for other source address types](https://github.com/opentofu/opentofu/issues/2495) -- potentially including OCI, Git, and Mercurial -- but that is a broader change to OpenTofu's architecture and outside the scope of this RFC.

## Modules in package sub-directories

Although we commonly just casually refer to installing "modules", the unit of module installation is technically the module _package_, which is the term we use for an artifact containing a filesystem tree that has zero or more module directories inside of it. For example, a Git repository _as a whole_ is a module package because `git clone` only supports cloning the entirety the filesystem associated with a particular commit, but a Git repository can potentially contain multiple modules.

An OCI artifact will represent a module package, as described in the following section. As usual OpenTofu expects to find a module in the root directory of an OCI artifact by default, but you can optionally specify a subdirectory using [the usual subdirectory syntax](https://opentofu.org/docs/language/modules/sources/#modules-in-package-sub-directories):

```hcl
module "foo" {
  source  = "oci://example.com/opentofu-vpc-module//subnets"
}
```

To combine that with the `tag` or `digest` arguments, place the query string after the subdirectory portion: `oci://example.com/opentofu-vpc-module//subnets?tag=1.0.0`.

## Artifact layout

Module packages will be represented as a direct ORAS-style image artifact, without a multi-platform image manifest, because module packages are platform-agnostic. The `artifactType` property of the image manifest must be set to `application/vnd.opentofu.module`.

The image manifest _must_ have exactly one layer with `mediaType` set to `archive/zip`, which refers to a blob containing a `.zip` archive of the module package contents.

The manifest may include other layers with different `mediaType` values, which OpenTofu will ignore. Future versions of OpenTofu might recognize layers with other `mediaType` values.

If needed, you can publish additional artifacts that refer to the image manifest using the `subject` property in the additional artifact's manifest, making the child artifact discoverable using [the OCI Distribution "referers" API](https://github.com/opencontainers/distribution-spec/blob/main/spec.md#listing-referrers). This is commonly used to attach signatures or metadata such as SBOM documents to an artifact without needing to directly modify the original artifact. OpenTofu will not initially make any use of referring artifacts, but may begin to make use of referrers with specific `artifactType` values in future versions.

## Tooling

The artifact layout described is compatible with [ORAS](https://oras.land/), so you can use ORAS to push modules:

```
oras push \
    --artifact-type application/vnd.opentofu.module \
    example.com/opentofu-vpc-module:latest \
    opentofu-vpc-module.zip:archive/zip
```

## OCI-based Modules through an OpenTofu Module Registry

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
