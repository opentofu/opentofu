// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"log"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// nodeModuleInputVariable represents an input variable from the configuration
// that has not yet been expanded to represent instances of its calling module,
// viewed from the perspective of the caller where the definition expression is
// written.
//
// There should also be a [nodeModuleReference] alongside each node of this
// type which deals with the part of the variable evaluation that needs to happen
// inside the callee, where expressions from the declaration can be evaluated.
type nodeModuleInputVariable struct {
	Addr   addrs.InputVariable
	Module addrs.Module
	Config *configs.Variable // The declaration, in the called module
	Expr   hcl.Expression    // The definition, from the call block in the calling module
}

var (
	_ GraphNodeExecutable       = (*nodeModuleInputVariable)(nil)
	_ GraphNodeReferenceOutside = (*nodeModuleInputVariable)(nil)
	_ GraphNodeReferenceable    = (*nodeModuleInputVariable)(nil)
	_ GraphNodeReferencer       = (*nodeModuleInputVariable)(nil)
	_ graphNodeTemporaryValue   = (*nodeModuleInputVariable)(nil)
	_ graphNodeExpandsInstances = (*nodeModuleInputVariable)(nil)
)

func (n *nodeModuleInputVariable) expandsInstances() {}

func (n *nodeModuleInputVariable) temporaryValue() bool {
	return true
}

func (n *nodeModuleInputVariable) Execute(evalCtx EvalContext, op walkOperation) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	// Input variable evaluation does not involve any I/O, and so for simplicity's sake
	// we deal with all of the instances of an input variable as sequential code rather
	// than trying to evaluate them concurrently.

	expander := evalCtx.InstanceExpander()
	for _, module := range expander.ExpandModule(n.Module) {
		absAddr := n.Addr.Absolute(module)
		// We evaluate in the scope of the calling module because that's where
		// the definition expression is written.
		callerEvalCtx := evalCtx.WithPath(absAddr.Module.Parent())
		log.Printf("[TRACE] nodeModuleInputVariable: evaluating %s", absAddr)
		moreDiags := n.executeInstance(absAddr, callerEvalCtx, op == walkValidate)
		diags = diags.Append(moreDiags)
	}

	return diags
}

// executeInstance deals with the execution side-effects for a single instance of the
// input variable, identified by absAddr.
func (n *nodeModuleInputVariable) executeInstance(absAddr addrs.AbsInputVariableInstance, callerEvalCtx EvalContext, validateOnly bool) tfdiags.Diagnostics {
	val, diags := n.evaluateInstance(absAddr, callerEvalCtx, validateOnly)
	if diags.HasErrors() {
		return diags
	}

	// Set values for arguments of the call that's providing this
	// value for future use in expression evaluation.
	_, call := absAddr.Module.CallInstance()
	callerEvalCtx.SetModuleCallArgument(call, n.Addr, val)

	return diags
}

// evaluateInstance determines the final value for a single instance of the input variable,
// identified by absAddr, without any side-effects.
//
// (Side-effects taken based on this result belong in nodeExpandModuleVariable.executeInstance
// instead.)
func (n *nodeModuleInputVariable) evaluateInstance(absAddr addrs.AbsInputVariableInstance, evalCtx EvalContext, validateOnly bool) (cty.Value, tfdiags.Diagnostics) {
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
			moduleInstanceRepetitionData = evalCtx.InstanceExpander().GetModuleInstanceRepetitionData(absAddr.Module)
		}

		scope := evalCtx.EvaluationScope(nil, nil, moduleInstanceRepetitionData)
		val, moreDiags := scope.EvalExpr(expr, cty.DynamicPseudoType)
		diags = diags.Append(moreDiags)
		if moreDiags.HasErrors() {
			return cty.DynamicVal, diags
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

	finalVal, moreDiags := prepareFinalInputVariableValue(absAddr, rawVal, n.Config)
	diags = diags.Append(moreDiags)

	return finalVal, diags
}

func (n *nodeModuleInputVariable) Name() string {
	path := n.Module.String()
	addr := n.Addr.String() + " (input)"

	if path != "" {
		return path + "." + addr
	}
	return addr
}

// GraphNodeModulePath
func (n *nodeModuleInputVariable) ModulePath() addrs.Module {
	return n.Module
}

// GraphNodeReferencer
func (n *nodeModuleInputVariable) References() []*addrs.Reference {
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
func (n *nodeModuleInputVariable) ReferenceOutside() (addrs.Module, addrs.Module) {
	return n.Module, n.Module.Parent()
}

// GraphNodeReferenceable
func (n *nodeModuleInputVariable) ReferenceableAddrs() []addrs.Referenceable {
	return []addrs.Referenceable{n.Addr}
}
