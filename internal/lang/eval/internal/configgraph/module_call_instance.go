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

	// SourceAddrValuer and VersionConstraintValuer together describe how
	// to select the module to be called.
	//
	// Allowing the entire module content to vary between phases is too
	// much chaos for our plan/apply model to really support, so we make
	// a pragmatic compromise here of disallowing the results from these
	// to be derived from any resource instances (even if the value happens
	// to be currently known) and just generally disallowing unknown
	// values regardless of where they are coming from. In practice resource
	// instances are the main place unknown values come from, but this
	// also excludes specifying the module to use based on impure functions
	// like "timestamp" whose results aren't decided until the apply step.
	//
	// These are associated with call instances rather than the main call,
	// and so it's possible for different instances of the same call to
	// select completely different modules. While that's a somewhat esoteric
	// thing to do, it would make it possible to e.g. write a module call that
	// uses for_each where the associated values choose between multiple
	// implementations of the same general abstraction. However, our surface
	// language doesn't currently allow that -- it always evaluates these
	// in the global scope rather than per-instance scope -- because the
	// way module blocks are currently designed means that HCL wants the
	// set of arguments to be fixed statically rather than chosen
	// dynamically.
	SourceAddrValuer        *OnceValuer
	VersionConstraintValuer *OnceValuer

	// InputsValuer is a valuer for all of the input variable values taken
	// together as a single object. It's structured this way mainly for
	// consistency with how we deal with the objects representing arguments
	// in other blocks, but it also means that a future edition of the
	// language could potentially use different syntax for input variables
	// that allows constructing the entire map dynamically using expression
	// syntax.
	InputsValuer *OnceValuer
}

var _ exprs.Valuer = (*ModuleCallInstance)(nil)

// StaticCheckTraversal implements exprs.Valuer.
func (m *ModuleCallInstance) StaticCheckTraversal(traversal hcl.Traversal) tfdiags.Diagnostics {
	// We only do dynamic checks of accessing a module call because we
	// can't know what result type it will return without fetching and
	// compiling the child module source code, and that's too heavy
	// an operation for "static check".
	return nil
}

// Value implements exprs.Valuer.
func (m *ModuleCallInstance) Value(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	// TODO: Evaluate the source address and version constraint and then
	// use a new field with a callback to ask the compile layer to compile
	// us a [evalglue.CompiledModuleInstance] for the child module, and
	// then ask for its result value and return it.
	panic("unimplemented")
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
	announce(m.SourceAddrValuer.RequestID(), grapheval.RequestInfo{
		Name:        m.ModuleInstanceAddr.String() + " source address",
		SourceRange: m.SourceAddrValuer.ValueSourceRange(),
	})
	announce(m.VersionConstraintValuer.RequestID(), grapheval.RequestInfo{
		Name:        m.ModuleInstanceAddr.String() + " version constraint",
		SourceRange: m.VersionConstraintValuer.ValueSourceRange(),
	})
	announce(m.InputsValuer.RequestID(), grapheval.RequestInfo{
		Name:        m.ModuleInstanceAddr.String() + " input variable values",
		SourceRange: m.InputsValuer.ValueSourceRange(),
	})
}
