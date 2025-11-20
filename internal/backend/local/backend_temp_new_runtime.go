// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package local

import (
	"context"
	"log"
	"os"
	"sync/atomic"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

/////////////////////////
// The definitions in this file are intended as temporary shims to help support
// the development of the new runtime engine, by allowing experiments-enabled
// builds to be opted in to the new implementation by setting the environment
// variable TOFU_X_EXPERIMENTAL_RUNTIME to any non-empty value.
//
// These shims should remain here only as long as the new runtime engine is
// under active development and is not yet adopted as the primary engine. It's
// also acceptable for work being done for other separate projects to ignore
// these shims and let this code become broken, as long as the code continues
// to compile: only those working on the implementation of the new engine are
// responsible for updating this if the rest of the system evolves to the point
// of that being necessary.
//
// Note that "tofu validate" is implemented outside of the backend abstraction
// and so does not respond to the experiment opt-in environment variable. For
// now, try out validation-related behaviors of the new runtime through
// "tofu plan" instead, which should implement a superset of the validation
// behavior.
/////////////////////////

// SetExperimentalRuntimeAllowed must be called with the argument set to true
// at some point before calling [New] or [NewWithBackend] in order for the
// experimental opt-in to be effective.
//
// In practice this is called by code in the "command" package early in the
// backend initialization codepath and enables the experimental runtime only
// in an experiments-enabled OpenTofu build, to make sure that it's not
// possible to accidentally enable this experimental functionality in normal
// release builds.
//
// Refer to "cmd/tofu/experiments.go" for information on how to produce an
// experiments-enabled build.
func SetExperimentalRuntimeAllowed(allowed bool) {
	experimentalRuntimeAllowed.Store(allowed)
}

var experimentalRuntimeAllowed atomic.Bool

func experimentalRuntimeEnabled() bool {
	if !experimentalRuntimeAllowed.Load() {
		// The experimental runtime is never enabled when it hasn't been
		// explicitly allowed.
		return false
	}

	optIn := os.Getenv("TOFU_X_EXPERIMENTAL_RUNTIME")
	return optIn != ""
}

func (b *Local) opPlanWithExperimentalRuntime(stopCtx context.Context, cancelCtx context.Context, op *backend.Operation, runningOp *backend.RunningOperation) {
	log.Println("[WARN] Using plan implementation from the experimental language runtime")
	var diags tfdiags.Diagnostics
	diags = diags.Append(tfdiags.Sourceless(
		tfdiags.Error,
		"Operation unsupported in experimental language runtime",
		"The command \"tofu plan\" is not yet supported under the experimental language runtime.",
	))
	op.ReportResult(runningOp, diags)
}

func (b *Local) opApplyWithExperimentalRuntime(stopCtx context.Context, cancelCtx context.Context, op *backend.Operation, runningOp *backend.RunningOperation) {
	log.Println("[WARN] Using apply implementation from the experimental language runtime")
	var diags tfdiags.Diagnostics
	diags = diags.Append(tfdiags.Sourceless(
		tfdiags.Error,
		"Operation unsupported in experimental language runtime",
		"The command \"tofu apply\" is not yet supported under the experimental language runtime.",
	))
	op.ReportResult(runningOp, diags)
}

func (b *Local) opRefreshWithExperimentalRuntime(stopCtx context.Context, cancelCtx context.Context, op *backend.Operation, runningOp *backend.RunningOperation) {
	log.Println("[WARN] Using refresh implementation from the experimental language runtime")
	var diags tfdiags.Diagnostics
	diags = diags.Append(tfdiags.Sourceless(
		tfdiags.Error,
		"Operation unsupported in experimental language runtime",
		"The command \"tofu refresh\" is not yet supported under the experimental language runtime.",
	))
	op.ReportResult(runningOp, diags)
}
