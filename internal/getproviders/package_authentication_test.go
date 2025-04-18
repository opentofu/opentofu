// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	openpgpErrors "github.com/ProtonMail/go-crypto/openpgp/errors"
	"github.com/google/go-cmp/cmp"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/collections"
)

func TestPackageAuthenticationResult(t *testing.T) {
	tests := map[string]struct {
		result *PackageAuthenticationResult
		want   string
	}{
		"nil": {
			nil,
			"unauthenticated",
		},
		"SignedByGPGKeyIDs": {
			&PackageAuthenticationResult{
				hashes: HashDispositions{
					Hash("test:placeholder"): {
						SignedByGPGKeyIDs: collections.NewSet("abc123"),
					},
				},
			},
			"signed",
		},
		"VerifiedLocally": {
			&PackageAuthenticationResult{
				hashes: HashDispositions{
					Hash("test:placeholder"): {
						VerifiedLocally: true,
					},
				},
			},
			"verified checksum",
		},
		"ReportedByRegistry": {
			&PackageAuthenticationResult{
				hashes: HashDispositions{
					Hash("test:placeholder"): {
						ReportedByRegistry: true,
					},
				},
			},
			"signing skipped",
		},
		"SignedByGPGKeyIDs+VerifiedLocally": {
			&PackageAuthenticationResult{
				hashes: HashDispositions{
					Hash("test:placeholder"): {
						SignedByGPGKeyIDs: collections.NewSet("abc123"),
						VerifiedLocally:   true,
					},
				},
			},
			"signed",
		},
		"SignedByGPGKeyIDs+ReportedByRegistry": {
			&PackageAuthenticationResult{
				hashes: HashDispositions{
					Hash("test:placeholder"): {
						SignedByGPGKeyIDs:  collections.NewSet("abc123"),
						ReportedByRegistry: true,
					},
				},
			},
			"signed",
		},
		"ReportedByRegistry+VerifiedLocally": {
			&PackageAuthenticationResult{
				hashes: HashDispositions{
					Hash("test:placeholder"): {
						ReportedByRegistry: true,
						VerifiedLocally:    true,
					},
				},
			},
			"signing skipped",
		},
		"SignedByGPGKeyIDs+ReportedByRegistry+VerifiedLocally": {
			&PackageAuthenticationResult{
				hashes: HashDispositions{
					Hash("test:placeholder"): {
						SignedByGPGKeyIDs:  collections.NewSet("abc123"),
						ReportedByRegistry: true,
						VerifiedLocally:    true,
					},
				},
			},
			"signed",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := test.result.String(); got != test.want {
				t.Errorf("wrong value\ngot:  %q\nwant: %q", got, test.want)
			}
		})
	}
}

// mockAuthentication is an implementation of the PackageAuthentication
// interface which returns fixed values. This is used to test the combining
// logic of PackageAuthenticationAll.
type mockAuthentication struct {
	hashes HashDispositions
	err    error
}

func (m mockAuthentication) AuthenticatePackage(localLocation PackageLocation) (*PackageAuthenticationResult, error) {
	if m.err == nil {
		return &PackageAuthenticationResult{hashes: m.hashes}, nil
	} else {
		return nil, m.err
	}
}

var _ PackageAuthentication = (*mockAuthentication)(nil)

// If all authentications succeed, the returned result is based on the merger
// of all of the individual results.
func TestPackageAuthenticationAll_success(t *testing.T) {
	result, err := PackageAuthenticationAll(
		&mockAuthentication{hashes: HashDispositions{
			Hash("test:a"): {
				VerifiedLocally: true,
			},
		}},
		&mockAuthentication{hashes: HashDispositions{
			Hash("test:a"): {
				SignedByGPGKeyIDs: collections.NewSet("abc123"),
			},
		}},
	).AuthenticatePackage(nil)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if got, want := result.String(), "signed"; got != want {
		t.Errorf("wrong result summary\ngot:  %s\nwant: %s", got, want)
	}
}

// If an authentication fails, its error should be returned along with a nil
// result.
func TestPackageAuthenticationAll_failure(t *testing.T) {
	someError := errors.New("some error")
	result, err := PackageAuthenticationAll(
		&mockAuthentication{hashes: HashDispositions{
			Hash("test:a"): {
				VerifiedLocally: true,
			},
		}},
		&mockAuthentication{err: someError},
		&mockAuthentication{hashes: HashDispositions{
			Hash("test:a"): {
				SignedByGPGKeyIDs: collections.NewSet("abc123"),
			},
		}},
	).AuthenticatePackage(nil)

	if result != nil {
		t.Errorf("wrong result: got %#v, want nil", result)
	}
	if err != someError {
		t.Errorf("wrong error\ngot:  %s\nwant %s", err, someError)
	}
}

// Package hash authentication requires a zip file or directory fixture and a
// known-good set of hashes, of which the authenticator will pick one. The
// result should be "verified checksum".
func TestPackageHashAuthentication_success(t *testing.T) {
	// Location must be a PackageLocalArchive path
	location := PackageLocalDir("testdata/filesystem-mirror/registry.opentofu.org/hashicorp/null/2.0.0/linux_amd64")

	wantHashes := []Hash{
		// Known-good HashV1 result for this directory
		Hash("h1:qjsREM4DqEWECD43FcPqddZ9oxCG+IaMTxvWPciS05g="),
	}

	auth := NewPackageHashAuthentication(Platform{"linux", "amd64"}, wantHashes)
	result, err := auth.AuthenticatePackage(location)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if got, want := result.String(), "verified checksum"; got != want {
		t.Errorf("wrong result summary\ngot:  %s\nwant: %s", got, want)
	}
}

