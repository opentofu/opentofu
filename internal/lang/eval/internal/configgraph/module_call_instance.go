// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"

	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type ModuleCallInstance struct {
	// ModuleInstanceAddr is the address of the module instance that this
	// call instance is establishing.
	//
	// The difference between an instance of a module call and an instance
	// of a module is a little fussy and pedantic: the call instance is
	// viewed from the perspective of the caller while the module instance
	// is viewed from the perspective of the callee. But outside of package
	// configgraph that is not a distinction we make and so we don't have
	// a separate address type for an "absolute module call instance".
	ModuleInstanceAddr addrs.ModuleInstance

	// Glue is provided by whatever compiled this object to allow us to learn
	// more about the module that is being called.
	Glue ModuleCallInstanceGlue

	// InputsValuer is a valuer for all of the input variable values taken
	// together as a single object. It's structured this way mainly for
	// consistency with how we deal with the objects representing arguments
	// in other blocks, but it also means that a future edition of the
	// language could potentially use different syntax for input variables
	// that allows constructing the entire map dynamically using expression
	// syntax.
	InputsValuer *OnceValuer

	// TODO: Also something for the "providers side-channel", as represented
	// by the "providers" meta-argument in the current language.
}

var _ exprs.Valuer = (*ModuleCallInstance)(nil)

// StaticCheckTraversal implements exprs.Valuer.
func (m *ModuleCallInstance) StaticCheckTraversal(traversal hcl.Traversal) tfdiags.Diagnostics {
	// We don't perform any static type checking of accesses to a module's
	// output value. Instead, we just wait until we have the final result
	// in Value.
	return nil
}

// Value implements exprs.Valuer.
func (m *ModuleCallInstance) Value(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	inputsVal, diags := m.InputsValuer.Value(ctx)
	if diags.HasErrors() {
		return cty.DynamicVal, diags
	}

	moreDiags := m.Glue.ValidateInputs(ctx, inputsVal)
	// FIXME: Our "contextual diagnostics" mechanism, where the callee provides
	// an attribute path and then the caller discovers a suitable source range
	// for each diagnostic based on information in the body, can only work
	// when we have direct access to a [hcl.Body], but we intentionally
	// abstracted that away here. We'll need to find a different design for
	// contextual diagnostics that can work through the [exprs.Valuer]
	// abstraction to make a best effort to interpret attribute paths against
	// whatever the valuer was evaluating.
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return cty.DynamicVal, diags
	}

	// The actual result value is decided by our caller, which is expected
	// to know how to actually find, compile, and evaluate the target module.
	return exprs.EvalResult(m.Glue.OutputsValue(ctx, inputsVal))
}

// ValueSourceRange implements exprs.Valuer.
func (m *ModuleCallInstance) ValueSourceRange() *tfdiags.SourceRange {
	return nil
}

// CheckAll implements allChecker.
func (c *ModuleCallInstance) CheckAll(ctx context.Context) tfdiags.Diagnostics {
	var cg CheckGroup
	cg.CheckValuer(ctx, c)
	return cg.Complete(ctx)
}

func (m *ModuleCallInstance) AnnounceAllGraphevalRequests(announce func(workgraph.RequestID, grapheval.RequestInfo)) {
	announce(m.InputsValuer.RequestID(), grapheval.RequestInfo{
		Name:        m.ModuleInstanceAddr.String() + " input variable values",
		SourceRange: m.InputsValuer.ValueSourceRange(),
	})
}
