// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/configs/configload"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// exitStatusFromDiags returns an error code if there are errors in
// the diags or a `tofu init` required specific error code if the
// diags indicate this exit code through ExtraInfo
func exitStatusFromDiags(diags tfdiags.Diagnostics) int {
	var exit int
	if diags.HasErrors() {
		exit = 1
	}

	for _, d := range diags {
		if _, ok := d.ExtraInfo().(configload.InitExit); ok {
			exit = int(backend.OperationInitRequired)
			break
		}
	}
	return exit
}
