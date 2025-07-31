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

func evalContextScope(ctx context.Context, evalCtx EvalContext) evalchecks.ContextFunc {
	scope := evalCtx.EvaluationScope(nil, nil, EvalDataForNoInstanceKey)
	return func(refs []*addrs.Reference) (*hcl.EvalContext, tfdiags.Diagnostics) {
		if scope == nil {
			// This shouldn't happen in real code, but it can unfortunately arise
			// in unit tests due to incompletely-implemented mocks. :(
			return &hcl.EvalContext{}, nil
		}
		return scope.EvalContext(ctx, refs)
	}
}

func evalContextEvaluate(ctx context.Context, evalCtx EvalContext) evalchecks.EvaluateFunc {
	return func(expr hcl.Expression) (cty.Value, tfdiags.Diagnostics) {
		return evalCtx.EvaluateExpr(ctx, expr, cty.Number, nil)
	}
}

func evaluateForEachExpression(ctx context.Context, expr hcl.Expression, evalCtx EvalContext, excludeableAddr addrs.Targetable) (map[string]cty.Value, tfdiags.Diagnostics) {
	return evalchecks.EvaluateForEachExpression(expr, evalContextScope(ctx, evalCtx), excludeableAddr)
}

func evaluateForEachExpressionValue(ctx context.Context, expr hcl.Expression, evalCtx EvalContext, allowUnknown bool, allowTuple bool, excludeableAddr addrs.Targetable) (cty.Value, tfdiags.Diagnostics) {
	return evalchecks.EvaluateForEachExpressionValue(expr, evalContextScope(ctx, evalCtx), allowUnknown, allowTuple, excludeableAddr)
}

func evaluateCountExpression(ctx context.Context, expr hcl.Expression, evalCtx EvalContext, excludeableAddr addrs.Targetable) (int, tfdiags.Diagnostics) {
	return evalchecks.EvaluateCountExpression(expr, evalContextEvaluate(ctx, evalCtx), excludeableAddr)
}

func evaluateCountExpressionValue(ctx context.Context, expr hcl.Expression, evalCtx EvalContext) (cty.Value, tfdiags.Diagnostics) {
	return evalchecks.EvaluateCountExpressionValue(expr, evalContextEvaluate(ctx, evalCtx))
}
