// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"iter"
	"maps"
)

// Set represents a set of addresses of types that implement UniqueKeyer.
//
// Modify the set only by the methods on this type. This type exposes its
// internals for convenience during reading, such as iterating over set elements
// by ranging over the map values, but making direct modifications could
// potentially make the set data invalid or inconsistent, leading to undefined
// behavior elsewhere.
type Set[T UniqueKeyer] map[UniqueKey]T

func MakeSet[T UniqueKeyer](elems ...T) Set[T] {
	ret := Set[T](make(map[UniqueKey]T, len(elems)))
	for _, elem := range elems {
		ret.Add(elem)
	}
	return ret
}

// CollectSet produces a set containing all of the distinct values from the
// given sequence.
func CollectSet[T UniqueKeyer](seq iter.Seq[T]) Set[T] {
	ret := Set[T](make(map[UniqueKey]T))
	for elem := range seq {
		ret.Add(elem)
	}
	return ret
}

// Has returns true if and only if the set includes the given address.
func (s Set[T]) Has(addr T) bool {
	_, exists := s[addr.UniqueKey()]
	return exists
}

// All returns a sequence of all addresses in the set in a pseudorandom order.
func (s Set[T]) All() iter.Seq[T] {
	return maps.Values(s)
}

// Add inserts the given address into the set, if not already present. If
// an equivalent address is already in the set, this replaces that address
// with the new value.
func (s Set[T]) Add(addr T) {
	s[addr.UniqueKey()] = addr
}

// Remove deletes the given address from the set, if present. If not present,
// this is a no-op.
func (s Set[T]) Remove(addr T) {
	delete(s, addr.UniqueKey())
}

// Union returns a new set which contains the union of all of the elements
// of both the receiver and the given other set.
func (s Set[T]) Union(other Set[T]) Set[T] {
	ret := make(Set[T])
	for k, addr := range s {
		ret[k] = addr
	}
	for k, addr := range other {
		ret[k] = addr
	}
	return ret
}

// Intersection returns a new set which contains the intersection of all of the
// elements of both the receiver and the given other set.
func (s Set[T]) Intersection(other Set[T]) Set[T] {
	ret := make(Set[T])
	for k, addr := range s {
		if _, exists := other[k]; exists {
			ret[k] = addr
		}
	}
	for k, addr := range other {
		if _, exists := s[k]; exists {
			ret[k] = addr
		}
	}
	return ret
}
