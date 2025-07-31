// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"maps"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/collections"
)

func TestParseHash(t *testing.T) {
	tests := []struct {
		Input   string
		Want    Hash
		WantErr string
	}{
		{
			Input: "h1:foo",
			Want:  HashScheme1.New("foo"),
		},
		{
			Input: "zh:bar",
			Want:  HashSchemeZip.New("bar"),
		},
		{
			// A scheme we don't know is considered valid syntax, it just won't match anything.
			Input: "unknown:baz",
			Want:  HashScheme("unknown:").New("baz"),
		},
		{
			// A scheme with an empty value is weird, but allowed.
			Input: "unknown:",
			Want:  HashScheme("unknown:").New(""),
		},
		{
			Input:   "",
			WantErr: "hash string must start with a scheme keyword followed by a colon",
		},
		{
			// A naked SHA256 hash in hex format is not sufficient
			Input:   "1e5f7a5f3ade7b8b1d1d59c5cea2e1a2f8d2f8c3f41962dbbe8647e222be8239",
			WantErr: "hash string must start with a scheme keyword followed by a colon",
		},
		{
			// An empty scheme is not allowed
			Input:   ":blah",
			WantErr: "hash string must start with a scheme keyword followed by a colon",
		},
	}

	for _, test := range tests {
		t.Run(test.Input, func(t *testing.T) {
			got, err := ParseHash(test.Input)

			if test.WantErr != "" {
				if err == nil {
					t.Fatalf("want error: %s", test.WantErr)
				}
				if got, want := err.Error(), test.WantErr; got != want {
					t.Fatalf("wrong error\ngot:  %s\nwant: %s", got, want)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %s", err.Error())
			}

			if got != test.Want {
				t.Errorf("wrong result\ngot:  %#v\nwant: %#v", got, test.Want)
			}
		})
	}
}

func TestMergeHashDisposition(t *testing.T) {
	tests := map[string]struct {
		a, b *HashDisposition
		want *HashDisposition
	}{
		"empties": {
			a:    &HashDisposition{},
			b:    &HashDisposition{},
			want: &HashDisposition{},
		},
		"empty with VerifiedLocally": {
			a: &HashDisposition{
				VerifiedLocally: true,
			},
			b: &HashDisposition{},
			want: &HashDisposition{
				VerifiedLocally: true,
			},
		},
		"empty with ReportedByRegistry": {
			a: &HashDisposition{
				ReportedByRegistry: true,
			},
			b: &HashDisposition{},
			want: &HashDisposition{
				ReportedByRegistry: true,
			},
		},
		"empty with one GPG key": {
			a: &HashDisposition{
				SignedByGPGKeyIDs: collections.NewSet("abc123"),
			},
			b: &HashDisposition{},
			want: &HashDisposition{
				SignedByGPGKeyIDs: collections.NewSet("abc123"),
			},
		},
		"many GPG keys": {
			a: &HashDisposition{
				SignedByGPGKeyIDs: collections.NewSet("abc123", "def456"),
			},
			b: &HashDisposition{
				SignedByGPGKeyIDs: collections.NewSet("def456", "ghi789"),
			},
			want: &HashDisposition{
				SignedByGPGKeyIDs: collections.NewSet("abc123", "def456", "ghi789"),
			},
		},
		"VerifiedLocally with ReportedByRegistry": {
			a: &HashDisposition{
				ReportedByRegistry: true,
			},
			b: &HashDisposition{
				VerifiedLocally: true,
			},
			want: &HashDisposition{
				ReportedByRegistry: true,
				VerifiedLocally:    true,
			},
		},
		"VerifiedLocally with itself": {
			a: &HashDisposition{
				VerifiedLocally: true,
			},
			b: &HashDisposition{
				VerifiedLocally: true,
			},
			want: &HashDisposition{
				VerifiedLocally: true,
			},
		},
		"ReportedByRegistry with itself": {
			a: &HashDisposition{
				ReportedByRegistry: true,
			},
			b: &HashDisposition{
				ReportedByRegistry: true,
			},
			want: &HashDisposition{
				ReportedByRegistry: true,
			},
		},
		"Everything at once": {
			a: &HashDisposition{
				SignedByGPGKeyIDs:  collections.NewSet("def456", "ghi789"),
				ReportedByRegistry: true,
			},
			b: &HashDisposition{
				VerifiedLocally: true,
			},
			want: &HashDisposition{
				SignedByGPGKeyIDs:  collections.NewSet("def456", "ghi789"),
				ReportedByRegistry: true,
				VerifiedLocally:    true,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// MergeHashDisposition is supposed to be commutative, so
			// we'll test each case in both orders and expect an
			// equivalent result in each case.
			t.Run("a,b", func(t *testing.T) {
				got := MergeHashDisposition(test.a, test.b)
				if diff := cmp.Diff(test.want, got); diff != "" {
					t.Error("wrong result\n" + diff)
				}
			})
			t.Run("b,a", func(t *testing.T) {
				got := MergeHashDisposition(test.b, test.a)
				if diff := cmp.Diff(test.want, got); diff != "" {
					t.Error("wrong result\n" + diff)
				}
			})
		})
	}
}

func TestHashDispositionsMerge(t *testing.T) {
	// HashDispositions.Merge delegates to MergeHashDisposition when both
	// arguments refer to the same hash. We already have lots of tests for
	// MergeHashDisposition in TestMergeHashDisposition, so this test
	// intentionally does not duplicate all of those cases and focuses
	// only on the different cases that HashDispositions.Merge is directly
	// concernd with.

	tests := map[string]struct {
		a, b HashDispositions
		want HashDispositions
	}{
		"empties": {
			a:    HashDispositions{},
			b:    HashDispositions{},
			want: HashDispositions{},
		},
		"one into empty": {
			a: HashDispositions{},
			b: HashDispositions{
				Hash("test:foo"): &HashDisposition{
					ReportedByRegistry: true,
				},
			},
			want: HashDispositions{
				Hash("test:foo"): &HashDisposition{
					ReportedByRegistry: true,
				},
			},
		},
		"independent hashes": {
			a: HashDispositions{
				Hash("test:foo"): &HashDisposition{
					ReportedByRegistry: true,
				},
			},
			b: HashDispositions{
				Hash("test:bar"): &HashDisposition{
					VerifiedLocally: true,
				},
			},
			want: HashDispositions{
				Hash("test:foo"): &HashDisposition{
					ReportedByRegistry: true,
				},
				Hash("test:bar"): &HashDisposition{
					VerifiedLocally: true,
				},
			},
		},
		"overlapping hashes": {
			a: HashDispositions{
				Hash("test:foo"): &HashDisposition{
					ReportedByRegistry: true,
				},
			},
			b: HashDispositions{
				Hash("test:foo"): &HashDisposition{
					VerifiedLocally: true,
				},
			},
			want: HashDispositions{
				// This should be the result of MergeHashDispositions
				// on the two different entries for test:foo.
				Hash("test:foo"): &HashDisposition{
					ReportedByRegistry: true,
					VerifiedLocally:    true,
				},
			},
		},
		"mix of overlapping and independent": {
			a: HashDispositions{
				Hash("test:foo"): &HashDisposition{
					SignedByGPGKeyIDs: collections.NewSet("abc123"),
				},
				Hash("test:bar"): &HashDisposition{
					ReportedByRegistry: true,
				},
			},
			b: HashDispositions{
				Hash("test:foo"): &HashDisposition{
					SignedByGPGKeyIDs: collections.NewSet("def456"),
				},
				Hash("test:baz"): &HashDisposition{
					VerifiedLocally: true,
				},
			},
			want: HashDispositions{
				Hash("test:foo"): &HashDisposition{
					SignedByGPGKeyIDs: collections.NewSet("abc123", "def456"),
				},
				Hash("test:bar"): &HashDisposition{
					ReportedByRegistry: true,
				},
				Hash("test:baz"): &HashDisposition{
					VerifiedLocally: true,
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// HashDispositions.Merge is supposed to be commutative,
			// so we'll test each case in both orders and expect an
			// equivalent result in each case.
			t.Run("a,b", func(t *testing.T) {
				// We'll make a shallow copy of test.a so that we
				// aren't directly modifying the test table, since
				// otherwise we'll pollute the input to the
				// opposite order test below.
				got := maps.Clone(test.a)
				got.Merge(test.b)
				if diff := cmp.Diff(test.want, got); diff != "" {
					t.Error("wrong result\n" + diff)
				}
			})
			t.Run("b,a", func(t *testing.T) {
				got := maps.Clone(test.b)
				got.Merge(test.a)
				if diff := cmp.Diff(test.want, got); diff != "" {
					t.Error("wrong result\n" + diff)
				}
			})
		})
	}
}
