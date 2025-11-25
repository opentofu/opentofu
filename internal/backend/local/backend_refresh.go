// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package local

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func (b *Local) opRefresh(
	stopCtx context.Context,
	cancelCtx context.Context,
	op *backend.Operation,
	runningOp *backend.RunningOperation) {

	// TEMP: Opt-in support for testing with the new experimental language
	// runtime. Refer to backend_temp_new_runtime.go for more information.
	if experimentalRuntimeEnabled() {
		b.opRefreshWithExperimentalRuntime(stopCtx, cancelCtx, op, runningOp)
		return
	}

	var diags tfdiags.Diagnostics

	// For the moment we have a bit of a tangled mess of context.Context here, for
	// historical reasons. Hopefully we'll clean this up one day, but here's the
	// guide for now:
	// - ctx is used only for its values, and should be connected to the top-level ctx
	//   from "package main" so that we can obtain telemetry objects, etc from it.
	// - stopCtx is cancelled to trigger a graceful shutdown.
	// - cancelCtx is cancelled for a graceless shutdown.
	ctx := context.WithoutCancel(stopCtx)

	// Check if our state exists if we're performing a refresh operation. We
	// only do this if we're managing state with this backend.
	if b.Backend == nil {
		if _, err := os.Stat(b.StatePath); err != nil {
			if os.IsNotExist(err) {
				err = nil
			}

			if err != nil {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Cannot read state file",
					fmt.Sprintf("Failed to read %s: %s", b.StatePath, err),
				))
				op.ReportResult(runningOp, diags)
				return
			}
		}
	}

	// Refresh now happens via a plan, so we need to ensure this is enabled
	op.PlanRefresh = true

	// Get our context
	lr, _, opState, contextDiags := b.localRun(ctx, op)
	diags = diags.Append(contextDiags)
	if contextDiags.HasErrors() {
		op.ReportResult(runningOp, diags)
		return
	}

	// the state was locked during successful context creation; unlock the state
	// when the operation completes
	defer func() {
		diags := op.StateLocker.Unlock()
		if diags.HasErrors() {
			op.View.Diagnostics(diags)
			runningOp.Result = backend.OperationFailure
		}
	}()

	// If we succeed then we'll overwrite this with the resulting state below,
	// but otherwise the resulting state is just the input state.
	runningOp.State = lr.InputState
	if !runningOp.State.HasManagedResourceInstanceObjects() {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Warning,
			"Empty or non-existent state",
			"There are currently no remote objects tracked in the state, so there is nothing to refresh.",
		))
	}

	// get schemas before writing state
	schemas, moreDiags := lr.Core.Schemas(ctx, lr.Config, lr.InputState)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		op.ReportResult(runningOp, diags)
		return
	}

	// Perform the refresh in a goroutine so we can be interrupted
	var newState *states.State
	var refreshDiags tfdiags.Diagnostics
	doneCh := make(chan struct{})
	panicHandler := logging.PanicHandlerWithTraceFn()
	go func() {
		defer panicHandler()
		defer close(doneCh)
		newState, refreshDiags = lr.Core.Refresh(ctx, lr.Config, lr.InputState, lr.PlanOpts)
		log.Printf("[INFO] backend/local: refresh calling Refresh")
	}()

	if b.opWait(doneCh, stopCtx, cancelCtx, lr.Core, opState, op.View) {
		return
	}

	// Write the resulting state to the running op
	runningOp.State = newState
	diags = diags.Append(refreshDiags)
	if refreshDiags.HasErrors() {
		op.ReportResult(runningOp, diags)
		return
	}

	err := statemgr.WriteAndPersist(context.TODO(), opState, newState, schemas)
	if err != nil {
		diags = diags.Append(fmt.Errorf("failed to write state: %w", err))
		op.ReportResult(runningOp, diags)
		return
	}

	// Show any remaining warnings before exiting
	op.ReportResult(runningOp, diags)
}
