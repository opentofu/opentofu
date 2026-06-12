// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"context"
	"fmt"
	"maps"
	"math"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// compileDependsOn compiles a set of traversals decoded from a "depends_on"
// argument into an object that can be used to obtain marks describing the
// depends_on elements once the caller is ready to evaluate expressions.
func compileDependsOn(
	traversals []hcl.Traversal,
	declScope exprs.Scope,
	extraMarks cty.ValueMarks,
) dependsOn {
	if len(traversals) == 0 && len(extraMarks) == 0 {
		return dependsOn{
			// No marks at all, then.
			valuer: exprs.ConstantValuer(cty.NullVal(cty.DynamicPseudoType)),
		}
	}

	var items []exprs.Valuer
	if len(traversals) != 0 {
		items = make([]exprs.Valuer, len(traversals))
		for i, traversal := range traversals {
			items[i] = compileDependsOnItem(traversal, declScope)
		}
	}

	return dependsOn{
		valuer: &dependsOnValuer{
			items:      items,
			extraMarks: extraMarks,
		},
	}
}

// dependsOn is the compiled form of a "depends_on" argument, as produced by
// [compileDependsOn].
type dependsOn struct {
	// We use a valuer in here just because that allows us to reuse a bunch of
	// our existing machinery for expression evaluation, but it's encapsulated
	// inside this [dependsOn] type so it's clearer in caller code that the
	// result is used only for its marks and not for its actual value.
	valuer exprs.Valuer
}

// Marks returns a cty-marks-based representation of the declared dependencies,
// ready to be attached to a value to make it appear to have referred to
// everything that the depends_on argument referred to.
func (do dependsOn) Marks(ctx context.Context) (cty.ValueMarks, tfdiags.Diagnostics) {
	if do.valuer == nil {
		// The zero value of dependsOn is valid but has no marks.
		return nil, nil
	}

	v, diags := do.valuer.Value(ctx)
	var ret cty.ValueMarks
	if v != cty.NilVal {
		_, ret = v.UnmarkDeep()
	}
	return ret, diags
}

type dependsOnValuer struct {
	items      []exprs.Valuer
	extraMarks cty.ValueMarks
}

// Value implements [exprs.Valuer].
func (d *dependsOnValuer) Value(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	marks := make(cty.ValueMarks)
	maps.Copy(marks, d.extraMarks)

	// We'll now ask all of our nested valuers for their value and collect up
	// whatever marks they have to use for our overall return value.
	for _, item := range d.items {
		itemV, moreDiags := item.Value(ctx)
		diags = diags.Append(moreDiags)
		_, moreMarks := itemV.UnmarkDeep()
		maps.Copy(marks, moreMarks)
	}

	// The caller is expected to discard everything except the marks from our
	// result and so we'll just return a null value carrying the marks.
	return cty.NullVal(cty.DynamicPseudoType).WithMarks(marks), diags
}

// ValueSourceRange implements [exprs.Valuer].
func (d *dependsOnValuer) ValueSourceRange() *tfdiags.SourceRange {
	if len(d.items) == 0 {
		// No source range at all, then.
		return nil
	}

	// Unfortunately the "configs" package doesn't retain the source range
	// for the entire depends_on expression, so we'll approximate it by
	// just finding the widest range covering all of the items. This
	// assumes that all of the items will have come from the same source
	// file, because there's no situation where a single depends_on argument
	// can be split over multiple files.
	minPos := tfdiags.SourcePos{
		Byte: math.MaxInt,
	}
	maxPos := tfdiags.SourcePos{
		Byte: math.MinInt,
	}
	var filename string
	for _, item := range d.items {
		rng := item.ValueSourceRange()
		if rng == nil {
			continue
		}
		filename = rng.Filename // we assume they're all the same anyway
		if rng.Start.Byte < minPos.Byte {
			minPos = rng.Start
		}
		if rng.End.Byte > maxPos.Byte {
			maxPos = rng.End
		}
	}
	if maxPos.Byte == math.MinInt {
		// None of the items had source ranges, then.
		return nil
	}
	return &tfdiags.SourceRange{
		Filename: filename,
		Start:    minPos,
		End:      maxPos,
	}
}

// StaticCheckTraversal implements [exprs.Valuer].
func (d *dependsOnValuer) StaticCheckTraversal(traversal hcl.Traversal) tfdiags.Diagnostics {
	// We don't expect to use this valuer implementation in any context where
	// it would make sense to call this method.
	return nil
}

