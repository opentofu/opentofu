// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"log"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// nodeLocalValue represents a named local value in a configuration module,
// which has not yet been expanded.
type nodeLocalValue struct {
	Addr   addrs.LocalValue
	Module addrs.Module
	Config *configs.Local
}

var (
	_ GraphNodeReferenceable    = (*nodeLocalValue)(nil)
	_ GraphNodeReferencer       = (*nodeLocalValue)(nil)
	_ GraphNodeExecutable       = (*nodeLocalValue)(nil)
	_ graphNodeTemporaryValue   = (*nodeLocalValue)(nil)
	_ graphNodeExpandsInstances = (*nodeLocalValue)(nil)
)

func (n *nodeLocalValue) expandsInstances() {}

// graphNodeTemporaryValue
func (n *nodeLocalValue) temporaryValue() bool {
	return true
}

func (n *nodeLocalValue) Name() string {
	path := n.Module.String()
	addr := n.Addr.String()

	if path != "" {
		return path + "." + addr
	}
	return addr
}

// GraphNodeModulePath
func (n *nodeLocalValue) ModulePath() addrs.Module {
	return n.Module
}

// GraphNodeReferenceable
func (n *nodeLocalValue) ReferenceableAddrs() []addrs.Referenceable {
	return []addrs.Referenceable{n.Addr}
}

// GraphNodeReferencer
func (n *nodeLocalValue) References() []*addrs.Reference {
	refs, _ := lang.ReferencesInExpr(addrs.ParseRef, n.Config.Expr)
	return refs
}

func (n *nodeLocalValue) Execute(evalCtx EvalContext, _ walkOperation) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	// Local value evaluation does not involve any I/O, and so for simplicity's sake
	// we deal with all of the instances of a local value as sequential code rather
	// than trying to evaluate them concurrently.

	expander := evalCtx.InstanceExpander()
	for _, module := range expander.ExpandModule(n.Module) {
		absAddr := n.Addr.Absolute(module)
		modEvalCtx := evalCtx.WithPath(absAddr.Module)
		log.Printf("[TRACE] nodeLocalValue: evaluating %s", absAddr)
		moreDiags := n.executeInstance(absAddr, modEvalCtx)
		diags = diags.Append(moreDiags)
	}

	return diags
}

func (n *nodeLocalValue) executeInstance(absAddr addrs.AbsLocalValue, evalCtx EvalContext) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	expr := n.Config.Expr
	localAddr := absAddr.LocalValue

	// We ignore diags here because any problems we might find will be found
	// again in EvaluateExpr below.
	refs, _ := lang.ReferencesInExpr(addrs.ParseRef, expr)
	for _, ref := range refs {
		if ref.Subject == localAddr {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Self-referencing local value",
				Detail:   fmt.Sprintf("Local value %s cannot use its own result as part of its expression.", localAddr),
				Subject:  ref.SourceRange.ToHCL().Ptr(),
				Context:  expr.Range().Ptr(),
			})
		}
	}
	if diags.HasErrors() {
		return diags
	}

	val, moreDiags := evalCtx.EvaluateExpr(expr, cty.DynamicPseudoType, nil)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return diags
	}

	state := evalCtx.State()
	if state == nil {
		// Should not get here: there should always be a working state while we're working.
		diags = diags.Append(fmt.Errorf("cannot write local value to nil state"))
		return diags
	}

	state.SetLocalValue(absAddr, val)
	return diags
}
