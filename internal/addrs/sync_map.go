// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import "sync"

// SyncMap represents a concurrent mapping whose keys are address types that implement
// UniqueKeyer.
//
// Since not all address types are comparable in the Go language sense, this
// type cannot work with the typical Go map access syntax, and so instead has
// a method-based syntax. Use this type only for situations where the key
// type isn't guaranteed to always be a valid key for a standard Go map.
//
// This implementation is safe for concurrent use; however, we recommend
// you use the Map[K,V] type in this package instead for any workload
// that does not require concurrent safety when accessing the underlying
// map data structure.
type SyncMap[K UniqueKeyer, V any] struct {
	// elems is the internal data structure of the map.
	elems *sync.Map
}

func MakeSyncMap[K UniqueKeyer, V any](initialElems ...MapElem[K, V]) SyncMap[K, V] {
	ret := SyncMap[K, V]{elems: &sync.Map{}}
	for _, elem := range initialElems {
		ret.Put(elem.Key, elem.Value)
	}
	return ret
}

// Put inserts a new element into the map, or replaces an existing element
// which has an equivalent key.
func (m SyncMap[K, V]) Put(key K, value V) {
	realKey := key.UniqueKey()
	m.elems.Store(realKey, MapElem[K, V]{key, value})
}

// PutElement is like Put but takes the key and value from the given MapElement
// structure instead of as individual arguments.
func (m SyncMap[K, V]) PutElement(elem MapElem[K, V]) {
	m.Put(elem.Key, elem.Value)
}

// Remove deletes the element with the given key from the map, or does nothing
// if there is no such element.
func (m SyncMap[K, V]) Remove(key K) {
	realKey := key.UniqueKey()
	m.elems.Delete(realKey)
}

// Get returns the value of the element with the given key, or the zero value
// of V if there is no such element.
func (m SyncMap[K, V]) Get(key K) V {
	realKey := key.UniqueKey()
	elem, ok := m.elems.Load(realKey)
	if !ok {
		var ret V
		return ret
	}
	return elem.(MapElem[K, V]).Value
}

// GetOk is like Get but additionally returns a flag for whether there was an
// element with the given key present in the map.
func (m SyncMap[K, V]) GetOk(key K) (V, bool) {
	realKey := key.UniqueKey()
	elem, ok := m.elems.Load(realKey)
	if !ok {
		var ret V
		return ret, ok
	}
	return elem.(MapElem[K, V]).Value, ok
}

// Has returns true if and only if there is an element in the map which has the
// given key.
func (m SyncMap[K, V]) Has(key K) bool {
	realKey := key.UniqueKey()
	_, ok := m.elems.Load(realKey)
	return ok
}

// Range calls f sequentially for each key and value present in the map.
// If f returns false, range stops the iteration. Elements in the map
// are visited in an unpredictable order.
func (m SyncMap[K, V]) Range(f func(key K, value V) bool) {
	m.elems.Range(func(_, v any) bool {
		elem := v.(MapElem[K, V])
		return f(elem.Key, elem.Value)
	})
}

// keys returns a Set[K] containing a snapshot of the current keys of elements
// of the map. Do not use this outside of tests...
func (m SyncMap[K, V]) keys() Set[K] {
	ret := make(Set[K])
	m.Range(func(k K, _ V) bool {
		realKey := k.UniqueKey()
		ret[realKey] = k
		return true
	})
	return ret
}