// Package has authentication can fail for various reasons.
func TestPackageHashAuthentication_failure(t *testing.T) {
	tests := map[string]struct {
		location PackageLocation
		err      string
	}{
		"missing file": {
			PackageLocalArchive("testdata/no-package-here.zip"),
			"failed to verify provider package checksums: lstat testdata/no-package-here.zip: no such file or directory",
		},
		"checksum mismatch": {
			PackageLocalDir("testdata/filesystem-mirror/registry.opentofu.org/hashicorp/null/2.0.0/linux_amd64"),
			"provider package doesn't match the expected checksum \"h1:invalid\"",
		},
		"invalid zip file": {
			PackageLocalArchive("testdata/filesystem-mirror/registry.opentofu.org/hashicorp/null/terraform-provider-null_2.1.0_linux_amd64.zip"),
			"failed to verify provider package checksums: zip: not a valid zip file",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Invalid expected hash, either because we'll error before we
			// reach it, or we want to force a checksum mismatch.
			auth := NewPackageHashAuthentication(Platform{"linux", "amd64"}, []Hash{"h1:invalid"})
			result, err := auth.AuthenticatePackage(test.location)

			if result != nil {
				t.Errorf("wrong result: got %#v, want nil", result)
			}
			if gotErr := err.Error(); gotErr != test.err {
				t.Errorf("wrong err: got %q, want %q", gotErr, test.err)
			}
		})
	}
}

// Archive checksum authentication requires a file fixture and a known-good
// SHA256 hash. The result should be "verified checksum".
func TestArchiveChecksumAuthentication_success(t *testing.T) {
	// Location must be a PackageLocalArchive path
	location := PackageLocalArchive("testdata/filesystem-mirror/registry.opentofu.org/hashicorp/null/terraform-provider-null_2.1.0_linux_amd64.zip")

	// Known-good SHA256 hash for this archive
	wantSHA256Sum := [sha256.Size]byte{
		0x4f, 0xb3, 0x98, 0x49, 0xf2, 0xe1, 0x38, 0xeb,
		0x16, 0xa1, 0x8b, 0xa0, 0xc6, 0x82, 0x63, 0x5d,
		0x78, 0x1c, 0xb8, 0xc3, 0xb2, 0x59, 0x01, 0xdd,
		0x5a, 0x79, 0x2a, 0xde, 0x97, 0x11, 0xf5, 0x01,
	}

	auth := NewArchiveChecksumAuthentication(Platform{"linux", "amd64"}, wantSHA256Sum)
	result, err := auth.AuthenticatePackage(location)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if got, want := result.String(), "verified checksum"; got != want {
		t.Errorf("wrong result summary\ngot:  %s\nwant: %s", got, want)
	}
}

// Archive checksum authentication can fail for various reasons. These test
// cases are almost exhaustive, missing only an io.Copy error which is
// difficult to induce.
func TestArchiveChecksumAuthentication_failure(t *testing.T) {
	tests := map[string]struct {
		location PackageLocation
		err      string
	}{
		"missing file": {
			PackageLocalArchive("testdata/no-package-here.zip"),
			"failed to compute checksum for testdata/no-package-here.zip: lstat testdata/no-package-here.zip: no such file or directory",
		},
		"checksum mismatch": {
			PackageLocalArchive("testdata/filesystem-mirror/registry.opentofu.org/hashicorp/null/terraform-provider-null_2.1.0_linux_amd64.zip"),
			"archive has incorrect checksum zh:4fb39849f2e138eb16a18ba0c682635d781cb8c3b25901dd5a792ade9711f501 (expected zh:0000000000000000000000000000000000000000000000000000000000000000)",
		},
		"invalid location": {
			PackageLocalDir("testdata/filesystem-mirror/tfe.example.com/AwesomeCorp/happycloud/0.1.0-alpha.2/darwin_amd64"),
			"cannot check archive hash for non-archive location testdata/filesystem-mirror/tfe.example.com/AwesomeCorp/happycloud/0.1.0-alpha.2/darwin_amd64",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Zero expected checksum, either because we'll error before we
			// reach it, or we want to force a checksum mismatch
			auth := NewArchiveChecksumAuthentication(Platform{"linux", "amd64"}, [sha256.Size]byte{0})
			result, err := auth.AuthenticatePackage(test.location)

			if result != nil {
				t.Errorf("wrong result: got %#v, want nil", result)
			}
			if gotErr := err.Error(); gotErr != test.err {
				t.Errorf("wrong err: got %q, want %q", gotErr, test.err)
			}
		})
	}
}