func compileDependsOnItem(traversal hcl.Traversal, scope exprs.Scope) exprs.Valuer {
	ref, compileDiags := addrs.ParseRef(traversal)
	if compileDiags.HasErrors() {
		// If parsing failed then further analysis is pointless.
		return exprs.ForcedErrorValuerWithSourceRange(compileDiags, tfdiags.SourceRangeFromHCL(traversal.SourceRange()))
	}
	if len(ref.Remaining) != 0 {
		// In the long run we're considering allowing arbitrary expressions
		// in depends_on so it's possible to depend on only what a small part
		// of an object depends on, but for now we're preserving the assumption
		// from the old runtime that explicit dependencies are always on
		// entire objects, and returning an error to reserve the more general
		// syntax for potential use in a later version.
		compileDiags = compileDiags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid explicit dependency",
			Detail:   fmt.Sprintf("Explicit dependencies must be to entire declarations like %s, and not to attributes or elements inside them.", ref.Subject),
			Subject:  ref.Remaining.SourceRange().Ptr(),
		})
		return exprs.ForcedErrorValuerWithSourceRange(compileDiags, tfdiags.SourceRangeFromHCL(traversal.SourceRange()))
	}

	// To emulate some special behaviors from the old language runtime we will
	// use different behavior for certain expression types.
	switch subj := ref.Subject.(type) {
	case addrs.ModuleCall:
		return dependsOnModuleCallItemValuer{
			addr:  subj,
			scope: scope,
		}
	case addrs.ModuleCallOutput:
		return dependsOnModuleCallItemValuer{
			addr:  subj.Call,
			scope: scope,
		}
	case addrs.ModuleCallInstance:
		return dependsOnModuleCallInstanceItemValuer{
			addr:  subj,
			scope: scope,
		}
	case addrs.ModuleCallInstanceOutput:
		return dependsOnModuleCallInstanceItemValuer{
			addr:  subj.Call,
			scope: scope,
		}
	default:
		// We'll use a synthetic scope traversal expression here just so we can
		// reuse our existing expression evaluation machinery instead of making
		// a special case for naked traversals.
		evalable := exprs.EvalableHCLExpression(&hclsyntax.ScopeTraversalExpr{
			Traversal: traversal,
			SrcRange:  traversal.SourceRange(),
		})
		return exprs.NewClosure(evalable, scope)
	}
}

type dependsOnModuleCallItemValuer struct {
	addr  addrs.ModuleCall
	scope exprs.Scope
	rng   tfdiags.SourceRange
}

// Value implements [exprs.Valuer].
func (v dependsOnModuleCallItemValuer) Value(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	marks := make(cty.ValueMarks)
	for resourceInst := range moduleCallResourceInstancesDeep(ctx, v.scope, v.addr) {
		mark := configgraph.NewResourceInstanceMark(resourceInst)
		marks[mark] = struct{}{}
	}
	return cty.NullVal(cty.DynamicPseudoType).WithMarks(marks), nil
}

// ValueSourceRange implements [exprs.Valuer].
func (v dependsOnModuleCallItemValuer) ValueSourceRange() *tfdiags.SourceRange {
	return &v.rng
}

// StaticCheckTraversal implements [exprs.Valuer].
func (v dependsOnModuleCallItemValuer) StaticCheckTraversal(traversal hcl.Traversal) tfdiags.Diagnostics {
	// We don't expect to use this valuer implementation in any context where
	// it would make sense to call this method.
	return nil
}

type dependsOnModuleCallInstanceItemValuer struct {
	addr  addrs.ModuleCallInstance
	scope exprs.Scope
	rng   tfdiags.SourceRange
}

// Value implements [exprs.Valuer].
func (v dependsOnModuleCallInstanceItemValuer) Value(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	marks := make(cty.ValueMarks)
	for resourceInst := range moduleCallInstanceResourceInstancesDeep(ctx, v.scope, v.addr) {
		mark := configgraph.NewResourceInstanceMark(resourceInst)
		marks[mark] = struct{}{}
	}
	return cty.NullVal(cty.DynamicPseudoType).WithMarks(marks), nil
}

// ValueSourceRange implements [exprs.Valuer].
func (v dependsOnModuleCallInstanceItemValuer) ValueSourceRange() *tfdiags.SourceRange {
	return &v.rng
}

// StaticCheckTraversal implements [exprs.Valuer].
func (v dependsOnModuleCallInstanceItemValuer) StaticCheckTraversal(traversal hcl.Traversal) tfdiags.Diagnostics {
	// We don't expect to use this valuer implementation in any context where
	// it would make sense to call this method.
	return nil
}
