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
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

type ProviderConfig struct {
	// FIXME: The current form of AbsProviderConfig is weird and not quite
	// right, because the "Abs" prefix is supposed to represent something
	// belonging to an addrs.ModuleInstance while this models addrs.Module
	// instead. We'll probably need to introduce some temporary new types
	// alongside the existing ones for the sake of this experiment, and then
	// have the new ones replace the old ones if we decide to move forward
	// with something like this.
	Addr      addrs.AbsProviderConfigCorrect
	DeclRange tfdiags.SourceRange

	// InstanceSelector represents a rule for deciding which instances of
	// this resource have been declared.
	InstanceSelector InstanceSelector

	// ProviderAddr is the address of the provider that this is a configuration
	// for. This object can produce zero or more instances of this provider.
	ProviderAddr addrs.Provider

	// CompileProviderInstance is a callback function provided by whatever
	// compiled this [Provider] object that knows how to produce a compiled
	// [ProviderInstance] object once we know of the instance key and associated
	// repetition data for it.
	//
	// This indirection allows the caller to take into account the same
	// context it had available when it built this [Provider] object, while
	// incorporating the new information about this specific instance.
	CompileProviderInstance func(ctx context.Context, key addrs.InstanceKey, repData instances.RepetitionData) *ProviderInstance

	// instancesResult tracks the process of deciding which instances are
	// currently declared for this provider config, and the result of that process.
	//
	// Only the decideInstances method accesses this directly. Use that
	// method to obtain the coalesced result for use elsewhere.
	instancesResult grapheval.Once[*compiledInstances[*ProviderInstance]]
}

var _ exprs.Valuer = (*ProviderConfig)(nil)

// Instances returns the instances that are selected for this provider config in
// its configuration, without evaluating their configuration objects yet.
//
// Use this when performing a tree walk to discover provider instances to
// make sure that it's possible to tell whatever process is running alongside
// that it needs to produce a result value for a particular provider instance
// before we actually request that value.
func (p *ProviderConfig) Instances(ctx context.Context) map[addrs.InstanceKey]*ProviderInstance {
	// We ignore the diagnostics here because they will be returned by
	// the Value method instead.
	result, _ := p.decideInstances(ctx)
	return result.Instances
}

func (p *ProviderConfig) decideInstances(ctx context.Context) (*compiledInstances[*ProviderInstance], tfdiags.Diagnostics) {
	return p.instancesResult.Do(ctx, func(ctx context.Context) (*compiledInstances[*ProviderInstance], tfdiags.Diagnostics) {
		return compileInstances(ctx, p.InstanceSelector, p.CompileProviderInstance)
	})
}

// StaticCheckTraversal implements exprs.Valuer.
func (p *ProviderConfig) StaticCheckTraversal(traversal hcl.Traversal) tfdiags.Diagnostics {
	return staticCheckTraversalForInstances(p.InstanceSelector, traversal)
}

// Value implements exprs.Valuer.
func (p *ProviderConfig) Value(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	selection, diags := p.decideInstances(ctx)
	return valueForInstances(ctx, selection), diags
}

// ValueSourceRange implements exprs.Valuer.
func (p *ProviderConfig) ValueSourceRange() *tfdiags.SourceRange {
	return &p.DeclRange
}

// CheckAll implements allChecker.
func (p *ProviderConfig) CheckAll(ctx context.Context) tfdiags.Diagnostics {
	var cg CheckGroup
	// Our InstanceSelector itself might block on expression evaluation,
	// so we'll run it async as part of the checkGroup.
	cg.Await(ctx, func(ctx context.Context) {
		for _, inst := range p.Instances(ctx) {
			cg.CheckChild(ctx, inst)
		}
	})
	// This is where an invalid for_each expression would be reported.
	cg.CheckValuer(ctx, p)
	return cg.Complete(ctx)
}

func (p *ProviderConfig) AnnounceAllGraphevalRequests(announce func(workgraph.RequestID, grapheval.RequestInfo)) {
	// There might be other grapheval requests in our dynamic instances, but
	// they are hidden behind another request themselves so we'll try to
	// report them only if that request was already started.
	instancesReqId := p.instancesResult.RequestID()
	if instancesReqId == workgraph.NoRequest {
		return
	}
	announce(instancesReqId, grapheval.RequestInfo{
		Name:        fmt.Sprintf("decide instances for %s", p.Addr),
		SourceRange: p.InstanceSelector.InstancesSourceRange(),
	})
	// The Instances method potentially starts a new request, but we already
	// confirmed above that this request was already started and so we
	// can safely just await its result here.
	for _, inst := range p.Instances(grapheval.ContextWithNewWorker(context.Background())) {
		inst.AnnounceAllGraphevalRequests(announce)
	}
}