// Matching checksum authentication takes a SHA256SUMS document, an archive
// filename, and an expected SHA256 hash. On success both return values should
// be nil.
func TestMatchingChecksumAuthentication_success(t *testing.T) {
	// Location is unused
	location := PackageLocalArchive("testdata/my-package.zip")

	// Two different checksums for other files
	wantSHA256Sum := [sha256.Size]byte{0xde, 0xca, 0xde}
	otherSHA256Sum := [sha256.Size]byte{0xc0, 0xff, 0xee}

	document := []byte(
		fmt.Sprintf(
			"%x README.txt\n%x my-package.zip\n",
			otherSHA256Sum,
			wantSHA256Sum,
		),
	)
	filename := "my-package.zip"

	auth := NewMatchingChecksumAuthentication(document, filename, wantSHA256Sum)
	result, err := auth.AuthenticatePackage(location)

	// NOTE: This also tests the expired key ignore logic as they key in the test is expired
	if result != nil {
		t.Errorf("wrong result: got %#v, want nil", result)
	}
	if err != nil {
		t.Errorf("wrong err: got %s, want nil", err)
	}
}

// Matching checksum authentication can fail for three reasons: no checksum
// in the document for the filename, invalid checksum value, and non-matching
// checksum value.
func TestMatchingChecksumAuthentication_failure(t *testing.T) {
	wantSHA256Sum := [sha256.Size]byte{0xde, 0xca, 0xde}
	filename := "my-package.zip"

	tests := map[string]struct {
		document []byte
		err      string
	}{
		"no checksum for filename": {
			[]byte(
				fmt.Sprintf(
					"%x README.txt",
					[sha256.Size]byte{0xbe, 0xef},
				),
			),
			`checksum list has no SHA-256 hash for "my-package.zip"`,
		},
		"invalid checksum": {
			[]byte(
				fmt.Sprintf(
					"%s README.txt\n%s my-package.zip",
					"horses",
					"chickens",
				),
			),
			`checksum list has invalid SHA256 hash "chickens": encoding/hex: invalid byte: U+0068 'h'`,
		},
		"checksum mismatch": {
			[]byte(
				fmt.Sprintf(
					"%x README.txt\n%x my-package.zip",
					[sha256.Size]byte{0xbe, 0xef},
					[sha256.Size]byte{0xc0, 0xff, 0xee},
				),
			),
			"checksum list has unexpected SHA-256 hash c0ffee0000000000000000000000000000000000000000000000000000000000 (expected decade0000000000000000000000000000000000000000000000000000000000)",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Location is unused
			location := PackageLocalArchive("testdata/my-package.zip")

			auth := NewMatchingChecksumAuthentication(test.document, filename, wantSHA256Sum)
			result, err := auth.AuthenticatePackage(location)

			if result != nil {
				t.Errorf("wrong result: got %#v, want nil", result)
			}
			if gotErr := err.Error(); gotErr != test.err {
				t.Errorf("wrong err: got %q, want %q", gotErr, test.err)
			}
		})
	}
}

// Signature authentication takes a checksum document, a signature, and a list
// of signing keys. If the document is signed by one of the given keys, the
// authentication is successful. The value of the result depends on the signing
// key.
func TestSignatureAuthentication_success(t *testing.T) {
	// To make it easier for us to make changes to the constants we use
	// to test this process we hard-code only the data that is to be signed
	// and then dynamically generate a signing key and associated signature
	// on each test run. In realistic use the signature would be generated
	// in the provider's release process and OpenTofu would have access only
	// to the public part of the GPG key.
	pgpEntity := pgpTestEntity(t, "TestSignatureAuthentication_success")
	publicKeyArmor, keyID := pgpPublicKeyForTestEntity(t, pgpEntity)
	signature := pgpSignForTesting(t, []byte(testShaSumsRealistic), pgpEntity)
	t.Logf("generated PGP key %s\n%s", keyID, publicKeyArmor)
	t.Logf("generated signature\n%x", signature)

	// The following are the hashes included in testShaSumsRealistic, which
	// should therefore be reported in a successful result based on that input.
	wantHashes := []Hash{
		Hash("zh:086119a26576d06b8281a97e8644380da89ce16197cd955f74ea5ee664e9358b"),
		Hash("zh:0e9fd0f3e2254b526a0e81e0cfdfc82583b0cd343778c53ead21aa7d52f776d7"),
		Hash("zh:17e0b496022bc4e4137be15e96d2b051c8acd6e14cb48d9b13b262330464f6cc"),
		Hash("zh:1e5f7a5f3ade7b8b1d1d59c5cea2e1a2f8d2f8c3f41962dbbe8647e222be8239"),
		Hash("zh:2696c86228f491bc5425561c45904c9ce39b1c676b1e17734cb2ee6b578c4bcd"),
		Hash("zh:48f1826ec31d6f104e46cc2022b41f30cd1019ef48eaec9697654ef9ec37a879"),
		Hash("zh:66a947e7de1c74caf9f584c3ed4e91d2cb1af6fe5ce8abaf1cf8f7ff626a09d1"),
		Hash("zh:7d7e888fdd28abfe00894f9055209b9eec785153641de98e6852aa071008d4ee"),
		Hash("zh:a5ba9945606bb7bfb821ba303957eeb40dd9ee4e706ba8da1eaf7cbeb0356e63"),
		Hash("zh:def1b73849bec0dc57a04405847921bf9206c75b52ae9de195476facb26bd85e"),
		Hash("zh:df3a5a8d6ffff7bacf19c92d10d0d500f98169ea17b3764b01a789f563d1aad7"),
		Hash("zh:f8b6cf9ade087c17826d49d89cef21261cdc22bd27065bbc5b27d7dbf7fbbf6c"),
	}

	tests := map[string]struct {
		signature  []byte
		keys       []SigningKey
		wantResult string
		wantKeyID  string
		wantHashes []Hash
	}{
		"validly-signed provider": {
			signature,
			[]SigningKey{
				{ASCIIArmor: string(publicKeyArmor)},
			},
			"signed",
			keyID,
			wantHashes,
		},
		"multiple signing keys": {
			signature,
			[]SigningKey{
				{ASCIIArmor: anotherPublicKey},
				{ASCIIArmor: string(publicKeyArmor)},
			},
			"signed",
			keyID,
			wantHashes,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			location := PackageLocalArchive("testdata/my-package.zip")

			auth := NewSignatureAuthentication(
				PackageMeta{Location: location},
				[]byte(testShaSumsRealistic),
				test.signature,
				test.keys,
				addrs.NewDefaultProvider("test"),
			)
			result, err := auth.AuthenticatePackage(location)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if got, want := result.String(), test.wantResult; got != want {
				t.Errorf("wrong result\ngot:  %s\nwant: %s", got, want)
			}
			if got, want := result.GPGKeyIDsString(), test.wantKeyID; got != want {
				t.Errorf("wrong GPG key IDs string\ngot:  %s\nwant: %s", got, want)
			}

			gotHashes := slices.Collect(result.HashesWithDisposition(func(hd *HashDisposition) bool {
				return hd.SignedByGPGKeyIDs.Has(test.wantKeyID)
			}))
			sort.Slice(gotHashes, func(i, j int) bool {
				return gotHashes[i].String() < gotHashes[j].String()
			})
			if diff := cmp.Diff(test.wantHashes, gotHashes); diff != "" {
				t.Error("wrong hashes\n" + diff)
			}
		})
	}
}

