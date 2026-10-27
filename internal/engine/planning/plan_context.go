// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"
	"fmt"
	"sync"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/plugins"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// planContext is our shared state for the various parts of a single call
// to [PlanChanges], and is mainly used as part of our [eval.PlanGlue]
// implementation [planGlue], through which the evaluator calls us to ask for
// planning results.
type planContext struct {
	evalCtx *eval.EvalContext

	// resourceInstObjs is where we gradually construct our intermediate
	// representation of the graph of resource instance objects.
	//
	// This gets modified by methods of [planGlue] gradually as we learn of
	// new resource instance objects. Use [planContext.Close] after the
	// work is complete to obtain the finalized object.
	resourceInstObjs *resourceInstanceObjectsBuilder

	// TODO: The following should probably track a reason why each resource
	// instance was deferred, but since deferral is not the focus of this
	// current experiment we'll just keep this boolean for now.
	deferred addrs.Map[addrs.AbsResourceInstance, struct{}]

	// TODO once deferred is fully functional, we might be able to use that
	// combined with resourceInstanceObjs to replace these fields
	desiredMu sync.Mutex
	desired   addrs.Set[addrs.AbsResourceInstance]

	forceReplace []addrs.AbsResourceInstance

	moveMu          sync.Mutex
	recordedMoves   addrs.Map[addrs.AbsResourceInstance, addrs.AbsResourceInstance]
	configuredMoves addrs.Map[addrs.AbsResourceInstance, []*moveStep]

	// prevRoundState MUST be treated as immutable
	prevRoundState *states.State

	// rootOutput is the values and dependencies of the root module outputs
	rootOutput rootOutput

	providers plugins.Providers

	skipRefresh bool
	refreshOnly bool

	skipImport           bool
	importCommandTargets []*eval.ImportStatement
	generateConfigPath   string
}

func newPlanContext(evalCtx *eval.EvalContext, prevRoundState *states.State, providers plugins.Providers, opts *PlanOpts) *planContext {
	if prevRoundState == nil {
		prevRoundState = states.NewState()
	}
	return &planContext{
		evalCtx:          evalCtx,
		resourceInstObjs: newResourceInstanceObjectsBuilder(),
		deferred:         addrs.MakeMap[addrs.AbsResourceInstance, struct{}](),
		desired:          addrs.MakeSet[addrs.AbsResourceInstance](),
		forceReplace:     opts.ForceReplace,
		recordedMoves:    addrs.MakeMap[addrs.AbsResourceInstance, addrs.AbsResourceInstance](),
		configuredMoves:  addrs.MakeMap[addrs.AbsResourceInstance, []*moveStep](),
		prevRoundState:   prevRoundState,
		providers:        providers,
		skipRefresh:      opts.SkipRefresh,
		refreshOnly:      opts.Mode == plans.RefreshOnlyMode,

		importCommandTargets: opts.ImportCommandTargets,
		generateConfigPath:   opts.GenerateConfigPath,
	}
}

// Close marks the end of the use of the [planContext] object, returning a
// [plans.Plan] representation of the plan that was created.
//
// After calling this function the [planContext] object is invalid and must
// not be used anymore.
func (p *planContext) Close(ctx context.Context) (*planContextResult, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	objs := p.resourceInstObjs.Close()

	drift, moreDiags := p.driftedResources(ctx, objs)
	diags = diags.Append(moreDiags)

	result := &planContextResult{
		ResourceInstanceObjects: objs,
		PrevRoundState:          p.prevRoundState,
		RootOutput:              p.rootOutput,
		DriftedResources:        drift,
	}

	return result, diags
}

type rootOutput struct {
	Previous map[string]*states.OutputValue
	Current  eval.RootModuleOutputs
}

// planContextResult collects together the intermediate results produced by
// [planContext], ready to be used by the next pass of the planning engine
// to produce the finalized changes and execution graph.
type planContextResult struct {
	ResourceInstanceObjects *resourceInstanceObjects
	PrevRoundState          *states.State
	RootOutput              rootOutput

	// Unfortunately, we need to signal to the apply engine that some things
	// like output values need to be handled a bit differently.
	Destroying bool

	DriftedResources []*plans.ResourceInstanceChangeSrc
}

