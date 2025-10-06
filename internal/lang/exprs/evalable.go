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
	"github.com/hashicorp/hcl/v2/ext/dynblock"
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
	//
	// If Evaluate returns diagnostics then it must also return a suitable
	// placeholder value that could be use for downstream expression evaluation
	// despite the error. Returning [cty.DynamicVal] is acceptable if all else
	// fails, but returning an unknown value with a more specific type
	// constraint can give more opportunities to proactively detect downstream
	// errors in a single evaluation pass.
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

	// EvalableSourceRange returns a description of a source location that this
	// Evalable was derived from.
	EvalableSourceRange() tfdiags.SourceRange
}

// ForcedErrorEvalable returns an [Evalable] that always fails with
// [cty.DynamicVal] as its placeholder result and with the given diagnostics,
// which must include at least one error or this function will panic.
//
// This is primarily intended for unit testing purposes for creating
// placeholders for upstream objects that have failed, but might also be useful
// sometimes for handling early-detected error situations in "real" code.
func ForcedErrorEvalable(diags tfdiags.Diagnostics, sourceRange tfdiags.SourceRange) Evalable {
	if !diags.HasErrors() {
		panic("ForcedErrorEvalable without any error diagnostics")
	}
	// We reuse the same type as ForcedErrorValuer here because it can
	// implement both interfaces just fine with the information available here.
	return forcedErrorValuer{diags, &sourceRange}
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
	if hclDiags.HasErrors() {
		v = cty.UnknownVal(h.ResultTypeConstraint()).WithSameMarks(v)
	}
	return v, diags
}

// FunctionCalls implements Evalable.
func (h hclExpression) FunctionCalls() iter.Seq[*hcl.StaticCall] {
	// FIXME: The underlying HCL API somewhat-misuses [hcl.Traversal] to
	// enumerate the names of the functions without providing any
	// information about their arguments. We can get away with leaving
	// the args unpopulated for now since we're not actually relying on
	// them but it would be nice to update HCL to return [hcl.StaticCall]
	// for the function analysis instead, for consistency with how
	// [hcl.ExprCall] works.
	expr, ok := h.expr.(hcl.ExpressionWithFunctions)
	if !ok {
		return func(yield func(*hcl.StaticCall) bool) {}
	}
	return func(yield func(*hcl.StaticCall) bool) {
		for _, traversal := range expr.Functions() {
			wantMore := yield(&hcl.StaticCall{
				Name:      traversal.RootName(),
				NameRange: traversal.SourceRange(),
			})
			if !wantMore {
				return
			}
		}
	}
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

// SourceRange implements Evalable.
func (h hclExpression) EvalableSourceRange() tfdiags.SourceRange {
	return tfdiags.SourceRangeFromHCL(h.expr.Range())
}

// hclBody implements [Evalable] for a [hcl.Body] and associated [hcldec.Spec].
type hclBody struct {
	body     hcl.Body
	spec     hcldec.Spec
	dynblock bool
}

// EvalableHCLBody returns an [Evalable] that evaluates the given HCL body
// using the given [hcldec] specification.
func EvalableHCLBody(body hcl.Body, spec hcldec.Spec) Evalable {
	return &hclBody{
		body:     body,
		spec:     spec,
		dynblock: false,
	}
}

// EvalableHCLBodyWithDynamicBlocks is a variant of [EvalableHCLBody] that
// calls [dynblock.Expand] before evaluating the body so that "dynamic" blocks
// would be supported and expanded to their equivalent static blocks.
func EvalableHCLBodyWithDynamicBlocks(body hcl.Body, spec hcldec.Spec) Evalable {
	return &hclBody{
		body:     body,
		spec:     spec,
		dynblock: true,
	}
}

// Evaluate implements Evalable.
func (h *hclBody) Evaluate(ctx context.Context, hclCtx *hcl.EvalContext) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	body := h.body
	if h.dynblock {
		// [dynblock.Expand] wraps our body so that hcldec.Decode below will
		// indirectly cause the "dynamic" blocks to be expanded, using the
		// same evaluation context for the for_each expressions.
		body = dynblock.Expand(body, hclCtx)
	}
	v, hclDiags := hcldec.Decode(body, h.spec, hclCtx)
	diags = diags.Append(hclDiags)
	if hclDiags.HasErrors() {
		v = cty.UnknownVal(h.ResultTypeConstraint()).WithSameMarks(v)
	}
	return v, diags
}

// FunctionCalls implements Evalable.
func (h hclBody) FunctionCalls() iter.Seq[*hcl.StaticCall] {
	// FIXME: The underlying HCL API somewhat-misuses [hcl.Traversal] to
	// enumerate the names of the functions without providing any
	// information about their arguments. We can get away with leaving
	// the args unpopulated for now since we're not actually relying on
	// them but it would be nice to update HCL to return [hcl.StaticCall]
	// for the function analysis instead, for consistency with how
	// [hcl.ExprCall] works.
	return func(yield func(*hcl.StaticCall) bool) {
		for _, traversal := range hcldec.Functions(h.body, h.spec) {
			wantMore := yield(&hcl.StaticCall{
				Name:      traversal.RootName(),
				NameRange: traversal.SourceRange(),
			})
			if !wantMore {
				return
			}
		}
	}
}

