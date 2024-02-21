package collections

import (
	"fmt"
	"strings"
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

// Has returns true
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
	}
	return strings.Join(parts, ", ")
}
