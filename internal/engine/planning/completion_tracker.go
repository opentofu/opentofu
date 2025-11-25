// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/lifecycle"
	"github.com/opentofu/opentofu/internal/states"
)

// completionTracker is the concrete type of [lifecycle.CompletionTracker] we
// use to track completion during a planning operation.
//
// This uses implementations of [completionEvent] to represent the items that
// need to be completed.
type completionTracker = lifecycle.CompletionTracker[completionEvent]

func (p *planContext) reportResourceInstancePlanCompletion(addr addrs.AbsResourceInstance) {
	p.completion.ReportCompletion(resourceInstancePlanningComplete{addr.UniqueKey()})
}

func (p *planContext) reportResourceInstanceDeposedPlanCompletion(addr addrs.AbsResourceInstance, deposedKey states.DeposedKey) {
	p.completion.ReportCompletion(resourceInstanceDeposedPlanningComplete{addr.UniqueKey(), deposedKey})
}

func (p *planContext) reportProviderInstanceClosed(addr addrs.AbsProviderInstanceCorrect) {
	p.completion.ReportCompletion(providerInstanceClosed{addr.UniqueKey()})
}

// completionEvent is the type we use to represent events in
// a [completionTracker].
//
// All implementations of this interface must be comparable.
type completionEvent interface {
	completionEvent()
}

// resourceInstancePlanningComplete represents that we've completed planning
// of a specific resource instance.
//
// Provider instances remain open until all of the resource instances that
// belong to them have completed planning, and ephemeral resource instances
// remain open until all of the other resource instances that depend on them
// have completed planning.
type resourceInstancePlanningComplete struct {
	// key MUST be the unique key of an addrs.ResourceInstance.
	key addrs.UniqueKey
}

// completionEvent implements completionEvent.
func (r resourceInstancePlanningComplete) completionEvent() {}

// resourceInstanceDeposedPlanningComplete is like
// [resourceInstancePlanningComplete] but for "deposed" objects, rather than
// for "current" objects.
type resourceInstanceDeposedPlanningComplete struct {
	// key MUST be the unique key of an addrs.ResourceInstance.
	instKey addrs.UniqueKey

	// deposedKey is the DeposedKey of the deposed object whose planning
	// is being tracked.
	deposedKey states.DeposedKey
}

// completionEvent implements completionEvent.
func (r resourceInstanceDeposedPlanningComplete) completionEvent() {}

// providerInstanceClosed represents that a previously-opened provider
// instance has now been closed.
//
// Ephemeral resource instances remain open until all of the provider instances
// that depend on them have been closed.
type providerInstanceClosed struct {
	// key MUST be the unique key of an addrs.AbsProviderInstanceCorrect.
	key addrs.UniqueKey
}

// completionEvent implements completionEvent.
func (r providerInstanceClosed) completionEvent() {}
