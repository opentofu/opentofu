// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/evalchecks"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func evalContextScope(ctx EvalContext) evalchecks.ContextFunc {
	scope := ctx.EvaluationScope(nil, nil, EvalDataForNoInstanceKey)
	return func(refs []*addrs.Reference) (*hcl.EvalContext, tfdiags.Diagnostics) {
		if scope == nil {
			// This shouldn't happen in real code, but it can unfortunately arise
			// in unit tests due to incompletely-implemented mocks. :(
			return &hcl.EvalContext{}, nil
		}
		return scope.EvalContext(refs)
	}
}

func evalContextEvaluate(ctx EvalContext) evalchecks.EvaluateFunc {
	return func(expr hcl.Expression) (cty.Value, tfdiags.Diagnostics) {
		return ctx.EvaluateExpr(expr, cty.Number, nil)
	}
}

func evaluateForEachExpression(expr hcl.Expression, ctx EvalContext) (map[string]cty.Value, tfdiags.Diagnostics) {
	return evalchecks.EvaluateForEachExpression(expr, evalContextScope(ctx))
}

func evaluateForEachExpressionValue(expr hcl.Expression, ctx EvalContext, allowUnknown bool, allowTuple bool) (cty.Value, tfdiags.Diagnostics) {
	return evalchecks.EvaluateForEachExpressionValue(expr, evalContextScope(ctx), allowUnknown, allowTuple)
}

func evaluateCountExpression(expr hcl.Expression, ctx EvalContext) (int, tfdiags.Diagnostics) {
	return evalchecks.EvaluateCountExpression(expr, evalContextEvaluate(ctx))
}

func evaluateCountExpressionValue(expr hcl.Expression, ctx EvalContext) (cty.Value, tfdiags.Diagnostics) {
	return evalchecks.EvaluateCountExpressionValue(expr, evalContextEvaluate(ctx))
}
