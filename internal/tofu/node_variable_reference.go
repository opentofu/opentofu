// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"log"

	"github.com/hashicorp/hcl/v2"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// nodeInputVariableReference is the placeholder for an variable reference that has not yet had
// its module path expanded.  It is a dependency on the evaluation (in a different scope) of
// the nodes which provide the actual variable value to the evaluation context.  This split
// allows the evaluation and validation of the variable in the two different scopes required.
type nodeInputVariableReference struct {
	Addr   addrs.InputVariable
	Module addrs.Module
	Config *configs.Variable // The declaration, in the called module
	Expr   hcl.Expression    // The definition, from the call block in the calling module, for diagnostics only
}

var (
	_ GraphNodeExecutable       = (*nodeInputVariableReference)(nil)
	_ GraphNodeReferenceable    = (*nodeInputVariableReference)(nil)
	_ GraphNodeReferencer       = (*nodeInputVariableReference)(nil)
	_ graphNodeExpandsInstances = (*nodeInputVariableReference)(nil)
	_ graphNodeTemporaryValue   = (*nodeInputVariableReference)(nil)
)

// graphNodeExpandsInstances
func (n *nodeInputVariableReference) expandsInstances() {}

// Abuse graphNodeTemporaryValue to keep the validation rule around
func (n *nodeInputVariableReference) temporaryValue() bool {
	return len(n.Config.Validations) == 0
}

// GraphNodeExecutable
func (n *nodeInputVariableReference) Execute(evalCtx EvalContext, op walkOperation) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	expander := evalCtx.InstanceExpander()
	moduleInstAddrs := expander.ExpandModule(n.Module)

	// If this variable has preconditions, we need to report these checks now.
	//
	// We should only do this during planning as the apply phase starts with
	// all the same checkable objects that were registered during the plan.
	var checkableAddrs addrs.Set[addrs.Checkable]
	if checkState := evalCtx.Checks(); checkState.ConfigHasChecks(n.Addr.InModule(n.Module)) {
		checkableAddrs = addrs.MakeSet[addrs.Checkable]()
		for _, module := range moduleInstAddrs {
			checkableAddrs.Add(n.Addr.Absolute(module))
		}
		evalCtx.Checks().ReportCheckableObjects(n.Addr.InModule(n.Module), checkableAddrs)
	}

	for _, module := range expander.ExpandModule(n.Module) {
		addr := n.Addr.Absolute(module)
		// Since this graph node deals with the input variable from the
		// perspective of the declaration, we use the callee's module
		// path to instantiate our local evaluation context.
		calleeEvalCtx := evalCtx.WithPath(addr.Module)

		log.Printf("[TRACE] nodeInputVariableReference: evaluating %s", addr)
		moreDiags := n.executeInstance(addr, calleeEvalCtx, op == walkValidate)
		diags = diags.Append(moreDiags)
	}

	return diags
}

func (n *nodeInputVariableReference) executeInstance(absAddr addrs.AbsInputVariableInstance, evalCtx EvalContext, validating bool) tfdiags.Diagnostics {
	diags := evalVariableValidations(absAddr, n.Config, n.Expr, evalCtx)

	if validating {
		var filtered tfdiags.Diagnostics
		// Validate may contain unknown values, we can ignore that until plan/apply
		for _, diag := range diags {
			if !tfdiags.DiagnosticCausedByUnknown(diag) {
				filtered = append(filtered, diag)
			}
		}
		return filtered
	}

	return diags
}

func (n *nodeInputVariableReference) Name() string {
	addrStr := n.Addr.String()
	if len(n.Module) != 0 {
		addrStr = n.Module.String() + "." + addrStr
	}
	return fmt.Sprintf("%s (reference)", addrStr)
}

// GraphNodeModulePath
func (n *nodeInputVariableReference) ModulePath() addrs.Module {
	return n.Module
}

// GraphNodeReferencer
func (n *nodeInputVariableReference) References() []*addrs.Reference {
	var refs []*addrs.Reference
	if n.Config != nil {
		for _, validation := range n.Config.Validations {
			condFuncs, _ := lang.ReferencesInExpr(addrs.ParseRef, validation.Condition)
			refs = append(refs, condFuncs...)
			errFuncs, _ := lang.ReferencesInExpr(addrs.ParseRef, validation.ErrorMessage)
			refs = append(refs, errFuncs...)
		}
	}
	return refs
}

// GraphNodeReferenceable
func (n *nodeInputVariableReference) ReferenceableAddrs() []addrs.Referenceable {
	return []addrs.Referenceable{n.Addr}
}
