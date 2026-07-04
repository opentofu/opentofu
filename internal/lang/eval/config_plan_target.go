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
		// TODO this does not actually translate through to apply as the "deferral" logic is incomplete.
		return cty.DynamicVal, nil
	}
	return g.parent.PlanDesiredResourceInstance(ctx, inst)
}

func (g *targetingGlue) PlanResourceInstanceOrphans(ctx context.Context, resourceAddr addrs.AbsResource, desiredInstances iter.Seq[addrs.InstanceKey]) tfdiags.Diagnostics {
	/*if g.excluded(resourceAddr) {
		return nil
	}*/
	return g.parent.PlanResourceInstanceOrphans(ctx, resourceAddr, desiredInstances)
}

func (g *targetingGlue) PlanResourceOrphans(ctx context.Context, moduleInstAddr addrs.ModuleInstance, desiredResources iter.Seq[addrs.Resource]) tfdiags.Diagnostics {
	/*if g.excluded(moduleInstAddr) {
		return nil
	}*/
	return g.parent.PlanResourceOrphans(ctx, moduleInstAddr, desiredResources)
}

func (g *targetingGlue) PlanModuleCallInstanceOrphans(ctx context.Context, moduleCallAddr addrs.AbsModuleCall, desiredInstances iter.Seq[addrs.InstanceKey]) tfdiags.Diagnostics {
	/*if g.excluded(moduleCallAddr.Module) {
		return nil
	}*/
	// TODO better module exclude filtering
	return g.parent.PlanModuleCallInstanceOrphans(ctx, moduleCallAddr, desiredInstances)
}

func (g *targetingGlue) PlanModuleCallOrphans(ctx context.Context, callerModuleInstAddr addrs.ModuleInstance, desiredCalls iter.Seq[addrs.ModuleCall]) tfdiags.Diagnostics {
	/*if g.excluded(callerModuleInstAddr) {
		return nil
	}*/
	return g.parent.PlanModuleCallOrphans(ctx, callerModuleInstAddr, desiredCalls)
}
