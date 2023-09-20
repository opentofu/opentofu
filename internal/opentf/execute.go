// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package opentf

import "github.com/opentofu/opentofu/internal/tfdiags"

// GraphNodeExecutable is the interface that graph nodes must implement to
// enable execution.
type GraphNodeExecutable interface {
	Execute(EvalContext, walkOperation) tfdiags.Diagnostics
}
