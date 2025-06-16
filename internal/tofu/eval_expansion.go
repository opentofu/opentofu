// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/evalchecks"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func evalContextScope(evalCtx EvalContext) evalchecks.ContextFunc {
	scope := evalCtx.EvaluationScope(nil, nil, EvalDataForNoInstanceKey)
	return func(refs []*addrs.Reference) (*hcl.EvalContext, tfdiags.Diagnostics) {
		if scope == nil {
			// This shouldn't happen in real code, but it can unfortunately arise
			// in unit tests due to incompletely-implemented mocks. :(
			return &hcl.EvalContext{}, nil
		}
		return scope.EvalContext(context.TODO(), refs)
	}
}

func evalContextEvaluate(evalCtx EvalContext) evalchecks.EvaluateFunc {
	return func(expr hcl.Expression) (cty.Value, tfdiags.Diagnostics) {
		return evalCtx.EvaluateExpr(expr, cty.Number, nil)
	}
}

func evaluateForEachExpression(expr hcl.Expression, evalCtx EvalContext, excludeableAddr addrs.Targetable) (map[string]cty.Value, tfdiags.Diagnostics) {
	return evalchecks.EvaluateForEachExpression(expr, evalContextScope(evalCtx), excludeableAddr)
}

func evaluateForEachExpressionValue(expr hcl.Expression, evalCtx EvalContext, allowUnknown bool, allowTuple bool, excludeableAddr addrs.Targetable) (cty.Value, tfdiags.Diagnostics) {
	return evalchecks.EvaluateForEachExpressionValue(expr, evalContextScope(evalCtx), allowUnknown, allowTuple, excludeableAddr)
}

func evaluateCountExpression(expr hcl.Expression, evalCtx EvalContext, excludeableAddr addrs.Targetable) (int, tfdiags.Diagnostics) {
	return evalchecks.EvaluateCountExpression(expr, evalContextEvaluate(evalCtx), excludeableAddr)
}

func evaluateCountExpressionValue(expr hcl.Expression, evalCtx EvalContext) (cty.Value, tfdiags.Diagnostics) {
	return evalchecks.EvaluateCountExpressionValue(expr, evalContextEvaluate(evalCtx))
}
