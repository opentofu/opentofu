// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/refactoring"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// A PlanningOracle provides information from the configuration that is needed
// by the planning engine to help orchestrate the planning process.
type PlanningOracle struct {
	root           evalglue.CompiledModuleInstance
	providers      *managedProviders
	moveStatements []refactoring.MoveStatement
	moveResults    moveResults
}

// HasAddress queries the root module instance to determine if the address is
// present in the configuration. Note: if instances for the address's underlying
// resource cannot be resolved, this will return false.
func (o *PlanningOracle) HasAddress(ctx context.Context, addr addrs.AbsResourceInstance) bool {
	return evalglue.ResourceInstance(ctx, o.root, addr) != nil
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

func (o *PlanningOracle) Close(ctx context.Context) tfdiags.Diagnostics {
	return o.providers.Close(ctx)
}
