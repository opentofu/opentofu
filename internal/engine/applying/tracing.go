// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package applying

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// Tracer is a container for various callbacks used to report various
// events that can occur during a call to [ApplyPlannedChanges].
//
// Pass a pointer to an object of this type to [ContextWithTracer] to
// get an annotated [context.Context], and then pass it to [ApplyPlannedChanges].
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

	////////// Managed Resource Applying Events

	// StartManagedResourceInstanceObjectFinalPlan and
	// EndManagedResourceInstanceObjectFinalPlan mark the beginning and end of
	// the work to produce the final plan for the identified managed resource
	// instance object.
	StartManagedResourceInstanceObjectFinalPlan func(ctx context.Context, addr addrs.AbsResourceInstanceObject) context.Context
	EndManagedResourceInstanceObjectFinalPlan   func(ctx context.Context, addr addrs.AbsResourceInstanceObject, plannedVal cty.Value, diags tfdiags.Diagnostics)

	// StartManagedResourceInstanceObjectApply and
	// EndManagedResourceInstanceObjectApply mark the beginning and end of
	// the work to apply the final plan for the identified managed resource
	// instance object.
	StartManagedResourceInstanceObjectApply func(ctx context.Context, addr addrs.AbsResourceInstanceObject) context.Context
	EndManagedResourceInstanceObjectApply   func(ctx context.Context, addr addrs.AbsResourceInstanceObject, resultVal cty.Value, diags tfdiags.Diagnostics)

	////////// Data Resource Applying Events

	// StartDataResourceInstanceRead and EndDataResourceInstanceRead mark the
	// beginning and end of the work to read data for the identified data
	// resource instance.
	//
	// These events occur only when the data resource instance read was delayed
	// due to there not being enough information to read it during the planning
	// phase.
	StartDataResourceInstanceRead func(ctx context.Context, addr addrs.AbsResourceInstance) context.Context
	EndDataResourceInstanceRead   func(ctx context.Context, addr addrs.AbsResourceInstance, resultVal cty.Value, diags tfdiags.Diagnostics)

	////////// Ephemeral Resource Lifecycle Events
	// TODO: StartEphemeralResourceInstanceOpen, EndEphemeralResourceInstanceOpen,
	// StartEphemeralResourceInstanceRenew, EndEphemeralResourceInstanceRenew,
	// StartEphemeralResourceInstanceClose, and EndEphemeralResourceInstanceClose,
	// but these are trickier because the evaluator manages them so we will
	// probably need some additional indirection in this case. Maybe we'll
	// define a shared "Tracer" object in package eval and just embed that
	// here so we can reuse it during both plan and apply in a way that allows
	// our caller to share the same set of tracing hooks between the plan and
	// apply phases.

	////////// Provider Instance Lifecycle Events
	// TODO: Similar to ephemeral resource lifecycle events above, probably
	// want to embed something we can share between the plan and apply tracer
	// types.
}

// ContextWithTracer returns a new context derived from parent that carries
// a [Tracer].
//
// Use the resulting context when calling [ApplyPlannedChanges], to get event
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