func TestNewSignatureAuthentication_success(t *testing.T) {
	tests := map[string]struct {
		signature  string
		keys       []SigningKey
		wantResult string
		wantKeyID  string
	}{
		"official provider": {
			testHashicorpSignatureGoodBase64,
			[]SigningKey{
				{
					ASCIIArmor: TestingPublicKey,
				},
			},
			"signed",
			testHashiCorpPublicKeyID,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Location is unused
			location := PackageLocalArchive("testdata/my-package.zip")

			signature, err := base64.StdEncoding.DecodeString(test.signature)
			if err != nil {
				t.Fatal(err)
			}

			auth := NewSignatureAuthentication(
				PackageMeta{Location: location},
				[]byte(testProviderShaSums),
				signature,
				test.keys,
				addrs.NewDefaultProvider("test"),
			)
			result, err := auth.AuthenticatePackage(location)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if got, want := result.String(), test.wantResult; got != want {
				t.Errorf("wrong result\ngot:  %s\nwant: %s", got, want)
			}
			if got, want := result.GPGKeyIDsString(), test.wantKeyID; got != want {
				t.Errorf("wrong GPG key IDs string\ngot:  %s\nwant: %s", got, want)
			}
		})
	}
}
func TestNewSignatureAuthentication_expired(t *testing.T) {
	tests := map[string]struct {
		signature string
		keys      []SigningKey
	}{
		"official provider": {
			testHashicorpSignatureGoodBase64,
			[]SigningKey{
				{
					ASCIIArmor: TestingPublicKey,
				},
			},
		},
	}
	t.Setenv(enforceGPGExpirationEnvName, "true")

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Location is unused
			location := PackageLocalArchive("testdata/my-package.zip")

			signature, err := base64.StdEncoding.DecodeString(test.signature)
			if err != nil {
				t.Fatal(err)
			}

			auth := NewSignatureAuthentication(
				PackageMeta{Location: location},
				[]byte(testProviderShaSums),
				signature,
				test.keys,
				addrs.NewDefaultProvider("test"),
			)
			_, err = auth.AuthenticatePackage(location)

			if err == nil {
				t.Errorf("wrong err: got %s, want %s", err, openpgpErrors.ErrKeyExpired)
			}
		})
	}
	t.Setenv(enforceGPGExpirationEnvName, "")
}

// Signature authentication can fail for many reasons, most of which are due
// to OpenPGP failures from malformed keys or signatures.
func TestSignatureAuthentication_failure(t *testing.T) {
	tests := map[string]struct {
		signature    string
		keys         []SigningKey
		errorType    any
		errorMessage string
	}{
		"invalid key": {
			testHashicorpSignatureGoodBase64,
			[]SigningKey{
				{
					ASCIIArmor: "invalid PGP armor value",
				},
			},
			openpgpErrors.InvalidArgumentError(""),
			"no armored data found",
		},
		"invalid signature": {
			testSignatureBadBase64,
			[]SigningKey{
				{
					ASCIIArmor: testAuthorKeyArmor,
				},
			},
			openpgpErrors.InvalidArgumentError(""),
			"signature subpacket truncated",
		},
		"no keys match signature": {
			testAuthorSignatureGoodBase64,
			[]SigningKey{
				{
					ASCIIArmor: TestingPublicKey,
				},
			},
			nil,
			ErrUnknownIssuer.Error(),
		},
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			// Location is unused
			location := PackageLocalArchive("testdata/my-package.zip")

			signature, err := base64.StdEncoding.DecodeString(test.signature)
			if err != nil {
				t.Fatal(err)
			}

			auth := NewSignatureAuthentication(
				PackageMeta{Location: location},
				[]byte(testShaSumsPlaceholder),
				signature,
				test.keys,
				addrs.NewDefaultProvider("test"),
			)
			result, err := auth.AuthenticatePackage(location)

			if result != nil {
				t.Errorf("wrong result: got %#v, want nil", result)
			}
			if test.errorType != nil {
				if err == nil {
					t.Errorf("expected error of type %v, got nil", test.errorType)
				}
				if !errors.As(err, &test.errorType) {
					t.Errorf("wrong error type: got %v, want %v", err, test.errorType)
				}
			}
			if test.errorMessage != "" {
				if err == nil {
					t.Errorf("expected error of type %v, got nil", test.errorType)
				}
				if !strings.Contains(err.Error(), test.errorMessage) {
					t.Errorf("wrong error message: %s (expected an error message containing %s)", err.Error(), test.errorMessage)
				}
			}
		})
	}
}

