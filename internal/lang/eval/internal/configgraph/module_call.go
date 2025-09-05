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
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type ModuleCall struct {
	Addr      addrs.AbsModuleCall
	DeclRange tfdiags.SourceRange

	// InstanceSelector represents a rule for deciding which instances of
	// this resource have been declared.
	InstanceSelector InstanceSelector

	// CompileCallInstance is a callback function provided by whatever
	// compiled this [ModuleCall] object that knows how to produce a compiled
	// [ModuleCallInstance] object once we know of the instance key and
	// associated repetition data for it.
	//
	// This indirection allows the caller to take into account the same
	// context it had available when it built this [ModuleCall] object, while
	// incorporating the new information about this specific instance.
	CompileCallInstance func(ctx context.Context, key addrs.InstanceKey, repData instances.RepetitionData) *ModuleCallInstance

	// instancesResult tracks the process of deciding which instances are
	// currently declared for this provider config, and the result of that process.
	//
	// Only the decideInstances method accesses this directly. Use that
	// method to obtain the coalesced result for use elsewhere.
	instancesResult grapheval.Once[*compiledInstances[*ModuleCallInstance]]
}

var _ exprs.Valuer = (*ModuleCall)(nil)

// Instances returns the instances that are selected for this module call in
// its configuration, without evaluating their configuration objects yet.
func (c *ModuleCall) Instances(ctx context.Context) map[addrs.InstanceKey]*ModuleCallInstance {
	// We ignore the diagnostics here because they will be returned by
	// the Value method instead.
	result, _ := c.decideInstances(ctx)
	return result.Instances
}

func (c *ModuleCall) decideInstances(ctx context.Context) (*compiledInstances[*ModuleCallInstance], tfdiags.Diagnostics) {
	return c.instancesResult.Do(ctx, func(ctx context.Context) (*compiledInstances[*ModuleCallInstance], tfdiags.Diagnostics) {
		return compileInstances(ctx, c.InstanceSelector, c.CompileCallInstance)
	})
}

// StaticCheckTraversal implements exprs.Valuer.
func (c *ModuleCall) StaticCheckTraversal(traversal hcl.Traversal) tfdiags.Diagnostics {
	return staticCheckTraversalForInstances(c.InstanceSelector, traversal)
}

// Value implements exprs.Valuer.
func (c *ModuleCall) Value(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	selection, diags := c.decideInstances(ctx)
	return valueForInstances(ctx, selection), diags
}

// ValueSourceRange implements exprs.Valuer.
func (c *ModuleCall) ValueSourceRange() *tfdiags.SourceRange {
	return &c.DeclRange
}

// CheckAll implements allChecker.
func (c *ModuleCall) CheckAll(ctx context.Context) tfdiags.Diagnostics {
	var cg CheckGroup
	// Our InstanceSelector itself might block on expression evaluation,
	// so we'll run it async as part of the checkGroup.
	cg.Await(ctx, func(ctx context.Context) {
		for _, inst := range c.Instances(ctx) {
			cg.CheckValuer(ctx, inst)
		}
	})
	// This is where an invalid for_each expression would be reported.
	cg.CheckValuer(ctx, c)
	return cg.Complete(ctx)
}

func (c *ModuleCall) AnnounceAllGraphevalRequests(announce func(workgraph.RequestID, grapheval.RequestInfo)) {
	// There might be other grapheval requests in our dynamic instances, but
	// they are hidden behind another request themselves so we'll try to
	// report them only if that request was already started.
	instancesReqId := c.instancesResult.RequestID()
	if instancesReqId == workgraph.NoRequest {
		return
	}
	announce(instancesReqId, grapheval.RequestInfo{
		Name:        fmt.Sprintf("decide instances for %s", c.Addr),
		SourceRange: c.InstanceSelector.InstancesSourceRange(),
	})
	// The Instances method potentially starts a new request, but we already
	// confirmed above that this request was already started and so we
	// can safely just await its result here.
	for _, inst := range c.Instances(grapheval.ContextWithNewWorker(context.Background())) {
		inst.AnnounceAllGraphevalRequests(announce)
	}
}
