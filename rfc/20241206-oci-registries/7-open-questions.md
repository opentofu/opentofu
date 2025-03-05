# Open questions

---

This document is part of the [OCI registries RFC](../20241206-oci-registries.md).

| [« Previous](6-authentication.md) | [Up](../20241206-oci-registries.md) | [Next »](8-auth-implementation-details.md) |

---

## How does this affect Terragrunt?

Terragrunt uses a [Provider Cache Server](https://terragrunt.gruntwork.io/docs/features/provider-cache-server/) to mitigate [OpenTofu's lack of support for concurrent modification of plugin cache directories](https://github.com/opentofu/opentofu/issues/1483).

We will need to review Terragrunt's implementation to learn whether it is compatible with the new `oci_mirror` provider installation method.

According to the documentation, this workaround uses the undocumented `host` block type in the CLI configuration to override the [remote service discovery protocol](https://opentofu.org/docs/internals/remote-service-discovery/) and thus force registry requests to be sent to the cache server's endpoint instead of to the real registry. That suggests that it can be used only with the `direct` installation method, since that is the one that uses the provider registry protocol.

Therefore it seems that at first `oci_mirror` will be mutually exclusive with Terragrunt's caching solution: using `oci_mirror` instead of `direct` will bypass the mechanism that Terragrunt relies on to intercept provider registry requests. However, no existing deployment should be broken because we do not intend to change the behavior of `direct` in the first release, and so Terragrunt users can keep using the Terragrunt cache server as long as they don't update their OpenTofu CLI configuration to include `oci_mirror` installation methods.

## How does a user configure multiple authentication realms?

Users working in multiple codebases may want to use different credentials for the same host depending on their current codebase.

This proposal supports that only indirectly in the same way as it's currently supported indirectly for OpenTofu's own `credentials` blocks: using the `TF_CLI_CONFIG_FILE` environment variable to tell OpenTofu to use a codebase-specific CLI Configuration file, which can therefore contain whatever `credentials` and `oci_credentials` blocks are needed for that specific codebase.

If we learn that this is a common situation then we may wish to introduce support for switching between authentication contexts in a future OpenTofu release. If we do that, it should ideally work for both OpenTofu-native and OCI-specific credentials together, and so is not directly in the scope of this project.

## Provider Source Addresses with Unsupported Unicode Characters

OpenTofu's provider source address syntax allows a wide variety of Unicode characters in all three components, following the [RFC 3491 "Nameprep"](https://datatracker.ietf.org/doc/rfc3491/) rules.

However, the OCI Distribution specification has a considerably more restrictive allowed character set for repository names: it supports only ASCII letters and digits along with a small set of punctuation characters.

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

However, Punycode in particular is not human-readable and so translation strategies like this often require some UI support to automatically transcode the data back into human-readable form for display. Any OpenTofu-specific mapping strategy we might invent is unlikely to be handled automatically by the UI associated with any general-purpose OCI registry implementation.

---

| [« Previous](6-authentication.md) | [Up](../20241206-oci-registries.md) | [Next »](8-auth-implementation-details.md) |

---