// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"
	"fmt"
	"iter"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
)

// A PlanningOracle provides information from the configuration that is needed
// by the planning engine to help orchestrate the planning process.
type PlanningOracle struct {
	relationships *ResourceRelationships

	// NOTE: Any method of PlanningOracle that interacts with methods of
	// this or anything accessible through it MUST use
	// [grapheval.ContextWithNewWorker] to make sure it's using a
	// workgraph-friendly context, since the methods of this type are
	// exported entry points for use by callers in other packages that
	// don't necessarily participate in workgraph directly.
	rootModuleInstance evalglue.CompiledModuleInstance
}

// ProviderInstanceConfig returns a value representing the configuration to
// use when configuring the provider instance with the given address.
//
// The result might contain unknown values, but those should still typically
// be sent to the provider so that it can decide how to deal with them. Some
// providers just immediately fail in that case, but others are able to work
// in a partially-configured mode where some resource types are plannable while
// others need to be deferred to a later plan/apply round.
//
// If the requested provider instance does not exist in the configuration at
// all then this will return [cty.NilVal]. That should not occur for any
// provider instance address reported by this package as part of the same
// planning phase, but could occur in subsequent work done by the planning
// phase to deal with resource instances that are in prior state but no longer
// in desired state, if their provider instances have also been removed from
// the desired state at the same time. In that case the planning phase must
// report that the "orphaned" resource instance cannot be planned for deletion
// unless its provider instance is re-added to the configuration.
func (o *PlanningOracle) ProviderInstanceConfig(ctx context.Context, addr addrs.AbsProviderInstanceCorrect) cty.Value {
	ctx = grapheval.ContextWithNewWorker(ctx)

	providerInst := evalglue.ProviderInstance(ctx, o.rootModuleInstance, addr)
	if providerInst == nil {
		return cty.NilVal
	}
	// We ignore diagnostics here because the CheckAll tree walk should collect
	// them when it visits the provider instance, th
	ret, _ := providerInst.ConfigValue(ctx)
	return ret
}

// ProviderInstanceUsers returns an object representing which resource instances
// are associated with the provider instance that has the given address.
//
// The planning phase must keep the provider open at least long enough for
// all of the reported resource instances to be planned.
//
// Note that the planning engine will need to plan destruction of any resource
// instances that aren't in the desired state once
// [ConfigInstance.DrivePlanning] returns, and provider instances involved in
// those followup steps will need to remain open until that other work is
// done. This package is not concerned with those details; that's the planning
// engine's responsibility.
func (o *PlanningOracle) ProviderInstanceUsers(ctx context.Context, addr addrs.AbsProviderInstanceCorrect) ProviderInstanceUsers {
	ctx = grapheval.ContextWithNewWorker(ctx)
	_ = ctx // not using this right now, but keeping this to remind future maintainers that we'd need this

	return o.relationships.ProviderInstanceUsers.Get(addr)
}

// EphemeralResourceInstanceUsers returns an object describing which other
// resource instances and providers rely on the result value of the ephemeral
// resource with the given address.
//
// The planning phase must keep the ephemeral resource instance open at least
// long enough for all of the reported resource instances to be planned and
// for all of the reported provider instances to be closed.
//
// The given address must be for an ephemeral resource instance or this function
// will panic.
//
// Note that the planning engine will need to plan destruction of any resource
// instances that aren't in the desired state once
// [ConfigInstance.DrivePlanning] returns, and provider instances involved in
// those followup steps might be included in a result from this method, in
// which case the planning phase must hold the provider instance open long
// enough to complete those followup steps.
func (o *PlanningOracle) EphemeralResourceInstanceUsers(ctx context.Context, addr addrs.AbsResourceInstance) EphemeralResourceInstanceUsers {
	ctx = grapheval.ContextWithNewWorker(ctx)
	_ = ctx // not using this right now, but keeping this to remind future maintainers that we'd need this

	if addr.Resource.Resource.Mode != addrs.EphemeralResourceMode {
		panic(fmt.Sprintf("EphemeralResourceInstanceUsers with non-ephemeral %s", addr))
	}
	return o.relationships.EphemeralResourceUsers.Get(addr)
}

