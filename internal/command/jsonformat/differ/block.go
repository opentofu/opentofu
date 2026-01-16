// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package differ

import (
	"github.com/opentofu/opentofu/internal/command/jsonformat/collections"
	"github.com/opentofu/opentofu/internal/command/jsonformat/computed"
	"github.com/opentofu/opentofu/internal/command/jsonformat/computed/renderers"
	"github.com/opentofu/opentofu/internal/command/jsonformat/structured"
	"github.com/opentofu/opentofu/internal/command/jsonprovider"
	"github.com/opentofu/opentofu/internal/plans"
)

func ComputeDiffForBlock(change structured.Change, block *jsonprovider.Block) computed.Diff {
	if sensitive, ok := checkForSensitiveBlock(change, block); ok {
		return sensitive
	}

	if unknown, ok := checkForUnknownBlock(change, block); ok {
		return unknown
	}

	current := change.GetDefaultActionForIteration()

	blockValue := change.AsMap()

	attributes := make(map[string]computed.Diff)

	// In the first iteration we generate the diffs for all non write-only attributes
	// and only collect the write-only attributes for a second run.
	// This is necessary because [block.Attributes] is a map and since the order is not
	// guarantee in a map we cannot reliably include all the write-only attributes
	// in the rendered diff. Therefore, we generate the diffs for all non write-only attributes,
	// which will generate the actual action of the resource, action that will decide if write-only
	// attributes will be included in the rendered output or not.
	var writeOnlyAttributes []string
	for key, attr := range block.Attributes {
		if attr.WriteOnly {
			writeOnlyAttributes = append(writeOnlyAttributes, key)
			continue
		}
		attrChange := blockValue.GetChild(key)
		childChange := diffChildAttribute(attrChange, attr, current)
		if childChange == nil {
			continue
		}
		current = collections.CompareActions(current, childChange.Action)
		attributes[key] = *childChange
	}
	// In the second iteration, now that the action of the resource is decided, we process only the write-only
	// attributes. If the [current] action is NoOp, then none of the write-only attributes will be included,
	// otherwise, will include all the write-only attributes.
	for _, key := range writeOnlyAttributes {
		attr := block.Attributes[key]
		attrChange := blockValue.GetChild(key)
		childChange := diffChildAttribute(attrChange, attr, current)
		if childChange == nil {
			continue
		}
		current = collections.CompareActions(current, childChange.Action)
		attributes[key] = *childChange
	}

	blocks := renderers.Blocks{
		ReplaceBlocks:         make(map[string]bool),
		BeforeSensitiveBlocks: make(map[string]bool),
		AfterSensitiveBlocks:  make(map[string]bool),
		SingleBlocks:          make(map[string]computed.Diff),
		ListBlocks:            make(map[string][]computed.Diff),
		SetBlocks:             make(map[string][]computed.Diff),
		MapBlocks:             make(map[string]map[string]computed.Diff),
	}

	for key, blockType := range block.BlockTypes {
		childChange := blockValue.GetChild(key)

		if !childChange.RelevantAttributes.MatchesPartial() {
			// Mark non-relevant attributes as unchanged.
			childChange = childChange.AsNoOp()
		}

		beforeSensitive := childChange.IsBeforeSensitive()
		afterSensitive := childChange.IsAfterSensitive()
		forcesReplacement := childChange.ReplacePaths.Matches()

		if diff, ok := checkForUnknownBlock(childChange, block); ok {
			if diff.Action == plans.NoOp && childChange.Before == nil && childChange.After == nil {
				continue
			}
			blocks.AddSingleBlock(key, diff, forcesReplacement, beforeSensitive, afterSensitive)
			continue
		}

		switch NestingMode(blockType.NestingMode) {
		case nestingModeSet:
			diffs, action := computeBlockDiffsAsSet(childChange, blockType.Block)
			if action == plans.NoOp && childChange.Before == nil && childChange.After == nil {
				// Don't record nil values in blocks.
				continue
			}
			blocks.AddAllSetBlock(key, diffs, forcesReplacement, beforeSensitive, afterSensitive)
			current = collections.CompareActions(current, action)
		case nestingModeList:
			diffs, action := computeBlockDiffsAsList(childChange, blockType.Block)
			if action == plans.NoOp && childChange.Before == nil && childChange.After == nil {
				// Don't record nil values in blocks.
				continue
			}
			blocks.AddAllListBlock(key, diffs, forcesReplacement, beforeSensitive, afterSensitive)
			current = collections.CompareActions(current, action)
		case nestingModeMap:
			diffs, action := computeBlockDiffsAsMap(childChange, blockType.Block)
			if action == plans.NoOp && childChange.Before == nil && childChange.After == nil {
				// Don't record nil values in blocks.
				continue
			}
			blocks.AddAllMapBlocks(key, diffs, forcesReplacement, beforeSensitive, afterSensitive)
			current = collections.CompareActions(current, action)
		case nestingModeSingle, nestingModeGroup:
			diff := ComputeDiffForBlock(childChange, blockType.Block)
			if diff.Action == plans.NoOp && childChange.Before == nil && childChange.After == nil {
				// Don't record nil values in blocks.
				continue
			}
			blocks.AddSingleBlock(key, diff, forcesReplacement, beforeSensitive, afterSensitive)
			current = collections.CompareActions(current, diff.Action)
		default:
			panic("unrecognized nesting mode: " + blockType.NestingMode)
		}
	}

	return computed.NewDiff(renderers.Block(attributes, blocks), current, change.ReplacePaths.Matches())
}

// diffChildAttribute computes a new [compute.Diff] for the given attribute change and its schema.
// When a resource for which the diff is built contains also write-only attributes, we want to process first
// all the non write-only attributes to get the actual change action on that resource, action that will decide
// if the write-only attributes will or not be included in the rendered output.
func diffChildAttribute(attrChange structured.Change, attrSchema *jsonprovider.Attribute, currentAction plans.Action) *computed.Diff {
	if !attrChange.RelevantAttributes.MatchesPartial() {
		// Mark non-relevant attributes as unchanged.
		attrChange = attrChange.AsNoOp()
	}

	// Empty strings in blocks should be considered null for legacy reasons.
	// The SDK doesn't support null strings yet, so we work around this now.
	if before, ok := attrChange.Before.(string); ok && len(before) == 0 {
		attrChange.Before = nil
	}
	if after, ok := attrChange.After.(string); ok && len(after) == 0 {
		attrChange.After = nil
	}

	// Always treat changes to blocks as implicit.
	attrChange.BeforeExplicit = false
	attrChange.AfterExplicit = false

	// Because we want to render also the write-only attributes, we need to pass in the parent block
	// action instead of the child one.
	// This is because the child action will always result in NoOp since for write-only attributes, the
	// values returned will be null.
	childChange := ComputeDiffForAttribute(attrChange, attrSchema, currentAction)
	if childChange.Action == plans.NoOp && attrChange.Before == nil && attrChange.After == nil {
		// This validation is specifically added for `tofu show`.
		// Since "current" will be NoOp during rendering the output for `tofu show`,
		// we need this validation to include the write-only attributes in the output.
		if !attrSchema.WriteOnly {
			// Don't record nil values at all in blocks.
			return nil
		}
	}
	return &childChange
}
