// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package differ

import (
	"github.com/opentofu/opentofu/internal/command/jsondiffer/computed"
	"github.com/opentofu/opentofu/internal/command/jsondiffer/structured"
)

// asDiff is a helper function to abstract away some simple and common
// functionality when converting a renderer into a concrete diff.
func asDiff(change structured.Change, renderer computed.DiffRenderer) computed.Diff {
	return computed.NewDiff(renderer, change.CalculateAction(), change.ReplacePaths.Matches())
}
