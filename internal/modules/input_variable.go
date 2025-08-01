// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package modules

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

type InputVariable struct {
	Name      string
	DeclRange tfdiags.SourceRange
}

func decodeInputVariableBlock(block *hcl.Block) (*InputVariable, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	ret := &InputVariable{
		Name:      block.Labels[0],
		DeclRange: tfdiags.SourceRangeFromHCL(block.DefRange),
	}
	if !hclsyntax.ValidIdentifier(ret.Name) {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid name for input variable",
			Detail:   fmt.Sprintf("Cannot use %q as the name of an input variable. Name must contain only letters, digits, underscores, and dashes.", ret.Name),
			Subject:  block.LabelRanges[0].Ptr(),
		})
	}

	// TODO: Shallowly decode the arguments inside the block

	return ret, diags
}
