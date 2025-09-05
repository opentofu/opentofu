// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"
	"fmt"

	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type ModuleInstance struct {
	Addr addrs.ModuleInstance

	// For the sake of this package a [ModuleInstance] is really just here
	// to be an [exprs.Valuer] to expose as part of the value of a module
	// call in the parent module, and so it doesn't know anything about
	// what might have been declared inside the module instance. The
	// relationships between child objects and their containing module
	// instance are represented in the "compiler" packages, such as
	// package tofu2024, to allow the details to vary between language
	// editions as long as somehow the module instance is able to return
	// a set of output values.

	// OutputValuers are the valuers for each of the output values declared
	// in the module. The result value of a module instance is an object
	// value with an attribute for each element in this map.
	OutputValuers map[addrs.OutputValue]*OnceValuer

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

	valuer, ok := m.OutputValuers[addrs.OutputValue{Name: outputName}]
	if !ok {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Reference to undeclared output value",
			Detail:   fmt.Sprintf("The child module does not declare any output value named %q.", outputName),
			Subject:  traversal[0].SourceRange().Ptr(),
		})
		return diags
	}

	diags = diags.Append(valuer.StaticCheckTraversal(traversal[1:]))
	return diags
}

// Value implements exprs.Valuer.
func (m *ModuleInstance) Value(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	// The following is mechanically similar to evaluating an object constructor
	// expression gathering all of the output value results into a single
	// object, but because we're not using the expression evaluator to do it
	// we need to explicitly discard indirect diagnostics with
	// [diagsHandledElsewhere].
	attrs := make(map[string]cty.Value, len(m.OutputValuers))
	for addr, valuer := range m.OutputValuers {
		attrs[addr.Name] = diagsHandledElsewhere(valuer.Value(ctx))
	}
	return cty.ObjectVal(attrs), nil
}

// ValueSourceRange implements exprs.Valuer.
func (m *ModuleInstance) ValueSourceRange() *tfdiags.SourceRange {
	return m.CallDeclRange
}

// CheckAll for a ModuleInstance doesn't actually really do anything at all
// because a module instance only acts as a place to aggregate some output
// value [exprs.Valuers] and so it doesn't actually have any "children" in
// the sense this package means that. ("children" in this package means,
// for example, the relationship between a resource and its instances where
// we think of the instances as being more tightly coupled to the resource they
// belong to, likely to all be sharing the same configuration etc.)
//
// However, callers implementing [evalglue.CompiledModuleInstance.CheckAll]
// should still call this method for completeness, just in case it begins
// doing something in future.
func (m *ModuleInstance) CheckAll(ctx context.Context) tfdiags.Diagnostics {
	return nil
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
	for addr, valuer := range m.OutputValuers {
		announce(valuer.RequestID(), grapheval.RequestInfo{
			Name:        fmt.Sprintf("%s value for %s", m.Addr, addr),
			SourceRange: m.ValueSourceRange(),
		})
	}
}
