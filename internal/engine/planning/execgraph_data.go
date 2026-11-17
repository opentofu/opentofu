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
	// already have been read, and so there is no plan, or
	// we execute a read action now.
	switch plannedChange.Action {
	case plans.Read:
		return b.dataResourceInstanceSubgraphRead(plannedChange)
	case plans.Delete:
		return b.dataResourceInstanceSubgraphDelete(plannedChange)
	default:
		// We should not get here: the cases above should cover every action
		// that [planGlue.planDesiredDataResourceInstance] can possibly
		// produce.
		panic(fmt.Sprintf("unsupported change action %s for %s", plannedChange.Action, plannedChange.Addr))
	}
}

func (b *execGraphBuilder) dataResourceInstanceSubgraphRead(
	plannedChange *plans.ResourceInstanceChange,
) resourceInstanceObjectSubgraph {
	waitFor, addReadDep := b.lower.MutableWaiter()

	metadataRef, desiredRef, _, _ := b.managedResourceInstanceChangeInputs(plannedChange)

	return resourceInstanceObjectSubgraph{
		valueRef: b.lower.DataRead(
			metadataRef,
			desiredRef,
			waitFor,
		),
		addDesiredDep: addReadDep,
	}
}

func (b *execGraphBuilder) dataResourceInstanceSubgraphDelete(
	plannedChange *plans.ResourceInstanceChange,
) resourceInstanceObjectSubgraph {
	metadataRef, _, priorRef, _ := b.managedResourceInstanceChangeInputs(plannedChange)
	waitFor, addDeleteDep := b.lower.MutableWaiter()
	deletionRef := b.lower.DataDestroy(
		metadataRef,
		waitFor,
	)
	return resourceInstanceObjectSubgraph{
		// We report the prior state reference as the "valueRef" for a delete.
		// In a normal plan that doesn't do anything because nothing is allowed
		// to refer to a resource instance that's being deleted anyway, but
		// it's important for destroy-mode plans because annoyingly it _is_ valid
		// to refer to an object being deleted in that case, so that ephemeral
		// objects like provider instances can configure themselves based on
		// the prior state before the object is deleted.
		valueRef:     priorRef,
		deletionRef:  deletionRef,
		addOrphanDep: addDeleteDep,
	}
}
