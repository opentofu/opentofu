// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package renderers

import (
	"fmt"

	"github.com/terramate-io/opentofulib/internal/command/jsonformat/computed"

	"github.com/terramate-io/opentofulib/internal/plans"
)

var _ computed.DiffRenderer = (*unknownRenderer)(nil)

func Unknown(before computed.Diff) computed.DiffRenderer {
	return &unknownRenderer{
		before: before,
	}
}

type unknownRenderer struct {
	NoWarningsRenderer

	before computed.Diff
}

func (renderer unknownRenderer) RenderHuman(diff computed.Diff, indent int, opts computed.RenderHumanOpts) string {
	if diff.Action == plans.Create {
		return fmt.Sprintf("(known after apply)%s", forcesReplacement(diff.Replace, opts))
	}

	// Never render null suffix for children of unknown changes.
	opts.OverrideNullSuffix = true
	return fmt.Sprintf("%s -> (known after apply)%s", renderer.before.RenderHuman(indent, opts), forcesReplacement(diff.Replace, opts))
}
