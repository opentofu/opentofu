// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func compileInstanceSelector(ctx context.Context, declScope exprs.Scope, forEachExpr hcl.Expression, countExpr hcl.Expression, enabledExpr hcl.Expression) configgraph.InstanceSelector {
	// We don't current verify that only one of the given expressions is set
	// because we expect the configs package to check that.

	if forEachExpr != nil {
		return compileInstanceSelectorForEach(ctx, declScope, forEachExpr)
	}
	if countExpr != nil {
		return compileInstanceSelectorCount(ctx, declScope, countExpr)
	}
	if enabledExpr != nil {
		return compileInstanceSelectorEnabled(ctx, declScope, enabledExpr)
	}
	return compileInstanceSelectorSingleton(ctx)
}

func compileInstanceSelectorSingleton(_ context.Context) configgraph.InstanceSelector {
	return &instanceSelector{
		keyType:     addrs.NoKeyType,
		sourceRange: nil,
		selectInstances: func(ctx context.Context) (configgraph.Maybe[configgraph.InstancesSeq], cty.ValueMarks, tfdiags.Diagnostics) {
			seq := func(yield func(addrs.InstanceKey, instances.RepetitionData) bool) {
				yield(addrs.NoKey, instances.RepetitionData{})
			}
			return configgraph.Known(seq), nil, nil
		},
	}
}

func compileInstanceSelectorCount(_ context.Context, declScope exprs.Scope, expr hcl.Expression) configgraph.InstanceSelector {
	panic("unimplemented")
}

func compileInstanceSelectorEnabled(_ context.Context, declScope exprs.Scope, expr hcl.Expression) configgraph.InstanceSelector {
	panic("unimplemented")
}

func compileInstanceSelectorForEach(_ context.Context, declScope exprs.Scope, expr hcl.Expression) configgraph.InstanceSelector {
	panic("unimplemented")
}

type instanceSelector struct {
	keyType         addrs.InstanceKeyType
	sourceRange     *tfdiags.SourceRange
	selectInstances func(ctx context.Context) (configgraph.Maybe[configgraph.InstancesSeq], cty.ValueMarks, tfdiags.Diagnostics)
}

// InstanceKeyType implements configgraph.InstanceSelector.
func (i *instanceSelector) InstanceKeyType() addrs.InstanceKeyType {
	return i.keyType
}

// Instances implements configgraph.InstanceSelector.
func (i *instanceSelector) Instances(ctx context.Context) (configgraph.Maybe[configgraph.InstancesSeq], cty.ValueMarks, tfdiags.Diagnostics) {
	return i.selectInstances(ctx)
}

// InstancesSourceRange implements configgraph.InstanceSelector.
func (i *instanceSelector) InstancesSourceRange() *tfdiags.SourceRange {
	return i.sourceRange
}
