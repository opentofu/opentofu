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
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/dag"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// nodeVariableReference is the placeholder for an variable reference that has not yet had
// its module path expanded.  It is a dependency on the evaluation (in a different scope) of
// the nodes which provide the actual variable value to the evaluation context.  This split
// allows the evaluation and validation of the variable in the two different scopes required.
type nodeVariableReference struct {
	Addr   addrs.InputVariable
	Module addrs.Module
	Config *configs.Variable
	Expr   hcl.Expression // Used for diagnostics only

	// VariableFromRemoteModule is indicating if this variable is coming from a module that is referenced from the root module
	// in "local" or "remote" manner.
	VariableFromRemoteModule bool
}

var (
	_ GraphNodeDynamicExpandable = (*nodeVariableReference)(nil)
	_ GraphNodeReferenceable     = (*nodeVariableReference)(nil)
	_ GraphNodeReferencer        = (*nodeVariableReference)(nil)
	_ graphNodeExpandsInstances  = (*nodeVariableReference)(nil)
	_ graphNodeTemporaryValue    = (*nodeVariableReference)(nil)
)

// graphNodeExpandsInstances
func (n *nodeVariableReference) expandsInstances() {}

// Abuse graphNodeTemporaryValue to keep the validation rule around
func (n *nodeVariableReference) temporaryValue() bool {
	return len(n.Config.Validations) == 0
}

// GraphNodeDynamicExpandable
func (n *nodeVariableReference) DynamicExpand(ctx EvalContext) (*Graph, error) {
	var g Graph

	// If this variable has preconditions, we need to report these checks now.
	//
	// We should only do this during planning as the apply phase starts with
	// all the same checkable objects that were registered during the plan.
	var checkableAddrs addrs.Set[addrs.Checkable]
	if checkState := ctx.Checks(); checkState.ConfigHasChecks(n.Addr.InModule(n.Module)) {
		checkableAddrs = addrs.MakeSet[addrs.Checkable]()
	}

	expander := ctx.InstanceExpander()
	for _, module := range expander.ExpandModule(n.Module) {
		addr := n.Addr.Absolute(module)
		if checkableAddrs != nil {
			checkableAddrs.Add(addr)
		}

		o := &nodeVariableReferenceInstance{
			Addr:   addr,
			Config: n.Config,
			Expr:   n.Expr,

			VariableFromRemoteModule: n.VariableFromRemoteModule,
		}
		g.Add(o)
	}
	addRootNodeToGraph(&g)

	if checkableAddrs != nil {
		ctx.Checks().ReportCheckableObjects(n.Addr.InModule(n.Module), checkableAddrs)
	}

	return &g, nil
}

func (n *nodeVariableReference) Name() string {
	addrStr := n.Addr.String()
	if len(n.Module) != 0 {
		addrStr = n.Module.String() + "." + addrStr
	}
	return fmt.Sprintf("%s (expand, reference)", addrStr)
}

// GraphNodeModulePath
func (n *nodeVariableReference) ModulePath() addrs.Module {
	return n.Module
}

// GraphNodeReferencer
func (n *nodeVariableReference) References() []*addrs.Reference {
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
func (n *nodeVariableReference) ReferenceableAddrs() []addrs.Referenceable {
	return []addrs.Referenceable{n.Addr}
}

// nodeVariableReferenceInstance represents a module variable reference during
// the apply step.
type nodeVariableReferenceInstance struct {
	Addr   addrs.AbsInputVariableInstance
	Config *configs.Variable // Config is the var in the config
	Expr   hcl.Expression    // Used for diagnostics only

	// VariableFromRemoteModule is indicating if this variable is coming from a module that is referenced from the root module
	// in "local" or "remote" manner.
	VariableFromRemoteModule bool
}

// Ensure that we are implementing all of the interfaces we think we are
// implementing.
var (
	_ GraphNodeModuleInstance = (*nodeVariableReferenceInstance)(nil)
	_ GraphNodeExecutable     = (*nodeVariableReferenceInstance)(nil)
	_ dag.GraphNodeDotter     = (*nodeVariableReferenceInstance)(nil)
)

func (n *nodeVariableReferenceInstance) Name() string {
	return n.Addr.String() + " (reference)"
}

// GraphNodeModuleInstance
func (n *nodeVariableReferenceInstance) Path() addrs.ModuleInstance {
	return n.Addr.Module
}

// GraphNodeModulePath
func (n *nodeVariableReferenceInstance) ModulePath() addrs.Module {
	return n.Addr.Module.Module()
}

// GraphNodeExecutable
func (n *nodeVariableReferenceInstance) Execute(ctx context.Context, evalCtx EvalContext, op walkOperation) tfdiags.Diagnostics {
	log.Printf("[TRACE] nodeVariableReferenceInstance: evaluating %s", n.Addr)
	diags := evalVariableValidations(ctx, n.Addr, n.Config, n.Expr, evalCtx)

	if op == walkValidate {
		var filtered tfdiags.Diagnostics
		// Validate may contain unknown values, we can ignore that until plan/apply
		for _, diag := range diags {
			if !tfdiags.DiagnosticCausedByUnknown(diag) {
				filtered = append(filtered, diag)
			}
		}
		return filtered
	} else {
		// do not run this during the "validate" phase to ensure that the diagnostics are not duplicated
		diags = diags.Append(evalVariableDeprecation(n.Addr, n.Config, n.Expr, evalCtx, n.VariableFromRemoteModule))
	}

	return diags
}

// dag.GraphNodeDotter impl.
func (n *nodeVariableReferenceInstance) DotNode(name string, _ *dag.DotOpts) *dag.DotNode {
	return &dag.DotNode{
		Name: name,
		Attrs: map[string]string{
			"label": n.Name(),
			"shape": "note",
		},
	}
}
