// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"
	"log"

	"github.com/apparentlymart/go-workgraph/workgraph"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/refactoring"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// A PlanningOracle provides information from the configuration that is needed
// by the planning engine to help orchestrate the planning process.
type PlanningOracle struct {
	root      evalglue.CompiledModuleInstance
	providers *managedProviders
}

func (o *PlanningOracle) CheckTarget(ctx context.Context, target addrs.Targetable) {
	ctx = grapheval.ContextWithNewWorker(ctx)

	addTarget := func(ri *configgraph.ResourceInstance) {
		// Populate the value before the glue disables itself for the rest of processing
		log.Printf("[TRACE] %s targeting %s", target, ri.Addr)
		ri.Value(ctx)
	}

	switch target.AddrType() {
	case addrs.ConfigResourceAddrType:
		configResource := target.(addrs.ConfigResource)
		for _, modInst := range evalglue.ConfigModuleInstances(ctx, o.root, configResource.Module) {
			for resInst := range modInst.ResourceInstancesForResource(ctx, configResource.Resource) {
				addTarget(resInst)
			}
		}
	case addrs.AbsResourceAddrType:
		absResource := target.(addrs.AbsResource)
		modInst := evalglue.ModuleInstance(ctx, o.root, absResource.Module)
		if modInst != nil {
			for resInst := range modInst.ResourceInstancesForResource(ctx, absResource.Resource) {
				addTarget(resInst)
			}
		}
	case addrs.AbsResourceInstanceAddrType:
		absResourceInstance := target.(addrs.AbsResourceInstance)
		resInst := evalglue.ResourceInstance(ctx, o.root, absResourceInstance)
		if resInst != nil {
			addTarget(resInst)
		}
	case addrs.ModuleAddrType:
		module := target.(addrs.Module)
		for _, modInst := range evalglue.ConfigModuleInstances(ctx, o.root, module) {
			for resInst := range evalglue.ResourceInstancesDeep(ctx, modInst) {
				addTarget(resInst)
			}
		}
	case addrs.ModuleInstanceAddrType:
		moduleInstance := target.(addrs.ModuleInstance)
		modInst := evalglue.ModuleInstance(ctx, o.root, moduleInstance)
		if modInst != nil {
			for resInst := range evalglue.ResourceInstancesDeep(ctx, modInst) {
				addTarget(resInst)
			}
		}
	}
}

// CheckAll visits everything in the configuration and gathers up any
// diagnostics that are reported. This has the important side-effect of forcing
// evaluation of everything in the configuration and therefore should cause
// the associated [PlanGlue] implementation to get called for each desired
// resource instance found in the configuration.
//
// The caller is expected to provide a context which has a
// [grapheval.RequestTracker] that at least calls
// [PlanningOracle.AnnounceAllGraphevalRequests] when asked for requests,
// along with announcing any other grapheval requests the caller is managing
// directly itself.
func (o *PlanningOracle) CheckAll(ctx context.Context) tfdiags.Diagnostics {
	// The plan phase is driven forward by us evaluating expressions during
	// the "CheckAll" process, and so we can just run that here and then
	// it'll cause various calls out to the "glue" object whenever we're
	// ready to provide configuration for a resource instance and need to
	// obtain its result for downstream use.
	return o.root.CheckAll(ctx)
}

func (o *PlanningOracle) PlanningResult(ctx context.Context) *PlanningResult {
	return &PlanningResult{
		RootModuleOutputs: CollectRootModuleOutputs(ctx, o.root),
	}
}

// MoveStatementsFor queries the root module for any move statements along the given path.
// This should eventually be replaced with ResourceInstanceObjectMeta.
func (o *PlanningOracle) MoveStatementsFor(ctx context.Context, addr addrs.Module) []refactoring.MoveStatement {
	return o.root.GetMoveStatementsFor(ctx, addr)
}

// DetectImplicitMoveForAddress spiders the configuration, attempting to determine if there is an implicit
// move that could be found given the supplied state address.
func (o *PlanningOracle) DetectImplicitMoveForAddress(ctx context.Context, addr addrs.AbsResourceInstance) *addrs.AbsResourceInstance {
	return o.root.DetectImplicitMoveForAddress(ctx, addr)
}