// driftedResources is a best-effort attempt to compare the current and prior
// state. If we cannot decode the prior state for some reason, this should only
// return warnings to help the user correlate any missing resources in the
// report. This is known to happen when targeting a subset of resources,
// because the excluded instances will have been removed from the plan and
// not upgraded.
//
// This is a direct port from tofu/context_plan.go
func (p *planContext) driftedResources(ctx context.Context, objects *resourceInstanceObjects) ([]*plans.ResourceInstanceChangeSrc, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	/*TODO potential optimization, compare with skipping the decodes below
	if newState.ManagedResourcesEqual(oldState) && moves.Changes.Len() == 0 {
		// Nothing to do, because we only detect and report drift for managed
		// resource instances.
		return nil, diags
	}*/

	p.moveMu.Lock()
	// Inverted map lookup
	movesTo := addrs.MakeMap[addrs.AbsResourceInstance, addrs.AbsResourceInstance]()
	for _, entry := range p.recordedMoves.Elements() {
		movesTo.Put(entry.Value, entry.Key)
	}
	p.moveMu.Unlock()

	var drs []*plans.ResourceInstanceChangeSrc

	for _, obj := range objects.All() {
		if obj.Addr.InstanceAddr.Resource.Resource.Mode != addrs.ManagedResourceMode {
			// Drift reporting is only for managed resources
			continue
		}

		if !obj.Addr.IsCurrent() {
			// Not interested in instances that only have deposed objects
			continue
		}

		oldIS := obj.UpgradedState
		if oldIS == nil {
			// Did not exist before refresh
			continue
		}

		provider := obj.Provider
		addr := obj.Addr.InstanceAddr

		// Previous run address defaults to the current address, but
		// can differ if the resource moved before refreshing
		prevRunAddr, ok := movesTo.GetOk(addr)
		if !ok {
			prevRunAddr = obj.PrevRoundAddr.InstanceAddr
		}

		if obj.PlannedChange != nil {
			prevRunAddr = obj.PlannedChange.PrevRunAddr
		} else {
			for _, from := range p.recordedMoves.Keys() {
				to := p.recordedMoves.Get(from)
				if to.Equal(addr) {
					prevRunAddr = from
					break
				}
			}
		}

		// Note: provider addr is provided twice;
		// we cannot compare the currently configured provider
		// with the resource's provider from the previous state, so
		// we'll skip the provider check. TODO
		if addr.Resource.Resource.Type != prevRunAddr.Resource.Resource.Type {
			// We don't report drift in case of resource type change
			continue
		}
		newIS := obj.RefreshedState

		schema, _ := p.providers.ResourceTypeSchema(
			ctx,
			provider,
			addr.Resource.Resource.Mode,
			addr.Resource.Resource.Type,
		)
		if schema == nil {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Warning,
				"Missing resource schema from provider",
				fmt.Sprintf("No resource schema found for %s when decoding prior state", addr.Resource.Resource.Type),
			))
			continue
		}
		ty := schema.Block.ImpliedType()

		// TODO revisit this decode as it's extra work that's not nessesary
		oldVal, err := oldIS.Value.Decode(ty)
		if err != nil {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Warning,
				"Failed to decode resource from state",
				fmt.Sprintf("Error decoding %q from prior state: %s", addr.String(), err),
			))
			continue
		}

		newVal := cty.NullVal(ty)
		if newIS != nil {
			newVal, err = newIS.Value.Decode(ty)
			if err != nil {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Warning,
					"Failed to decode resource from state",
					fmt.Sprintf("Error decoding %q from prior state: %s", addr.String(), err),
				))
				continue
			}
		}

		if oldVal.RawEquals(newVal) && addr.Equal(prevRunAddr) {
			// No drift if the two values are semantically equivalent
			// and no move has happened
			continue
		}

		// We can detect three types of changes after refreshing state,
		// only two of which are easily understood as "drift":
		//
		// - Resources which were deleted outside OpenTofu;
		// - Resources where the object value has changed outside OpenTofu;
		// - Resources which have been moved without other changes.
		//
		// All of these are returned as drift, to allow refresh-only plans
		// to present a full set of changes which will be applied.
		var action plans.Action
		switch {
		case newVal.IsNull():
			action = plans.Delete
		case !oldVal.RawEquals(newVal):
			action = plans.Update
		default:
			action = plans.NoOp
		}

		/* TODO identity
		oldObjIdentity := oldIS.Identity
		var newObjIdentity cty.Value
		if newObj != nil {
			newObjIdentity = newIS.Identity
		}*/

		providerInst := obj.ProviderInst
		change := &plans.ResourceInstanceChange{
			Addr:        addr,
			PrevRunAddr: prevRunAddr,
			ProviderAddr: addrs.AbsProviderConfig{
				// FIXME: This is a lossy shim to the old-style provider instance
				// address representation, since our old models aren't yet updated
				// to support the modern one. It cannot handle a provider config
				// inside a module call that uses count or for_each.
				Module:   providerInst.Config.Module.Module(),
				Provider: providerInst.Config.Config.Provider,
				Alias:    providerInst.Config.Config.Alias,
			},
			Change: plans.Change{
				Action: action,
				Before: oldVal,
				After:  newVal,
				//BeforeIdentity: oldObjIdentity,
				//AfterIdentity:  newObjIdentity,
			},
		}

		changeSrc, err := change.Encode(schema)
		if err != nil {
			diags = diags.Append(err)
			return nil, diags
		}

		drs = append(drs, changeSrc)
	}

	return drs, diags
}
