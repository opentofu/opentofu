# Open questions

## Do we publish SLSA Provenance documents?

SLSA Provenance, first suggested in [#315](https://github.com/opentofu/opentofu/issues/315) for OpenTofu itself, is somewhat a duplicate of the aims of Cosign. It attests to the SBOM and that it was generated in a specific way. Publishing such documents for OpenTofu and the HashiCorp providers may be as simple as including a GitHub Actions step, but build pipelines incur a high maintenance cost and are extremely hard to test.

We could support it on an experimental basis, and remove it if we see only a small number of downloads. It would fit with the security/supply chain aims of this release.

## TF_PLUGIN_CACHE_DIR

https://github.com/opentofu/opentofu/issues/1483
https://terragrunt.gruntwork.io/docs/features/provider-cache-server/
