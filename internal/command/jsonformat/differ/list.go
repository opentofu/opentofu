// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package differ

import (
	"github.com/opentofu/opentofu/internal/command/jsonformat/collections"
	"github.com/opentofu/opentofu/internal/command/jsonformat/computed"
	"github.com/opentofu/opentofu/internal/command/jsonformat/computed/renderers"
	"github.com/opentofu/opentofu/internal/command/jsonformat/jsondiff"
	"github.com/opentofu/opentofu/internal/command/jsonformat/structured"
	"github.com/opentofu/opentofu/internal/command/jsonformat/structured/attribute_path"
	"github.com/opentofu/opentofu/internal/command/jsonprovider"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/zclconf/go-cty/cty"
)

func computeAttributeDiffAsList(change structured.Change, elementType cty.Type) computed.Diff {
	sliceValue := change.AsSlice()

	processIndices := func(beforeIx, afterIx int) computed.Diff {
		value := sliceValue.GetChild(beforeIx, afterIx)

		// It's actually really difficult to render the diffs when some indices
		// within a slice are relevant and others aren't. To make this simpler
		// we just treat all children of a relevant list or set as also
		// relevant.
		//
		// Interestingly the tofu plan builder also agrees with this, and
		// never sets relevant attributes beneath lists or sets. We're just
		// going to enforce this logic here as well. If the collection is
		// relevant (decided elsewhere), then every element in the collection is
		// also relevant. To be clear, in practice even if we didn't do the
		// following explicitly the effect would be the same. It's just nicer
		// for us to be clear about the behaviour we expect.
		//
		// What makes this difficult is the fact that the beforeIx and afterIx
		// can be different, and it's quite difficult to work out which one is
		// the relevant one. For nested lists, block lists, and tuples it's much
		// easier because we always process the same indices in the before and
		// after.
		value.RelevantAttributes = attribute_path.AlwaysMatcher()

		return ComputeDiffForType(value, elementType)
	}

	// This callback is used to determine if we should diff the elements in the slices instead of marking them as deleted and created.
	shouldDiffElement := func(a, b interface{}) bool {
		return elementType.IsObjectType() || jsondiff.ShouldDiffMultilineStrings(a, b)
	}
	elements, current := collections.TransformSlice(sliceValue.Before, sliceValue.After, processIndices, shouldDiffElement)
	return computed.NewDiff(renderers.List(elements), current, change.ReplacePaths.Matches())
}

func computeAttributeDiffAsNestedList(change structured.Change, attributes map[string]*jsonprovider.Attribute) computed.Diff {
	var elements []computed.Diff
	current := change.GetDefaultActionForIteration()
	processNestedList(change, func(value structured.Change) {
		element := computeDiffForNestedAttribute(value, &jsonprovider.NestedType{
			Attributes:  attributes,
			NestingMode: "single",
		})
		elements = append(elements, element)
		current = collections.CompareActions(current, element.Action)
	})
	return computed.NewDiff(renderers.NestedList(elements), current, change.ReplacePaths.Matches())
}

func computeBlockDiffsAsList(change structured.Change, block *jsonprovider.Block) ([]computed.Diff, plans.Action) {
	var elements []computed.Diff
	current := change.GetDefaultActionForIteration()
	processNestedList(change, func(value structured.Change) {
		element := ComputeDiffForBlock(value, block)
		elements = append(elements, element)
		current = collections.CompareActions(current, element.Action)
	})
	return elements, current
}

func processNestedList(change structured.Change, process func(value structured.Change)) {
	sliceValue := change.AsSlice()
	for ix := 0; ix < len(sliceValue.Before) || ix < len(sliceValue.After); ix++ {
		value := sliceValue.GetChild(ix, ix)
		if !value.RelevantAttributes.MatchesPartial() {
			// Mark non-relevant attributes as unchanged.
			value = value.AsNoOp()
		}
		process(value)
	}
}
