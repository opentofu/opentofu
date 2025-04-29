// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package statestore

import (
	"errors"
	"fmt"
	"iter"
	"unique"

	"github.com/opentofu/opentofu/internal/collections"
)

// Key is an opaque token used to uniquely identify an object (or the potential
// for an object) in a [Storage].
//
// Key strings are guaranteed to include only the following characters:
// - Lowercase ASCII letters: a-z.
// - ASCII digits: 0-9.
// - The ASCII "hyphen-minus" character, "-".
//
// The zero value of this type is NOT a valid key, and a valid key must
// always include at least one hyphen-minus character.
//
// This limited vocabulary is a compromise to work within the key naming
// constraints of various different storage implementations, including
// systems that use case-insensitive key matching.
//
// In situations where OpenTofu needs to represent identifiers that exceed this
// alphabet, it will use the base32hex encoding as defined in [RFC 4648]
// section 7 and implemented in Go as [encoding/base32.HexEncoding], with
// padding disabled. However, that is only a guideline for internal
// implementation and not the specification of an external integration point;
// everything outside of OpenTofu Core must treat all keys as opaque, comparing
// them only for string equality.
type Key struct {
	// We use unique.Unique here to intern the strings so that we can compare
	// them by pointer rather than by content, e.g. when working with a set
	// of keys represented as a map.
	raw unique.Handle[string]
}

// NoKey is the zero value of [Key] and represents the absense of a key. This
// is NOT a valid Key value.
var NoKey Key

// MakeKey converts the given string into a [Key] without first checking whether
// it uses the valid alphabet. Use this only for key strings constructed by
// OpenTofu Core itself, which are guaranteed to be valid by construction.
func MakeKey(raw string) Key {
	if len(raw) == 0 { // this is a relatively-cheap check, at least
		panic("can't build statestore.Key from empty string")
	}
	return Key{raw: unique.Make(raw)}
}

// ParseKey verifies whether the given string conforms to the expected alphabet
// for [Key] values and returns the same string wrapped in one if so, or
// otherwise returns an error.
//
// This is intended only for handling keys loaded from the underlying store
// of a [Storage] implementation where someone might have introduced something
// invalid into the data store. Keys constructed inside OpenTofu itself should
// instead use a generation strategy that ensures that the alphabet will
// definitely be followed and then use [MakeKey].
func ParseKey(raw string) (Key, error) {
	// NOTE: All of the error paths in this function intentionally avoid
	// allocating anything on the heap so that callers can use this
	// function to filter a large list of candidate key names without
	// generating garbage for each one.
	if len(raw) == 0 {
		return Key{}, errKeyEmpty
	}
	foundHyphen := false
	for _, c := range raw {
		if !inKeyAlphabet(c) {
			return Key{}, errKeyInvalidChar(c)
		}
		if c == '-' {
			foundHyphen = true
		}
	}
	if !foundHyphen {
		// (this rule exists to ensure that a valid key name can never coincide
		// with one of the filenames that is reserved on Windows for use as
		// a legacy DOS-style device name, such as "nul", "con", etc, so that
		// all keys can be assumed to be valid filenames on all of our
		// supported platforms for the benefit of filesystem-based storage.)
		return Key{}, errKeyNoHyphen
	}
	return MakeKey(raw), nil
}

func (k Key) Name() string {
	return k.raw.Value()
}

func inKeyAlphabet(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-'
}

var errKeyEmpty = errors.New("state storage key must have at least one character")
var errKeyNoHyphen = errors.New("state storage key name must include at least one hyphen-minus character")

type errKeyInvalidChar rune

func (err errKeyInvalidChar) Error() string {
	return fmt.Sprintf("state storage key contains invalid character %q", rune(err))
}

// Value is an opaque byte sequence that can be associated with a [Key].
//
// A valid Value always has a length of at least one. A zero-length (typically
// nil) Value represents the absence of a value.
//
// The guarantee against zero-length blobs is a pragmatic concession to allow
// the use of empty object placeholders as part of a representation of a lock
// on a key that has not yet been created, when backed by a storage system that
// can only acquire locks for objects that already exist.
type Value []byte

// NoValue is the zero value of [Value] representing the absense of a value.
//
// A non-nil [Value] of length zero also represents the absence of a value,
// but implementers should avoid creating such a value to minimize risk of
// surprise and bugs.
var NoValue Value

// NormalizeValue translates a value loaded from outside of OpenTofu into
// a normalized form for internal use.
//
// Currently this just normalizes the "absence of a value" representation
// to always be [NoValue], rather than some non-nil zero-length [Value].
func NormalizeValue(given Value) Value {
	if len(given) == 0 {
		return NoValue
	}
	return given
}

// KeySet is a set of [Key].
type KeySet = collections.Set[Key]

// NewKeySet constructs a new, empty [KeySet].
func NewKeySet() KeySet {
	return collections.NewSet[Key]()
}

// CollectKeySet collects results from [Storage.Keys] into a [KeySet].
func CollectKeySet(items iter.Seq2[Key, error]) (KeySet, error) {
	ret := collections.NewSet[Key]()
	for key, err := range items {
		if err != nil {
			return nil, err
		}
		ret[key] = struct{}{}
	}
	return ret, nil
}
