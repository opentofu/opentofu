// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package experiments

import (
	"os"
	"sync/atomic"
)

// The functions in this file are temporary helpers to allow us to gradually
// build out the new language runtime while ensuring it cannot affect the
// behavior of OpenTofu release builds.
//
// This entire file and all callers of the functions within it should be removed
// once this experiment is over, regardless of whether the experiment is
// successful or unsuccessful.

var experimentalRuntimeAllowed atomic.Bool

// SetExperimentalRuntimeAllowed must be called with the argument set to true
// at some point before instantiating the command implementations in package
// command in order for the dynamic opt-in to the new experimental runtime
// to be considered at all.
//
// In practice this is called by code in the "main" package and enables the
// experimental runtime only in an experiments-enabled OpenTofu build, to make
// sure that it's not possible to accidentally enable this experimental
// functionality in normal release builds.
//
// Refer to "cmd/tofu/experiments.go" for information on how to produce an
// experiments-enabled build.
func SetExperimentalRuntimeAllowed(allowed bool) {
	experimentalRuntimeAllowed.Store(allowed)
}

// ExperimentalRuntimeEnabled returns true only if some caller has previously
// called [SetExperimentalRuntimeAllowed] with its argument set to true
// AND if the environment variable "TOFU_X_EXPERIMENTAL_RUNTIME" is set to
// any non-empty value.
//
// All codepaths that involve the experimental new language runtime MUST be
// dominated by a check that this function returns true, to ensure that the
// experimental code can only run when explicitly enabled and even then only
// in builds that were intentionally built with experimental features enabled.
func ExperimentalRuntimeEnabled() bool {
	if !experimentalRuntimeAllowed.Load() {
		// The experimental runtime is never enabled when it hasn't been
		// explicitly allowed.
		return false
	}

	optIn := os.Getenv("TOFU_X_EXPERIMENTAL_RUNTIME")
	return optIn != ""
}
