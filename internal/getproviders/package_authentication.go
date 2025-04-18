// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"iter"
	"log"
	"os"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
	openpgpErrors "github.com/ProtonMail/go-crypto/openpgp/errors"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/collections"
)

type packageAuthenticationResult int

const (
	unauthenticated packageAuthenticationResult = iota
	verifiedChecksum
	signed
	signingSkipped
)

const (
	enforceGPGValidationEnvName = "OPENTOFU_ENFORCE_GPG_VALIDATION"
	enforceGPGExpirationEnvName = "OPENTOFU_ENFORCE_GPG_EXPIRATION"
)

// PackageAuthenticationResult is returned from a PackageAuthentication
// implementation. It is a mostly-opaque type intended for use in UI, which
// implements Stringer.
//
// A failed PackageAuthentication attempt will return an "unauthenticated"
// result, which is represented by nil.
type PackageAuthenticationResult struct {
	hashes HashDispositions
}

// NewPackageAuthenticationResult constructs a new [PackageAuthenticationResult]
// based on the given hash dispositions.
//
// This is here primarily to allow constructing expected result values for tests
// in other packages. There isn't really any reason for non-test code outside
// of this package to directly construct package authentication results, since
// all of the "real" package authentication implementations should live in this
// package.
func NewPackageAuthenticationResult(hashes HashDispositions) *PackageAuthenticationResult {
	return &PackageAuthenticationResult{hashes}
}

func (t *PackageAuthenticationResult) summaryResult() packageAuthenticationResult {
	if t == nil {
		return unauthenticated
	}
	signedCount := 0
	registryReportCount := 0
	locallyVerifiedCount := 0
	for _, disp := range t.hashes {
		if disp.SignedByAnyGPGKeys() {
			signedCount++
		}
		if disp.ReportedByRegistry {
			registryReportCount++
		}
		if disp.VerifiedLocally {
			locallyVerifiedCount++
		}
	}
	switch {
	case signedCount > 0:
		return signed
	case registryReportCount > 0:
		return signingSkipped
	case locallyVerifiedCount > 0:
		return verifiedChecksum
	default:
		return unauthenticated
	}
}

func (t *PackageAuthenticationResult) String() string {
	return map[packageAuthenticationResult]string{
		unauthenticated:  "unauthenticated",
		verifiedChecksum: "verified checksum",
		signingSkipped:   "signing skipped",
		signed:           "signed",
	}[t.summaryResult()]
}

// HashesWithDisposition returns a sequence of hashes whose disposition after
// authentication matches the rule implemented by the given function cond.
//
// Use this to select the appropriate subset of hashes to record for the
// associated provider version in the dependency lock file, with the selection
// condition varying based on the authentication result and the current
// policy for whether signature verification is required and which keys
// are trusted.
func (t *PackageAuthenticationResult) HashesWithDisposition(cond func(*HashDisposition) bool) iter.Seq[Hash] {
	if t == nil {
		// A nil result has no hashes at all
		return func(yield func(Hash) bool) {}
	}
	return func(yield func(Hash) bool) {
		for hash, disp := range t.hashes {
			if !cond(disp) {
				continue
			}
			if keepGoing := yield(hash); !keepGoing {
				break
			}
		}
	}
}

// GPGKeyIDsString returns a UI-oriented string representation of all of the
// GPG key IDs that asserted the validity of at least one of the hashes
// related to this package's provider version.
func (t *PackageAuthenticationResult) GPGKeyIDsString() string {
	return t.hashes.AllGPGSigningKeysString()
}

// Signed returns whether the package was authenticated as signed by anyone.
func (t *PackageAuthenticationResult) Signed() bool {
	if t == nil {
		return false
	}
	return t.hashes.HasAnySignedByGPGKeys()
}

// SigningSkipped returns whether the package was authenticated but the key
// validation was skipped.
func (t *PackageAuthenticationResult) SigningSkipped() bool {
	if t == nil {
		return false
	}
	return t.hashes.HasAnyReportedByRegistry()
}

// SigningKey represents a key used to sign packages from a registry. These are
// both in ASCII armored OpenPGP format.
//
// The JSON struct tags represent the field names used by the Registry API.
type SigningKey struct {
	ASCIIArmor string `json:"ascii_armor"`
}