// AwaitResourceInstancesCompletion blocks until all of the resource instances
// identified in the given sequence have completed plan-time evaluation, whether
// successfully or with errors.
//
// This is intended for use with the results from
// [PlanningOracle.EphemeralResourceInstanceUsers] or
// [PlanningOracle.ProviderInstanceUsers] to provide a signal about the earliest
// time that it might be okay to close a previously-opened ephemeral resource
// instance or provider instance.
//
// Because the sets of resource instances returned by those functions are
// potentially imprecise -- they may contain placeholder resource instance
// addresses where there wasn't yet enough information to finalize expansion
// before the planning process began -- this function automatically handles
// a wildcard address by blocking on the completion of every instance that
// could potentially match it. This might mean reporting later than would
// strictly be necessary if the analysis functions had access to full planning
// detail, but this concession is necessary because those analysis functions
// essentially need to "predict the future" by making an approximate decision
// before any provider instances or ephemeral resource instances have been
// opened.
//
// Cancelling the context passed to this function is NOT guaranteed to cause
// it to return promptly. The contexts used by the concurrent planning work
// on all of the requested resource instances must be cancelled so that those
// planning operations themselves can fail promptly with a cancellation-related
// error, after which we will assume that the resource instance planning logic
// will make no further use of any associated provider instance or ephemeral
// resource instances.
//
// Note that awaiting the completion of a call to this function is necessary but
// not sufficient: the planning engine may need to keep these objects open
// beyond the end of the part of the planning process driven by this package in
// order to plan to destroy "orphaned" resource instances that are in the prior
// state but are not visible to this package.
func (o *PlanningOracle) AwaitResourceInstancesCompletion(ctx context.Context, resourceInstAddrs iter.Seq[addrs.AbsResourceInstance]) {
	ctx = grapheval.ContextWithNewWorker(ctx)

	// The contract for this function is to block until _all_ of the given
	// addresses have completed plan-time evaluation, so we can achieve this
	// by just waiting for each item in turn and assuming that we'll quickly
	// move past any that were already completed by the time we reach them.
	//
	// We currently consider "completed plan-time evaluation" to mean that
	// the resource instance's result value is available, because once that
	// value has been finalized there should be no further need for
	// interacting with the associated provider instance or any ephemeral
	// resource instances that the configuration referred to.
	for addr := range resourceInstAddrs {
		// We use plain []addrs.ModuleInstanceStep instead of
		// addrs.ModuleInstance here because the methods on the named type
		// don't make sense unless the slice of steps is relative to the
		// root module, whereas awaitResourceInstancesCompletion is going
		// to consume it step-by-step and so it won't be rooted after the
		// first call here.
		moduleSteps := []addrs.ModuleInstanceStep(addr.Module)
		resourceInstAddr := addr.Resource
		o.awaitResourceInstancesCompletion(ctx, o.rootModuleInstance, moduleSteps, resourceInstAddr)
	}
}

// awaitResourceInstancesCompletion is the main recursive body of
// [PlanningOracle.AwaitResourceInstancesCompletion], which keeps recursively
// consuming moduleSteps elements until none are left and then waits for
// the matching resource instances in each of the matching module instances.
func (o *PlanningOracle) awaitResourceInstancesCompletion(ctx context.Context, currentModuleInst evalglue.CompiledModuleInstance, moduleSteps []addrs.ModuleInstanceStep, resourceInstAddr addrs.ResourceInstance) {
	if len(moduleSteps) == 0 {
		// This is where we stop recursion and just wait for the resource
		// instances in the current module.
		o.awaitResourceInstanceCompletion(ctx, currentModuleInst, resourceInstAddr)
		return
	}
	// If we have at least one moduleStep left then we've got another level
	// of recursion to do. Whether we make one or many recursive calls
	// depends on whether this is an exact step or a placeholder for zero
	// or more steps whose instance keys are not decided yet.
	step, remainSteps := moduleSteps[0], moduleSteps[1:]
	if step.IsPlaceholder() {
		callAddr := addrs.ModuleCall{Name: step.Name}
		for _, childInst := range currentModuleInst.ChildModuleInstancesForCall(ctx, callAddr) {
			o.awaitResourceInstancesCompletion(ctx, childInst, remainSteps, resourceInstAddr)
		}
	} else {
		callInstAddr := addrs.ModuleCallInstance{
			Call: addrs.ModuleCall{Name: step.Name},
			Key:  step.InstanceKey,
		}
		childInst := currentModuleInst.ChildModuleInstance(ctx, callInstAddr)
		if childInst == nil {
			// This suggests that the requested object isn't declared in
			// the configuration at all, so there's nothing to wait for.
			// (This is effectively the same as finding no instances for
			// the call in the step.IsPlaceholder case above.)
			return
		}
		o.awaitResourceInstancesCompletion(ctx, childInst, remainSteps, resourceInstAddr)
	}
}

// awaitResourceInstanceCompletion handles the final leaf waiting step once
// [PlanningOracle.awaitResourceInstancesCompletion] has finished recursion
// through any intermediate module instance steps. It blocks until the
// resource instances matching the given address inside the given module
// instance have all completed their plan-time evaluation.
func (o *PlanningOracle) awaitResourceInstanceCompletion(ctx context.Context, currentModuleInst evalglue.CompiledModuleInstance, addr addrs.ResourceInstance) {
	// Regardless of whether we have an exact or placeholder resource instance
	// address we will need to block for the instances to be decided; the
	// exact case just means we only need to wait for the _completion_ of
	// one of those instances.
	exactMatch := !addr.IsPlaceholder()
	for inst := range currentModuleInst.ResourceInstancesForResource(ctx, addr.Resource) {
		if exactMatch && inst.Addr.Resource != addr {
			continue // this instance doesn't match the given address
		}
		// Now we just wait for the "Value" method to return, which is a good
		// enough signal that this resource instance's planning work should
		// be finished, whether successfully or not. The actual result is
		// unimportant; we care only that there's no more ongoing work to
		// produce it.
		_, _ = inst.Value(ctx)
	}
}