const testAuthorKeyID = `37A6AB3BCF2C170A`

// testAuthorKeyArmor is test key ID 5BFEEC4317E746008621970637A6AB3BCF2C170A.
const testAuthorKeyArmor = `-----BEGIN PGP PUBLIC KEY BLOCK-----

mQENBF5vhgYBCAC40OcC2hEx3yGiLhHMbt7DAVEQ0nZwAWy6oL98niknLumBa1VO
nMYshP+o/FKOFatBl8aXhmDo606P6pD9d4Pg/WNehqT7hGNHcAFlm+8qjQAvE5uX
Z/na/Np7dmWasCiL5hYyHEnKU/XFpc9KyicbkS7n8igP1LEb8xDD1pMLULQsQHA4
258asvtwjoYTZIij1I6bUE178bGFPNCfj+FzQM8nKzPpDVxZ7njN9c2sB9FEdJ1+
S9mZQNK5PbJuEAOpD5Jp9BnGE16jsLUhDmvGHBjFZAXMBkNSloEMHhs2ty9lEzoF
eJmJx7XCGw+ds1SWp4MsHQPWzXxAlrfa4GMlABEBAAG0R1RlcnJhZm9ybSBUZXN0
aW5nIChwbHVnaW4vZGlzY292ZXJ5LykgPHRlcnJhZm9ybSt0ZXN0aW5nQGhhc2hp
Y29ycC5jb20+iQFOBBMBCAA4FiEEW/7sQxfnRgCGIZcGN6arO88sFwoFAl5vhgYC
GwMFCwkIBwIGFQoJCAsCBBYCAwECHgECF4AACgkQN6arO88sFwpWvQf/apaMu4Bm
ea8AGjdl9acQhHBpWsyiHLIfZvN11xxN/f3+YITvPXIe2PMgveqNfXxu6PIeZGDb
0DBvnBQy/vqmA+sCQ8t8+kIWdfZ1EeM2YcXdmAEtriooLvc85JFYjafLIKSj9N7o
V/R/e1BCW/v1/7Je47c+6FSt3HHhwyT5AZ3BCq1zpw6PeCDSQ/gZr3Mvq4CjeLA/
K+8TM3KyOF4qBGDvzGzp/t9umQSS2L0ozd90lxJtf5Q8ozqDaBiDo+f/osXT2EvN
VwPP/xh/gABkXiNrPylFbeD+XPAC4N7NmYK5aPDzRYXXknP8e9PDMykoJKZ+bSdz
F3IZ4q5RDHmmNbkBDQReb4YGAQgAt15e1F8TPQQm1jK8+scypHgfmPHbp7Qsulo1
GTcUd8QmhbR4kayuLDEpJYzq6+IoTM4TPqsdVuq/1Nwey9oyK0wXk/SUR29nRIQh
3GBg7JVg1YsObsfVTvEflYOdjk8T/Udqs4I6HnmSbtzsaohzybutpWXPUkW8OzFI
ATwfVTrrz70Yxs+ly0nSEH2Yf+kg2uYZvv5KsJ3MNENhXnHnlaTy2IfhsxAX0xOG
pa9fXV3NzdEbl0mYaEzMi77qRAyIQ9VrIL5F0yY/LlbpLSl6xk2+BB2v3a1Ey6SJ
w4/le6AM0wlH2hKPCTlkvM0IvUWjlzrPzCkeu027iVc+fqdyiQARAQABiQE2BBgB
CAAgFiEEW/7sQxfnRgCGIZcGN6arO88sFwoFAl5vhgYCGwwACgkQN6arO88sFwqz
nAf/eF4oZG9F8sJX01mVdDm/L7Uthe4xjTdl7jwV4ygNX+pCyWrww3qc3qbd3QKg
CFqIt/TAPE/OxHxCFuxalQefpOqfxjKzvcktxzWmpgxaWsvHaXiS4bKBPz78N/Ke
MUtcjGHyLeSzYPUfjquqDzQxqXidRYhyHGSy9c0NKZ6wCElLZ6KcmCQb4sZxVwfu
ssjwAFbPMp1nr0f5SWCJfhTh7QF7lO2ldJaKMlcBM8aebmqFQ52P7ZWOFcgeerng
G7Zdrci1KEd943HhzDCsUFz4gJwbvUyiAYb2ddndpUBkYwCB/XrHWPOSnGxHgZoo
1gIqed9OV/+s5wKxZPjL0pCStQ==
=mYqJ
-----END PGP PUBLIC KEY BLOCK-----`