// PackageAuthentication is an interface implemented by the optional package
// authentication implementations a source may include on its PackageMeta
// objects.
//
// A PackageAuthentication implementation is responsible for authenticating
// that a package is what its distributor intended to distribute and that it
// has not been tampered with.
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

type packageAuthenticationAll []PackageAuthentication

// PackageAuthenticationAll combines several authentications together into a
// single check value, which passes only if all of the given ones pass.
//
// The checks are processed in the order given, so a failure of an earlier
// check will prevent execution of a later one.
//
// The returned result is the union of the results of all authentications,
// describing all of the checksums that were somehow involved in the
// authentication process and what we learned about each one along the way.
func PackageAuthenticationAll(checks ...PackageAuthentication) PackageAuthentication {
	return packageAuthenticationAll(checks)
}

func (checks packageAuthenticationAll) AuthenticatePackage(localLocation PackageLocation) (*PackageAuthenticationResult, error) {
	authResult := &PackageAuthenticationResult{
		hashes: make(HashDispositions),
	}
	for _, check := range checks {
		thisAuthResult, err := check.AuthenticatePackage(localLocation)
		if err != nil {
			return nil, err
		}
		if thisAuthResult == nil {
			continue // this result has nothing to contribute to our overall result
		}
		authResult.hashes.Merge(thisAuthResult.hashes)
	}
	return authResult, nil
}

type packageHashAuthentication struct {
	RequiredHashes []Hash
	AllHashes      []Hash
	Platform       Platform
}

// NewPackageHashAuthentication returns a PackageAuthentication implementation
// that checks whether the contents of the package match whatever subset of the
// given hashes are considered acceptable by the current version of OpenTofu.
//
// This uses the hash algorithms implemented by functions PackageHash and
// MatchesHash. The PreferredHashes function will select which of the given
// hashes are considered by OpenTofu to be the strongest verification, and
// authentication succeeds as long as one of those matches.
func NewPackageHashAuthentication(platform Platform, validHashes []Hash) PackageAuthentication {
	requiredHashes := PreferredHashes(validHashes)
	return packageHashAuthentication{
		RequiredHashes: requiredHashes,
		AllHashes:      validHashes,
		Platform:       platform,
	}
}

func (a packageHashAuthentication) AuthenticatePackage(localLocation PackageLocation) (*PackageAuthenticationResult, error) {
	if len(a.RequiredHashes) == 0 {
		// Indicates that none of the hashes given to
		// NewPackageHashAuthentication were considered to be usable by this
		// version of OpenTofu.
		return nil, fmt.Errorf("this version of OpenTofu does not support any of the checksum formats given for this provider")
	}

	hashes := make(HashDispositions, len(a.RequiredHashes))
	for verifiedHash, err := range HashesMatchingPackage(localLocation, a.RequiredHashes) {
		if err != nil {
			return nil, fmt.Errorf("failed to verify provider package checksums: %w", err)
		}
		hashes[verifiedHash] = &HashDisposition{
			VerifiedLocally: true,
		}
	}

	if len(hashes) > 0 {
		return &PackageAuthenticationResult{hashes: hashes}, nil
	}
	if len(a.RequiredHashes) == 1 {
		return nil, fmt.Errorf("provider package doesn't match the expected checksum %q", a.RequiredHashes[0].String())
	}
	// It's non-ideal that this doesn't actually list the expected checksums,
	// but in the many-checksum case the message would get pretty unwieldy.
	// In practice today we typically use this authenticator only with a
	// single hash returned from a network mirror, so the better message
	// above will prevail in that case. Maybe we'll improve on this somehow
	// if the future introduction of a new hash scheme causes there to more
	// commonly be multiple hashes.
	return nil, fmt.Errorf("provider package doesn't match the any of the expected checksums")
}

type archiveHashAuthentication struct {
	Platform      Platform
	WantSHA256Sum [sha256.Size]byte
}

// NewArchiveChecksumAuthentication returns a PackageAuthentication
// implementation that checks that the original distribution archive matches
// the given hash.
//
// This authentication is suitable only for PackageHTTPURL and
// PackageLocalArchive source locations, because the unpacked layout
// (represented by PackageLocalDir) does not retain access to the original
// source archive. Therefore this authenticator will return an error if its
// given localLocation is not PackageLocalArchive.
//
// NewPackageHashAuthentication is preferable to use when possible because
// it uses the newer hashing scheme (implemented by function PackageHash) that
// can work with both packed and unpacked provider packages.
func NewArchiveChecksumAuthentication(platform Platform, wantSHA256Sum [sha256.Size]byte) PackageAuthentication {
	return archiveHashAuthentication{platform, wantSHA256Sum}
}

