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

## Provider Source Addresses with Unsupported Unicode Characters

OpenTofu's provider source address syntax allows a wide variety of Unicode characters in all three components, following the [RFC 3491 "Nameprep"](https://datatracker.ietf.org/doc/rfc3491/) rules.

However, the OCI Distribution specification has a considerably more restrictive allowed character set for repository names: it supports only ASCII letters and digits along with a small set of punctuation.

Because of this, there some valid OpenTofu provider source addresses that cannot be translated mechanically to valid OCI Distribution repository addresses via template substitution alone. A provider source address that, for example, has a Japanese alphabet character in its "type" portion would be projected into a syntactically-invalid OCI repository address.

Our initial prototype assumed that in practice non-ASCII characters in these addresses are very rare, and so just returns an error message whenever this situation arises:

```
requested provider address example.com/foo/ほげ contains characters that
are not valid in an OCI distribution repository name, so this provider
cannot be installed from an OCI repository as
ghcr.io/examplecom-otf-providers/foo-ほげ
```

Of course, we cannot see into every organization to know whether they have in-house providers that are named with non-ASCII characters, and the fact that the OpenTofu project works primarily in English means that we are less likely to hear from those whose typical working language is not English.

If we learn in future that supporting non-ASCII characters in provider source addresses installed from OCI registries is important, we could potentially force a specific scheme for automatically transforming those names into ones that are compatible with the OCI repository name requirements, such as applying a "[Punycode](https://en.wikipedia.org/wiki/Punycode)-like" encoding to them before rendering them into the template.

However, Punycode in particular is not generally human-readable and so translation strategies like this often require some UI support to automatically transcode the data back into human-readable form for display. Any OpenTofu-specific mapping strategy we might invent is unlikely to be handled automatically by the UI associated with any general-purpose OCI registry implementation.

---

| [« Previous](7-authentication.md) | [Up](../20241206-oci-registries.md) |

---