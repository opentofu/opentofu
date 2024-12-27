// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"log"

	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/checks"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

var (
	_ GraphNodeModulePath = (*nodeReportCheck)(nil)
	_ GraphNodeExecutable = (*nodeReportCheck)(nil)
)

// nodeReportCheck calls the ReportCheckableObjects function for our assertions
// within the check blocks.
//
// We need this to happen before the checks are actually verified and before any
// nested data blocks, so the creator of this structure should make sure this
// node is a parent of any nested data blocks.
//
// This needs to be separate to nodeExpandCheck, because the actual checks
// should happen after referenced data blocks rather than before.
type nodeReportCheck struct {
	addr addrs.ConfigCheck
}

func (n *nodeReportCheck) ModulePath() addrs.Module {
	return n.addr.Module
}

func (n *nodeReportCheck) Execute(ctx EvalContext, _ walkOperation) tfdiags.Diagnostics {
	exp := ctx.InstanceExpander()
	modInsts := exp.ExpandModule(n.ModulePath())

	instAddrs := addrs.MakeSet[addrs.Checkable]()
	for _, modAddr := range modInsts {
		instAddrs.Add(n.addr.Check.Absolute(modAddr))
	}
	ctx.Checks().ReportCheckableObjects(n.addr, instAddrs)
	return nil
}

func (n *nodeReportCheck) Name() string {
	return n.addr.String() + " (report)"
}

var (
	_ GraphNodeModulePath = (*nodeEvaluateCheck)(nil)
	_ GraphNodeExecutable = (*nodeEvaluateCheck)(nil)
	_ GraphNodeReferencer = (*nodeEvaluateCheck)(nil)
)

// nodeEvaluateCheck creates child nodes that actually execute the assertions for
// a given check block.
//
// This must happen after any other nodes/resources/data sources that are
// referenced, so we implement GraphNodeReferencer.
//
// This needs to be separate to nodeReportCheck as nodeReportCheck must happen
// first, while nodeEvaluateCheck must execute after any referenced blocks.
type nodeEvaluateCheck struct {
	addr   addrs.ConfigCheck
	config *configs.Check

	executeChecks bool
}

func (n *nodeEvaluateCheck) ModulePath() addrs.Module {
	return n.addr.Module
}

func (n *nodeEvaluateCheck) Execute(evalCtx EvalContext, _ walkOperation) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	// Check block evaluation does not involve any I/O, and so for simplicity's sake
	// we deal with all of the instances of a check block as sequential code rather
	// than trying to evaluate them concurrently.
	exp := evalCtx.InstanceExpander()
	modInsts := exp.ExpandModule(n.ModulePath())
	for _, modAddr := range modInsts {
		absAddr := n.addr.Check.Absolute(modAddr)
		modEvalCtx := evalCtx.WithPath(absAddr.Module)
		log.Printf("[TRACE] nodeEvaluateCheck: checking %s", absAddr)
		moreDiags := n.executeInstance(absAddr, modEvalCtx)
		diags = diags.Append(moreDiags)
	}

	return diags
}

func (n *nodeEvaluateCheck) executeInstance(absAddr addrs.AbsCheck, evalCtx EvalContext) tfdiags.Diagnostics {
	// We only want to actually execute the checks during specific
	// operations, such as plan and applies.
	if n.executeChecks {
		if status := evalCtx.Checks().ObjectCheckStatus(absAddr); status == checks.StatusFail || status == checks.StatusError {
			// This check is already failing, so we won't try and evaluate it.
			// This typically means there was an error in a data block within
			// the check block.
			return nil
		}

		return evalCheckRules(
			addrs.CheckAssertion,
			n.config.Asserts,
			evalCtx,
			absAddr,
			EvalDataForNoInstanceKey,
			tfdiags.Warning,
		)
	}

	// Otherwise let's still validate the config and references and return
	// diagnostics if references do not exist etc.
	var diags tfdiags.Diagnostics
	for ix, assert := range n.config.Asserts {
		_, _, moreDiags := validateCheckRule(
			addrs.NewCheckRule(absAddr, addrs.CheckAssertion, ix),
			assert,
			evalCtx,
			EvalDataForNoInstanceKey,
		)
		diags = diags.Append(moreDiags)
	}
	return diags
}

func (n *nodeEvaluateCheck) References() []*addrs.Reference {
	var refs []*addrs.Reference
	for _, assert := range n.config.Asserts {
		// Check blocks reference anything referenced by conditions or messages
		// in their check rules.
		condition, _ := lang.ReferencesInExpr(addrs.ParseRef, assert.Condition)
		message, _ := lang.ReferencesInExpr(addrs.ParseRef, assert.ErrorMessage)
		refs = append(refs, condition...)
		refs = append(refs, message...)
	}
	if n.config.DataResource != nil {
		// We'll also always reference our nested data block if it exists, as
		// there is nothing enforcing that it has to also be referenced by our
		// conditions or messages.
		//
		// We don't need to make this addr absolute, because the check block and
		// the data resource are always within the same module/instance.
		traversal, _ := hclsyntax.ParseTraversalAbs(
			[]byte(n.config.DataResource.Addr().String()),
			n.config.DataResource.DeclRange.Filename,
			n.config.DataResource.DeclRange.Start)
		ref, _ := addrs.ParseRef(traversal)
		refs = append(refs, ref)
	}
	return refs
}

func (n *nodeEvaluateCheck) Name() string {
	return n.addr.String() + " (evaluate)"
}

var (
	_ GraphNodeExecutable = (*nodeCheckStart)(nil)
)

// We need to ensure that any nested data sources execute after all other
// resource changes have been applied. This node acts as a single point of
// dependency that can enforce this ordering.
type nodeCheckStart struct{}

func (n *nodeCheckStart) Execute(context EvalContext, operation walkOperation) tfdiags.Diagnostics {
	// This node doesn't actually do anything, except simplify the underlying
	// graph structure.
	return nil
}

func (n *nodeCheckStart) Name() string {
	return "(execute checks)"
}
