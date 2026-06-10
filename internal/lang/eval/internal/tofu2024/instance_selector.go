// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"math/big"

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

func compileInstanceSelector(ctx context.Context, declScope exprs.Scope, forEachExpr hcl.Expression, countExpr hcl.Expression, enabledExpr hcl.Expression, dependsOn []hcl.Expression) configgraph.InstanceSelector {
	// We don't current verify that only one of the given expressions is set
	// because we expect the configs package to check that.

	if forEachExpr != nil {
		return compileInstanceSelectorForEach(ctx, declScope, forEachExpr, dependsOn)
	}
	if countExpr != nil {
		return compileInstanceSelectorCount(ctx, declScope, countExpr, dependsOn)
	}
	if enabledExpr != nil {
		return compileInstanceSelectorEnabled(ctx, declScope, enabledExpr, dependsOn)
	}
	return compileInstanceSelectorSingleton(ctx, declScope, dependsOn)
}

func seqBuilder(ctx context.Context, declScope exprs.Scope, repData instances.RepetitionData, dependsOn []hcl.Expression) configgraph.InstanceSeq {
	localScope := instanceLocalScope(declScope, repData)

	marks := cty.ValueMarks{}
	for _, dep := range dependsOn {
		// we don't care about error diags here, just any marks provided on the value
		val, _ := exprs.NewClosure(
			exprs.EvalableHCLExpression(dep),
			localScope,
		).Value(ctx)
		// ValueMarksOfTypeDeep is probably more efficient
		_, moreMarks := val.UnmarkDeep()
		maps.Copy(marks, moreMarks)
	}

	// This adds an implicit depends_on from marks in repetition data
	maps.Copy(marks, repData.CountIndex.Marks())
	maps.Copy(marks, repData.EachKey.Marks())
	maps.Copy(marks, repData.EachValue.Marks())

	return configgraph.InstanceSeq{repData, marks}
}

func compileInstanceSelectorSingleton(ctx context.Context, declScope exprs.Scope, dependsOn []hcl.Expression) configgraph.InstanceSelector {
	return &instanceSelector{
		keyType:     addrs.NoKeyType,
		sourceRange: nil,
		selectInstances: func(ctx context.Context) (configgraph.Maybe[configgraph.InstancesSeq], cty.ValueMarks, tfdiags.Diagnostics) {
			seq := func(yield func(addrs.InstanceKey, configgraph.InstanceSeq) bool) {
				repData := instances.RepetitionData{}
				yield(addrs.NoKey, seqBuilder(ctx, declScope, repData, dependsOn))
			}

			return configgraph.Known(seq), nil, nil
		},
	}
}

func compileInstanceSelectorCount(ctx context.Context, declScope exprs.Scope, countExpr hcl.Expression, dependsOn []hcl.Expression) configgraph.InstanceSelector {
	countValuer := configgraph.ValuerOnce(exprs.NewClosure(
		exprs.EvalableHCLExpression(countExpr),
		declScope,
	))
	return &instanceSelector{
		keyType:     addrs.IntKeyType,
		sourceRange: countValuer.ValueSourceRange(),
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
				// We'll do a few range checks explicitly here just because
				// cty's own error messages for numeric range are quite general
				// and overpromise what is actually allowed here.
				bf := countVal.AsBigFloat()
				if !bf.IsInt() {
					err = errors.New("must be a whole number")
				} else if bf.Cmp(big.NewFloat(0)) < 0 {
					err = errors.New("must not be a negative number")
				} else if v, acc := bf.Int64(); acc != big.Exact || v > math.MaxInt {
					// This will eventually result in a Go slice of the
					// requested length, so we are constrained by Go's maximum
					// slice length on the current platform.
					err = fmt.Errorf("must be between 0 and %d, inclusive", math.MaxInt)
				}
			}
			if err == nil {
				// If all of the checks above failed then the following should
				// always succeed, but we check and handle the error anyway for
				// robustness.
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
			seq := func(yield func(addrs.InstanceKey, configgraph.InstanceSeq) bool) {
				for i := range count {
					repData := instances.RepetitionData{
						CountIndex: cty.NumberIntVal(int64(i)),
					}
					more := yield(addrs.IntKey(i), seqBuilder(ctx, declScope, repData, dependsOn))
					if !more {
						break
					}
				}
			}

			return configgraph.Known(seq), marks, nil
		},
	}
}

