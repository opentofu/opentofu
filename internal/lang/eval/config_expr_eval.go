// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// NewExpressionEvaluator returns an [ExpressionEvaluator] that will attempt
// to evaluate any expressions it's asked about in the evaluation scope with
// the given address.
//
// The caller must provide an [ExprEvalGlue] implementation to specify what
// value to use for each resource instance in the configuration, which
// could for example either pull values directly out of the most recent state
// snapshot, or return the expected new values from a previously-created plan.
//
// This function pre-visits everything in the configuration and checks it first,
// returning any diagnostics encountered along the way. If the returned
// diagnostics returns errors then the expression evaluator MAY OR MAY NOT be
// nil. Only if non-nil, it's acceptable to use it to attempt expression
// evaluation anyway and the evaluator will make a best effort to resolve them
// despite errors in the configuration, but results may be incomplete or
// confusing in that case and so a user-facing caller should warn the operator
// of this possibility.
func (c *ConfigInstance) NewExpressionEvaluator(ctx context.Context, scope addrs.EvalScope, glue ExprEvalGlue) (*ExpressionEvaluator, tfdiags.Diagnostics) {
	// During this initial planning work we can assume that all evaluation
	// work is happening beneath this function call, because nobody else
	// will have access to the compiled module instances tree until we return,
	// so it's safe to just mint a new worker identity here.
	ctx = grapheval.ContextWithNewWorker(ctx)

	rootModInst, diags := c.newRootModuleInstance(ctx, &expressionEvaluatorEvalGlue{
		impl:    glue,
		evalCtx: c.evalContext,
	})
	if diags.HasErrors() {
		return nil, diags
	}

	moreDiags := rootModInst.CheckAll(ctx)
	diags = diags.Append(moreDiags)

	scopeModuleInstAddr, ok := scope.(addrs.ModuleInstance)
	if !ok {
		// Currently [addrs.ModuleInstance] is the only possible implementation of
		// [addrs.EvalScope], and we should update this if we add any new
		// implementations in future, but we'll return an error anyway just to be
		// a little robust.
		diags = diags.Append(fmt.Errorf("unsupported evaluation scope type %T; this is a bug in OpenTofu", scope))
	}
	scopeModuleInst := evalglue.ModuleInstance(ctx, rootModInst, scopeModuleInstAddr)
	if scopeModuleInst == nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid expression scope",
			// We can safely stringify scopeModuleInstAddr here because we can
			// assume we would never get here for the root module instance
			// (whose string representation is an empty string): the root module
			// instance _always_ exists.
			fmt.Sprintf("Can't evaluate expressions inside %s because no such module instance is declared in the configuration.", scopeModuleInstAddr),
		))
		return nil, diags
	}

	return &ExpressionEvaluator{
		scopeAddr:       scope,
		scopeModuleInst: scopeModuleInst,
	}, diags
}

// ExpressionEvaluator attempts to evaluate arbitrary expressions provided by
// the caller inside an evaluation scope decided when the object was
// constructed.
//
// This is intended for ancillary features like "tofu console" that accept
// expressions from locations other than the configuration itself.
type ExpressionEvaluator struct {
	scopeAddr       addrs.EvalScope
	scopeModuleInst evalglue.CompiledModuleInstance
}

// ScopeAddr returns the address of the scope where expressions would be
// evaluated by this evaluator.
func (ee *ExpressionEvaluator) ScopeAddr() addrs.EvalScope {
	return ee.scopeAddr
}

