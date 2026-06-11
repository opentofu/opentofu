// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/zclconf/go-cty/cty"
)

// dependsOnForTesting constructs a [dependsOn] that just returns exactly
// the given marks, without any further processing.
func dependsOnForTesting(marks ...any) dependsOn {
	var markSet cty.ValueMarks
	if len(marks) != 0 {
		markSet = make(cty.ValueMarks, len(marks))
		for _, mark := range marks {
			markSet[mark] = struct{}{}
		}
	}
	return dependsOn{
		valuer: exprs.ConstantValuer(cty.NullVal(cty.DynamicPseudoType).WithMarks(markSet)),
	}
}