func compileInstanceSelectorEnabled(ctx context.Context, declScope exprs.Scope, enabledExpr hcl.Expression, dependsOn []hcl.Expression) configgraph.InstanceSelector {
	enabledValuer := configgraph.ValuerOnce(exprs.NewClosure(
		exprs.EvalableHCLExpression(enabledExpr),
		declScope,
	))
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
			seq := func(yield func(addrs.InstanceKey, configgraph.InstanceSeq) bool) {
				if enabled {
					repData := instances.RepetitionData{}
					yield(addrs.NoKey, seqBuilder(ctx, declScope, repData, dependsOn))
				}
			}

			return configgraph.Known(seq), marks, nil
		},
	}
}

func compileInstanceSelectorForEach(ctx context.Context, declScope exprs.Scope, forEachExpr hcl.Expression, dependsOn []hcl.Expression) configgraph.InstanceSelector {
	forEachValuer := configgraph.ValuerOnce(exprs.NewClosure(
		exprs.EvalableHCLExpression(forEachExpr),
		declScope,
	))
	return &instanceSelector{
		keyType:     addrs.StringKeyType,
		sourceRange: forEachValuer.ValueSourceRange(),
		selectInstances: func(ctx context.Context) (configgraph.Maybe[configgraph.InstancesSeq], cty.ValueMarks, tfdiags.Diagnostics) {
			const errSummary = "Invalid for_each argument"

			rawVal, diags := forEachValuer.Value(ctx)
			if diags.HasErrors() {
				return nil, nil, diags
			}
			rawVal, marks := rawVal.Unmark()
			if rawVal.IsNull() {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  errSummary,
					Detail:   "The for_each value must not be null.",
					Subject:  forEachValuer.ValueSourceRange().ToHCL().Ptr(),
					// TODO: Need some way to get the expression and evalcontext
					// that were used in forEachValuer.Value above so that
					// we can describe what upstream values contributed
					// to this result. (This is true for all of the other
					// diagnostics based on rawVal below, too.)
				})
				return nil, marks, diags
			}

			typ := rawVal.Type()
			if typ.IsSetType() {
				if !typ.ElementType().Equals(cty.String) {
					diags = diags.Append(&hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  errSummary,
						Detail:   "When using a set with for_each, the element type must be string because the element values will be used as instance keys. To work with collections of values of other types, use a map instead.",
						Subject:  forEachValuer.ValueSourceRange().ToHCL().Ptr(),
					})
					return nil, marks, diags
				}
				if !rawVal.IsWhollyKnown() {
					return nil, marks, diags
				}
			} else if typ.IsMapType() {
				if !rawVal.IsKnown() {
					return nil, marks, diags
				}
			} else if typ.IsObjectType() {
				// An object type is always acceptable, because in that case
				// the attribute names are part of the type and so available
				// even if the value isn't known yet.
				//
				// If the value is unknown though, then we need to produce the
				// result differently by iterating the attributes from the
				// type instead of from the value.
				if !rawVal.IsKnown() {
					seq := func(yield func(addrs.InstanceKey, configgraph.InstanceSeq) bool) {
						for name, ty := range typ.AttributeTypes() {
							repData := instances.RepetitionData{
								EachKey:   cty.StringVal(name),
								EachValue: cty.UnknownVal(ty),
							}
							more := yield(addrs.StringKey(name), seqBuilder(ctx, declScope, repData, dependsOn))
							if !more {
								break
							}
						}
					}
					return configgraph.Known(seq), marks, nil

				}
			} else if typ.Equals(cty.DynamicPseudoType) {
				// If we don't even know the type then we have to just assume
				// it'll become something valid in a later phase.
				return nil, marks, diags
			} else {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  errSummary,
					Detail:   "The for_each value must be either a mapping or a set of strings.",
					Subject:  forEachValuer.ValueSourceRange().ToHCL().Ptr(),
				})
				return nil, marks, diags
			}

			// For all of the types we accepted above, cty.Value.Elements
			// returns a sequence that we can directly use for each.key
			// and each.value, because this feature is designed to be a subset
			// of the behavior of HCL's 'for' expressions and they also
			// rely on cty.Value.Elements. In particular, the rules above
			// should ensure that the key is always a known, non-null string.
			seq := func(yield func(addrs.InstanceKey, configgraph.InstanceSeq) bool) {
				for k, v := range rawVal.Elements() {
					repData := instances.RepetitionData{
						EachKey:   k,
						EachValue: v,
					}
					keyStr := k.AsString()
					more := yield(addrs.StringKey(keyStr), seqBuilder(ctx, declScope, repData, dependsOn))
					if !more {
						break
					}
				}
			}

			return configgraph.Known(seq), marks, nil
		},
	}
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
