// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/lang/exprs"
)

// InputValuesForTesting returns input variable definitions based on a constant
// map, intended for convenient test setup in unit tests where it only matters
// what the variable values are and not how they are provided.
func InputValuesForTesting(vals map[string]cty.Value) exprs.Valuer {
	return exprs.ConstantValuer(cty.ObjectVal(vals))
}
