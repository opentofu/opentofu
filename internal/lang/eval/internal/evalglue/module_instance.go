// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package evalglue

import (
	"context"
	"iter"

	"github.com/apparentlymart/go-workgraph/workgraph"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// CompiledModuleInstance is the interface implemented by the top-level object
// that our "module compiler" layer returns, which this package's exported
// functions use to access [configgraph] objects whose results we translate to
// the package's public API.
//
// A [CompiledModuleInstance] represents a top-level module instance _and_ all
// of the module instances beneath it. This is what ties together all of the
// [configgraph] nodes representing a whole configuration tree to provide
// global information. Logic in package eval typically interacts directly only
// with the [CompiledModuleInstance] object representing the root module of
// the configuration, but that object will internally use objects of this
// type to delegate to child module instances that might be using different
// implementations of this interface.
//
// This module instance boundary is therefore the layer that all language
// editions/experiments must agree on to some extent. Edition-based language
// evolution will need to carefully consider how to handle any feature that
// affects this API boundary between parent and child module instances, to
// ensure that modules of different editions are still able to interact
// successfully.
type CompiledModuleInstance interface {
	// CheckAll collects diagnostics for everything in this module instance
	// and its child module instances, recursively.
	//
	// All callers in package eval should call this method at some point on every
	// [CompiledModuleInstance] they construct, because this is the only
	// method that guarantees to visit everything declared in the configuration
	// and make sure it gets a chance to evaluate itself and return diagnostics.
	// (Other methods will typically only interact with parts of the config
	// that are relevant to the questions they are answering.)
	//
	// If the [Glue] implementation passed to this module instance has
	// operations that block on the completion of outside operations then this
	// function must run concurrently with those outside operations (i.e. in
	// a separate goroutine) because CheckAll will block until all of the
	// values in the configuration have been resolved, which means that e.g.
	// in the planning phase this won't return until all resources have been
	// planned and so their "planned new state" values have been decided.
	CheckAll(ctx context.Context) tfdiags.Diagnostics

	// ResultValuer returns the [exprs.Valuer] representing the module
	// instance's overall result value, which is what should be used to
	// represent this module instance when referred to in its parent module.
	//
	// In the current language this always has an object type whose attribute
	// names match the output values declared in the child module, but we
	// don't enforce that at this layer to allow the result to potentially
	// use special cty features like marks and unknown values when needed.
	// Callers require this to be a known object type should work defensively
	// to do _something_ reasonable -- even if just returning an error message
	// about the module instance returning an unsuitable value -- so that
	// the decisions here can potentially evolve in future without causing
	// panics or other misbehavior.
	ResultValuer(ctx context.Context) exprs.Valuer

	// ChildModuleCalls returns a sequence of addresses of all of the module
	// calls that are declared in this module instance.
	//
	// The set of _calls_ can currently be assumed to be statically declared
	// in the configuration, and so should be immediately available once
	// a module instance has been compiled. Use
	// [CompiledModuleInstance.ChildModuleInstancesForCall] to find the
	// dynamically-decided call instances for each call.
	ChildModuleCalls(ctx context.Context) iter.Seq[addrs.ModuleCall]

	// ChildModuleInstances returns a sequence of all of the child module
	// instances that are declared by calls in this module instance.
	//
	// The set of child module instances can be decided dynamically based on
	// references to other objects, and so reads from the returned sequence
	// may block until the needed upstream objects have finished resolving.
	//
	// Some of the enumerated objects might be placeholders for zero or more
	// instances where there isn't yet enough information to determine exactly
	// which dynamic instances are declared.
	ChildModuleInstances(ctx context.Context) iter.Seq2[addrs.ModuleCallInstance, CompiledModuleInstance]

	// ChildModuleInstancesForCall returns a sequence of all of the child module
	// instances that are declared by the specific call given in callAddr.
	//
	// This has the same caveats as
	// [CompiledModuleInstance.ChildModuleInstances] except that this will block
	// only on deciding the instances for the specific module call given, and
	// not on the expansion of any other module calls unless those decisions
	// are indirectly needed to decide the requested call.
	ChildModuleInstancesForCall(ctx context.Context, callAddr addrs.ModuleCall) iter.Seq2[addrs.ModuleCallInstance, CompiledModuleInstance]

	// ChildModuleInstance returns a single child module instance with the
	// given address, or nil if there is no such instance declared.
	//
	// This blocks on the decision about which instances are available for the
	// relevant module call.
	ChildModuleInstance(ctx context.Context, addr addrs.ModuleCallInstance) CompiledModuleInstance

	// Resources returns a sequence of addresses of all of the resources
	// that are declared in this module instance.
	//
	// The set of _resources_ can currently be assumed to be statically declared
	// in the configuration, and so should be immediately available once
	// a module instance has been compiled. Use
	// [CompiledModuleInstance.ResourceInstancesForResource] to find the
	// dynamically-decided resource instances for each resource.
	Resources(ctx context.Context) iter.Seq[addrs.Resource]

	// ResourceInstances returns a sequence of all of the resource instances
	// declared in the module.
	//
	// The set of resource instances can be decided dynamically based on
	// references to other objects, and so reads from the returned sequence
	// may block until the needed upstream objects have finished resolving.
	//
	// Some of the enumerated objects might be placeholders for zero or more
	// instances where there isn't yet enough information to determine exactly
	// which dynamic instances are declared.
	ResourceInstances(ctx context.Context) iter.Seq[*configgraph.ResourceInstance]

	// ResourceInstancesForResource returns a sequence of all of the resource
	// instances declared for the given resource in the module.
	//
	// This has the same caveats as [CompiledModuleInstance.ResourceInstances]
	// except that this will block only on deciding the instances for the
	// specific resource given, and not on the expansion of any other resources
	// unless those decisions are indirectly needed to decide the requested
	// resource.
	ResourceInstancesForResource(ctx context.Context, addr addrs.Resource) iter.Seq[*configgraph.ResourceInstance]

	// ProviderInstances returns a sequence of all of the provider instances
	// declared in the module.
	//
	// The set of provider instances can be decided dynamically based on
	// references to other objects, and so reads from the returned sequence
	// may block until the needed upstream objects have finished resolving.
	//
	// Some of the enumerated objects might be placeholders for zero or more
	// instances where there isn't yet enough information to determine exactly
	// which dynamic instances are declared.
	ProviderInstances(ctx context.Context) iter.Seq[*configgraph.ProviderInstance]

	// ProviderInstance returns the [configgraph.ProviderInstance]
	// representation of the provider instance with the given address, or
	// nil if there is no such instance declared.
	//
	// This blocks on the decision about which instances are available for the
	// relevant provider config.
	ProviderInstance(ctx context.Context, addr addrs.ProviderInstanceCorrect) *configgraph.ProviderInstance

	// AnnounceAllGraphevalRequests calls announce for each [grapheval.Once],
	// [OnceValuer], or other [workgraph.RequestID] anywhere in the tree under this
	// object.
	//
	// This is used only when [workgraph] detects a self-dependency or failure to
	// resolve and we want to find a nice human-friendly name and optional source
	// range to use to describe each of the requests that were involved in the
	// problem.
	AnnounceAllGraphevalRequests(announce func(workgraph.RequestID, grapheval.RequestInfo))
}

// ModuleInstance finds the [CompiledModuleInstance] representation of the
// module instance with the given address, or nil if there is no such instance.
//
// The decision about which instances exist can be made dynamically by arbitrary
// expressions, so this call will block until the necessary information is
// resolved.
func ModuleInstance(ctx context.Context, root CompiledModuleInstance, addr addrs.ModuleInstance) CompiledModuleInstance {
	current := root
	for _, step := range addr {
		callInstAddr := addrs.ModuleCallInstance{
			Call: addrs.ModuleCall{Name: step.Name},
			Key:  step.InstanceKey,
		}
		current = current.ChildModuleInstance(ctx, callInstAddr)
		if current == nil {
			return nil
		}
	}
	return current
}

// ModuleInstancesDeep produces all of the module instances from the given root
// and down into the module tree.
//
// The decision about which instances exist can be made dynamically by arbitrary
// expressions, so any step in the returned sequence may block until further
// information becomes available.
//
// The [addrs.ModuleInstance] values provided as the first results may share
// backing arrays and so if the caller wants to keep them for more than one
// iteration they must copy the address to a new slice over a privately-owned
// backing array.
func ModuleInstancesDeep(ctx context.Context, root CompiledModuleInstance) iter.Seq2[addrs.ModuleInstance, CompiledModuleInstance] {
	// We use a separate workgraph worker here because when using [iter.Seq2]
	// in a range loop the call to the returned function is treated as a
	// coroutine with the loop and so the two execution paths can potentially
	// block one another.
	ctx = grapheval.ContextWithNewWorker(ctx)

	// We'll start with enough capacity for four levels of nesting and then
	// reuse this array for all calls to minimize garbage. We'll only reallocate
	// if there are more than four levels of module nesting.
	moduleInstAddr := make(addrs.ModuleInstance, 0, 4)
	return func(yield func(addrs.ModuleInstance, CompiledModuleInstance) bool) {
		yieldModuleInstancesDeep(ctx, moduleInstAddr, root, yield)
	}
}

// yieldModuleInstancesDeep is the recursive body of [ModuleInstancesDeep].
func yieldModuleInstancesDeep(ctx context.Context, instAddr addrs.ModuleInstance, current CompiledModuleInstance, yield func(addrs.ModuleInstance, CompiledModuleInstance) bool) bool {
	if !yield(instAddr, current) {
		return false
	}
	for callAddr, inst := range current.ChildModuleInstances(ctx) {
		// Note: we are intentionally reusing the backing array of instAddr
		// when possible to avoid generating lots of memory allocation garbage.
		childInstAddr := append(instAddr, addrs.ModuleInstanceStep{
			Name:        callAddr.Call.Name,
			InstanceKey: callAddr.Key,
		})
		if !yieldModuleInstancesDeep(ctx, childInstAddr, inst, yield) {
			return false
		}
	}
	return true
}

// ProviderInstance digs through the tree of module instances with the given
// root to try to find the [configgraph.ResourceInstance] representation
// of the resource instance with the given address.
//
// The decision about which instances exist can be made dynamically by arbitrary
// expressions, so this call will block until the necessary information is
// resolved.
//
// This is implemented in terms of [ModuleInstance] and
// [CompiledModuleInstance.ResourceInstancesForResource].
func ResourceInstance(ctx context.Context, root CompiledModuleInstance, addr addrs.AbsResourceInstance) *configgraph.ResourceInstance {
	moduleInst := ModuleInstance(ctx, root, addr.Module)
	if moduleInst == nil {
		return nil
	}
	// For now we use the method that enumerates all of the instances of
	// a resource and try to pluck out the one with a matching instance key.
	// That therefore avoids [CompiledModuleInstance] needing a separate method
	// for pulling out an individual resource instance. However, this is a
	// little inefficient so we might want to push this responsibility down into
	// a new method of CompiledModuleInstance eventually.
	for ri := range moduleInst.ResourceInstancesForResource(ctx, addr.Resource.Resource) {
		if ri.Addr.Equal(addr) {
			return ri
		}
	}
	return nil
}

// ResourceInstancesDeep produces all of the resource instances across the given
// root module instance and all of its descendents.
//
// The decision about which instances exist can be made dynamically by arbitrary
// expressions, so any step in the returned sequence may block until further
// information becomes available.
//
// This is implemented in terms of [ModuleInstancesDeep].
func ResourceInstancesDeep(ctx context.Context, root CompiledModuleInstance) iter.Seq[*configgraph.ResourceInstance] {
	ctx = grapheval.ContextWithNewWorker(ctx)
	return func(yield func(*configgraph.ResourceInstance) bool) {
		for _, moduleInst := range ModuleInstancesDeep(ctx, root) {
			for resourceInst := range moduleInst.ResourceInstances(ctx) {
				if !yield(resourceInst) {
					return
				}
			}
		}
	}
}

// ProviderInstancesDeep produces all of the provider instances across the given
// root module instance and all of its descendents.
//
// The decision about which instances exist can be made dynamically by arbitrary
// expressions, so any step in the returned sequence may block until further
// information becomes available.
//
// This is implemented in terms of [ModuleInstancesDeep].
func ProviderInstancesDeep(ctx context.Context, root CompiledModuleInstance) iter.Seq[*configgraph.ProviderInstance] {
	ctx = grapheval.ContextWithNewWorker(ctx)
	return func(yield func(*configgraph.ProviderInstance) bool) {
		for _, moduleInst := range ModuleInstancesDeep(ctx, root) {
			for providerInst := range moduleInst.ProviderInstances(ctx) {
				if !yield(providerInst) {
					return
				}
			}
		}
	}
}

// ProviderInstance digs through the tree of module instances with the given
// root to try to find the [configgraph.ProviderInstance] representation
// of the provider instance with the given address.
//
// The decision about which instances exist can be made dynamically by arbitrary
// expressions, so this call will block until the necessary information is
// resolved.
//
// This is implemented in terms of [ModuleInstance] and
// [CompiledModuleInstance.ProviderInstance].
func ProviderInstance(ctx context.Context, root CompiledModuleInstance, addr addrs.AbsProviderInstanceCorrect) *configgraph.ProviderInstance {
	moduleInst := ModuleInstance(ctx, root, addr.Config.Module)
	if moduleInst == nil {
		return nil
	}
	return moduleInst.ProviderInstance(ctx, addr.LocalConfig())
}
