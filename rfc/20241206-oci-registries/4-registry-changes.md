# 4. Changes to the OpenTofu Registry

---

This document is part of the [OCI registries RFC](../20241206-oci-registries.md).

| [« Previous](3-design-considerations.md) | [Up](../20241206-oci-registries.md) | [Next »](5-providers.md) |

---

As outlined in the [Design Considerations](3-design-considerations.md), we intend to start supporting the publication of SBOM artifacts. This requires changes to the existing OpenTofu registry and an extension to the [Provider Registry Protocol](https://opentofu.org/docs/internals/provider-registry-protocol/) and the [Module Registry Protocol](https://opentofu.org/docs/internals/module-registry-protocol/).

## Changes to the Provider Registry Protocol and the OpenTofu Registry

To support SBOM artifacts, we are changing the ["Find a Provider Package"](https://opentofu.org/docs/internals/provider-registry-protocol/#find-a-provider-package) endpoint located at `:namespace/:type/:version/download/:os/:arch` to include the `attestations` field:

```json
{
  "protocols": ["4.0", "5.1"],
  "os": "linux",
  "arch": "amd64",
  "filename": "terraform-provider-random_2.0.0_linux_amd64.zip",
  "download_url": "https://releases.example.com/terraform-provider-random/2.0.0/terraform-provider-random_2.0.0_linux_amd64.zip",
  "shasums_url": "https://releases.example.com/terraform-provider-random/2.0.0/terraform-provider-random_2.0.0_SHA256SUMS",
  "shasums_signature_url": "https://releases.example.com/terraform-provider-random/2.0.0/terraform-provider-random_2.0.0_SHA256SUMS.sig",
  "shasum": "5f9c7aa76b7c34d722fc9123208e26b22d60440cb47150dd04733b9b94f4541a",
  "signing_keys": {
    "gpg_public_keys": [
      {
        "key_id": "51852D87348FFC4C",
        "ascii_armor": "-----BEGIN PGP PUBLIC KEY BLOCK-----\nVersion: GnuPG v1\n\nmQENBFMO...",
        "trust_signature": "",
        "source": "ExampleCorp",
        "source_url": "https://www.examplecorp.com/security.html"
      }
    ]
  },
  "attestations": [
    {
      "mediaType": "application/spdx+json",
      "name": "sbom.spdx.json",
      "url": "https://releases.example.com/terraform-provider-random/2.0.0/sbom.spdx.json"
    }
  ]
}
```

The OpenTofu Registry will, when scanning provider releases, identify the following file names as attestations and include their reference in the provider protocol responses.

- `*.spdx.json` (SPDX) will be identified as [`application/spdx+json`](https://www.iana.org/assignments/media-types/application/spdx+json)
- `*.intoto.jsonl` ([in-toto attestation framework](https://github.com/in-toto/attestation), such as [SLSA Provenance](https://slsa.dev/spec/v1.0/provenance)) will be identified as `application/vnd.in-toto+json`.

Note, however:

- `*.spdx.xml` (SPDX) will *not* be supported until [there is an approved MIME type for it](https://github.com/spdx/spdx-spec/issues/577#issuecomment-960295523).
- `bom.xml` and `bom.json` (CycloneDX) will *not* be supported [until there is an approved MIME type for it](https://github.com/CycloneDX/specification/issues/210).

> [!NOTE]
> OpenTofu will not validate the contents of the attestations as there are too many possible formats to support. It is between the provider/module author and their community to ensure that the attestations are correct.

## Changes to the Module Registry Protocol and the OpenTofu Registry

Modules work differently to providers. Therefore, the module registry protocol will not be changed. However, when mirroring a module into an OCI registry, OpenTofu will consider the same file endings as described above **in the root directory only** as an SBOM artifact and include it in the OCI distribution.

## Changes to the HashiCorp providers

As part of the effort for supply chain security, OpenTofu will modify the [mirroring of HashiCorp providers](https://search.opentofu.org/docs/users/providers#using-hashicorp-maintained-providers-aws-azurerm-etc) to include generating an SPDX JSON SBOM document using [Goreleaser](https://goreleaser.com/customization/sbom/).


> [!NOTE]
> In an effort to support supply-chain security, we will also make these changes for the main OpenTofu release pipeline. The version including the OCI feature will also include an SBOM artifact.

---

| [« Previous](3-design-considerations.md) | [Up](../20241206-oci-registries.md) | [Next »](5-providers.md) |

---