func (a archiveHashAuthentication) AuthenticatePackage(localLocation PackageLocation) (*PackageAuthenticationResult, error) {
	archiveLocation, ok := localLocation.(PackageLocalArchive)
	if !ok {
		// A source should not use this authentication type for non-archive
		// locations.
		return nil, fmt.Errorf("cannot check archive hash for non-archive location %s", localLocation)
	}

	gotHash, err := PackageHashLegacyZipSHA(archiveLocation)
	if err != nil {
		return nil, fmt.Errorf("failed to compute checksum for %s: %w", archiveLocation, err)
	}
	wantHash := HashLegacyZipSHAFromSHA(a.WantSHA256Sum)
	if gotHash != wantHash {
		return nil, fmt.Errorf("archive has incorrect checksum %s (expected %s)", gotHash, wantHash)
	}
	return &PackageAuthenticationResult{
		hashes: HashDispositions{
			gotHash: &HashDisposition{
				VerifiedLocally: true,
			},
		},
	}, nil
}

type matchingChecksumAuthentication struct {
	Document      []byte
	Filename      string
	WantSHA256Sum [sha256.Size]byte
}

// NewMatchingChecksumAuthentication returns a PackageAuthentication
// implementation that scans a registry-provided SHA256SUMS document for a
// specified filename, and compares the SHA256 hash against the expected hash.
// This is necessary to ensure that the signed SHA256SUMS document matches the
// declared SHA256 hash for the package, and therefore that a valid signature
// of this document authenticates the package.
//
// This authentication always returns a nil result, since it alone cannot offer
// any assertions about package integrity. It should be combined with other
// authentications to be useful.
func NewMatchingChecksumAuthentication(document []byte, filename string, wantSHA256Sum [sha256.Size]byte) PackageAuthentication {
	return matchingChecksumAuthentication{
		Document:      document,
		Filename:      filename,
		WantSHA256Sum: wantSHA256Sum,
	}
}

func (m matchingChecksumAuthentication) AuthenticatePackage(location PackageLocation) (*PackageAuthenticationResult, error) {
	// Find the checksum in the list with matching filename. The document is
	// in the form "0123456789abcdef filename.zip".
	filename := []byte(m.Filename)
	var checksum []byte
	for _, line := range bytes.Split(m.Document, []byte("\n")) {
		parts := bytes.Fields(line)
		if len(parts) > 1 && bytes.Equal(parts[1], filename) {
			checksum = parts[0]
			break
		}
	}
	if checksum == nil {
		return nil, fmt.Errorf("checksum list has no SHA-256 hash for %q", m.Filename)
	}

	// Decode the ASCII checksum into a byte array for comparison.
	var gotSHA256Sum [sha256.Size]byte
	if _, err := hex.Decode(gotSHA256Sum[:], checksum); err != nil {
		return nil, fmt.Errorf("checksum list has invalid SHA256 hash %q: %w", string(checksum), err)
	}

	// If the checksums don't match, authentication fails.
	if !bytes.Equal(gotSHA256Sum[:], m.WantSHA256Sum[:]) {
		return nil, fmt.Errorf("checksum list has unexpected SHA-256 hash %x (expected %x)", gotSHA256Sum, m.WantSHA256Sum[:])
	}

	// Success! But this doesn't result in any real authentication, only a
	// lack of authentication errors, so we return a nil result.
	return nil, nil
}

type signatureAuthentication struct {
	Document       []byte
	Signature      []byte
	Keys           []SigningKey
	ProviderSource addrs.Provider
	Meta           PackageMeta
}

// NewSignatureAuthentication returns a PackageAuthentication implementation
// that verifies the cryptographic signature for a package against any of the
// provided keys.
//
// The signing key for a package will be auto detected by attempting each key
// in turn until one is successful. If such a key is found, there are three
// possible successful authentication results:
//
//  1. If the signing key is the HashiCorp official key, it is an official
//     provider;
//  2. Otherwise, if the signing key has a trust signature from the HashiCorp
//     Partners key, it is a partner provider;
//  3. If neither of the above is true, it is a community provider.
//
// Any failure in the process of validating the signature will result in an
// unauthenticated result.
func NewSignatureAuthentication(meta PackageMeta, document, signature []byte, keys []SigningKey, source addrs.Provider) PackageAuthentication {
	return signatureAuthentication{
		Document:       document,
		Signature:      signature,
		Keys:           keys,
		ProviderSource: source,
		Meta:           meta,
	}
}

