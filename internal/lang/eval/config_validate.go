// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"

	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Validate checks whether the configuration instance is valid when called
// with the previously-provided input variables and dependencies.
//
// Returns at least one error diagnostic if the configuration call is not valid.
//
// This is exposed for use by "validation-only" callers like the "tofu validate"
// command, but does NOT need to be called before other methods like
// [ConfigInstance.DrivePlanning] because equivalent checks occur within those
// operations.
func (c *ConfigInstance) Validate(ctx context.Context) tfdiags.Diagnostics {
	// All of our work will be associated with a workgraph worker that serves
	// as the initial worker node in the work graph.
	ctx = grapheval.ContextWithNewWorker(ctx)

	_, diags := c.precheckedModuleInstance(ctx)
	// For validation purposes we don't need to do anything more with
	// the module instance we checked... the check result _is_ the validation
	// result.
	return diags
}
