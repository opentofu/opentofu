// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/plans"
)

// DataResourceInstanceSubgraph adds graph nodes needed to apply changes for a
// data resource instance, and returns various items needed to describe
// its relationships with other resource instance and provider instance
// subgraphs.
func (b *execGraphBuilder) DataResourceInstanceSubgraph(plannedChange *plans.ResourceInstanceChange) resourceInstanceObjectSubgraph {
	// This is fairly simple, as the data resource will either
	// already have been read, and so the plan is a NoOp, or
	// we execute a read action now.

	// The shape of execution subgraph we generate here varies depending on
	// which change action was planned.
	switch plannedChange.Action {
	case plans.Read:
		return b.dataResourceInstanceSubgraphRead(plannedChange)
	case plans.NoOp:
		return b.dataResourceInstanceSubgraphNoOp(plannedChange)
	default:
		// We should not get here: the cases above should cover every action
		// that [planGlue.planDesiredManagedResourceInstance] can possibly
		// produce.
		panic(fmt.Sprintf("unsupported change action %s for %s", plannedChange.Action, plannedChange.Addr))
	}
}
func (b *execGraphBuilder) dataResourceInstanceSubgraphRead(
	plannedChange *plans.ResourceInstanceChange,
) resourceInstanceObjectSubgraph {
	/*
		How this is happening:
		  - for each planned change for each resource instance, we build out the exec graph
		  - We're here! It's because a data resource needed to do a read.
		  - When we compile the exec graph, we run into opDataRead as one of the ops in the built exec graph => compileOpDataRead
		  - Finally, we execute the op: *execOperations.DataRead
		The upshot: the operands defined here will later be used in the compile and exec op.
	*/
	// TODO: Obtain the stuff to do a Data Read:
	// - desiredInst execgraph.ResultRef[*eval.DesiredResourceInstance]
	//   - This creates the DesiredResourceInstance, with the Addr, ProviderInstance, ConfigVal, etc. VERY IMPORTANT!
	// - waitFor (???????) execgraph.AnyResultRef

	waitFor, addReadDep := b.lower.MutableWaiter()

	// Most of these should come from "managedResourceInstanceChangeInputs"
	// except it's for data resources
	// hey, how does this function work, anyway?
	_, desiredRef, _, _ := b.managedResourceInstanceChangeInputs(plannedChange)

	return resourceInstanceObjectSubgraph{
		valueRef: b.lower.DataRead(
			// metadataRef,
			desiredRef,
			waitFor,
		),
		addDesiredDep: addReadDep,
	}
}

func (b *execGraphBuilder) dataResourceInstanceSubgraphNoOp(
	plannedChange *plans.ResourceInstanceChange,
) resourceInstanceObjectSubgraph {
	_, addCreateDep := b.lower.MutableWaiter()

	// TODO replace with "dataResourceInstanceChangeInputs"... perchance
	_, _, priorStateRef, _ := b.managedResourceInstanceChangeInputs(plannedChange)
	return resourceInstanceObjectSubgraph{
		valueRef:      priorStateRef,
		addDesiredDep: addCreateDep,
	}
}
