# Open questions

---

This document is part of the [OCI registries RFC](../20241206-oci-registries.md).

| [« Previous](7-authentication.md) | [Up](../20241206-oci-registries.md) |

---

## Do we publish SLSA Provenance documents?

SLSA Provenance, first suggested in [#315](https://github.com/opentofu/opentofu/issues/315) for OpenTofu itself, is somewhat a duplicate of the aims of Cosign. It attests to the SBOM and that it was generated in a specific way. Publishing such documents for OpenTofu and the HashiCorp providers may be as simple as including a GitHub Actions step, but build pipelines incur a high maintenance cost and are extremely hard to test.

We could support it on an experimental basis, and remove it if we see only a small number of downloads. It would fit with the security/supply chain aims of this release.

## How does this affect Terragrunt?

Terragrunt uses a [Provider Cache Server](https://terragrunt.gruntwork.io/docs/features/provider-cache-server/) to mitigate [#1483](https://github.com/opentofu/opentofu/issues/1483). This issue causes a crash on parallel access to the `TF_PLUGIN_CACHE_DIR`. Since OCI is a viable alternative to the Provider Cache Server, we need to test how to work around that.

## How does a user configure multiple authentication realms?

Users working on multiple projects may want to use different credentials for the same host depending on their project. This is currently not configurable. Does moving the OCI authentication block into the `terraform{}` block make more sense?

## Do we follow OpenTofu Registry or Git semantics for modules?

Currently, this RFC [proscribes registry semantics](6-modules.md) for module versions. Do we want to use this or switch to Git-like semantics?

---

| [« Previous](7-authentication.md) | [Up](../20241206-oci-registries.md) |

---