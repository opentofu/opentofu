// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package renderers

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/command/jsonformat/computed"
)

func WriteOnly() computed.DiffRenderer {
	return &writeOnlyRenderer{}
}

type writeOnlyRenderer struct {
	NoWarningsRenderer
}

func (renderer writeOnlyRenderer) RenderHuman(diff computed.Diff, _ int, opts computed.RenderHumanOpts) string {
	return fmt.Sprintf("(write-only attribute)%s%s", forcesReplacement(diff.Replace, opts), nullSuffix(diff.Action, opts))
}
