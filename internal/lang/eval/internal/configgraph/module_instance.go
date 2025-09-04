// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"
	"fmt"
	"iter"

	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type ModuleInstance struct {
	// Any other kinds of "node" we add in future will likely need coverage
	// added in both [ModuleInstance.CheckAll] and
	// [ModuleInstance.AnnounceAllGraphevalRequests].
	InputVariableNodes  map[addrs.InputVariable]*InputVariable
	LocalValueNodes     map[addrs.LocalValue]*LocalValue
	OutputValueNodes    map[addrs.OutputValue]*OutputValue
	ResourceNodes       map[addrs.Resource]*Resource
	ProviderConfigNodes map[addrs.LocalProviderConfig]*ProviderConfig

	// moduleSourceAddr is the source address of the module this is an
	// instance of, which will be used as the base address for resolving
	// any relative local source addresses in child calls.
	//
	// This must always be either [addrs.ModuleSourceLocal] or
	// [addrs.ModuleSourceRemote]. If the module was discovered indirectly
	// through an [addrs.ModuleSourceRegistry] then this records the
	// remote address that the registry address was resolved to, to ensure
	// that local source addresses will definitely resolve within exactly
	// the same remote package.
	ModuleSourceAddr addrs.ModuleSource

	// callDeclRange is used for module instances that are produced because
	// of a "module" block in a parent module, or by some similar mechanism
	// like a .tftest.hcl "run" block, which can then be used as a source
	// range for the overall object value representing the module instance's
	// results.
	//
	// This is left as nil for module instances that are created implicitly,
	// such as a root module which is being "called" directly from OpenTofu CLI
	// in a command like "tofu plan".
	CallDeclRange *tfdiags.SourceRange
}

// The methods implemented in this file are primarily concerned with
// [ModuleInstance] acting as an [exprs.Valuer]. Module instances also act
// as [exprs.Scope], but the implementations for that are in
// module_instance_scope.go instead.
var _ exprs.Valuer = (*ModuleInstance)(nil)

// StaticCheckTraversal implements exprs.Valuer.
func (m *ModuleInstance) StaticCheckTraversal(traversal hcl.Traversal) tfdiags.Diagnostics {
	if len(traversal) == 0 {
		return nil // empty traversal is always valid
	}

	var diags tfdiags.Diagnostics

	// The Value representation of a module instance is an object with an
	// attribute for each output value, and so the first step traverses
	// through that first level of attributes.
	outputName, ok := exprs.TraversalStepAttributeName(traversal[0])
	if !ok {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid reference to output value",
			Detail:   "A module instance is represented by an object value whose attributes match the names of the output values declared inside the module.",
			Subject:  traversal[0].SourceRange().Ptr(),
		})
		return diags
	}

	output, ok := m.OutputValueNodes[addrs.OutputValue{Name: outputName}]
	if !ok {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Reference to undeclared output value",
			Detail:   fmt.Sprintf("The child module does not declare any output value named %q.", outputName),
			Subject:  traversal[0].SourceRange().Ptr(),
		})
		return diags
	}
	diags = diags.Append(
		exprs.StaticCheckTraversalThroughType(traversal[1:], output.ResultTypeConstraint()),
	)
	return diags
}

// Value implements exprs.Valuer.
func (m *ModuleInstance) Value(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	// The following is mechanically similar to evaluating an object constructor
	// expression gathering all of the output value results into a single
	// object, but because we're not using the expression evaluator to do it
	// we need to explicitly discard indirect diagnostics with
	// [diagsHandledElsewhere].
	attrs := make(map[string]cty.Value, len(m.OutputValueNodes))
	for addr, ov := range m.OutputValueNodes {
		attrs[addr.Name] = diagsHandledElsewhere(ov.Value(ctx))
	}
	return cty.ObjectVal(attrs), nil
}

// ValueSourceRange implements exprs.Valuer.
func (m *ModuleInstance) ValueSourceRange() *tfdiags.SourceRange {
	return m.CallDeclRange
}

// ResourceInstancesDeep returns a sequence of all of the resource instances
// declared both in this module instance and across all child resource
// instances.
//
// The result is trustworthy only if [ModuleInstance.CheckAll] returns without
// errors. When errors are present the result is best-effort and likely to
// be incomplete.
func (m *ModuleInstance) ResourceInstancesDeep(ctx context.Context) iter.Seq[*ResourceInstance] {
	return func(yield func(*ResourceInstance) bool) {
		for _, r := range m.ResourceNodes {
			// NOTE: r.Instances will block if the resource's [InstanceSelector]
			// depends on other parts of the configuration that aren't yet
			// ready to produce their value.
			for _, inst := range r.Instances(ctx) {
				if !yield(inst) {
					return
				}
			}
		}

		// TODO: Once we actually support child module calls, ask for the
		// instances of each one and then collect its resource instances too.
	}
}

// CheckAll visits this module and everything it contains to drive evaluation
// of all of the expressions in the configuration and collect any diagnostics
// they return.
//
// We can implement this as a just concurrent _tree_ walk rather than as a
// graph walk because the expression dependency relationships will get handled
// automatically behind the scenes as the different objects try to resolve
// their [OnceValuer] objects.
//
// This function, and the other downstream CheckAll methods it delegates to,
// therefore only need to worry about making sure that every blocking evaluation
// is happening in a separate goroutine so that the blocking calls can all
// resolve in whatever order makes sense for the dependency graph implied by the
// configuration.
func (m *ModuleInstance) CheckAll(ctx context.Context) tfdiags.Diagnostics {
	// This method is an implementation of [allChecker], but we don't mention
	// that in the docs above because it's an unexported type that would
	// therefore be weird to mention in our exported docs.
	var cg checkGroup
	for _, n := range m.InputVariableNodes {
		cg.CheckChild(ctx, n)
	}
	for _, n := range m.LocalValueNodes {
		cg.CheckChild(ctx, n)
	}
	for _, n := range m.OutputValueNodes {
		cg.CheckChild(ctx, n)
	}
	for _, n := range m.ResourceNodes {
		cg.CheckChild(ctx, n)
	}
	for _, n := range m.ProviderConfigNodes {
		cg.CheckChild(ctx, n)
	}
	return cg.Complete(ctx)
}

// AnnounceAllGraphevalRequests calls announce for each [grapheval.Once],
// [OnceValuer], or other [workgraph.RequestID] anywhere in the tree under this
// object.
//
// This is used only when [workgraph] detects a self-dependency or failure to
// resolve and we want to find a nice human-friendly name and optional source
// range to use to describe each of the requests that were involved in the
// problem.
func (m *ModuleInstance) AnnounceAllGraphevalRequests(announce func(workgraph.RequestID, grapheval.RequestInfo)) {
	// A ModuleInstance does not have any grapheval requests of its own,
	// but all of our child nodes might.
	for _, n := range m.InputVariableNodes {
		n.AnnounceAllGraphevalRequests(announce)
	}
	for _, n := range m.LocalValueNodes {
		n.AnnounceAllGraphevalRequests(announce)
	}
	for _, n := range m.OutputValueNodes {
		n.AnnounceAllGraphevalRequests(announce)
	}
	for _, n := range m.ResourceNodes {
		n.AnnounceAllGraphevalRequests(announce)
	}
}