// testAuthorEccKeyArmor uses Curve 25519 and has test key ID D01ED5C4BB1ED36A014B0D376540DDA046E5E135
const testAuthorEccKeyArmor = `-----BEGIN PGP PUBLIC KEY BLOCK-----

mDMEY1B7+hYJKwYBBAHaRw8BAQdAFRDpASP+iDY+QotOBP9DF5CfuhSBD8Dl0hSG
D7plEsO0M1RlcnJhZm9ybSBUZXN0aW5nIDx0ZXJyYWZvcm0rdGVzdGluZ0BoYXNo
aWNvcnAuY29tPoiTBBMWCgA7FiEE0B7VxLse02oBSw03ZUDdoEbl4TUFAmNQe/oC
GwMFCwkIBwICIgIGFQoJCAsCBBYCAwECHgcCF4AACgkQZUDdoEbl4TWhwwD+N/BR
pR9NhRFDm+JRhA3saKmpTSRo9yJnr6tRlumE4KQA/A2cOCDeezf6t3SXltoYUKIt
EYmbLxgMDlffVkFyC8IMuDgEY1B7+hIKKwYBBAGXVQEFAQEHQJ7frE76Le1qI1Go
dfrVIzEgAcYjDW6T01/V95wgqPIuAwEIB4h4BBgWCgAgFiEE0B7VxLse02oBSw03
ZUDdoEbl4TUFAmNQe/oCGwwACgkQZUDdoEbl4TWvsAD/YSQAigAH5hq4OmK4gs0J
O74RFokGZzbPtoIvutb8eYoA/1QxxyqE/8A4Z21azYEO0j563LRa8SkZcB5UPDy3
7ngJ
=Xb0o
-----END PGP PUBLIC KEY BLOCK-----`

// testShaSumsPlaceholder is a string that represents a signed document that
// the signature authenticator will check. Some of the signature values in
// other constants in this file are signing this string.
const testShaSumsPlaceholder = "example shasums data"

// testShaSumsRealistic is a more realistic SHA256SUMS document that we can use
// to test the AcceptableHashes method. The signature values in other constants
// in this file do not sign this string.
const testShaSumsRealistic = `7d7e888fdd28abfe00894f9055209b9eec785153641de98e6852aa071008d4ee  terraform_0.14.0-alpha20200923_darwin_amd64.zip
f8b6cf9ade087c17826d49d89cef21261cdc22bd27065bbc5b27d7dbf7fbbf6c  terraform_0.14.0-alpha20200923_freebsd_386.zip
a5ba9945606bb7bfb821ba303957eeb40dd9ee4e706ba8da1eaf7cbeb0356e63  terraform_0.14.0-alpha20200923_freebsd_amd64.zip
df3a5a8d6ffff7bacf19c92d10d0d500f98169ea17b3764b01a789f563d1aad7  terraform_0.14.0-alpha20200923_freebsd_arm.zip
086119a26576d06b8281a97e8644380da89ce16197cd955f74ea5ee664e9358b  terraform_0.14.0-alpha20200923_linux_386.zip
1e5f7a5f3ade7b8b1d1d59c5cea2e1a2f8d2f8c3f41962dbbe8647e222be8239  terraform_0.14.0-alpha20200923_linux_amd64.zip
0e9fd0f3e2254b526a0e81e0cfdfc82583b0cd343778c53ead21aa7d52f776d7  terraform_0.14.0-alpha20200923_linux_arm.zip
66a947e7de1c74caf9f584c3ed4e91d2cb1af6fe5ce8abaf1cf8f7ff626a09d1  terraform_0.14.0-alpha20200923_openbsd_386.zip
def1b73849bec0dc57a04405847921bf9206c75b52ae9de195476facb26bd85e  terraform_0.14.0-alpha20200923_openbsd_amd64.zip
48f1826ec31d6f104e46cc2022b41f30cd1019ef48eaec9697654ef9ec37a879  terraform_0.14.0-alpha20200923_solaris_amd64.zip
17e0b496022bc4e4137be15e96d2b051c8acd6e14cb48d9b13b262330464f6cc  terraform_0.14.0-alpha20200923_windows_386.zip
2696c86228f491bc5425561c45904c9ce39b1c676b1e17734cb2ee6b578c4bcd  terraform_0.14.0-alpha20200923_windows_amd64.zip`

// testAuthorSignatureGoodBase64 is a signature of testShaSums signed with
// testAuthorKeyArmor, which represents the SHA256SUMS.sig file downloaded for
// a release.
const testAuthorSignatureGoodBase64 = `iQEzBAABCAAdFiEEW/7sQxfnRgCGIZcGN6arO88s` +
	`FwoFAl5vh7gACgkQN6arO88sFwrAlQf6Al77qzjxNIj+NQNJfBGYUE5jHIgcuWOs1IPRTYUI` +
	`rHQIUU2RVrdHoAefKTKNzGde653JK/pYTflSV+6ini3/aZZnXlF6t001w3wswmakdwTr0hXx` +
	`Ez/hHYio72Gpn7+T/L+nl6dKkjeGqd/Kor5x2TY9uYB737ESmAe5T8ZlPaGMFHh0mYlNTeRq` +
	`4qIKqL6DwddBF4Ju2svn2MeNMGfE358H31mxAl2k4PPrwBTR1sFUCUOzAXVA/g9Ov5Y9ni2G` +
	`rkTahBtV9yuUUd1D+oRTTTdP0bj3A+3xxXmKTBhRuvurydPTicKuWzeILIJkcwp7Kl5UbI2N` +
	`n1ayZdaCIw/r4w==`

