// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"context"
	"iter"

	"github.com/apparentlymart/go-workgraph/workgraph"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// CompiledModuleInstance is our implementation of
// [evalglue.CompiledModuleInstance], providing the API that package eval
// uses to interact with tofu2024-edition modules.
type CompiledModuleInstance struct {
	// Any other kinds of "node" we add in future will likely need coverage
	// added in both [CompiledModuleInstance.CheckAll] and
	// [CompiledModuleInstance.AnnounceAllGraphevalRequests].
	moduleInstanceNode  *configgraph.ModuleInstance
	inputVariableNodes  map[addrs.InputVariable]*configgraph.InputVariable
	localValueNodes     map[addrs.LocalValue]*configgraph.LocalValue
	outputValueNodes    map[addrs.OutputValue]*configgraph.OutputValue
	resourceNodes       map[addrs.Resource]*configgraph.Resource
	moduleCallNodes     map[addrs.ModuleCall]*configgraph.ModuleCall
	providerConfigNodes map[addrs.LocalProviderConfig]*configgraph.ProviderConfig
	providerLocalNames  map[addrs.Provider]string
}

var _ evalglue.CompiledModuleInstance = (*CompiledModuleInstance)(nil)

// CheckAll implements evalglue.CompiledModuleInstance.
func (c *CompiledModuleInstance) CheckAll(ctx context.Context) tfdiags.Diagnostics {
	var cg configgraph.CheckGroup
	cg.CheckChild(ctx, c.moduleInstanceNode)
	for _, n := range c.inputVariableNodes {
		cg.CheckChild(ctx, n)
	}
	for _, n := range c.localValueNodes {
		cg.CheckChild(ctx, n)
	}
	for _, n := range c.outputValueNodes {
		cg.CheckChild(ctx, n)
	}
	for _, n := range c.resourceNodes {
		cg.CheckChild(ctx, n)
	}
	for _, n := range c.moduleCallNodes {
		cg.CheckChild(ctx, n)
	}
	for _, n := range c.providerConfigNodes {
		cg.CheckChild(ctx, n)
	}
	// TODO: Once we have support for module calls we'll need to check both
	// the calls _and_ the nested [evalglue.CompiledModuleInstance] objects
	// that encapsulate the child contents, which might potentially use a
	// different implementation of [evalglue.CompiledModuleInstance] if the
	// other module is targeting for a different language edition.
	return cg.Complete(ctx)
}

// ResultValuer implements evalglue.CompiledModuleInstance.
func (c *CompiledModuleInstance) ResultValuer(ctx context.Context) exprs.Valuer {
	// This causes our module instance to effectively be bridged directly into
	// the calling module instance, without the caller needing to be aware
	// of that implementation detail.
	return c.moduleInstanceNode
}

// ChildModuleInstance implements evalglue.CompiledModuleInstance.
func (c *CompiledModuleInstance) ChildModuleInstance(ctx context.Context, addr addrs.ModuleCallInstance) evalglue.CompiledModuleInstance {
	// TODO: rework our internal API here so that we can actually answer
	// this question. The current structure makes this impossible because
	// the child [CompiledModuleInstance] only exists temporarily as a
	// local variable when building a child instance's result value.
	return nil
}

// ChildModuleInstances implements evalglue.CompiledModuleInstance.
func (c *CompiledModuleInstance) ChildModuleInstances(ctx context.Context) iter.Seq2[addrs.ModuleCallInstance, evalglue.CompiledModuleInstance] {
	// TODO: rework our internal API here so that we can actually answer
	// this question. The current structure makes this impossible because
	// the child [CompiledModuleInstance] only exists temporarily as a
	// local variable when building a child instance's result value.
	return func(yield func(addrs.ModuleCallInstance, evalglue.CompiledModuleInstance) bool) {}
}

// ProviderInstance implements evalglue.CompiledModuleInstance.
func (c *CompiledModuleInstance) ProviderInstance(ctx context.Context, addr addrs.ProviderInstanceCorrect) *configgraph.ProviderInstance {
	localName, ok := c.providerLocalNames[addr.Config.Provider]
	if !ok {
		return nil
	}
	localAddr := addrs.LocalProviderConfig{
		LocalName: localName,
		Alias:     addr.Config.Alias,
	}
	node, ok := c.providerConfigNodes[localAddr]
	if !ok {
		return nil
	}
	// This call is where we will block if there isn't yet enough information
	// to evaluate the expression that decides the instances.
	insts := node.Instances(ctx)
	return insts[addr.Key]
}

// ResourceInstances implements evalglue.CompiledModuleInstance.
func (c *CompiledModuleInstance) ResourceInstances(ctx context.Context) iter.Seq[*configgraph.ResourceInstance] {
	return func(yield func(*configgraph.ResourceInstance) bool) {
		for _, r := range c.resourceNodes {
			// NOTE: r.Instances will block if the resource's [InstanceSelector]
			// depends on other parts of the configuration that aren't yet
			// ready to produce their value.
			for _, inst := range r.Instances(ctx) {
				if !yield(inst) {
					return
				}
			}
		}
	}
}

// ProviderInstancesDeep implements evalglue.CompiledModuleInstance.
func (c *CompiledModuleInstance) ProviderInstancesDeep(ctx context.Context) iter.Seq[*configgraph.ProviderInstance] {
	return func(yield func(*configgraph.ProviderInstance) bool) {
		for _, r := range c.providerConfigNodes {
			// NOTE: r.Instances will block if the provider config's
			// [InstanceSelector] depends on other parts of the configuration
			// that aren't yet ready to produce their value.
			for _, inst := range r.Instances(ctx) {
				if !yield(inst) {
					return
				}
			}
		}

		// TODO: Collect provider instances from child module calls too.
	}
}

// AnnounceAllGraphevalRequests implements evalglue.CompiledModuleInstance.
func (c *CompiledModuleInstance) AnnounceAllGraphevalRequests(announce func(workgraph.RequestID, grapheval.RequestInfo)) {
	c.moduleInstanceNode.AnnounceAllGraphevalRequests(announce)
	for _, n := range c.inputVariableNodes {
		n.AnnounceAllGraphevalRequests(announce)
	}
	for _, n := range c.localValueNodes {
		n.AnnounceAllGraphevalRequests(announce)
	}
	for _, n := range c.outputValueNodes {
		n.AnnounceAllGraphevalRequests(announce)
	}
	for _, n := range c.resourceNodes {
		n.AnnounceAllGraphevalRequests(announce)
	}
	for _, n := range c.moduleCallNodes {
		n.AnnounceAllGraphevalRequests(announce)
	}
	for _, n := range c.providerConfigNodes {
		n.AnnounceAllGraphevalRequests(announce)
	}
}
