// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
	"github.com/zclconf/go-cty/cty/gocty"

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
		return compileInstanceSelectorForEach(ctx, exprs.NewClosure(
			exprs.EvalableHCLExpression(forEachExpr),
			declScope,
		))
	}
	if countExpr != nil {
		return compileInstanceSelectorCount(ctx, exprs.NewClosure(
			exprs.EvalableHCLExpression(countExpr),
			declScope,
		))
	}
	if enabledExpr != nil {
		return compileInstanceSelectorEnabled(ctx, exprs.NewClosure(
			exprs.EvalableHCLExpression(enabledExpr),
			declScope,
		))
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

func compileInstanceSelectorCount(_ context.Context, countValuer exprs.Valuer) configgraph.InstanceSelector {
	countValuer = configgraph.ValuerOnce(countValuer)
	return &instanceSelector{
		keyType:     addrs.IntKeyType,
		sourceRange: nil,
		selectInstances: func(ctx context.Context) (configgraph.Maybe[configgraph.InstancesSeq], cty.ValueMarks, tfdiags.Diagnostics) {
			var count int
			countVal, diags := countValuer.Value(ctx)
			if diags.HasErrors() {
				return nil, nil, diags
			}
			countVal, marks := countVal.Unmark()
			countVal, err := convert.Convert(countVal, cty.Number)
			if err == nil && !countVal.IsKnown() {
				// We represent "unknown" by returning a nil configgraph.Maybe
				// without any error diagnostics, but we will still report
				// what marks we found on the unknown value.
				return nil, marks, diags
			}
			if err == nil && countVal.IsNull() {
				err = errors.New("must not be null")
			}
			if err == nil {
				err = gocty.FromCtyValue(countVal, &count)
			}
			if err != nil {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid value for instance count",
					Detail:   fmt.Sprintf("Unsuitable value for the \"count\" meta-argument: %s.", tfdiags.FormatError(err)),
					Subject:  configgraph.MaybeHCLSourceRange(countValuer.ValueSourceRange()),
				})
				return nil, marks, diags
			}
			// If we manage to get here then "count" is the desired number of
			// instances, and so we'll yield incrementing integers up to
			// that number, exclusive.
			seq := func(yield func(addrs.InstanceKey, instances.RepetitionData) bool) {
				for i := range count {
					more := yield(addrs.IntKey(i), instances.RepetitionData{
						CountIndex: cty.NumberIntVal(int64(i)),
					})
					if !more {
						break
					}
				}
			}
			return configgraph.Known(seq), nil, nil
		},
	}
}

func compileInstanceSelectorEnabled(_ context.Context, enabledValuer exprs.Valuer) configgraph.InstanceSelector {
	enabledValuer = configgraph.ValuerOnce(enabledValuer)
	return &instanceSelector{
		keyType:     addrs.NoKeyType,
		sourceRange: nil,
		selectInstances: func(ctx context.Context) (configgraph.Maybe[configgraph.InstancesSeq], cty.ValueMarks, tfdiags.Diagnostics) {
			var enabled bool
			enabledVal, diags := enabledValuer.Value(ctx)
			if diags.HasErrors() {
				return nil, nil, diags
			}
			enabledVal, marks := enabledVal.Unmark()
			enabledVal, err := convert.Convert(enabledVal, cty.Bool)
			if err == nil && !enabledVal.IsKnown() {
				// We represent "unknown" by returning a nil configgraph.Maybe
				// without any error diagnostics, but we will still report
				// what marks we found on the unknown value.
				return nil, marks, diags
			}
			if err == nil && enabledVal.IsNull() {
				err = errors.New("must not be null")
			}
			if err == nil {
				err = gocty.FromCtyValue(enabledVal, &enabled)
			}
			if err != nil {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid value for instance enabled",
					Detail:   fmt.Sprintf("Unsuitable value for the \"enabled\" meta-argument: %s.", tfdiags.FormatError(err)),
					Subject:  configgraph.MaybeHCLSourceRange(enabledValuer.ValueSourceRange()),
				})
				return nil, marks, diags
			}
			// If we manage to get here then "enabled" is true only if there
			// should be an instance of this resource.
			seq := func(yield func(addrs.InstanceKey, instances.RepetitionData) bool) {
				if enabled {
					yield(addrs.NoKey, instances.RepetitionData{})
				}
			}
			return configgraph.Known(seq), nil, nil
		},
	}
}

func compileInstanceSelectorForEach(_ context.Context, forEachValuer exprs.Valuer) configgraph.InstanceSelector {
	// TODO: The logic for this one is a little more complex so I'll come
	// back to this once more of the rest of this is working.
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
