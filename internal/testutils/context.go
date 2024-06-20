// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils

import (
	"context"
	"testing"
	"time"
)

const minCleanupSafety = time.Second * 30
const maxCleanupSafety = time.Minute * 5

// Context returns a context configured for the test deadline. This function configures a context with 25% safety for
// any cleanup tasks to finish.
func Context(t *testing.T) context.Context {
	ctx := context.Background()
	if deadline, ok := t.Deadline(); ok {
		var cancel func()
		timeoutDuration := time.Until(deadline)
		cleanupSafety := min(max(timeoutDuration/4, minCleanupSafety), maxCleanupSafety) //nolint:mnd //This will never change.
		ctx, cancel = context.WithDeadline(ctx, deadline.Add(-1*cleanupSafety))
		t.Cleanup(cancel)
	}
	return ctx
}

// CleanupContext returns a context that provides a deadline until the test finishes, but maximum of maxCleanupSafety.
func CleanupContext(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), maxCleanupSafety)
	t.Cleanup(cancel)
	if deadline, ok := t.Deadline(); ok {
		var cancelDeadline func()
		ctx, cancelDeadline = context.WithDeadline(ctx, deadline)
		t.Cleanup(cancelDeadline)
	}
	return ctx
}
