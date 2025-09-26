// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"
	"fmt"

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

	evalContext *EvalContext
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

func (o *PlanningOracle) EvalContext(ctx context.Context) *EvalContext {
	return o.evalContext
}
