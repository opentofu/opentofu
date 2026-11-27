// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package collections

import (
	"github.com/opentofu/opentofu/internal/command/jsonformat/computed"

	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/plans/objchange"
)

type TransformIndices func(before, after int) computed.Diff
type ProcessIndices func(before, after int)
type ShouldDiffElement[Input any] func(inputA, inputB Input) bool

// Equals reports whether two elements are the same element.
type Equals[Input any] func(before, after Input) bool

// TransformSlice compares two slices and returns a slice of computed.Diff and the action that was taken for the entire slice.
// This function calls ProcessSlice to process the elements in the slices, which in turn uses the TransformIndices function to create the computed.Diff for each element based on their type.
// ShouldDiffElement argument is used to determine if before and after elements should be 'diffed' with each other instead of marking the old element as deleted and the new element as created.
// ShouldDiffElement argument is primarily useful to provide detailed differences for Object types and strings with multiple lines. It is called for each element in the both slices.
// Equals argument is used to determine if two elements are the same element.
func TransformSlice[Input any](before, after []Input, process TransformIndices, shouldDiffElement ShouldDiffElement[Input], equals Equals[Input]) ([]computed.Diff, plans.Action) {
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
	}, shouldDiffElement, equals)
	return elements, current
}

// ProcessSlice compares two slices and returns a slice of computed.Diff, this function handles everything TransformSlice does, other than determining the operation.
// Uses TransformIndices function to create the computed.Diff for each element based on their type.
// ShouldDiffElement argument is used to determine if before and after elements should be 'diffed' with each other instead of marking the old element as deleted and the new element as created.
// ShouldDiffElement argument is primarily useful to provide detailed differences for Object types and strings with multiple lines.
// Equals argument is used to determine if two elements are the same element.
func ProcessSlice[Input any](before, after []Input, process ProcessIndices, shouldDiffElement ShouldDiffElement[Input], equals Equals[Input]) {
	lcs := objchange.LongestCommonSubsequence(before, after, equals)

	var beforeIx, afterIx, lcsIx int
	for beforeIx < len(before) || afterIx < len(after) || lcsIx < len(lcs) {
		// Step through all the before values until we hit the next item in the
		// longest common subsequence. We are going to just say that all of
		// these have been deleted.
		for beforeIx < len(before) && (lcsIx >= len(lcs) || !equals(before[beforeIx], lcs[lcsIx])) {
			diffElements := afterIx < len(after) && shouldDiffElement(before[beforeIx], after[afterIx])
			afterIsPartOfLCS := lcsIx < len(lcs) && equals(after[afterIx], lcs[lcsIx])
			// After the longest common sequence, we've reached elements in before that are different and want to determine whether or not to diff them
			// At this point, if the following becomes false, we will have two Changes in ChangeSlice, one for delete and one for create
			// if the expression is true, we are processing the two before and after elements as a single change of Update type.
			if diffElements && !afterIsPartOfLCS {
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
		for afterIx < len(after) && (lcsIx >= len(lcs) || !equals(after[afterIx], lcs[lcsIx])) {
			diffElements := beforeIx < len(before) && shouldDiffElement(before[beforeIx], after[afterIx])
			if diffElements {
				// If we are diffing the elements, we will process them as a single change of Update type.
				process(beforeIx, afterIx)
				beforeIx++
				afterIx++
				continue
			}
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