// Evaluate returns the value of the given [exprs.Evalable] as evaluated in
// the scope associated with this evaluator. If you have an [hcl.Expression]
// to evaluate, use [exprs.EvalableHCLExpression] to wrap it.
//
// This method must not be called concurrently with any other evaluation against
// the associated configuration.
//
// Regardless of whether the returned diagnostics contains errors, the result
// may contain unknown values which could also be marked with [exprs.EvalError]
// to represent that they were derived from something invalid. The returned
// diagnostics typically describes only problems specific to the given
// expression and not general problems elsewhere in the configuration. (Those
// should've been returned when originally constructing the evaluator.)
func (ee *ExpressionEvaluator) Evaluate(ctx context.Context, expr exprs.Evalable) (cty.Value, tfdiags.Diagnostics) {
	valuer := ee.scopeModuleInst.CompileEvalable(expr)

	// We can safely establish a new worker identity here because the
	// documentation says that this method must not be called concurrently with
	// other work on the same configuration, and so we can assume the calling
	// goroutine isn't responsible for any associated workgraph promises.
	ctx = grapheval.ContextWithNewWorker(ctx)
	return valuer.Value(ctx)
}

// ExprEvalGlue is used with [ConfigInstance.NewExpressionEvaluator] to provide
// the values that are to be used for any resource instances that any given
// expressions are derived from.
type ExprEvalGlue interface {
	// ResourceInstanceValue returns the value to use to represent the specified
	// resource instance.
	//
	// The direct caller does not impose any requirements on the returned value,
	// but in order for the results to be sensible the value ought to be
	// something that would be valid for the resource type as far as the
	// associated provider is concerned.
	//
	// This is called once for each distinct resource instance address declared
	// in the configuration before any arbitrary expression evaluation is
	// accepted. If any diagnostics are returned they'll be included in the
	// initial set of diagnostics returned from
	// [ConfigInstance.NewExpressionEvaluator]. Implementations should still return
	// a suitable unknown value placeholder for the requested object even if
	// error diagnostics are returned, since callers are still allowed to
	// attempt expression evaluation against a partially-invalid configuration.
	//
	// Note that, because calls are driven by what's declared in the
	// configuration, a purely-state-based implementation may be asked about
	// resource instances that don't exist yet and so aren't present in the
	// state. In that case the implementation should return an unknown value of
	// a suitable type to act as a placeholder for the not-yet-created object.
	ResourceInstanceValue(ctx context.Context, addr addrs.AbsResourceInstance) (cty.Value, tfdiags.Diagnostics)
}

type expressionEvaluatorEvalGlue struct {
	impl    ExprEvalGlue
	evalCtx *EvalContext
}

// ResourceInstanceValue implements [evalglue.Glue].
func (e *expressionEvaluatorEvalGlue) ResourceInstanceValue(ctx context.Context, ri *configgraph.ResourceInstance, _ cty.Value, _ exprs.FromValue[*configgraph.ProviderInstance], _ addrs.Set[addrs.AbsResourceInstance]) (cty.Value, tfdiags.Diagnostics) {
	// Although the evaluator provides us with a bunch of information here,
	// we're assuming that ExpressionEvaluator is used only to expose resource
	// instance values that were already decided somewhere else -- e.g. in a
	// prior state snapshot, or in an already-constructed plan -- so we just
	// ignore all that here and return whatever the underlying implementation
	// decides to return.
	return e.impl.ResourceInstanceValue(ctx, ri.Addr)
}

// ProviderFunction implements [evalglue.Glue].
func (e *expressionEvaluatorEvalGlue) ProviderFunction(ctx context.Context, provider addrs.Provider, providerInst exprs.FromValue[*configgraph.ProviderInstance], pf addrs.ProviderFunction, rng hcl.Range) (function.Function, tfdiags.Diagnostics) {
	// TODO: Implement this based on the providers available in e.evalCtx,
	// but we already have an implementation of this for planning so we
	// should factor that out into a place we can share it rather than
	// duplicating similar logic in here. We'll need to think about whether
	// it's acceptable to actually configure providers here, since that would
	// mean any caller of [ExpressionEvaluator] (e.g. "tofu console") would
	// only work fully when a provider's underlying API is accessible.
	return function.Function{}, tfdiags.New(fmt.Errorf("provider-defined functions are not available here"))
}
