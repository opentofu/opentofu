// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"sort"

	"github.com/zclconf/go-cty/cty"
)

// marksEqual compares 2 unordered sets of PathValue marks for equality, with
// the comparison using the cty.PathValueMarks.Equal method.
func marksEqual(a, b []cty.PathValueMarks) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}

	if len(a) != len(b) {
		return false
	}

	less := func(s []cty.PathValueMarks) func(i, j int) bool {
		return func(i, j int) bool {
			// the sort only needs to be consistent, so use the GoString format
			// to get a comparable value
			return fmt.Sprintf("%#v", s[i]) < fmt.Sprintf("%#v", s[j])
		}
	}

	sort.Slice(a, less(a))
	sort.Slice(b, less(b))

	for i := 0; i < len(a); i++ {
		if !a[i].Equal(b[i]) {
			return false
		}
	}

	return true
}

func copyPathValueMarks(marks cty.PathValueMarks) cty.PathValueMarks {
	newMarks := make(cty.ValueMarks, len(marks.Marks))
	result := cty.PathValueMarks{Path: marks.Path}
	for k, v := range marks.Marks {
		newMarks[k] = v
	}
	result.Marks = newMarks
	return result
}

// combinePathValueMarks will combine the marks from two sets of marks with paths, ensuring that we don't duplicate marks
// for the same path, but instead combine the marks for the same path
// This ensures that we don't lose user marks when combining 2 different sets of marks for the same path
func combinePathValueMarks(marks []cty.PathValueMarks, other []cty.PathValueMarks) []cty.PathValueMarks {
	// skip some work if we don't have any marks in either of the lists
	if len(marks) == 0 {
		return other
	}
	if len(other) == 0 {
		return marks
	}

	combined := make([]cty.PathValueMarks, 0, len(marks))
	// construct the initial set of marks
	combined = append(combined, marks...)

	// check if we've already inserted this by looping over and calling .Equals().
	// This isn't so nice but there is no nice comparison for cty.PathValueMarks
	// so we have to do it this way
	for _, mark := range other {
		exists := false
		for i, existing := range combined {
			if mark.Path.Equals(existing.Path) {
				// if we found a matching path, we should combine the marks and update the existing item
				dupe := copyPathValueMarks(existing)
				for k, v := range mark.Marks {
					dupe.Marks[k] = v
				}
				combined[i] = dupe
				exists = true
				break
			}
		}
		// Otherwise we haven't seen this path before, so we should add it to the list
		// no merging required
		if !exists {
			combined = append(combined, mark)
		}
	}

	return combined
}
