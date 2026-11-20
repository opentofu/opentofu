// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package collections

import (
	"reflect"

	"github.com/opentofu/opentofu/internal/command/jsonformat/computed"

	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/plans/objchange"
)

type TransformIndices func(before, after int) computed.Diff
type ProcessIndices func(before, after int)
type IsComparableType[Input any] func(input Input) bool

// TransformSlice compares two slices and returns a slice of computed.Diff and the action that was taken for the entire slice.
// This function calls ProcessSlice to process the elements in the slices, which in turn uses the TransformIndices function to create the computed.Diff for each element based on their type.
// IsComparable argument is used to determine if before and after elements should be 'diffed' with each other instead of marking the old element as deleted.
// IsComparable argument is primarily useful to provide detailed differences for Object types and strings. It is called for each element in the both slices.
func TransformSlice[Input any](before, after []Input, process TransformIndices, isComparableType IsComparableType[Input]) ([]computed.Diff, plans.Action) {
	current := plans.NoOp
	if before != nil && after == nil {
		current = plans.Delete
	}
	if before == nil && after != nil {
		current = plans.Create
	}

	var elements []computed.Diff
	ProcessSlice(before, after, func(before, after int) {
		element := process(before, after)
		elements = append(elements, element)
		current = CompareActions(current, element.Action)
	}, isComparableType)
	return elements, current
}

// ProcessSlice compares two slices and returns a slice of computed.Diff, this function handles everything TransformSlice does, other than determining the operation.
// Uses TransformIndices function to create the computed.Diff for each element based on their type.
// ShouldDiffElement argument is used to determine if before and after elements should be 'diffed' with each other instead of marking the old element as deleted and the new element as created.
// ShouldDiffElement argument is primarily useful to provide detailed differences for Object types and strings with multiple lines.
func ProcessSlice[Input any](before, after []Input, process ProcessIndices, isComparableType IsComparableType[Input]) {
	lcs := objchange.LongestCommonSubsequence(before, after, func(before, after Input) bool {
		return reflect.DeepEqual(before, after)
	})

	var beforeIx, afterIx, lcsIx int
	for beforeIx < len(before) || afterIx < len(after) || lcsIx < len(lcs) {
		// Step through all the before values until we hit the next item in the
		// longest common subsequence. We are going to just say that all of
		// these have been deleted.
		for beforeIx < len(before) && (lcsIx >= len(lcs) || !reflect.DeepEqual(before[beforeIx], lcs[lcsIx])) {
			isObjectDiff := isComparableType(before[beforeIx]) && afterIx < len(after) && isComparableType(after[afterIx]) && (lcsIx >= len(lcs) || !reflect.DeepEqual(after[afterIx], lcs[lcsIx]))
			if isObjectDiff {
				process(beforeIx, afterIx)
				beforeIx++
				afterIx++
				continue
			}

			process(beforeIx, len(after))
			beforeIx++
		}

		// Now, step through all the after values until hit the next item in the
		// LCS. We are going to say that all of these have been created.
		for afterIx < len(after) && (lcsIx >= len(lcs) || !reflect.DeepEqual(after[afterIx], lcs[lcsIx])) {
			process(len(before), afterIx)
			afterIx++
		}

		// Finally, add the item in common as unchanged.
		if lcsIx < len(lcs) {
			process(beforeIx, afterIx)
			beforeIx++
			afterIx++
			lcsIx++
		}
	}
}