// ResourceInstanceObjectMeta returns whatever metadata applies to the
// given resource instance object based only on information available in
// the configuration.
//
// This is intended to be called by methods of the [PlanGlue] implementation
// provided by the planning engine during the planning process, when handling
// both desired and non-desired objects, whereas only desired objects have
// a full [DesiredResourceInstance]. Callers should typically combine the
// result of this method with information from the prior state to produce the
// full metadata for the requested object.
//
// Callers must be careful about how they ask this question if a particular
// resource instance is changing its address as part of the current plan, such
// as with "moved" blocks. The config-based metadata is always associated with
// the new address that the object would be bound to after the apply phase
// completes, whereas the associated state-based metadata would belong instead
// to the old address.
//
// This method returns nil if there is absolutely no configuration-based
// metadata for the given object, in which case the caller will need to rely
// on the state exclusively for deciding the metadata. Callers can assume that
// a "desired" resource instance object will always have non-nil metadata.
//
// If errors in the configuration prevent producing the full metadata for the
// resource instance then the result may include unknown values as placeholders
// for the erroneous configuration values, marked with [exprs.EvalError] to
// allow callers to distinguish them from truly-unknown values whenever that's
// needed. In particular, callers should avoid reporting any new errors based
// on unknown values that are marked in that way, because that'll tend to cause
// the same problem to be reported more than once in different ways and that's
// confusing.
func (o *PlanningOracle) ResourceInstanceObjectMeta(ctx context.Context, addr addrs.AbsResourceInstanceObject) *ConfiguredResourceInstanceObjectMeta {
	moduleInst := evalglue.ModuleInstance(ctx, o.root, addr.InstanceAddr.Module)
	if moduleInst == nil {
		// The relevant module instance is not currently configured at all,
		// so the caller will need to rely on the state exclusively for this one.
		return nil
	}
	return moduleInst.ResourceInstanceObjectMeta(ctx, addr.ModuleRelative())
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
// all then this will return nil. That should not occur for any
// provider instance address reported by this package as part of the same
// planning phase, but could occur in subsequent work done by the planning
// phase to deal with resource instances that are in prior state but no longer
// in desired state, if their provider instances have also been removed from
// the desired state at the same time. In that case the planning phase must
// report that the "orphaned" resource instance cannot be planned for deletion
// unless its provider instance is re-added to the configuration.
func (o *PlanningOracle) ProviderInstance(ctx context.Context, addr addrs.AbsProviderInstanceCorrect) (providers.Interface, tfdiags.Diagnostics) {
	return o.providers.ProviderInstance(ctx, addr)
}

func (o *PlanningOracle) PreventDestroy(ctx context.Context, addr addrs.AbsResourceInstance) (exprs.FromValue[bool], *tfdiags.SourceRange, tfdiags.Diagnostics) {
	mod := evalglue.ModuleInstance(ctx, o.root, addr.Module)
	if mod == nil {
		return exprs.Known(false), nil, nil
	}
	resource := mod.Resource(ctx, addr.Resource.Resource)
	if resource == nil {
		return exprs.Known(false), nil, nil
	}
	return resource.PreventDestroy(ctx)
}

// AnnounceAllGraphevalRequests calls the given function once for each internal
// workgraph request that has previously been started by requests to this
// oracle.
//
// This is used by the planning engine as part of its implementation of
// [grapheval.RequestTracker], so that promise-resolution-related diagnostics
// can include information about which requests were involved in the problem.
//
// This information is collected as a separate step only when needed because
// that avoids us needing to keep track of this metadata on the happy path,
// so that we only pay the cost of gathering this data when we're actually
// going to use it for something.
func (o *PlanningOracle) AnnounceAllGraphevalRequests(announce func(workgraph.RequestID, grapheval.RequestInfo)) {
	o.root.AnnounceAllGraphevalRequests(announce)
}

func (o *PlanningOracle) Close(ctx context.Context) tfdiags.Diagnostics {
	return o.providers.Close(ctx)
}