// ErrUnknownIssuer indicates an error when no valid signature for a provider could be found.
var ErrUnknownIssuer = fmt.Errorf("authentication signature from unknown issuer")

// ShouldEnforceGPGValidationForProvider returns true if GPG signature
// validation must be enforced for the given provider.
//
// OpenTofu requires a valid GPG signature for any provider for which this
// function returns true. The result of this function only applies if the
// provider's origin registry does not return any signing keys for the
// provider; GPG signature is always required for any provider whose
// origin registry returns a signing key.
//
// The situations where this function returns false are part of a pragmatic
// compromise to allow the main OpenTofu registry to serve providers for
// which it does not currently know a signing key. For more information,
// refer to:
//
//	https://github.com/opentofu/opentofu/issues/266
//
// The result of this function also determines whether signature
// verification is required in order for a particular hash to be tracked
// for the given provider in a dependency lock file. If this returns
// false then the dependency lock file should include any hash that
// was reported by the provider's origin registry, even if not signed.
func ShouldEnforceGPGValidationForProvider(addr addrs.Provider) bool {
	// GPG verification is always required for everything except the main
	// OpenTofu registry, since our possibility of skipping verification
	// is a concession to allow our official registry to distribute
	// providers that we don't have known private keys for, in which
	// case we're relying on the TLS certificate authentication of the
	// registry server as an alternative mechanism. (The registry
	// is ultimately what reports which keys would be valid anyway, so
	// if someone is able to compromise the connection to the registry
	// then they could arrange for it to report any signing key they like.)
	if addr.Hostname != addrs.DefaultProviderRegistryHost {
		return true
	}

	// For the primary registry we allow providers that don't have
	// GPG keys by default, but allow operators to opt out of this
	// special exception using an environment variable.
	enforceEnvVar, exists := os.LookupEnv(enforceGPGValidationEnvName)
	return exists && enforceEnvVar == "true"
}

func (s signatureAuthentication) shouldEnforceGPGValidation() bool {
	// If the registry returned at least one signing key then validation is always required.
	if len(s.Keys) > 0 {
		return true
	}

	// Otherwise the policy varies depending on what provider is being authenticated.
	return ShouldEnforceGPGValidationForProvider(s.ProviderSource)
}

func (s signatureAuthentication) shouldEnforceGPGExpiration() bool {
	// otherwise if the environment variable is set to true, we should enforce GPG expiration
	enforceEnvVar, exists := os.LookupEnv(enforceGPGExpirationEnvName)
	return exists && enforceEnvVar == "true"
}

func (s signatureAuthentication) AuthenticatePackage(location PackageLocation) (*PackageAuthenticationResult, error) {
	shouldValidate := s.shouldEnforceGPGValidation()

	var signingKeyIDs collections.Set[string]
	if shouldValidate {
		log.Printf("[DEBUG] Validating GPG signature of provider package %s", location)

		_, keyID, err := s.findSigningKey()
		if err != nil {
			return nil, fmt.Errorf("the provider is not signed with a valid signing key; please contact the provider author (%w)", err)
		}
		signingKeyIDs = collections.NewSet(keyID)
	} else {
		// As this is a temporary measure, we will log a warning to the user making it very clear what is happening
		// and why. This will be removed in a future release.
		log.Printf("[WARN] Skipping GPG validation of provider package %s as no keys were provided by the registry. See https://github.com/opentofu/opentofu/pull/309 for more information.", location)
	}

	// For each of the hashes mentioned in the document that was signed we'll announce that
	// it was reported by the provider's origin registry, since that's the only place that
	// this kind of signed hash file can come from, and _possibly_ report the key IDs that
	// signed it unless we decided above that validation wasn't actually needed.
	hashes := make(HashDispositions)
	for _, hash := range s.acceptableHashes() {
		hashes[hash] = &HashDisposition{
			ReportedByRegistry: true,
			SignedByGPGKeyIDs:  signingKeyIDs,
		}
	}
	return &PackageAuthenticationResult{hashes: hashes}, nil
}

