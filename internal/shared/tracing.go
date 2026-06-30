// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package shared

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Tracer represents a set of tracing event handlers for events that are common
// across the planning and applying phases.
type Tracer struct {
	////////// Ephemeral Resource Events

	// StartEphemeralResourceInstanceOpen and EndEphemeralResourceInstanceOpen
	// mark the beginning and end of the work to open the identified ephemeral
	// resource instance.
	StartEphemeralResourceInstanceOpen func(ctx context.Context, addr addrs.AbsResourceInstance) context.Context
	EndEphemeralResourceInstanceOpen   func(ctx context.Context, addr addrs.AbsResourceInstance, diags tfdiags.Diagnostics)

	// StartEphemeralResourceInstanceRenew and EndEphemeralResourceInstanceRenew
	// mark the beginning and end of the work to renew the identified ephemeral
	// resource instance.
	StartEphemeralResourceInstanceRenew func(ctx context.Context, addr addrs.AbsResourceInstance) context.Context
	EndEphemeralResourceInstanceRenew   func(ctx context.Context, addr addrs.AbsResourceInstance, diags tfdiags.Diagnostics)

	// StartEphemeralResourceInstanceClose and EndEphemeralResourceInstanceClose
	// mark the beginning and end of the work to close the identified ephemeral
	// resource instance.
	StartEphemeralResourceInstanceClose func(ctx context.Context, addr addrs.AbsResourceInstance) context.Context
	EndEphemeralResourceInstanceClose   func(ctx context.Context, addr addrs.AbsResourceInstance, diags tfdiags.Diagnostics)
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
