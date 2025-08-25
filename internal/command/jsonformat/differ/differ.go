// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package differ

import (
	"github.com/opentofu/opentofu/internal/command/jsonformat/computed"
	"github.com/opentofu/opentofu/internal/command/jsonformat/structured"
	"github.com/opentofu/opentofu/internal/plans"
)

// asDiff is a helper function to abstract away some simple and common
// functionality when converting a renderer into a concrete diff.
func asDiff(change structured.Change, renderer computed.DiffRenderer) computed.Diff {
	return computed.NewDiff(renderer, change.CalculateAction(), change.ReplacePaths.Matches())
}

// asDiffWithInheritedAction is a specific implementation of asDiff that gets also a parentAction plans.Action.
// This is used when the given change is known to always generate a NoOp diff, but it still should be shown
// in the printed diff.
func asDiffWithInheritedAction(change structured.Change, parentAction plans.Action, renderer computed.DiffRenderer) computed.Diff {
	return computed.NewDiff(renderer, parentAction, change.ReplacePaths.Matches())
}