// References implements Evalable.
func (h *hclBody) References() iter.Seq[hcl.Traversal] {
	if h.dynblock {
		// When we're doing dynblock expansion we need to do a little
		// more work to also detect references from the for_each
		// arguments in the "dynamic" blocks. The dynblock package
		// does this additional work for us as long as we ask its
		// wrapper function instead of hcldec.Variables directly.
		return slices.Values(dynblock.VariablesHCLDec(h.body, h.spec))
	}
	return slices.Values(hcldec.Variables(h.body, h.spec))
}

// ResultTypeConstraint implements Evalable.
func (h *hclBody) ResultTypeConstraint() cty.Type {
	return hcldec.ImpliedType(h.spec)
}

// SourceRange implements Evalable.
func (h *hclBody) EvalableSourceRange() tfdiags.SourceRange {
	// The "missing item range" is not necessarily a good range to use here,
	// but is the best we can do. At least in HCL native syntax this tends
	// to be in the header of the block that contained the body and so
	// is _close_ to the body being described.
	return tfdiags.SourceRangeFromHCL(h.body.MissingItemRange())
}

// hclBodyJustAttributes implements [Evalable] for a [hcl.Body] using HCL's
// "just attributes" evaluation mode.
type hclBodyJustAttributes struct {
	body hcl.Body
}

// EvalableHCLBody returns an [Evalable] that evaluates the given HCL body
// in HCL's "just attributes" mode, and then returns an object value whose
// attribute names and values are derived from the result.
func EvalableHCLBodyJustAttributes(body hcl.Body) Evalable {
	return &hclBodyJustAttributes{
		body: body,
	}
}

// EvalableSourceRange implements Evalable.
func (h *hclBodyJustAttributes) EvalableSourceRange() tfdiags.SourceRange {
	return tfdiags.SourceRangeFromHCL(h.body.MissingItemRange())
}

// Evaluate implements Evalable.
func (h *hclBodyJustAttributes) Evaluate(ctx context.Context, hclCtx *hcl.EvalContext) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	attrs, hclDiags := h.body.JustAttributes()
	diags = diags.Append(hclDiags)
	// If we have errors already then we might not have the full set of
	// attributes that were declared, so we'll return cty.DynamicVal below
	// to avoid overpromising that we know the result type. However,
	// we'll still visit all of the attributes we _did_ find in case
	// that allows us to collect up some more expression-errors to return so we
	// can tell the module author about as much as possible at once.
	typeKnown := !hclDiags.HasErrors()
	retAttrs := make(map[string]cty.Value, len(attrs))
	for name, attr := range attrs {
		val, hclDiags := attr.Expr.Value(hclCtx)
		diags = diags.Append(hclDiags)
		if hclDiags.HasErrors() {
			val = AsEvalError(cty.DynamicVal)
		}
		retAttrs[name] = val
	}
	if !typeKnown {
		return EvalResult(cty.DynamicVal, diags)
	}
	// We don't use a top-level EvalError here because we selectively
	// marked individual attribute values above, and we're confident
	// that the set of attribute names in this object is correct because
	// the original JustAttributes call succeeded.
	return cty.ObjectVal(retAttrs), diags
}

// FunctionCalls implements Evalable.
func (h *hclBodyJustAttributes) FunctionCalls() iter.Seq[*hcl.StaticCall] {
	// For now this is not implemented because the underlying HCL API
	// isn't the right shape to implement this method.
	return func(yield func(*hcl.StaticCall) bool) {}
}

// References implements Evalable.
func (h *hclBodyJustAttributes) References() iter.Seq[hcl.Traversal] {
	// This case is annoying because we need to perform the shallow
	// JustAttributes call to get the expressions to analyze, but then
	// we'll need to call it again in Evaluate once the scope has
	// been built. But only if we find this being a performance problem
	// should we consider trying to cache this result.
	//
	// We ignore the diagnostics here because we're just making a best
	// effort to learn what traversals we might need and then we'll
	// return the same set of diagnostics from Evaluate.
	attrs, _ := h.body.JustAttributes()
	return func(yield func(hcl.Traversal) bool) {
		for _, attr := range attrs {
			for _, traversal := range attr.Expr.Variables() {
				if !yield(traversal) {
					return
				}
			}
		}
	}
}

// ResultTypeConstraint implements Evalable.
func (h *hclBodyJustAttributes) ResultTypeConstraint() cty.Type {
	// We cannot predict a result type in "just attributes" mode because
	// the type depends on the results of the expressions in the body.
	return cty.DynamicPseudoType
}
