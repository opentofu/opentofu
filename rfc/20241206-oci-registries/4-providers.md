# Provider installation in OCI

---

This document is part of the [OCI registries RFC](../20241206-oci-registries.md).

| [« Previous](3-design-considerations.md) | [Up](../20241206-oci-registries.md) | [Next »](5-modules.md) |

---

As stated in [Design Considerations](3-design-considerations.md), in this iteration we will focus on serving the provider mirroring use case.

This means that the first version will be focused on using an OCI registry as an alternative source for a provider whose origin is a traditional OpenTofu provider registry. It will also be technically possible to manually construct and publish provider package artifacts for in-house providers by following OpenTofu's expected conventions for the manifest formats, but we intend to support that use-case better in a subsequent release.

## Configuring a provider in OCI

In order to configure OCI as a provider source in OpenTofu, you will have to modify your [OpenTofu CLI Configuration](https://opentofu.org/docs/cli/config/config-file/). Specifically, you will need to add a `provider_installation` block with at least one `oci_mirror` installation method:

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

In this case, provider addresses matching the `include` arguments would be redirected to the specified OCI registries. Any provider that does not belong to one of the two configured hostnames would still be installed from its origin registry as normal, due to the (optional) inclusion of the `direct` installation method.

The templating is required because OCI registry addresses work differently to OpenTofu provider addresses and some registries require specific prefixes. The `repository_template` argument must include a substitution for each component of the provider source address that is a wildcard in the `include` argument:

* `${hostname}` represents the hostname (which defaults to `registry.opentofu.org` for source addresses that only have two parts, like `hashicorp/kubernetes`)
* `${namespace}` represents the namespace (the `foo` in `example.net/foo/bar`)
* `${name}` represents the provider name (the `bar` in `example.net/foo/bar`)

In practice, most commonly-used providers today belong to the `registry.opentofu.org` hostname, which the public registry run by the OpenTofu project. Therefore to support "air-gapped" systems (which cannot directly connect to `registry.opentofu.org`) an organization would need to copy the packages for providers they use from their origin locations returned by the OpenTofu registry into a systematic naming scheme under an OCI registry and then use an `oci_mirror` block that includes providers matching `registry.opentofu.org/*/*`:

```hcl
provider_installation {
  oci_mirror {
    repository_template = "example.com/opentofu-provider-mirror/${namespace}_${type}"
    include             = ["registry.opentofu.org/*/*"]
  }
}
```

With this CLI configuration, when initializing a module that depends on the `hashicorp/kubernetes` provider, OpenTofu would install the provider from the `opentofu-provider-mirror/hashicorp_kubernetes` repository in the OCI registry running at `example.com`, instead of contacting `registry.opentofu.org` directly. In this configuration, `registry.opentofu.org` is serving only as part of the provider's unique identifier and not as a physical network location. This is an OCI Distribution-compatible equivalent of OpenTofu's existing `network_mirror` installation method, which currently uses [an OpenTofu-specific protocol](https://opentofu.org/docs/internals/provider-network-mirror-protocol/).

> [!TIP]
> For example, Amazon ECR registries have the format of *aws_account_id*.dkr.ecr.*region*.amazonaws.com/*repository*:*tag*.
> 
> You could map to an ECR-based registry with a configuration like the following:
> 
> ```hcl
> provider_installation {
>   oci_mirror {
>     repository_template = "YOUR_AWS_ACCOUNT_ID.dkr.ecr.us-east-1.amazonaws.com/${namespace}_${type}"
>     include             = ["registry.opentofu.org/*/*"]
>   }
> }
> ```
> 
> This would then cause OpenTofu to install (for example) the `hashicorp/kubernetes` provider from the registry named `hashicorp_kubernetes` in the OCI registry `YOUR_AWS_ACCOUNT_ID.dkr.ecr.us-east-1.amazonaws.com`, without any need to rewrite all modules to refer to a new source address.

## Storage in OCI

OpenTofu takes some inspiration from how [ORAS](1-oci-primer.md#oras) stores files, but with a few key differences. At the time of writing [ORAS is intending to add support for multi-platform index manifests](https://github.com/oras-project/oras/pull/1514), and we aim to be compatible with that proposal.

1. Each OpenTofu provider OS and architecture (e.g. linux_amd64) will be stored as a ZIP file directly in an OCI blob. OpenTofu will not use tar files as it would be typical for a classic container image.
2. Each provider OS and architecture will have an image manifest with a single layer with the `mediaType` of `archive/zip` that is expected to be a direct copy of the provider developer's official distribution package for that platform.
3. The main manifest of the artifact will be an index manifest, containing separate entries for each OS and architecture supported by that release of the provider. Additionally, the main manifest must declare the `artifactType` attribute as `application/vnd.opentofu.provider` in order for OpenTofu to accept it as a provider image.
4. The provider artifact must be available at a tag whose name matches the upstream version number. OpenTofu will ignore any versions it cannot identify as a semver version number, including the `latest` tag.

    Because semver uses `+` as the delimiter for "build metadata" and that character is not allowed in an OCI tag name, any `+` characters in the version number must be replaced with `_` when naming the tag.
5. All entries in `manifests` in the index manifest are assumed to represent provider packages, and so no other manifests may be listed. However, the individual image manifests _may_ include additional layers with `mediaType` different from `archive/zip` which OpenTofu will ignore. Each image manifest must have exactly one `archive/zip` layer.

If needed, you can publish additional artifacts that refer to either the index manifest or to one of the individual image manifests using the `subject` property in the additional artifact's manifest, making the child artifact discoverable using [the OCI Distribution "referers" API](https://github.com/opencontainers/distribution-spec/blob/main/spec.md#listing-referrers). This is commonly used to attach signatures or metadata such as SBOM documents to an artifact without needing to directly modify the original artifact. OpenTofu will not initially make any use of referring artifacts, but may begin to make use of referrers with specific `artifactType` values in future versions.

> [!WARNING]
> Provider artifacts in OCI *must* have multi-platform (index) manifests. OpenTofu will refuse to download and use non-multi-platform artifacts as provider manifests. In contrast, [modules](5-modules.md) *must not* use multi-platform manifests.

## Publishing or mirroring a provider

Currently, there is no third-party tool capable of pushing an OCI artifact in the format we need for this RFC. We hope that a future version of ORAS CLI will support this layout, but we do not want to make this proposal dependent on the ORAS implementation.

Therefore if the ORAS multi-platform manifest proposal does not reach implementation and release before we complete implementation of provider installation from OCI mirrors then we will start by publishing instructions on how to manually write a multi-platform index manifest and push it with the lower-level ORAS manifest commands. The instructions we publish will produce the same effect as the `oras manifest index create` command proposed in [the ORAS Multi-arch Image Management proposal](https://github.com/oras-project/oras/blob/fb6e94d00e59ea6d468cbf8656cf760ef7f1751c/docs/proposals/multi-arch-image-mgmt.md).

We are considering offering a built-in tool for automatically mirroring a set of providers from their origin registries into an OCI mirror, similar to the current [`tofu providers mirror`](https://opentofu.org/docs/cli/commands/providers/mirror/) automating the population of a _filesystem_ mirror directory. However, we wish to minimize the scope for the initial release to maximize our ability to respond to feedback without making breaking changes.

---

| [« Previous](3-design-considerations.md) | [Up](../20241206-oci-registries.md) | [Next »](5-modules.md) |

---
