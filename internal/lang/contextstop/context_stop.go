// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package contextstop is the evolution of the stopHook structure, originally
// defined in the internal/tofu/hook_stop.go file. Rather than using the hook
// infrastructure to return the status of a stop flag, the caller is expected
// to check for the stop flag wherever hooks might've originally been used.
// While this is slightly cumbersome, it's preferable over returning errors
// in a hooks package which makes it hard to trace execution.
package contextstop

import (
	"context"
	"sync/atomic"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

type contextStopKey string

const stopKey contextStopKey = "stop"

// AddStopFlag adds a stop flag to the context.
func AddStopFlag(ctx context.Context) context.Context {
	var stop uint32
	return context.WithValue(ctx, stopKey, &stop)
}

// Stop sets the stop flag on the context
func Stop(ctx context.Context) {
	if stop, ok := ctx.Value(stopKey).(*uint32); ok {
		atomic.StoreUint32(stop, 1)
	}
}

// CheckStop checks if the stop flag on the context has been set, for example by Stop.
//
// This is safe to call on any context, and works with concurrent processes. Call this anywhere most hooks might be called.
func CheckStop(ctx context.Context) tfdiags.Diagnostics {
	if stop, ok := ctx.Value(stopKey).(*uint32); ok && atomic.LoadUint32(stop) == 1 {
		return tfdiags.New(tfdiags.Sourceless(tfdiags.Error, "Execution halted", "Providers and provisioners have been stopped in the course of ordinary operations."))
	}
	return nil
}
