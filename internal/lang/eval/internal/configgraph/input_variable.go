// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"
	"fmt"

	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
	"github.com/zclconf/go-cty/cty/function"

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

	// ValidationRules are user-defined checks that must succeed for the
	// final value to be considered valid for use in downstream expressions.
	//
	// The checking and error message evaluation for these rules must be
	// performed in a child scope where the raw value is directly exposed
	// under the same symbol where it would normally appear, because
	// otherwise checking these rules would depend on the success of these
	// very rules and so there would be a self-reference error.
	ValidationRules []CheckRule
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
	rawV, diags := i.RawValue.Value(ctx)
	if i.TargetDefaults != nil {
		rawV = i.TargetDefaults.Apply(rawV)
	}
	finalV, err := convert.Convert(rawV, i.TargetType)
	if err != nil {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid value for input variable",
			Detail:   fmt.Sprintf("Unsuitable value for variable %q: %s.", i.Addr.Variable.Name, tfdiags.FormatError(err)),
			Subject:  maybeHCLSourceRange(i.ValueSourceRange()),
		})
		finalV = cty.UnknownVal(i.TargetType.WithoutOptionalAttributesDeep())
	}

	// TODO: Probably need to factor this part out into a separate function
	// so that we can collect up check results for inclusion in the checks
	// summary in the plan or state, but for now we're not worrying about
	// that because it's pretty rarely-used functionality.
	scopeBuilder := func(ctx context.Context, parent exprs.Scope) exprs.Scope {
		return &inputVariableValidationScope{
			wantName:    i.Addr.Variable.Name,
			parentScope: parent,
			finalVal:    finalV,
		}
	}
	for _, rule := range i.ValidationRules {
		status, moreDiags := rule.Check(ctx, scopeBuilder)
		diags = diags.Append(moreDiags)
		if status == checks.StatusFail {
			errMsg, moreDiags := rule.ErrorMessage(ctx, scopeBuilder)
			diags = diags.Append(moreDiags)
			if !moreDiags.HasErrors() {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid value for input variable",
					Detail:   fmt.Sprintf("%s\n\nThis problem was reported by the validation rule at %s.", errMsg, rule.DeclRange().StartString()),
					Subject:  rule.ConditionRange().ToHCL().Ptr(),
				})
			}
		}
	}

	if diags.HasErrors() {
		// If we found any problems then we'll use an unknown result of the
		// expected type so that downstream expressions will only report
		// new problems and not consequences of the problems we already
		// reported.
		finalV = cty.UnknownVal(i.TargetType.WithoutOptionalAttributesDeep())
	}
	return finalV, diags
}

// ValueSourceRange implements exprs.Valuer.
func (i *InputVariable) ValueSourceRange() *tfdiags.SourceRange {
	return i.RawValue.ValueSourceRange()
}

func (i *InputVariable) CheckAll(ctx context.Context) tfdiags.Diagnostics {
	var cg checkGroup
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

// inputVariableValidationScope is a specialized [exprs.Scope] implementation
// that forces returning a constant value when accessing a specific input
// variable directly, but otherwise just passes everything else through from
// a parent scope.
//
// This is used for evaluating validation rules for an [InputVariable], where
// we need to be able to evaluate an expression referring to the variable
// as part of deciding the final value of the variable and so if we didn't
// handle it directly then there would be a self-reference error.
type inputVariableValidationScope struct {
	varTable    exprs.SymbolTable
	wantName    string
	parentScope exprs.Scope
	finalVal    cty.Value
}

var _ exprs.Scope = (*inputVariableValidationScope)(nil)
var _ exprs.SymbolTable = (*inputVariableValidationScope)(nil)

// HandleInvalidStep implements exprs.Scope.
func (i *inputVariableValidationScope) HandleInvalidStep(rng tfdiags.SourceRange) tfdiags.Diagnostics {
	return i.parentScope.HandleInvalidStep(rng)
}

// ResolveAttr implements exprs.Scope.
func (i *inputVariableValidationScope) ResolveAttr(ref hcl.TraverseAttr) (exprs.Attribute, tfdiags.Diagnostics) {
	if i.varTable == nil {
		// We're currently at the top-level scope where we're looking for
		// the "var." prefix to represent accessing any input variable at all.
		attr, diags := i.parentScope.ResolveAttr(ref)
		if diags.HasErrors() {
			return attr, diags
		}
		nestedTable := exprs.NestedSymbolTableFromAttribute(attr)
		if nestedTable != nil && ref.Name == "var" {
			// We'll return another instance of ourselves but with i.varTable
			// now populated to represent that the next step should try
			// to look up an input variable.
			return exprs.NestedSymbolTable(&inputVariableValidationScope{
				varTable:    nestedTable,
				wantName:    i.wantName,
				parentScope: i.parentScope,
			}), diags
		}
		// If it's anything other than the "var" prefix then we'll just return
		// whatever the parent scope returned directly, because we don't
		// need to be involved anymore.
		return attr, diags
	}

	// If we get here then we're now nested under the "var." prefix, but
	// we only need to get involved if the reference is to the variable
	// we're currently validating.
	if ref.Name == i.wantName {
		return exprs.ValueOf(exprs.ConstantValuer(i.finalVal)), nil
	}
	return i.varTable.ResolveAttr(ref)
}

// ResolveFunc implements exprs.Scope.
func (i *inputVariableValidationScope) ResolveFunc(call *hcl.StaticCall) (function.Function, tfdiags.Diagnostics) {
	return i.parentScope.ResolveFunc(call)
}
