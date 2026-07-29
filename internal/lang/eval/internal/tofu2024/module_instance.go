// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"context"
	"fmt"
	"iter"
	"maps"

	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/getproviders"
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

	missingProviders rootMissingProviders

	providerRequirements func(ctx context.Context) (getproviders.Requirements, *getproviders.ProvidersQualification, tfdiags.Diagnostics)
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
		// We also need to visit the module instances that each of the
		// calls caused so we can find errors within their own objects.
		cg.Await(ctx, func(ctx context.Context) {
			insts := n.Instances(ctx)
			for _, inst := range insts {
				// We assume that we're only dealing with ModuleCallInstance
				// objects that this package compiled and therefore the
				// "Glue" implementation should always be our one and
				// we can therefore use it to get the compiled child instance.
				glue := inst.Glue.(*moduleCallInstanceGlue)
				maybeCompiled, _ := glue.compiledModuleInstance(ctx)
				compiled, ok := maybeCompiled.ValueOk()
				if !ok {
					return
				}
				cg.CheckChild(ctx, compiled)
			}
		})
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

// ResourceInstanceObjectMeta implements [evalglue.CompiledModuleInstance].
func (c *CompiledModuleInstance) ResourceInstanceObjectMeta(ctx context.Context, addr addrs.ResourceInstanceObject) *evalglue.ConfiguredResourceInstanceObjectMeta {
	// TODO: The logic here should also consider whether any "removed" block
	// matches the requested object, and return information derived from
	// that block if so.

	// We'll start with a suitable placeholder to use if there's no mention
	// of this object in the configuration at all, and then improve it gradually
	// as we find relevant information in the configuration.
	ret := &evalglue.ConfiguredResourceInstanceObjectMeta{
		// In this language edition the author-specified resource type always
		// matches the provider's chosen resource type name, so we can just
		// derive these directly from the resource address.
		Provider:     addrs.ImpliedProviderForUnqualifiedType(addr.InstanceAddr.Resource.ImpliedProvider()),
		ResourceMode: addr.InstanceAddr.Resource.Mode,
		ResourceType: addr.InstanceAddr.Resource.Type,

		// An undeclared resource has no configured provider instance, which
		// means that a caller can know to fall back to an address specified in
		// the prior state, if any.
		ProviderInstance: exprs.Known[*addrs.AbsProviderInstanceCorrect](nil),
	}

	rsrc, ok := c.resourceNodes[addr.InstanceAddr.Resource]
	if !ok {
		// We'll just return the placeholder, then!
		return ret
	}

	// In the remaining code we intentionally ignore all diagnostics from
	// accessing the configgraph.Resource and configgraph.ResourceInstance
	// methods because our caller is expected to collect them separately
	// using [CompiledModuleInstance.CheckAll].

	preventDestroyVal, _, _ := rsrc.PreventDestroy(ctx)
	preventDestroy, _ := exprs.DeriveFromValue(preventDestroyVal, func(v cty.Value) (bool, error) {
		if v.True() {
			return true, nil
		}
		if v.False() {
			return false, nil
		}
		// No other value is expected, based on the documentation of [configgraph.Resource.PreventDestroy].
		panic(fmt.Sprintf("method PreventDestroy for %s returned unexpected value %#v", addr.InstanceAddr.Resource, v))
	})
	ret.DeletionInvalid = preventDestroy

	insts := rsrc.Instances(ctx)
	inst, ok := insts[addr.InstanceAddr.Key]
	if !ok {
		// Only the resource-level settings are used when we're dealing with
		// a resource instance that is not currently declared in the configuration.
		return ret
	}

	providerInst, _ := inst.ProviderInstance(ctx)
	ret.ProviderInstance, _ = exprs.DeriveFromDerived(providerInst, func(providerInst *configgraph.ProviderInstance) (*addrs.AbsProviderInstanceCorrect, error) {
		return &providerInst.Addr, nil
	})

	// TODO: All of the other fields of ConfiguredResourceInstanceObjectMeta

	return ret
}

// ChildModuleCalls implements evalglue.CompiledModuleInstance.
func (c *CompiledModuleInstance) ChildModuleCalls(_ context.Context) iter.Seq[addrs.ModuleCall] {
	return maps.Keys(c.moduleCallNodes)
}

// ChildModuleInstance implements evalglue.CompiledModuleInstance.
func (c *CompiledModuleInstance) ChildModuleInstance(ctx context.Context, addr addrs.ModuleCallInstance) evalglue.CompiledModuleInstance {
	callNode, ok := c.moduleCallNodes[addr.Call]
	if !ok {
		return nil
	}
	callInsts := callNode.Instances(ctx)
	callInst, ok := callInsts[addr.Key]
	if !ok {
		return nil
	}
	// We assume that we're only dealing with ModuleCallInstance
	// objects that this package compiled and therefore the
	// "Glue" implementation should always be our one and
	// we can therefore use it to get the compiled child instance.
	glue := callInst.Glue.(*moduleCallInstanceGlue)
	maybeCompiled, _ := glue.compiledModuleInstance(ctx)
	compiled, ok := maybeCompiled.ValueOk()
	if !ok {
		return nil
	}
	return compiled
}

