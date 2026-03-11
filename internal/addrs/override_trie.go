package addrs

import (
	"fmt"

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
	trie       map[InstanceKey]*OverrideTrie[T]
	value      *T
	defaultVal T

	// usesModernAddresses is used in the root trie to track
	// whether a wildcard was ever used in an override
	usesModernAddresses bool

	// noKeyEvidenceMap, for each step, provides a list of address ranges used in overrides
	// where, at that step of the process, the instance key was NoKey. This is used
	// for error handling to provide user feedback on which addresses to fix.
	noKeyEvidenceMap [][]*AbsResourceInstance
}

// NewOverrideTrie creates a new trie for mapping override values to addresses.
//
// A default value is provided in this constructor.
// This is used when the address is not found in the trie, that is,
// it is not set as an override.
func NewOverrideTrie[T any](defaultVal T) *OverrideTrie[T] {
	return &OverrideTrie[T]{
		trie:       make(map[InstanceKey]*OverrideTrie[T]),
		value:      nil,
		defaultVal: defaultVal,
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
func (ot *OverrideTrie[T]) Set(addr *AbsResourceInstance, val T) {
	current := ot
	for i, mod := range addr.Module {
		next, usesNoKey := ot.subSet(current, mod.InstanceKey)
		if usesNoKey {
			ot.SetNoKeyEvidence(i, addr)
		}
		ot.TrackModernAddressing(mod.InstanceKey)
		current = next
	}
	last, usesNoKey := ot.subSet(current, addr.Resource.Key)
	if usesNoKey {
		ot.SetNoKeyEvidence(-1, addr)
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
//
// Use i = -1 for the final resource
func (ot *OverrideTrie[T]) SetNoKeyEvidence(i int, addr *AbsResourceInstance) {
	if ot.noKeyEvidenceMap == nil {
		ot.noKeyEvidenceMap = make([][]*AbsResourceInstance, len(addr.Module)+1)
		for i := range len(addr.Module) + 1 {
			ot.noKeyEvidenceMap[i] = make([]*AbsResourceInstance, 0)
		}
	}
	ot.noKeyEvidenceMap[i+1] = append(ot.noKeyEvidenceMap[i+1], addr)
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
		current.trie[key] = NewOverrideTrie(ot.defaultVal)
		next = current.trie[key]
	}
	return next, usesNoKey
}

// Get returns the value in the OverrideTrie associated with the address. If part of the
// address is not found, but a WildCard address is set in the trie, that sub-trie is
// then used to continue the query.
//
// If it could not be found in the OverrideTrie as an override, the default is used and
// the boolean is set to false to indicate it was not found
//
// A wildcard instance key anywhere in the provided address will produce an error; it
// make sense to use this to obtain a single value when referencing a wildcard. Additionally,
// if the override addresses had no key in a module or resource where a key was expected,
// this method will also produce an error for that.
func (ot *OverrideTrie[T]) Get(addr *AbsResourceInstance) (T, bool, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	current := ot
	for i, mod := range addr.Module {
		next, ok := ot.subGet(current, mod.InstanceKey)
		modDiags := ot.checkKey(i, mod.InstanceKey, addr)
		diags = diags.Append(modDiags)
		if !ok {
			return ot.defaultVal, false, diags
		}
		current = next
	}
	last, ok := ot.subGet(current, addr.Resource.Key)
	resDiags := ot.checkKey(-1, addr.Resource.Key, addr)
	diags = diags.Append(resDiags)
	if !ok || last.value == nil {
		return ot.defaultVal, false, diags
	}
	return *last.value, true, diags
}

func (ot *OverrideTrie[T]) subGet(current *OverrideTrie[T], key InstanceKey) (*OverrideTrie[T], bool) {
	next, ok := current.trie[key]
	if !ok {
		next, ok = current.trie[WildcardKey{UnknownKeyType}]
		if !ok {
			return nil, false
		}
	}
	return next, true
}

func (ot *OverrideTrie[T]) checkKey(i int, key InstanceKey, addr *AbsResourceInstance) tfdiags.Diagnostics {
	if _, usesWildcard := key.(WildcardKey); usesWildcard {
		return tfdiags.Diagnostics{
			tfdiags.Sourceless(
				tfdiags.Error,
				"Wildcard key not expected in when retrieving override values",
				fmt.Sprintf("In the provided resource address \"%s\", a wildcard accessor is used for the instance key. A specific key should always be used, where applicable.", addr.String()),
			),
		}
	}
	if ot.noKeyEvidenceMap == nil || !ot.usesModernAddresses {
		return nil
	}
	// check if NoKey is being used in a place it shouldn't
	// i.e. this key isn't NoKey, but the override was NoKey
	// at this step
	var diags tfdiags.Diagnostics
	if key != NoKey && len(ot.noKeyEvidenceMap[i+1]) > 0 {
		for _, noKeyAddr := range ot.noKeyEvidenceMap[i+1] {
			// TODO this results in a crazy amount of diagnostics...
			// Like, for every instance of every instance of every instance, and every override therein,
			// has an error output example. How do I avoid this? Hash on AbsResource or something?
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				// TODO I'm open to a less-verbose version of this summary
				"The override address cannot contain unkeyed for-each resources if it is also using the wildcard syntax (i.e. \"[*]\"). Please switch entirely to wildcard syntax for test overrides.",
				fmt.Sprintf("Using %s to override %s", noKeyAddr.String(), addr.String()),
			))
		}
	}
	return diags
}
