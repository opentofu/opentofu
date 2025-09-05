// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"
	"fmt"
	"iter"

	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/checks"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type InputVariable struct {
	// Addr is the absolute address of this input variable.
	Addr addrs.AbsInputVariableInstance

	// RawValue produces the "raw" value, as chosen by the caller of the
	// module, which has not yet been type-converted or validated.
	RawValue *OnceValuer

	// TargetType and targetDefaults together represent the type conversion
	// and default object attribute value insertions that must be applied
	// to rawValue to produce the final result.
	TargetType     cty.Type
	TargetDefaults *typeexpr.Defaults

	// TODO: Default value
	// TODO: ForceEphemeral, ForceSensitive

	// Validation rules are user-defined checks that must succeed for the
	// final value to be considered valid for use in downstream expressions.
	//
	// CompileValidationRules takes the value of the variable after
	// type conversion and built-in validation rules have been applied to
	// it, and returns a sequence of compiled [CheckRule] objects that
	// test whether the author's configured conditions have been met
	// for the given value.
	CompileValidationRules func(ctx context.Context, value cty.Value) iter.Seq[*CheckRule]
}

var _ exprs.Valuer = (*InputVariable)(nil)

// StaticCheckTraversal implements exprs.Valuer.
func (i *InputVariable) StaticCheckTraversal(traversal hcl.Traversal) tfdiags.Diagnostics {
	// We're checking against the type constraint that the final value is
	// guaranteed to conform to, rather than whatever type the raw value
	// has, because conversion to a target type with optional attributes
	// can potentially introduce new attributes. However, we need to
	// discard the optional attribute information first because
	// exprs.StaticCheckTraversalThroughType wants a type constraint, not
	// a "target type" for type conversion.
	typeConstraint := i.TargetType.WithoutOptionalAttributesDeep()
	return exprs.StaticCheckTraversalThroughType(traversal, typeConstraint)
}

// Value implements exprs.Valuer.
func (i *InputVariable) Value(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	rawV, moreDiags := i.RawValue.Value(ctx)
	diags = diags.Append(moreDiags)
	if i.TargetDefaults != nil {
		rawV = i.TargetDefaults.Apply(rawV)
	}
	finalV, err := convert.Convert(rawV, i.TargetType)
	if err != nil {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid value for input variable",
			Detail:   fmt.Sprintf("Unsuitable value for variable %q: %s.", i.Addr.Variable.Name, tfdiags.FormatError(err)),
			Subject:  MaybeHCLSourceRange(i.ValueSourceRange()),
		})
		finalV = cty.UnknownVal(i.TargetType.WithoutOptionalAttributesDeep())
	}

	// Once we have our converted and prepared value we can finally compile
	// the validation rules against it and then check them.
	var validationMarks cty.ValueMarks
	if i.CompileValidationRules != nil {
		validationMarks, moreDiags = CheckAllRules(ctx,
			i.CompileValidationRules(ctx, finalV),
			func(ruleDeclRange tfdiags.SourceRange, status checks.Status, errMsg string) tfdiags.Diagnostics {
				var diags tfdiags.Diagnostics
				if status == checks.StatusFail {
					diags = diags.Append(&hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Invalid value for input variable",
						Detail:   fmt.Sprintf("%s\n\nThis problem was reported by the validation rule at %s.", errMsg, ruleDeclRange.StartString()),
						Subject:  i.ValueSourceRange().ToHCL().Ptr(),
					})
				}
				return diags
			},
		)
		diags = diags.Append(moreDiags)
	}

	if diags.HasErrors() {
		// If we found any problems then we'll use an unknown result of the
		// expected type so that downstream expressions will only report
		// new problems and not consequences of the problems we already
		// reported.
		finalV = exprs.AsEvalError(cty.UnknownVal(i.TargetType.WithoutOptionalAttributesDeep())).WithMarks(validationMarks)
	}
	return finalV.WithMarks(validationMarks), diags
}

// ValueSourceRange implements exprs.Valuer.
func (i *InputVariable) ValueSourceRange() *tfdiags.SourceRange {
	return i.RawValue.ValueSourceRange()
}

func (i *InputVariable) CheckAll(ctx context.Context) tfdiags.Diagnostics {
	var cg CheckGroup
	// We do a check on the InputVariable as a whole here, rather than
	// treating its CheckRules as children, because a CheckRule isn't
	// a standalone object that can self-check but rather just a detail
	// of our own evaluation that might contribute additional errors.
	cg.CheckValuer(ctx, i)
	return cg.Complete(ctx)
}

func (i *InputVariable) AnnounceAllGraphevalRequests(announce func(workgraph.RequestID, grapheval.RequestInfo)) {
	announce(i.RawValue.RequestID(), grapheval.RequestInfo{
		// FIXME: Have the "compiler" in package eval put an
		// addrs.AbsInputVariable in here so we can better avoid ambiguity
		// between module instances using variables of the same name.
		Name:        fmt.Sprintf("value for %s", i.Addr),
		SourceRange: i.RawValue.ValueSourceRange(),
	})
	// FIXME: This doesn't currently cover any of the validation rules because
	// we're not currently using a distinct workgraph request for each of
	// those. Should our Value method be evaluating those through a
	// grapheval.Once so that they can have their own RequestInfo values?
}