// ChildModuleInstances implements evalglue.CompiledModuleInstance.
func (c *CompiledModuleInstance) ChildModuleInstances(ctx context.Context) iter.Seq2[addrs.ModuleCallInstance, evalglue.CompiledModuleInstance] {
	ctx = grapheval.ContextWithNewWorker(ctx)
	return func(yield func(addrs.ModuleCallInstance, evalglue.CompiledModuleInstance) bool) {
		for callAddr := range c.ChildModuleCalls(ctx) {
			for instAddr, compiled := range c.ChildModuleInstancesForCall(ctx, callAddr) {
				if !yield(instAddr, compiled) {
					return
				}
			}
		}
	}
}

// ChildModuleInstancesForCall implements evalglue.CompiledModuleInstance.
func (c *CompiledModuleInstance) ChildModuleInstancesForCall(ctx context.Context, callAddr addrs.ModuleCall) iter.Seq2[addrs.ModuleCallInstance, evalglue.CompiledModuleInstance] {
	ctx = grapheval.ContextWithNewWorker(ctx)
	return func(yield func(addrs.ModuleCallInstance, evalglue.CompiledModuleInstance) bool) {
		callNode, ok := c.moduleCallNodes[callAddr]
		if !ok {
			return
		}
		for instKey, callInst := range callNode.Instances(ctx) {
			addr := callAddr.Instance(instKey)
			// We assume that we're only dealing with ModuleCallInstance
			// objects that this package compiled and therefore the
			// "Glue" implementation should always be our one and
			// we can therefore use it to get the compiled child instance.
			glue := callInst.Glue.(*moduleCallInstanceGlue)
			maybeCompiled, _ := glue.compiledModuleInstance(ctx)
			compiled, ok := maybeCompiled.ValueOk()
			if !ok {
				continue
			}
			if !yield(addr, compiled) {
				return
			}
		}
	}
}

// ProviderInstances implements evalglue.CompiledModuleInstance.
func (c *CompiledModuleInstance) ProviderInstances(ctx context.Context) iter.Seq[*configgraph.ProviderInstance] {
	ctx = grapheval.ContextWithNewWorker(ctx)
	return func(yield func(*configgraph.ProviderInstance) bool) {
		for _, node := range c.providerConfigNodes {
			for _, compiled := range node.Instances(ctx) {
				if !yield(compiled) {
					return
				}
			}
		}
	}
}

// ProviderInstance implements evalglue.CompiledModuleInstance.
func (c *CompiledModuleInstance) ProviderInstance(ctx context.Context, addr addrs.ProviderInstanceCorrect) *configgraph.ProviderInstance {
	localName, ok := c.providerLocalNames[addr.Config.Provider]
	if !ok {
		localName = addr.Config.Provider.Type
	}
	localAddr := addrs.LocalProviderConfig{
		LocalName: localName,
		Alias:     addr.Config.Alias,
	}
	node, ok := c.providerConfigNodes[localAddr]
	if !ok && c.missingProviders != nil {
		// Try root fallback
		var diags tfdiags.Diagnostics
		node, diags = c.missingProviders(localAddr)
		// TODO we are swallowing a diagnostic here
		ok = !diags.HasErrors()
	}
	if !ok {
		return nil
	}
	// This call is where we will block if there isn't yet enough information
	// to evaluate the expression that decides the instances.
	insts := node.Instances(ctx)
	return insts[addr.Key]
}

// Resources implements evalglue.CompiledModuleInstance.
func (c *CompiledModuleInstance) Resources(_ context.Context) iter.Seq[addrs.Resource] {
	return maps.Keys(c.resourceNodes)
}

// Resource implements evalglue.CompiledModuleInstance.
func (c *CompiledModuleInstance) Resource(ctx context.Context, addr addrs.Resource) *configgraph.Resource {
	return c.resourceNodes[addr]
}

// ResourceInstances implements evalglue.CompiledModuleInstance.
func (c *CompiledModuleInstance) ResourceInstances(ctx context.Context) iter.Seq[*configgraph.ResourceInstance] {
	return func(yield func(*configgraph.ResourceInstance) bool) {
		for addr := range c.Resources(ctx) {
			for inst := range c.ResourceInstancesForResource(ctx, addr) {
				if !yield(inst) {
					return
				}
			}
		}
	}
}

// ResourceInstancesForResource implements evalglue.CompiledModuleInstance.
func (c *CompiledModuleInstance) ResourceInstancesForResource(ctx context.Context, addr addrs.Resource) iter.Seq[*configgraph.ResourceInstance] {
	return func(yield func(*configgraph.ResourceInstance) bool) {
		r, ok := c.resourceNodes[addr]
		if !ok {
			return
		}
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

// ResourceInstancesForResource implements evalglue.ProviderRequirements.
func (c *CompiledModuleInstance) ProviderRequirements(ctx context.Context) (getproviders.Requirements, *getproviders.ProvidersQualification, tfdiags.Diagnostics) {
	return c.providerRequirements(ctx)
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
