// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/shared"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Tracer is a container for various callbacks used to report various
// events that can occur during a call to [PlanChanges].
//
// Pass a pointer to an object of this type to [ContextWithTracer] to
// get an annotated [context.Context], and then pass it to [PlanChanges].
//
// Any fields left as nil will be ignored. Non-nil callbacks will be called
// whenever the associated event (mentioned in the field's documentation comment)
// occurs.
//
// Some callback functions come in "Start" and "End" pairs that share a common
// suffix. In those cases, the Start function is expected to return a context
// is a child of the one passed to the callback function, possibly annotated
// with additional information such as OpenTelemetry trace metadata. The
// returned context is then used for all of the requests that occur between
// the Start and End calls, and finally the same context is passed to the
// corresponding End function so that e.g. an OpenTelemetry trace can be
// closed. Implementations that don't need to preserve additional context can
// just directly return the provided context without modification.
type Tracer struct {

	////////// Managed Resource Planning Events

	// StartManagedResourceInstanceObjectPlanning and
	// EndManagedResourceInstanceObjectPlanning mark the beginning and end of
	// the overall planning work for the identified managed resource instance
	// object.
	//
	// These events cover the whole planning process for the given resource
	// instance object, including the state upgrade and refresh steps alongside
	// the actual call to ask the provider to determine if any changes are
	// needed. There are other more specific events below, which nest inside
	// this pair of events.
	StartManagedResourceInstanceObjectPlanning func(ctx context.Context, addr addrs.AbsResourceInstanceObject) context.Context
	EndManagedResourceInstanceObjectPlanning   func(ctx context.Context, addr addrs.AbsResourceInstanceObject, diags tfdiags.Diagnostics)

	// StartManagedResourceInstanceObjectUpgrade and
	// EndManagedResourceInstanceObjectUpgrade mark the beginning and end
	// of the "state upgrade" step for the identified managed resource instance
	// object.
	//
	// These events always occur between calls to
	// StartManagedResourceInstanceObjectPlanning and
	// EndManagedResourceInstanceObjectPlanning for the same object address.
	StartManagedResourceInstanceObjectUpgrade func(ctx context.Context, addr addrs.AbsResourceInstanceObject) context.Context
	EndManagedResourceInstanceObjectUpgrade   func(ctx context.Context, addr addrs.AbsResourceInstanceObject, upgradedVal cty.Value, diags tfdiags.Diagnostics)

	// StartManagedResourceInstanceObjectRefresh and
	// EndManagedResourceInstanceObjectRefresh mark the beginning and end
	// of the "refresh" step for the identified managed resource instance
	// object.
	//
	// These events always occur between calls to
	// StartManagedResourceInstanceObjectPlanning and
	// EndManagedResourceInstanceObjectPlanning for the same object address.
	StartManagedResourceInstanceObjectRefresh func(ctx context.Context, addr addrs.AbsResourceInstanceObject, prevRoundVal cty.Value) context.Context
	EndManagedResourceInstanceObjectRefresh   func(ctx context.Context, addr addrs.AbsResourceInstanceObject, prevRoundVal, refreshedVal cty.Value, diags tfdiags.Diagnostics)

	// StartManagedResourceInstanceObjectPlanChanges and
	// EndManagedResourceInstanceObjectPlanChanges mark the beginning and end
	// of the step where we actually ask the provider to plan changes for
	// the identified resource instance object.
	//
	// These events always occur between calls to
	// StartManagedResourceInstanceObjectPlanning and
	// EndManagedResourceInstanceObjectPlanning for the same object address,
	// which act as a container for the upgrade, refresh, and change-planning
	// sequence.
	StartManagedResourceInstanceObjectPlanChanges func(ctx context.Context, addr addrs.AbsResourceInstanceObject, priorVal, configVal cty.Value) context.Context
	EndManagedResourceInstanceObjectPlanChanges   func(ctx context.Context, addr addrs.AbsResourceInstanceObject, action plans.Action, priorVal, plannedVal cty.Value, diags tfdiags.Diagnostics)

	////////// Data Resource Planning Events

	// StartDataResourceInstancePlanning and EndDataResourceInstancePlanning
	// mark the beginning and end of the overall planning work for the
	// identified data resource instance.
	//
	// These events cover the whole planning process for the given resource
	// instance. There are other more specific events below, which nest inside
	// this pair of events.
	StartDataResourceInstancePlanning func(ctx context.Context, addr addrs.AbsResourceInstance) context.Context
	EndDataResourceInstancePlanning   func(ctx context.Context, addr addrs.AbsResourceInstance, diags tfdiags.Diagnostics)

	// StartDataResourceInstanceRead and EndDataResourceInstanceRead mark the
	// beginning and end of the work to read data for the identified data
	// resource instance.
	//
	// These events occur only when the data resource instance is readable
	// during the planning phase. Some data resource instances get delayed until
	// the apply phase because there isn't enough information to read them
	// during the planning phase.
	//
	// These events always occur between calls to
	// StartDataResourceInstancePlanning and EndDataResourceInstancePlanning for
	// the same instance address.
	StartDataResourceInstanceRead func(ctx context.Context, addr addrs.AbsResourceInstance) context.Context
	EndDataResourceInstanceRead   func(ctx context.Context, addr addrs.AbsResourceInstance, resultVal cty.Value, diags tfdiags.Diagnostics)

	// We also embed [shared.Tracer] for some events that are common across
	// plan and apply. [PlanChanges] automatically ensures that this nested
	// tracer reaches the shared codepaths that rely on it.
	shared.Tracer
}

// ContextWithTracer returns a new context derived from parent that carries
// a [Tracer].
//
// Use the resulting context when calling [PlanChanges], to get event
// notification callbacks throughout the planning process.
func ContextWithTracer(parent context.Context, tracer *Tracer) context.Context {
	if tracer == nil {
		return parent
	}
	return context.WithValue(parent, ctxTracerKey{}, tracer)
}

// contextTracer returns the [Tracer] associated with the given context,
// or a pointer to [noopPlanTracer] if there is no associated tracer.
//
// The result is therefore guaranteed to never be nil, although the caller
// must still check to see if each specific field it intends to use is nil or
// not.
func contextTracer(ctx context.Context) *Tracer {
	ret, _ := ctx.Value(ctxTracerKey{}).(*Tracer)
	if ret == nil {
		return &noopTracer
	}
	return ret
}

type ctxTracerKey struct{}

// noopTracer is an all-nil [Tracer] we return from [contextTracer]
// when the given context has no tracer, so that calling code only needs to
// worry about the individual fields being nil and not the overall object
// being nil.
//
// A pointer to this object must never be exposed outside of this package
// because that would risk such a caller acceidentally modifying the shared
// noop tracer.
var noopTracer Tracer
