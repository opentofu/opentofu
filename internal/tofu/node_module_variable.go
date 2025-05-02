// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/dag"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// nodeExpandModuleVariable is the placeholder for an variable that has not yet had
// its module path expanded.
type nodeExpandModuleVariable struct {
	Addr   addrs.InputVariable
	Module addrs.Module
	Config *configs.Variable
	Expr   hcl.Expression
}

var (
	_ GraphNodeDynamicExpandable = (*nodeExpandModuleVariable)(nil)
	_ GraphNodeReferenceOutside  = (*nodeExpandModuleVariable)(nil)
	_ GraphNodeReferenceable     = (*nodeExpandModuleVariable)(nil)
	_ GraphNodeReferencer        = (*nodeExpandModuleVariable)(nil)
	_ graphNodeTemporaryValue    = (*nodeExpandModuleVariable)(nil)
	_ graphNodeExpandsInstances  = (*nodeExpandModuleVariable)(nil)
)

func (n *nodeExpandModuleVariable) expandsInstances() {}

func (n *nodeExpandModuleVariable) temporaryValue() bool {
	return true
}

func (n *nodeExpandModuleVariable) DynamicExpand(ctx EvalContext) (*Graph, error) {
	var g Graph

	expander := ctx.InstanceExpander()
	for _, module := range expander.ExpandModule(n.Module) {
		addr := n.Addr.Absolute(module)
		o := &nodeModuleVariable{
			Addr:           addr,
			Config:         n.Config,
			Expr:           n.Expr,
			ModuleInstance: module,
		}
		g.Add(o)
	}
	addRootNodeToGraph(&g)

	return &g, nil
}

func (n *nodeExpandModuleVariable) Name() string {
	return fmt.Sprintf("%s.%s (expand, input)", n.Module, n.Addr.String())
}

// GraphNodeModulePath
func (n *nodeExpandModuleVariable) ModulePath() addrs.Module {
	return n.Module
}

// GraphNodeReferencer
func (n *nodeExpandModuleVariable) References() []*addrs.Reference {
	// If we have no value expression, we cannot depend on anything.
	if n.Expr == nil {
		return nil
	}

	// Variables in the root don't depend on anything, because their values
	// are gathered prior to the graph walk and recorded in the context.
	if len(n.Module) == 0 {
		return nil
	}

	// Otherwise, we depend on anything referenced by our value expression.
	// We ignore diagnostics here under the assumption that we'll re-eval
	// all these things later and catch them then; for our purposes here,
	// we only care about valid references.
	//
	// Due to our GraphNodeReferenceOutside implementation, the addresses
	// returned by this function are interpreted in the _parent_ module from
	// where our associated variable was declared, which is correct because
	// our value expression is assigned within a "module" block in the parent
	// module.
	refs, _ := lang.ReferencesInExpr(addrs.ParseRef, n.Expr)
	return refs
}

// GraphNodeReferenceOutside implementation
func (n *nodeExpandModuleVariable) ReferenceOutside() (selfPath, referencePath addrs.Module) {
	return n.Module, n.Module.Parent()
}

// GraphNodeReferenceable
func (n *nodeExpandModuleVariable) ReferenceableAddrs() []addrs.Referenceable {
	return []addrs.Referenceable{n.Addr}
}

// nodeModuleVariable represents a module variable input during
// the apply step.
type nodeModuleVariable struct {
	Addr   addrs.AbsInputVariableInstance
	Config *configs.Variable // Config is the var in the config
	Expr   hcl.Expression    // Expr is the value expression given in the call
	// ModuleInstance in order to create the appropriate context for evaluating
	// ModuleCallArguments, ex. so count.index and each.key can resolve
	ModuleInstance addrs.ModuleInstance
}

// Ensure that we are implementing all of the interfaces we think we are
// implementing.
var (
	_ GraphNodeModuleInstance = (*nodeModuleVariable)(nil)
	_ GraphNodeExecutable     = (*nodeModuleVariable)(nil)
	_ graphNodeTemporaryValue = (*nodeModuleVariable)(nil)
	_ dag.GraphNodeDotter     = (*nodeModuleVariable)(nil)
)

