package addrs

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
	for _, mod := range addr.Module {
		current = ot.subSet(current, mod.InstanceKey)
	}
	last := ot.subSet(current, addr.Resource.Key)
	last.value = new(val)
}

func (ot *OverrideTrie[T]) subSet(current *OverrideTrie[T], key InstanceKey) *OverrideTrie[T] {
	if key == NoKey {
		key = WildcardKey{UnknownKeyType}
	}
	next, ok := current.trie[key]
	if !ok {
		current.trie[key] = NewOverrideTrie(ot.defaultVal)
		next = current.trie[key]
	}
	return next
}

// Get returns the value in the OverrideTrie associated with the address. If part of the
// address is not found, but a WildCard address is set in the trie, that sub-trie is
// then used to continue the query.
//
// If it could not be found in the OverrideTrie as an override, the default is used and
// the boolean is set to false to indicate it was not found
//
// A wildcard instance key is not expected anywhere in the provided address, nor does it
// make sense to use this to obtain a single value when referencing a wildcard.
// ^^^ TODO write a test where a wildcard address REFERENCES one of the overridden instances values
// ^^^ TODO in an output, for example, value = pets.cat[*].name or something
func (ot *OverrideTrie[T]) Get(addr *AbsResourceInstance) (T, bool) {
	current := ot
	for _, mod := range addr.Module {
		var ok bool
		current, ok = subGet(current, mod.InstanceKey)
		if !ok {
			return ot.defaultVal, false
		}
	}
	last, ok := subGet(current, addr.Resource.Key)
	if !ok || last.value == nil {
		return ot.defaultVal, false
	}
	return *last.value, true
}

func subGet[T any](current *OverrideTrie[T], key InstanceKey) (*OverrideTrie[T], bool) {
	next, ok := current.trie[key]
	if !ok {
		next, ok = current.trie[WildcardKey{UnknownKeyType}]
		if !ok {
			return nil, false
		}
	}
	return next, true
}
