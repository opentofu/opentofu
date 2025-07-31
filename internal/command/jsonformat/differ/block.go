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
	for key, attr := range block.Attributes {
		childValue := blockValue.GetChild(key)

		if !childValue.RelevantAttributes.MatchesPartial() {
			// Mark non-relevant attributes as unchanged.
			childValue = childValue.AsNoOp()
		}

		// Empty strings in blocks should be considered null for legacy reasons.
		// The SDK doesn't support null strings yet, so we work around this now.
		if before, ok := childValue.Before.(string); ok && len(before) == 0 {
			childValue.Before = nil
		}
		if after, ok := childValue.After.(string); ok && len(after) == 0 {
			childValue.After = nil
		}

		// Always treat changes to blocks as implicit.
		childValue.BeforeExplicit = false
		childValue.AfterExplicit = false

		childChange := ComputeDiffForAttribute(childValue, attr)
		if childChange.Action == plans.NoOp && childValue.Before == nil && childValue.After == nil {
			// Don't record nil values at all in blocks.
			continue
		}

		attributes[key] = childChange
		current = collections.CompareActions(current, childChange.Action)
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
