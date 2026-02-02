// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"

	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu/contract"
)

type ImportOpts contract.ImportOpts

// Import takes already-created external resources and brings them
// under OpenTofu management. Import requires the exact type, name, and ID
// of the resources to import.
//
// This operation is idempotent. If the requested resource is already
// imported, no changes are made to the state.
//
// Further, this operation also gracefully handles partial state. If during
// an import there is a failure, all previously imported resources remain
// imported.
func (c *Context) Import(ctx context.Context, config *configs.Config, prevRunState *states.State, opts *ImportOpts) (*states.State, tfdiags.Diagnostics) {

	impl, diags := c.impl()
	if diags.HasErrors() {
		return nil, diags
	}
	return impl.Import(ctx, config, prevRunState, (*contract.ImportOpts)(opts))
}
