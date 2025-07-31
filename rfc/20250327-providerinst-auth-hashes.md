# Tracking Provider Authentication on a Per-hash Basis

Issues:
- This is motivated by [Provider and module packages from OCI registries (first phase)](https://github.com/opentofu/opentofu/issues/2540), but represents a more general improvement to OpenTofu's handling of provider checksums from non-registry ("mirror") sources.

When OpenTofu's `tofu init` command installs a provider for the first time, or is switching to a different selected version for an existing provider, it follows a "trust on first use" model where it records some expected-valid checksums for the provider in the [dependency lock file](https://opentofu.org/docs/language/files/dependency-lock/) so that operators can verify those checksums once and can then expect OpenTofu will reject any package that doesn't match at least one of those checksums on future runs.

The current form of this behavior was retrofitted into the "package authentication" step of the provider installer, implemented in [`package getproviders`](https://pkg.go.dev/github.com/opentofu/opentofu/internal/getproviders), as an optional extension to the `PackageAuthentication` interface:

```go
type PackageAuthentication interface {
	// AuthenticatePackage takes the local location of a package (which may or
	// may not be the same as the original source location), and returns a
	// PackageAuthenticationResult, or an error if the authentication checks
	// fail.
	//
	// The local location is guaranteed not to be a PackageHTTPURL: a remote
	// package will always be staged locally for inspection first.
	AuthenticatePackage(localLocation PackageLocation) (*PackageAuthenticationResult, error)
}

type PackageAuthenticationHashes interface {
	PackageAuthentication

	// AcceptableHashes returns a set of hashes that this authenticator
	// considers to be valid for the current package or, where possible,
	// equivalent packages on other platforms. The order of the items in
	// the result is not significant, and it may contain duplicates
	// that are also not significant.
	//
	// This method's result should only be used to create a "lock" for a
	// particular provider if an earlier call to AuthenticatePackage for
	// the corresponding package succeeded. A caller might choose to apply
	// differing levels of trust for the acceptable hashes depending on
	// the authentication result: a "verified checksum" result only checked
	// that the downloaded package matched what the source claimed, which
	// could be considered to be less trustworthy than a check that includes
	// verifying a signature from the origin registry, depending on what the
	// hashes are going to be used for.
	//
	// Implementations of PackageAuthenticationHashes may return multiple
	// hashes with different schemes, which means that all of them are equally
	// acceptable. Implementors may also return hashes that use schemes the
	// current version of the authenticator would not allow but that could be
	// accepted by other versions of OpenTofu, e.g. if a particular hash
	// scheme has been deprecated.
	//
	// Authenticators that don't use hashes as their authentication procedure
	// will either not implement this interface or will have an implementation
	// that returns an empty result.
	AcceptableHashes() []Hash
}
```

Because this is handled as a separate _optional_ concern from `PackageAuthentication.AuthenticatePackage` its interaction with the package authentication is quite subtle, leading to the very long doc comment on the `AcceptableHashes` method above.

In practice `PackageAuthentication` and `PackageAuthenticationHashes` are normally used with multiple implementations at once using [`getproviders.PackageAuthenticationAll`](https://pkg.go.dev/github.com/opentofu/opentofu/internal/getproviders#PackageAuthenticationAll), which makes the behavior even more subtle:

- `AuthenticatePackage` delegates to the method of the same name on each of the multiple wrapped `PackageAuthentication` implementations, but returns only the result from the last one.
- `AcceptableHashes` delegates directly to the last one, completely ignoring all of the others.

In practice this means that `getproviders.Source` implementations must be careful to call `PackageAuthenticationAll` with the inner implementations carefully ordered so that the "strongest" is last, and the caller of these methods in the main provider installer doesn't get any information about how each of the individual "acceptable hashes" might relate to the single package authentication result, so the current behavior relies on some quite careful arrangements between all of these components which ultimately result in the following emergent behavior in today's OpenTofu:

- The provider installer _itself_ always saves a `h1:` hash generated from the cache directory as it exists after installation was successful, regardless of the provider source.
- The provider installer chooses to trust `AcceptableHashes` only when the `AuthenticatePackage` function returns a "signed" or "signature verification skipped" result, and in practice that can only happen when installing from a provider's origin registry and so all of the other sources can't contribute any additional hashes _at all_.

This situation works when operators use OpenTofu's features in the specific way they were intended: developers use the origin registry in their development environment, causing the lock file to get populated properly, and then only the automation used for "real" environments uses the mirror sources and so the content of the mirrors gets authenticated against the hashes originally gathered from the origin registry and recorded in the dependency lock file.

However, this situation _does not_ work when a mirror source is used as the _only_ installation source across all environments including development, because in that case `tofu init` never gets any opportunity to obtain the signed hashes from the origin registry, and so the dependency lock file is left incomplete. Today that means that somewhere in the development or deployment process someone needs to run the separate command `tofu providers lock` to prime the dependency lock file with correct hashes, which creates additional workflow friction and also has some annoying chicken/egg problems when upgrading a previously-selected provider.

## Proposed Solution

The current awkward behavior is in large part caused by treating the `AuthenticatePackage` result and the `AcceptableHashes` result as two separate concerns.

In practice, all reasonable concrete `PackageAuthentication` implementations somehow interact with package hashes. Some of those hashes are directly verified by OpenTofu itself by inspecting what was downloaded, while others are merely "promised" to OpenTofu using an artifact signed by a GPG key, but ultimately we can choose to think of a "package authentication result" as a collection of hashes where each hash has a _disposition_ that describes what OpenTofu learned about it during the installation process:

```go
// HashDisposition describes in what way a particular hash is related to
// a particular [PackageAuthenticationResult], and thus what a caller might
// be able to assume about the trustworthiness of that hash.
type HashDisposition struct {
	// SignedByGPGKeyIDs is a set of GPG key IDs which provided signatures
	// covering the associated hash.
	//
	// A hash that has at least one signing key ID but was not otherwise
	// verified (as indicated by the other fields of this type) should be
	// trusted only if at least one of the given signing keys is trusted.
	// It's the responsibility of any subsystem relying on this information
	// to define what it means for a key to be "trusted".
	SignedByGPGKeyIDs collections.Set[string]

	// ReportedByRegistry is set if this hash was reported by the associated
	// provider's origin registry as being one of the official hashes for
	// this provider release.
	//
	// Note that this signal alone is only trustworthy to the extent that
	// the origin registry is trusted. Unless there is also at least one
	// entry in SignedByGPGKeyIDs, this hash was not covered by any GPG
	// signing key.
	//
	// This field must be set only by a source that directly interacted
	// with the associated provider's origin registry. It MUST NOT be
	// set by a source that interacts with any sort of "mirror".
	ReportedByRegistry bool

	// VerifiedLocally is set for any hash that was calculated locally,
	// directly from a package for the provider that the hash is intended
	// to verify.
	//
	// Note that this represents only that the artifact matched the
	// associated hash; it does NOT mean that the associated hash is
	// one of the official hashes as designated by the provider developer,
	// unless the provider developer's signing key also appears in
	// SignedByGPGKeyIDs.
	VerifiedLocally bool
}

// HashDispositions represents a collection of hashes that are associated
// with a provider as a result of installing it, each of which has a
// "disposition" that calling code can use to decide in what ways it is
// appropriate to make use of the hash.
//
// For example, the provider installer might choose to ignore hashes that
// were not verified locally unless are marked as having been signed by a
// trusted GPG key.
type HashDispositions map[Hash]*HashDisposition
```

Under this new model, rather than treating `AcceptableHashes` as an entirely separate _optional_ extension of `PackageAuthentication`, the `PackageAuthenticationResult` _is_ the collection of `HashDispositions`:

```go
type PackageAuthenticationResult struct {
	hashes HashDispositions
}
```

By tracking the disposition of each hash individually, rather than trying to summarize the entire authentication result into a single flat enumeration of outcomes, the `getproviders.PackageAuthenticationAll` helper (which aggregates together a set of `PackageAuthentication` implementations) can call `AuthenticatePackage` on each of its wrapped authenticators and _merge their results together_, rather than returning only the result from the final one.

The previous flat enumeration of authentication outcomes (which we'd continue to use as a summary in the UI) can be derived from a final merged `HashDispositions` as follows (in priority order, first match "wins"):

- "signed" if at least one hash has at least one entry in `SignedByGPGKeyIDs`.
- "signing skipped" if at least one hash has `ReportedByRegistry` set to `true`. (in practice today this is possible only when the `OPENTOFU_ENFORCE_GPG_VALIDATION` environment variable isn't set, because otherwise failed validation is an authentication error that prevents the `PackageAuthenticationResult` from being used at all)
- "verified checksum" if at least one hash has `VerifiedLocally` set to `true`. (this would be the typical outcome for any "mirror" source that doesn't have access to the signed checksum set from the origin registry)
- "unauthenticated" otherwise.

The provider installer can also then use the `HashDispositions` structure as a whole to decide which subset of hashes to include in the dependency lock file, rather than that decision being forced by whatever `AcceptableHashes` returns. The updated rule for which to include would be the union of:

1. Any hash that has `ReportedByRegistry` set to true. (again, setting `OPENTOFU_ENFORCE_GPG_VALIDATION` forces this to be allowed only in combination with at least one `SignedByGPGKeyIDs` entry, but by default we trust the registry regardless of signatures today; if that changes in future then this rule might change to require at least one `SignedByGPGKeyIDs` entry)
2. Any hash that has `VerifiedLocally` set to true, meaning that the local OpenTofu process calculated the checksum from the artifact itself and compared it to those that were expected.

The first item above matches the current effective behavior resulting from how the different components are currently assembled. The second behavior is new, and is what will allow the `network_mirror` and `oci_mirror` sources to also include the `zh:` hash of the archive they downloaded and verified against information in the mirror, so that those who are exclusively using mirrors will no longer need to use `tofu providers lock` unless they want to capture a locally-verified hash for some other platform than the one where `tofu init` was run.

With the hashes now incorporated directly into the `PackageAuthenticationResult` as its primary representation of the outcome of all of the authenticators, the `PackageAuthenticationHashes` extension interface can be removed entirely: the result from `PackageAuthentication.AuthenticatePackage` itself now contains all of the information required to compute the same result, with the hash-selection policy now being centralized in the provider installer rather than clumsily spread across various different components that need to be assembled "just so".

### User Documentation

Much of this proposal involves just changing the internal implementation of package authentication rather than changing externally-visible behavior. In particular, for those using OpenTofu's default provider installation settings they should not notice any change in behavior whatsoever.

The only externally-visible change is for those using `network_mirror` or `oci_mirror` installation methods, or those using `filesystem_mirror` installation method with a directory that uses the "packed" layout. In all of those cases, OpenTofu will now additionally record the locally-verified `zh:` hash for the `.zip` file that was installed as part of the dependency lockfile, thereby allowing that same package to be installed again in future without any verification errors.

Importantly, no user should need to change how they use OpenTofu in response to this change. `tofu init` will work alone (without the support of `tofu providers lock`) in more situations than before, without any change to the CLI configuration or command line options.

## Future Considerations

### Future features for improved provider package verification

As described under "Potential Alternatives" below, this proposal is arguably overkill _just_ for the problem as stated.

However, we've previously heard requests for additional control over provider package verification, including but not limited to:
- Ability to specify a fixed set of trusted GPG key IDs that the operator allows, rejecting any package that cannot be verified against those keys.
- Ability to use other signing schemes besides GPG, which implies additional types of key that we'd need to keep track of.
- Ability for a local administrator to generate additional signatures for providers _they've_ somehow verified, separately from the keys used by the provider author, and require those signatures to be present.
- Ability to designate a particular "mirror" source as trustworthy even when it cannot provide signed checksums, if e.g. an organization is willing to accept TLS certificate authentication as sufficient guarantee that the mirror has not been compromised.

Tracking verification results on a per-hash basis rather than on an overall-install basis is a far better foundation for future extensions like these, because it allows us to combine the results of a wide variety of different authentication steps into a data structure that we can use for a number of different centralized processes in the provider installer.

### Resolving the hash type inconsistency

The distinction between `zh:` and `h1:` hashes in today's OpenTofu is unfortunate, but ultimately caused by the fact that the provider registry protocol was already using checksum files using hashes of _the zipfile itself_ before the introduction of the other installation methods, and those artifacts are created by the provider developer during their publishing process rather than by the provider registry so it is not possible to unilaterally switch all providers over to a new hash scheme at once.

This hash type inconsistency is also the root cause of some ineffectiveness of the optional _global_ plugin cache directory: it stores provider packages as unpacked directories so that they can be cheaply symlinked into the working-directory-specific local cache, but that means OpenTofu cannot calculate a `zh:` hash from a cache entry and so OpenTofu often ends up redownloading a cached package just to verify whether the cache entry is valid.

Ideally it would be nice to support having registries also return signed `h1:` hash to heal that mismatch. However, it's notable that for OCI registries we're constrained by what the OCI Distribution protocol is able to support, and it _natively_ deals in what we call `zh:` hash due to its content-addressable nature, so supporting `h1:` hashes with OCI registries would require some OpenTofu-specific extensions to the OCI manifest formats. We might find that it's ultimately better to just retain the `h1:`/`zh:` dichotomy indefinitely, while pursuing smaller improvements like this RFC to increase the number of situations where we're able to record both hash types.

## Potential Alternatives

### Download unexpected packages and verify them locally

The current friction is mainly caused by the fact that when possible OpenTofu chooses to quickly reject an incorrect package before downloading it whenever it's possible to do so, and to do so it relies on the checksums that the remote system hosting the packages is able to report. In many cases a remote system is only able to report the `zh:`-style hash of the zip file itself, which means that OpenTofu can't verify that against a `h1:`-style checksum (calculated from the zip file's _content_) until the archive is already downloaded.

The checksum-verification friction could therefore be addressed instead by just removing all of OpenTofu's attempts to pre-verify the available packages using metadata reported by the remote system, and instead have it _always_ download whatever it's offered and calculate both `zh:` and and `h1:` checksums from it locally to maximize the chance of a successful match regardless of what source the provider was originally installed from.

If avoiding the checksum verification friction were the _only_ motivation then this would be a potentially-simpler way to achieve that, but addressing the underlying design problem -- that authentication and "allowed checksums" are not handled together in a consistent way -- makes the overall system easier to understand by separating concerns more carefully, and can better support future enhancements like letting operators specify only a subset of GPG keys to trust, introducing non-GPG-based signing schemes, and potentially allowing operators to configure OpenTofu to treat a particular mirror as "trusted" regardless of its lack of verifiable checksums.

### Accept the current friction and change nothing

The friction of mirror sources not being able to report the checksums they verified for inclusion in the dependency lock file is a long-standing problem that already has a known workaround of running `tofu providers lock` to prime the dependency lock file with a providers "official" hashes, which mirror content can then be verified against.

The recently-added `oci_mirror` installation method exhibits similar friction as the long-standing `network_mirror` and `filesystem_mirror` installation methods, and the workarounds are the same, so we _could_ potentially choose to accept the continuation of this known limitation and recommend that providers always be verified against an origin registry.

However, we expect that in the short term `oci_mirror` will also be lightly "abused" as a funny sort of private registry for in-house-maintained providers that have no true origin registry, and wish to take some pragmatic steps to make that workaround behave as well as possible in the meantime until we're ready to support using OCI registries as a true alternative implementation of an "origin registry", with full support for artifact signing similar to OpenTofu's own Provider Registry Protocol.

Even if we did choose to accept this friction for `oci_mirror` today, many of the the emerging practices for OCI artifact signing do not rely on GPG as their cryptography foundation and so it's likely that the later implementation of "origin registries" in OCI registries would cause us to need to change the provider verification model in a similar way as proposed here so that we can track a mixture of different signing schemes associated with the same hashes.