func (n *nodeModuleVariable) temporaryValue() bool {
	return true
}

func (n *nodeModuleVariable) Name() string {
	return n.Addr.String() + "(input)"
}

// GraphNodeModuleInstance
func (n *nodeModuleVariable) Path() addrs.ModuleInstance {
	// We execute in the parent scope (above our own module) because
	// expressions in our value are resolved in that context.
	return n.Addr.Module.Parent()
}

// GraphNodeModulePath
func (n *nodeModuleVariable) ModulePath() addrs.Module {
	return n.Addr.Module.Module()
}

// GraphNodeExecutable
func (n *nodeModuleVariable) Execute(_ context.Context, evalCtx EvalContext, op walkOperation) (diags tfdiags.Diagnostics) {
	log.Printf("[TRACE] nodeModuleVariable: evaluating %s", n.Addr)

	val, err := n.evalModuleVariable(evalCtx, op == walkValidate)
	diags = diags.Append(err)
	if diags.HasErrors() {
		return diags
	}

	// Set values for arguments of a child module call, for later retrieval
	// during expression evaluation.
	_, call := n.Addr.Module.CallInstance()
	evalCtx.SetModuleCallArgument(call, n.Addr.Variable, val)
	return diags
}

// dag.GraphNodeDotter impl.
func (n *nodeModuleVariable) DotNode(name string, opts *dag.DotOpts) *dag.DotNode {
	return &dag.DotNode{
		Name: name,
		Attrs: map[string]string{
			"label": n.Name(),
			"shape": "note",
		},
	}
}

// evalModuleVariable produces the value for a particular variable as will
// be used by a child module instance.
//
// The result is written into a map, with its key set to the local name of the
// variable, disregarding the module instance address. A map is returned instead
// of a single value as a result of trying to be convenient for use with
// EvalContext.SetModuleCallArguments, which expects a map to merge in with any
// existing arguments.
//
// validateOnly indicates that this evaluation is only for config
// validation, and we will not have any expansion module instance
// repetition data.
func (n *nodeModuleVariable) evalModuleVariable(ctx EvalContext, validateOnly bool) (cty.Value, error) {
	var diags tfdiags.Diagnostics
	var givenVal cty.Value
	var errSourceRange tfdiags.SourceRange
	if expr := n.Expr; expr != nil {
		var moduleInstanceRepetitionData instances.RepetitionData

		switch {
		case validateOnly:
			// the instance expander does not track unknown expansion values, so we
			// have to assume all RepetitionData is unknown.
			moduleInstanceRepetitionData = instances.RepetitionData{
				CountIndex: cty.UnknownVal(cty.Number),
				EachKey:    cty.UnknownVal(cty.String),
				EachValue:  cty.DynamicVal,
			}

		default:
			// Get the repetition data for this module instance,
			// so we can create the appropriate scope for evaluating our expression
			moduleInstanceRepetitionData = ctx.InstanceExpander().GetModuleInstanceRepetitionData(n.ModuleInstance)
		}

		scope := ctx.EvaluationScope(nil, nil, moduleInstanceRepetitionData)
		val, moreDiags := scope.EvalExpr(expr, cty.DynamicPseudoType)
		diags = diags.Append(moreDiags)
		if moreDiags.HasErrors() {
			return cty.DynamicVal, diags.ErrWithWarnings()
		}
		givenVal = val
		errSourceRange = tfdiags.SourceRangeFromHCL(expr.Range())
	} else {
		// We'll use cty.NilVal to represent the variable not being set at all.
		givenVal = cty.NilVal
		errSourceRange = tfdiags.SourceRangeFromHCL(n.Config.DeclRange) // we use the declaration range as a fallback for an undefined variable
	}

	// We construct a synthetic InputValue here to pretend as if this were
	// a root module variable set from outside, just as a convenience so we
	// can reuse the InputValue type for this.
	rawVal := &InputValue{
		Value:       givenVal,
		SourceType:  ValueFromConfig,
		SourceRange: errSourceRange,
	}

	finalVal, moreDiags := prepareFinalInputVariableValue(n.Addr, rawVal, n.Config)
	diags = diags.Append(moreDiags)

	return finalVal, diags.ErrWithWarnings()
}