// testSignatureBadBase64 is an invalid signature.
const testSignatureBadBase64 = `iQEzBAABCAAdFiEEW/7sQxfnRgCGIZcGN6arO88s` +
	`4qIKqL6DwddBF4Ju2svn2MeNMGfE358H31mxAl2k4PPrwBTR1sFUCUOzAXVA/g9Ov5Y9ni2G` +
	`rkTahBtV9yuUUd1D+oRTTTdP0bj3A+3xxXmKTBhRuvurydPTicKuWzeILIJkcwp7Kl5UbI2N` +
	`n1ayZdaCIw/r4w==`

// testHashiCorpPublicKeyID is the Key ID of the HashiCorpPublicKey.
const testHashiCorpPublicKeyID = `34365D9472D7468F`

const testProviderShaSums = `fea4227271ebf7d9e2b61b89ce2328c7262acd9fd190e1fd6d15a591abfa848e  terraform-provider-null_3.1.0_darwin_amd64.zip
9ebf4d9704faba06b3ec7242c773c0fbfe12d62db7d00356d4f55385fc69bfb2  terraform-provider-null_3.1.0_darwin_arm64.zip
a6576c81adc70326e4e1c999c04ad9ca37113a6e925aefab4765e5a5198efa7e  terraform-provider-null_3.1.0_freebsd_386.zip
5f9200bf708913621d0f6514179d89700e9aa3097c77dac730e8ba6e5901d521  terraform-provider-null_3.1.0_freebsd_amd64.zip
fc39cc1fe71234a0b0369d5c5c7f876c71b956d23d7d6f518289737a001ba69b  terraform-provider-null_3.1.0_freebsd_arm.zip
c797744d08a5307d50210e0454f91ca4d1c7621c68740441cf4579390452321d  terraform-provider-null_3.1.0_linux_386.zip
53e30545ff8926a8e30ad30648991ca8b93b6fa496272cd23b26763c8ee84515  terraform-provider-null_3.1.0_linux_amd64.zip
cecb6a304046df34c11229f20a80b24b1603960b794d68361a67c5efe58e62b8  terraform-provider-null_3.1.0_linux_arm64.zip
e1371aa1e502000d9974cfaff5be4cfa02f47b17400005a16f14d2ef30dc2a70  terraform-provider-null_3.1.0_linux_arm.zip
a8a42d13346347aff6c63a37cda9b2c6aa5cc384a55b2fe6d6adfa390e609c53  terraform-provider-null_3.1.0_windows_386.zip
02a1675fd8de126a00460942aaae242e65ca3380b5bb192e8773ef3da9073fd2  terraform-provider-null_3.1.0_windows_amd64.zip
`

// testHashicorpSignatureGoodBase64 is a signature of testProviderShaSums signed with
// HashicorpPublicKey, which represents the SHA256SUMS.sig file downloaded for
// an official release.
const testHashicorpSignatureGoodBase64 = `wsFcBAABCAAQBQJgga+GCRCwtEEJdoW2dgAA` +
	`o0YQAAW911BGDr2WHLo5NwcZenwHyxL5DX9g+4BknKbc/WxRC1hD8Afi3eygZk1yR6eT4Gp2H` +
	`yNOwCjGL1PTONBumMfj9udIeuX8onrJMMvjFHh+bORGxBi4FKr4V3b2ZV1IYOjWMEyyTGRDvw` +
	`SCdxBkp3apH3s2xZLmRoAj84JZ4KaxGF7hlT0j4IkNyQKd2T5cCByN9DV80+x+HtzaOieFwJL` +
	`97iyGj6aznXfKfslK6S4oIrVTwyLTrQbxSxA0LsdUjRPHnJamL3sFOG77qUEUoXG3r61yi5vW` +
	`V4P5gCH/+C+VkfGHqaB1s0jHYLxoTEXtwthe66MydDBPe2Hd0J12u9ppOIeK3leeb4uiixWIi` +
	`rNdpWyjr/LU1KKWPxsDqMGYJ9TexyWkXjEpYmIEiY1Rxar8jrLh+FqVAhxRJajjgSRu5pZj50` +
	`CNeKmmbyolLhPCmICjYYU/xKPGXSyDFqonVVyMWCSpO+8F38OmwDQHIk5AWyc8hPOAZ+g5N95` +
	`cfUAzEqlvmNvVHQIU40Y6/Ip2HZzzFCLKQkMP1aDakYHq5w4ZO/ucjhKuoh1HDQMuMnZSu4eo` +
	`2nMTBzYZnUxwtROrJZF1t103avbmP2QE/GaPvLIQn7o5WMV3ZcPCJ+szzzby7H2e33WIynrY/` +
	`95ensBxh7mGFbcQ1C59b5o7viwIaaY2`

