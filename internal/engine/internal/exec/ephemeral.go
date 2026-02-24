// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package exec

import (
	"context"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

type OpenEphemeralResourceInstance struct {
	State *ResourceInstanceObject
	Close func(context.Context) tfdiags.Diagnostics
}