func (s signatureAuthentication) acceptableHashes() []Hash {
	// This is a bit of an abstraction leak because signatureAuthentication
	// otherwise just treats the document as an opaque blob that's been
	// signed, but here we're making assumptions about its format because
	// we only want to trust that _all_ of the checksums are valid (rather
	// than just the current platform's one) if we've also verified that the
	// bag of checksums is signed.
	//
	// In recognition of that layering quirk this implementation is intended to
	// be somewhat resilient to potentially using this authenticator with
	// non-checksums files in future (in which case it'll return nothing at all)
	// but it might be better in the long run to instead combine
	// signatureAuthentication and matchingChecksumAuthentication together and
	// be explicit that the resulting merged authenticator is exclusively for
	// checksums files.

	var ret []Hash
	sc := bufio.NewScanner(bytes.NewReader(s.Document))
	for sc.Scan() {
		parts := bytes.Fields(sc.Bytes())
		if len(parts) != 0 && len(parts) < 2 {
			// Doesn't look like a valid sums file line, so we'll assume
			// this whole thing isn't a checksums file.
			return nil
		}

		// If this is a checksums file then the first part should be a
		// hex-encoded SHA256 hash, so it should be 64 characters long
		// and contain only hex digits.
		hashStr := parts[0]
		if len(hashStr) != 64 {
			return nil // doesn't look like a checksums file
		}

		var gotSHA256Sum [sha256.Size]byte
		if _, err := hex.Decode(gotSHA256Sum[:], hashStr); err != nil {
			return nil // doesn't look like a checksums file
		}

		ret = append(ret, HashLegacyZipSHAFromSHA(gotSHA256Sum))
	}

	return ret
}

// findSigningKey attempts to verify the signature using each of the keys
// returned by the registry. If a valid signature is found, it returns the
// signing key.
//
// Note: currently the registry only returns one key, but this may change in
// the future.
func (s signatureAuthentication) findSigningKey() (*SigningKey, string, error) {
	var expiredKey *SigningKey
	var expiredKeyID string

	for _, key := range s.Keys {
		keyCopy := key
		keyring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(key.ASCIIArmor))
		if err != nil {
			return nil, "", fmt.Errorf("error decoding signing key: %w", err)
		}

		entity, err := openpgp.CheckDetachedSignature(keyring, bytes.NewReader(s.Document), bytes.NewReader(s.Signature), nil)
		if errors.Is(err, openpgpErrors.ErrUnknownIssuer) {
			continue
		}

		if err != nil {
			// If in enforcing mode (or if the error isn’t related to expiry) return immediately.
			if !errors.Is(err, openpgpErrors.ErrKeyExpired) && !errors.Is(err, openpgpErrors.ErrSignatureExpired) {
				return nil, "", fmt.Errorf("error checking signature: %w", err)
			}

			// Else if it's an expired key then save it for later incase we don't find a non‐expired key.
			if expiredKey == nil {
				expiredKey = &keyCopy
				if entity != nil && entity.PrimaryKey != nil {
					expiredKeyID = entity.PrimaryKey.KeyIdString()
				} else {
					expiredKeyID = "n/a"
				}
			}
			continue
		}

		// Success! This key verified without an error.
		keyID := "n/a"
		if entity.PrimaryKey != nil {
			keyID = entity.PrimaryKey.KeyIdString()
		}
		log.Printf("[DEBUG] Provider signed by %s", entityString(entity))
		return &key, keyID, nil
	}

	// Warn only once when ALL keys are expired.
	if expiredKey != nil && !s.shouldEnforceGPGExpiration() {
		fmt.Printf("[WARN] Provider %s/%s (%v) gpg key expired, this will fail in future versions of OpenTofu\n",
			s.Meta.Provider.Namespace, s.Meta.Provider.Type, s.Meta.Provider.Hostname)
		return expiredKey, expiredKeyID, nil
	}

	// If we got here, no candidate was acceptable.
	return nil, "", ErrUnknownIssuer
}

// entityString extracts the key ID and identity name(s) from an openpgp.Entity
// for logging.
func entityString(entity *openpgp.Entity) string {
	if entity == nil {
		return ""
	}

	keyID := "n/a"
	if entity.PrimaryKey != nil {
		keyID = entity.PrimaryKey.KeyIdString()
	}

	var names []string
	for _, identity := range entity.Identities {
		names = append(names, identity.Name)
	}

	return fmt.Sprintf("%s %s", keyID, strings.Join(names, ", "))
}
