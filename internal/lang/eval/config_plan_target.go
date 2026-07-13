// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"
	"iter"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

type targetingGlue struct {
	excluded func(addrs.Targetable) bool
	parent   PlanGlue
}

func (g *targetingGlue) PlanDesiredResourceInstance(ctx context.Context, inst *DesiredResourceInstance) (cty.Value, tfdiags.Diagnostics) {
	if g.excluded(inst.Addr) {
		inst.Deferred = true
		inst.ConfigVal = cty.DynamicVal
	}
	return g.parent.PlanDesiredResourceInstance(ctx, inst)
}

func (g *targetingGlue) PlanResourceInstanceOrphans(ctx context.Context, resourceAddr addrs.AbsResource, desiredInstances iter.Seq[addrs.InstanceKey]) tfdiags.Diagnostics {
	return g.parent.PlanResourceInstanceOrphans(ctx, resourceAddr, desiredInstances)
	/*return g.parent.PlanResourceInstanceOrphans(ctx, resourceAddr, func(yield func(addrs.InstanceKey) bool) {
		for key := range desiredInstances {
			if !g.excluded(resourceAddr.Instance(key)) {
				println("Included: " + resourceAddr.Instance(key).String())
				if !yield(key) {
					return
				}
			} else {
				println("Excluded: " + resourceAddr.Instance(key).String())
			}
		}
	})*/
}

func (g *targetingGlue) PlanResourceOrphans(ctx context.Context, moduleInstAddr addrs.ModuleInstance, desiredResources iter.Seq[addrs.Resource]) tfdiags.Diagnostics {
	return g.parent.PlanResourceOrphans(ctx, moduleInstAddr, desiredResources)
	/*return g.parent.PlanResourceOrphans(ctx, moduleInstAddr, func(yield func(addrs.Resource) bool) {
		for resource := range desiredResources {
			if !g.excluded(resource.Absolute(moduleInstAddr)) {
				println("Included: " + resource.Absolute(moduleInstAddr).String())
				if !yield(resource) {
					return
				}
			} else {
				println("Excluded: " + resource.Absolute(moduleInstAddr).String())
			}
		}
	})*/
}

func (g *targetingGlue) PlanModuleCallInstanceOrphans(ctx context.Context, moduleCallAddr addrs.AbsModuleCall, desiredInstances iter.Seq[addrs.InstanceKey]) tfdiags.Diagnostics {
	return g.parent.PlanModuleCallInstanceOrphans(ctx, moduleCallAddr, desiredInstances)
	/*return g.parent.PlanModuleCallInstanceOrphans(ctx, moduleCallAddr, func(yield func(addrs.InstanceKey) bool) {
		for key := range desiredInstances {
			if !g.excluded(moduleCallAddr.Instance(key)) {
				println("Included: " + moduleCallAddr.Instance(key).String())
				if !yield(key) {
					return
				}
			} else {
				println("Excluded: " + moduleCallAddr.Instance(key).String())
			}
		}
	})*/
}

func (g *targetingGlue) PlanModuleCallOrphans(ctx context.Context, callerModuleInstAddr addrs.ModuleInstance, desiredCalls iter.Seq[addrs.ModuleCall]) tfdiags.Diagnostics {
	return g.parent.PlanModuleCallOrphans(ctx, callerModuleInstAddr, desiredCalls)
	/*return g.parent.PlanModuleCallOrphans(ctx, callerModuleInstAddr, func(yield func(addrs.ModuleCall) bool) {
		for call := range desiredCalls {
			// TODO if !g.excluded(call.Absolute(callerModuleInstAddr)) {
			if !g.excluded(callerModuleInstAddr) && !g.excluded(callerModuleInstAddr.Module().Child(call.Name)) {
				println("Included: " + call.Absolute(callerModuleInstAddr).String())
				if !yield(call) {
					return
				}
			} else {
				println("Excluded: " + call.Absolute(callerModuleInstAddr).String())
			}
		}
	})*/
}
