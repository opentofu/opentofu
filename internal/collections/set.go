// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package collections

import (
	"fmt"
	"strings"

	"slices"
)

// Set is a container that can hold each item only once and has a fast lookup time.
//
// You can define a new set like this:
//
//	var validKeyLengths = collections.Set[int]{
//	    16: {},
//	    24: {},
//	    32: {},
//	}
//
// You can also use the constructor to create a new set
//
//	var validKeyLengths = collections.NewSet[int](16,24,32)
type Set[T comparable] map[T]struct{}

// Constructs a new set given the members of type T
func NewSet[T comparable](members ...T) Set[T] {
	set := Set[T]{}
	for _, member := range members {
		set[member] = struct{}{}
	}
	return set
}

// Has returns true if the item exists in the Set
func (s Set[T]) Has(value T) bool {
	_, ok := s[value]
	return ok
}

// String creates a comma-separated list of all values in the set.
func (s Set[T]) String() string {
	parts := make([]string, len(s))
	i := 0
	for v := range s {
		parts[i] = fmt.Sprintf("%v", v)
		i++
	}

	slices.SortStableFunc(parts, func(a, b string) int {
		if a < b {
			return -1
		} else if b > a {
			return 1
		} else {
			return 0
		}
	})
	return strings.Join(parts, ", ")
}
