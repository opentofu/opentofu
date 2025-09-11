// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"

	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

type moduleInstanceGlueForTesting struct {
	sourceAddr string
}

func (g *moduleInstanceGlueForTesting) ValidateInputs(ctx context.Context, configVal cty.Value) tfdiags.Diagnostics {
	return nil
}

func (g *moduleInstanceGlueForTesting) OutputsValue(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	// This simple test-only "glue" just echoes back the source address it was
	// given, as a stub for testing [ModuleCall] and [ModuleCallInstance] alone
	// without any "real" glue implementation.
	//
	// If you need something more specific in a particular test then it's
	// reasonable to write additional implementations of
	// [ModuleCallInstanceGlue].
	return cty.ObjectVal(map[string]cty.Value{
		"source": cty.StringVal(g.sourceAddr),
	}), nil
}
