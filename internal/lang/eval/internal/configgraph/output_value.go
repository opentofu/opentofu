// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"
	"fmt"
	"slices"

	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/checks"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type OutputValue struct {
	// Addr is the absolute address of this output value.
	Addr addrs.AbsOutputValue

	// Preconditions are user-defined checks that must succeed before OpenTofu
	// will evaluate the output value's expression.
	//
	// Unlike some other uses of [CheckRule], output value preconditions don't
	// have any special local symbols in scope and so are precompiled as part of
	// the [OutputValue] they belong to.
	Preconditions []*CheckRule

	// RawValue produces the "raw" value, as chosen by the caller of the
	// module, which has not yet been type-converted or validated.
	RawValue *OnceValuer

	// TargetType and TargetDefaults together represent the type conversion
	// and default object attribute value insertions that must be applied
	// to RawValue to produce the final result.
	TargetType     cty.Type
	TargetDefaults *typeexpr.Defaults

	// If ForceSensitive is true then the final value will be marked as
	// sensitive regardless of whether the associated raw value was sensitive.
	ForceSensitive bool

	// If ForceEphemeral is true then the final value will be marked as
	// ephemeral regardless of whether the associated raw value was ephemeral.
	ForceEphemeral bool
}

var _ exprs.Valuer = (*OutputValue)(nil)

// ResultTypeConstraint returns a type constraint that all possible results
// of this output value are guaranteed to conform to.
//
// The result is [cty.DynamicPseudoType] for an output value which has no
// declared type constraint, meaning that there is no guarantee whatsoever
// about the result type.
func (o *OutputValue) ResultTypeConstraint() cty.Type {
	return o.TargetType.WithoutOptionalAttributesDeep()
}

// StaticCheckTraversal implements exprs.Valuer.
func (o *OutputValue) StaticCheckTraversal(traversal hcl.Traversal) tfdiags.Diagnostics {
	// We're checking against the type constraint that the final value is
	// guaranteed to conform to, rather than whatever type the raw value
	// has, because conversion to a target type with optional attributes
	// can potentially introduce new attributes. However, we need to
	// discard the optional attribute information first because
	// exprs.StaticCheckTraversalThroughType wants a type constraint, not
	// a "target type" for type conversion.
	typeConstraint := o.TargetType.WithoutOptionalAttributesDeep()
	return exprs.StaticCheckTraversalThroughType(traversal, typeConstraint)
}

// Value implements exprs.Valuer.
func (o *OutputValue) Value(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// The preconditions "guard" the evaluation of the output value's
	// expression, so we need to check them first and skip trying to evaluate
	// if any of them fail. This allows module authors to use preconditions
	// to provide a more specialized error message for certain cases, which
	// would then replace a more general error message that might otherwise
	// be produced by expression evaluation.
	//
	// TODO: We probably need to find some way to collect up check results for
	// inclusion in the checks summary in the plan or state, but for now we're
	// not worrying about that because it's pretty rarely-used functionality.
	preconditionMarks, moreDiags := CheckAllRules(ctx,
		slices.Values(o.Preconditions),
		func(ruleDeclRange tfdiags.SourceRange, status checks.Status, errMsg string) tfdiags.Diagnostics {
			var diags tfdiags.Diagnostics
			if status == checks.StatusFail {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Output value precondition failed",
					Detail:   fmt.Sprintf("%s\n\nThis problem was reported by the precondition at %s.", errMsg, ruleDeclRange.StartString()),
					Subject:  MaybeHCLSourceRange(o.ValueSourceRange()),
				})
			}
			return diags
		},
	)
	diags = diags.Append(moreDiags)

	if diags.HasErrors() {
		// If the preconditions caused at least one error then we must
		// not proceed any further.
		return exprs.AsEvalError(cty.UnknownVal(o.TargetType.WithoutOptionalAttributesDeep())).WithMarks(preconditionMarks), diags
	}

	rawV, diags := o.RawValue.Value(ctx)
	if o.TargetDefaults != nil {
		rawV = o.TargetDefaults.Apply(rawV)
	}
	finalV, err := convert.Convert(rawV, o.TargetType)
	if err != nil {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid value for output value",
			Detail:   fmt.Sprintf("Unsuitable value for output value %q: %s.", o.Addr.OutputValue.Name, tfdiags.FormatError(err)),
			Subject:  MaybeHCLSourceRange(o.ValueSourceRange()),
		})
		finalV = exprs.AsEvalError(cty.UnknownVal(o.TargetType.WithoutOptionalAttributesDeep())).WithMarks(preconditionMarks)
	}

	finalV = finalV.WithMarks(preconditionMarks)
	if o.ForceSensitive {
		finalV = finalV.Mark(marks.Sensitive)
	}
	if o.ForceEphemeral {
		finalV = finalV.Mark(marks.Ephemeral)
	}

	return finalV, diags
}

// ValueSourceRange implements exprs.Valuer.
func (o *OutputValue) ValueSourceRange() *tfdiags.SourceRange {
	return o.RawValue.ValueSourceRange()
}

// CheckAll implements allChecker.
func (o *OutputValue) CheckAll(ctx context.Context) tfdiags.Diagnostics {
	var cg CheckGroup
	// We just check our overall Value method because it covers everything,
	// including the preconditions.
	cg.CheckValuer(ctx, o)
	return cg.Complete(ctx)
}

func (o *OutputValue) AnnounceAllGraphevalRequests(announce func(workgraph.RequestID, grapheval.RequestInfo)) {
	announce(o.RawValue.RequestID(), grapheval.RequestInfo{
		// FIXME: Have the "compiler" in package eval put an
		// addrs.AbsOutputValue in here so we can generate a useful name.
		Name:        o.Addr.String(),
		SourceRange: o.RawValue.ValueSourceRange(),
	})
	// FIXME: This doesn't currently cover any of the preconditions because
	// we're not currently using a distinct workgraph request for each of
	// those. Should our Value method be evaluating those through a
	// grapheval.Once so that they can have their own RequestInfo values?
}
