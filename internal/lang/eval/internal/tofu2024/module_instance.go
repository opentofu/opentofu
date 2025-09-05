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
	providerConfigNodes map[addrs.LocalProviderConfig]*configgraph.ProviderConfig
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

// ResourceInstancesDeep implements evalglue.CompiledModuleInstance.
func (c *CompiledModuleInstance) ResourceInstancesDeep(ctx context.Context) iter.Seq[*configgraph.ResourceInstance] {
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

		// TODO: Once we actually support child module calls, ask for the
		// instances of each one and then collect its resource instances too.
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
	for _, n := range c.providerConfigNodes {
		n.AnnounceAllGraphevalRequests(announce)
	}
}
