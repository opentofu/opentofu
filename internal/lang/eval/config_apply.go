// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"

	_ "github.com/apparentlymart/go-workgraph/workgraph" // for documentation links only
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ApplyGlue is used with [DriveApplying] to allow the evaluation system to
// communicate with the applying engine that called it.
//
// Methods of this type can be called concurrently with each other and with
// themselves, so implementations must use appropriate synchronization
// primitives to avoid race conditions.
type ApplyGlue interface {
	// ResourceInstanceFinalState blocks until the apply phase has completed
	// enough work to decide the final state value for the resource instance
	// with the given address and then returns that value.
	//
	// If operations that would contribute to that final value fail then this
	// function returns a suitable placeholder for the final state that can
	// would allow valid dependent expressions to evaluate successfully though
	// potentially to an unknown value. Returning the "planned state" that
	// was predicted during the planning phase is acceptable, and returning
	// [cty.DynamicVal] is also acceptable as a last resort when absolutely
	// no information is available.
	//
	// Diagnostics from apply-time actions must be reported through some other
	// channel controlled by the apply engine itself.
	ResourceInstanceFinalState(ctx context.Context, addr addrs.AbsResourceInstance) cty.Value
}

// DriveApplying uses this configuration instance to support an "apply"
// process being managed by some other part of the system.
//
// Applying is driven primarily by an execution graph that was built during
// the planning phase and so during apply the eval engine's only role is to
// provide information about the final configuration of different configuration
// objects and propagate the final state returned by the apply engine through
// dependent expressions. We achieve that by calling the given callback with
// an [ApplyOracle] object that the apply engine can use to pull the needed
// information at an appropriate time.
//
// The given callback is provided a [context.Context] that is associated
// with a [workgraph.Worker], and is required to use the facilities in
// [grapheval] and [workgraph] to track its work so that it can collaborate
// properly with the evaluation system's detection of self-references, to
// avoid deadlocks, but the apply phase is free to construct its own workers
// rather than using the one provided to the callback function.
func (c *ConfigInstance) DriveApplying(ctx context.Context, glue ApplyGlue, run func(ctx context.Context, oracle *ApplyOracle)) tfdiags.Diagnostics {
	// All of our work will be associated with a workgraph worker that serves
	// as the initial worker node in the work graph.
	ctx = grapheval.ContextWithNewWorker(ctx)

	// TODO: This should take an implementation of an interface that integrates
	// with the main applying engine.
	panic("unimplemented")
}

// An ApplyOracle is passed to the callback given to
// [ConfigInstance.DriveApplying] to give the main apply engine access to
// various information from the configuration that it will need during
// the apply process.
//
// The methods of an [ApplyOracle] must be called with a [context.Context]
// derived from one produced by [grapheval.ContextWithWorker].
//
// Whereas the planning process is driven primarily by the dependencies
// discovered dynamically during evaluation, the apply process is instead
// driven primarily by an execution graph that was built during the planning
// process. The apply-time execution steps therefore need to be able to
// pull the information they need from the evaluation engine on request
// instead of the evaluation engine pushing the information out, and an
// object of this type provides that information.
//
// It's the responsibilty of the planning engine to construct an execution
// graph that ensures that the apply phase will request information from
// the oracle only once it has already been made available by earlier work.
type ApplyOracle struct {
}
