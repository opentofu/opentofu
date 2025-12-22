// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package method

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
)

// Descriptor contains the details on an encryption method and produces a configuration structure with default values.
type Descriptor interface {
	// ID returns the unique identifier used when parsing HCL or JSON configs.
	ID() ID

	// DecodeConfig creates a new configuration struct. The Build() receiver on
	// this struct must be able to build a Method from the configuration.
	//
	// Common errors:
	// - Returning a struct without a pointer
	// - Returning a non-struct
	DecodeConfig(methodCtx EvalContext, body hcl.Body) (Config, hcl.Diagnostics)
}

type EvalContext struct {
	ValueForExpression func(expr hcl.Expression) (cty.Value, hcl.Diagnostics)
}
