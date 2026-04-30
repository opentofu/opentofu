// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0
package addrs

import (
	"fmt"
	"sync/atomic"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// OverrideTrie provides a step-wise method to obtain values
// for overridden resource. Each instance of the OverrideTrie
// represents one "hop" in the resource address, traversing
// through modules.
//
// A resource without a key, which is expected to have a key, may use
// either the Wildcard key "[*]" or no key at all, but not both. See Set
// and Get for more specific information on how that is handled.
type OverrideTrie[T any] struct {
	trie  map[InstanceKey]*OverrideTrie[T]
	value *T

	// usesModernAddresses is used in the root trie to track
	// whether a wildcard literal was ever used for the
	// index of an override
	usesModernAddresses bool

	// noKeyEvidenceMap, for each step, provides a list of address ranges used in overrides
	// where, at that step of the process, the instance key was NoKey. This is used
	// for error handling to provide user feedback on which addresses to fix.
	noKeyEvidenceMap [][]*hcl.Range
	// refuse is a boolean that, when set to true, refuses further queries.
	// This flag is set when there's a diagnostic is returned from
	// a Get request. This is to ensure only one diagnostic message is returned,
	// rather than one for each resource instance (which is pretty spammy).
	refuse atomic.Bool

	root *OverrideTrie[T]
}

// NewOverrideTrie creates a new trie for mapping override values to addresses.
//
// A default value is provided in this constructor.
// This is used when the address is not found in the trie, that is,
// it is not set as an override.
func NewOverrideTrie[T any]() *OverrideTrie[T] {
	return &OverrideTrie[T]{
		trie:  make(map[InstanceKey]*OverrideTrie[T]),
		value: nil,
	}
}

// Set takes an address and value and loads it into the OverrideTrie. Each
// instance key, for a module or resource, creates a trie, with the "leaf" trie
// containing the value for the address.
//
// The "NoKey" key is treated specially and made equivalent to WildCard. This is
// to provide backwards compatibility; before this was implemented, a non-instanced
// resource address was used to refer to every instance associated with the address.
//
// val is expected to be non-nil; it might complicate overrides if no value
// is provided but the resource is still considered "overridden"
//
// src is only used for diagnostic purposes, in the case that an override does not
// use a key where it ought to have a key set.
func (ot *OverrideTrie[T]) Set(addr *AbsResourceInstance, val T, src *hcl.Range) {
	current := ot
	for i, mod := range addr.Module {
		next, usesNoKey := ot.subSet(current, mod.InstanceKey)
		if usesNoKey {
			ot.SetNoKeyEvidence(i, addr, src)
		}
		ot.TrackModernAddressing(mod.InstanceKey)
		current = next
	}
	last, usesNoKey := ot.subSet(current, addr.Resource.Key)
	if usesNoKey {
		ot.SetNoKeyEvidence(len(addr.Module), addr, src)
	}
	ot.TrackModernAddressing(addr.Resource.Key)
	last.value = new(val)
}

func (ot *OverrideTrie[T]) TrackModernAddressing(key InstanceKey) {
	_, usesWildcard := key.(WildcardKey)
	ot.usesModernAddresses = ot.usesModernAddresses || usesWildcard
}

// SetNoKeyEvidence provides evidence that the NoKey instance key was used in a
// particular resource override. This is later used when getting a key; if
// this trie uses modern address syntax, but no key is used when a key is
// called for, this is how we obtain that evidence.
func (ot *OverrideTrie[T]) SetNoKeyEvidence(i int, addr *AbsResourceInstance, src *hcl.Range) {
	if ot.noKeyEvidenceMap == nil {
		ot.noKeyEvidenceMap = make([][]*hcl.Range, len(addr.Module)+1)
		for i := range len(addr.Module) + 1 {
			ot.noKeyEvidenceMap[i] = make([]*hcl.Range, 0)
		}
	}
	ot.noKeyEvidenceMap[i] = append(ot.noKeyEvidenceMap[i], src)
}

// subSet prepares one step in the module or resource chain
// as a trie of instance keys. It also provides evidence that a NoKey
// was used in one step of the override; if, during Get, a key is used,
// and the OverrideTrie is using "Modern Addresses Ranges" (i.e.
// override addresses with wildcards), we can return an error and call
// out the particular address.
func (ot *OverrideTrie[T]) subSet(current *OverrideTrie[T], key InstanceKey) (*OverrideTrie[T], bool) {
	usesNoKey := false
	if key == NoKey {
		key = WildcardKey{UnknownKeyType}
		usesNoKey = true
	}
	next, ok := current.trie[key]
	if !ok {
		current.trie[key] = NewOverrideTrie[T]()
		next = current.trie[key]
		next.root = ot
	}
	return next, usesNoKey
}

// Get returns the value in the OverrideTrie associated with the address. If part of the
// address is not found, but a WildCard address is set in the trie, that sub-trie is
// recursively searched against the query.
//
// If the address could not be found in the OverrideTrie as an override, Get returns nil.
//
// A wildcard instance key anywhere in the provided address will produce an error; it
// make sense to use this to obtain a single value when referencing a wildcard. Additionally,
// if the override addresses had no key in a module or resource where a key was expected,
// this method will also produce an error for that.
func (ot *OverrideTrie[T]) Get(addr *AbsResourceInstance) (*T, tfdiags.Diagnostics) {
	if ot.refuse.Load() {
		return nil, nil
	}
	n := len(addr.Module) + 1
	keyList := make([]InstanceKey, n)
	for i, mod := range addr.Module {
		keyList[i] = mod.InstanceKey
	}
	keyList[n-1] = addr.Resource.Key

	value, diags := ot.recursiveGet(0, n, keyList, addr.String())
	return value, diags
}
func (ot *OverrideTrie[T]) recursiveGet(i, n int, keyList []InstanceKey, addrString string) (*T, tfdiags.Diagnostics) {
	if ot == nil {
		return nil, nil
	}
	if i == n {
		return ot.value, nil
	}
	keyCheckDiags := ot.checkKey(i, keyList[i], addrString)
	if keyCheckDiags.HasErrors() {
		return nil, keyCheckDiags
	}
	value, diags := ot.trie[keyList[i]].recursiveGet(i+1, n, keyList, addrString)
	if diags.HasErrors() {
		return nil, diags
	}
	if value != nil {
		return value, diags
	}

	// Could not find against concrete instance key, try on Wildcard key
	return ot.trie[WildcardKey{UnknownKeyType}].recursiveGet(i+1, n, keyList, addrString)
}

func (ot *OverrideTrie[T]) checkKey(i int, key InstanceKey, addrString string) tfdiags.Diagnostics {
	if ot.root != nil && ot.root != ot {
		return ot.root.checkKey(i, key, addrString)
	}
	if _, usesWildcard := key.(WildcardKey); usesWildcard {
		return tfdiags.Diagnostics{
			tfdiags.Sourceless(
				tfdiags.Error,
				"Wildcard key not expected in when retrieving override values",
				fmt.Sprintf("In the provided resource address \"%s\", a wildcard accessor is used for the instance key. A specific key should always be used, where applicable.", addrString),
			),
		}
	}
	if ot.noKeyEvidenceMap == nil || !ot.usesModernAddresses {
		return nil
	}
	// check if NoKey is being used in a place it shouldn't
	// i.e. this key isn't NoKey, but the override was NoKey
	// at this step
	// Note: the Swap at the end prevents this error from being returned
	// more than once per resource. We rely on logic shortcutting for it
	// to not be executed when the first two conditions are not met.
	var diags tfdiags.Diagnostics
	if key != NoKey && len(ot.noKeyEvidenceMap[i]) > 0 && !ot.refuse.Swap(true) {
		for _, noKeyRange := range ot.noKeyEvidenceMap[i] {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Please switch overrides to wildcard syntax (i.e. \"[*]\") to refer to for-each resources",
				Detail:   fmt.Sprintf("When trying to override %s", addrString),
				Subject:  noKeyRange,
			})
		}
	}
	return diags
}
