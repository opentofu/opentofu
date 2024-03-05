package collections

import (
	"fmt"
	"strings"

	"golang.org/x/exp/slices"
)

// Set is a container that can hold each item only once and has a fast lookup time.
//
// You can define a new set like this:
//
//	var validKeyLengths = golang.Set[int]{
//	    16: {},
//	    24: {},
//	    32: {},
//	}
type Set[T comparable] map[T]struct{}

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
