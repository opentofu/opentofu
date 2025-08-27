// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package exprs

import (
	"context"
	"iter"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Evalable is implemented by types that encapsulate expressions that can be
// evaluated in some evaluation scope decided by a caller.
//
// An Evalable implementation must include any supporting metadata needed to
// analyze and evaluate the expressions inside. For example, an [Evalable]
// representing a HCL body must also include the expected schema for that body.
type Evalable interface {
	// References returns a sequence of references this Evalable makes to
	// values in its containing scope.
	References() iter.Seq[hcl.Traversal]

	// FunctionCalls returns a sequence of all of the function calls that
	// could be made if this were evaluated.
	//
	// TODO: Perhaps References and FunctionCalls should be combined together
	// somehow to return a tree that shows when a reference appears as part
	// of an argument to a function, to address the problem described in
	// this issue:
	//     https://github.com/opentofu/opentofu/issues/2630
	FunctionCalls() iter.Seq[*hcl.StaticCall]

	// Evaluate performs the actual expression evaluation, using the given
	// HCL evaluation context to satisfy any references.
	//
	// Callers must first use the References method to discover what the
	// wrapped expressions refer to, and make sure that the given evaluation
	// context contains at least the variables required to satisfy those
	// references.
	//
	// This method takes a [context.Context] because some implementations
	// may internally block on the completion of a potentially-time-consuming
	// operation, in which case they should respond gracefully to the
	// cancellation or deadline of the given context.
	Evaluate(ctx context.Context, hclCtx *hcl.EvalContext) (cty.Value, tfdiags.Diagnostics)

	// ResultTypeConstraint returns a type constrant that all possible results
	// from method Evaluate would conform to.
	//
	// This is used for static type checking. Return [cty.DynamicPseudoType]
	// if it's impossible to predict any single type constraint for the
	// possible results.
	//
	// TODO: Some implementations of this would be able to do better if they
	// knew the types of everything that'd be passed in hclCtx when calling
	// Evaluate. Is there some way we can approximate that?
	ResultTypeConstraint() cty.Type
}

func StaticCheckTraversal(traversal hcl.Traversal, evalable Evalable) tfdiags.Diagnostics {
	return StaticCheckTraversalThroughType(traversal, evalable.ResultTypeConstraint())
}

func StaticCheckTraversalThroughType(traversal hcl.Traversal, typeConstraint cty.Type) tfdiags.Diagnostics {
	// We perform a static check by attempting to apply the traversal to
	// an unknown value of the given type constraint, which will fail if
	// no possible value meeting that type constraint could possibly support
	// the traversal.
	var diags tfdiags.Diagnostics
	placeholder := cty.UnknownVal(typeConstraint)
	_, hclDiags := traversal.TraverseRel(placeholder)
	diags = diags.Append(hclDiags)
	return diags
}

// hclExpression implements [Evalable] for a standalone [hcl.Expression].
type hclExpression struct {
	expr hcl.Expression
}

// EvalableHCLExpression returns an [Evalable] that is just a thin wrapper
// around the given HCL expression.
func EvalableHCLExpression(expr hcl.Expression) Evalable {
	return hclExpression{expr}
}

// Evaluate implements Evalable.
func (h hclExpression) Evaluate(ctx context.Context, hclCtx *hcl.EvalContext) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	v, hclDiags := h.expr.Value(hclCtx)
	diags = diags.Append(hclDiags)
	return v, diags
}

// FunctionCalls implements Evalable.
func (h hclExpression) FunctionCalls() iter.Seq[*hcl.StaticCall] {
	// For now this is not implemented because the underlying HCL API
	// isn't the right shape to implement this method.
	return func(yield func(*hcl.StaticCall) bool) {}
}

// References implements Evalable.
func (h hclExpression) References() iter.Seq[hcl.Traversal] {
	return slices.Values(h.expr.Variables())
}

// ResultTypeConstraint implements Evalable.
func (h hclExpression) ResultTypeConstraint() cty.Type {
	// We can only predict the result type of the expression if it doesn't
	// include any references or function calls.
	v, hclDiags := h.expr.Value(nil)
	if hclDiags.HasErrors() {
		return cty.DynamicPseudoType
	}
	// For an expression that only uses constants, its type is guaranteed
	// to always be the same.
	return v.Type()
}

// hclBody implements [Evalable] for a [hcl.Body] and associated [hcldec.Spec].
type hclBody struct {
	body hcl.Body
	spec hcldec.Spec
}

// EvalableHCLBody returns an [Evalable] that evaluates the given HCL body
// using the given [hcldec] specification.
func EvalableHCLBody(body hcl.Body, spec hcldec.Spec) Evalable {
	return &hclBody{body, spec}
}

// Evaluate implements Evalable.
func (h *hclBody) Evaluate(ctx context.Context, hclCtx *hcl.EvalContext) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	v, hclDiags := hcldec.Decode(h.body, h.spec, hclCtx)
	diags = diags.Append(hclDiags)
	return v, diags
}

// FunctionCalls implements Evalable.
func (h hclBody) FunctionCalls() iter.Seq[*hcl.StaticCall] {
	// For now this is not implemented because the underlying HCL API
	// isn't the right shape to implement this method.
	return func(yield func(*hcl.StaticCall) bool) {}
}

// References implements Evalable.
func (h *hclBody) References() iter.Seq[hcl.Traversal] {
	return slices.Values(hcldec.Variables(h.body, h.spec))
}

// ResultTypeConstraint implements Evalable.
func (h *hclBody) ResultTypeConstraint() cty.Type {
	return hcldec.ImpliedType(h.spec)
}