// entityString function is used for logging the signing key.
func TestEntityString(t *testing.T) {
	var tests = []struct {
		name     string
		entity   *openpgp.Entity
		expected string
	}{
		{
			"nil",
			nil,
			"",
		},
		{
			"testAuthorEccKeyArmor",
			testReadArmoredEntity(t, testAuthorEccKeyArmor),
			"6540DDA046E5E135 Terraform Testing <terraform+testing@hashicorp.com>",
		},
		{
			"testAuthorKeyArmor",
			testReadArmoredEntity(t, testAuthorKeyArmor),
			"37A6AB3BCF2C170A Terraform Testing (plugin/discovery/) <terraform+testing@hashicorp.com>",
		},
		{
			"HashicorpPublicKey",
			testReadArmoredEntity(t, TestingPublicKey),
			"34365D9472D7468F HashiCorp Security (hashicorp.com/security) <security@hashicorp.com>",
		},
		{
			"HashicorpPartnersKey",
			testReadArmoredEntity(t, anotherPublicKey),
			"7D72D4268E4660FC HashiCorp Security (Terraform Partner Signing) <security+terraform@hashicorp.com>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := entityString(tt.entity)
			if actual != tt.expected {
				t.Errorf("expected %s, actual %s", tt.expected, actual)
			}
		})
	}
}

func testReadArmoredEntity(t *testing.T, armor string) *openpgp.Entity {
	data := strings.NewReader(armor)

	el, err := openpgp.ReadArmoredKeyRing(data)
	if err != nil {
		t.Fatal(err)
	}

	if count := len(el); count != 1 {
		t.Fatalf("expected 1 entity, got %d", count)
	}

	return el[0]
}

func TestShouldEnforceGPGValidation(t *testing.T) {
	tests := []struct {
		name           string
		providerSource addrs.Provider
		keys           []SigningKey
		envVarValue    string
		expected       bool
	}{
		{
			name: "default provider registry, no keys",
			providerSource: addrs.Provider{
				Hostname: addrs.DefaultProviderRegistryHost,
			},
			keys:        []SigningKey{},
			envVarValue: "",
			expected:    false,
		},
		{
			name: "default provider registry, some keys",
			providerSource: addrs.Provider{
				Hostname: addrs.DefaultProviderRegistryHost,
			},
			keys: []SigningKey{
				{
					ASCIIArmor: testAuthorKeyArmor,
				},
			},
			envVarValue: "",
			expected:    true,
		},
		{
			name: "non-default provider registry, no keys",
			providerSource: addrs.Provider{
				Hostname: "my-registry.com",
			},
			keys:        []SigningKey{},
			envVarValue: "",
			expected:    true,
		},
		{
			name: "non-default provider registry, some keys",
			providerSource: addrs.Provider{
				Hostname: "my-registry.com",
			},
			keys: []SigningKey{
				{
					ASCIIArmor: testAuthorKeyArmor,
				},
			},
			envVarValue: "",
			expected:    true,
		},
		// env var "true"
		{
			name: "default provider registry, no keys, env var true",
			providerSource: addrs.Provider{
				Hostname: addrs.DefaultProviderRegistryHost,
			},
			keys:        []SigningKey{},
			envVarValue: "true",
			expected:    true,
		},
		{
			name: "default provider registry, some keys, env var true",
			providerSource: addrs.Provider{
				Hostname: addrs.DefaultProviderRegistryHost,
			},
			keys: []SigningKey{
				{
					ASCIIArmor: testAuthorKeyArmor,
				},
			},
			envVarValue: "true",
			expected:    true,
		}, {
			name: "non-default provider registry, no keys, env var true",
			providerSource: addrs.Provider{
				Hostname: "my-registry.com",
			},
			keys:        []SigningKey{},
			envVarValue: "true",
			expected:    true,
		},
		{
			name: "non-default provider registry, some keys, env var true",
			providerSource: addrs.Provider{
				Hostname: "my-registry.com",
			},
			keys: []SigningKey{
				{
					ASCIIArmor: testAuthorKeyArmor,
				},
			},
			envVarValue: "true",
			expected:    true,
		},
		// env var "false"
		{
			name: "default provider registry, no keys, env var false",
			providerSource: addrs.Provider{
				Hostname: addrs.DefaultProviderRegistryHost,
			},
			keys:        []SigningKey{},
			envVarValue: "false",
			expected:    false,
		},
		{
			name: "default provider registry, some keys, env var false",
			providerSource: addrs.Provider{
				Hostname: addrs.DefaultProviderRegistryHost,
			},
			keys: []SigningKey{
				{
					ASCIIArmor: testAuthorKeyArmor,
				},
			},
			envVarValue: "false",
			expected:    true,
		}, {
			name: "non-default provider registry, no keys, env var false",
			providerSource: addrs.Provider{
				Hostname: "my-registry.com",
			},
			keys:        []SigningKey{},
			envVarValue: "false",
			expected:    true,
		},
		{
			name: "non-default provider registry, some keys, env var false",
			providerSource: addrs.Provider{
				Hostname: "my-registry.com",
			},
			keys: []SigningKey{
				{
					ASCIIArmor: testAuthorKeyArmor,
				},
			},
			envVarValue: "false",
			expected:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			sigAuth := signatureAuthentication{
				ProviderSource: tt.providerSource,
				Keys:           tt.keys,
			}

			if tt.envVarValue != "" {
				t.Setenv(enforceGPGValidationEnvName, tt.envVarValue)
			}

			actual := sigAuth.shouldEnforceGPGValidation()
			if actual != tt.expected {
				t.Errorf("expected %t, actual %t", tt.expected, actual)
			}
		})
	}
}
