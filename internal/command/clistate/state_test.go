// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package clistate

import (
	"context"
	"testing"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	"github.com/opentofu/opentofu/internal/terminal"
)

func TestUnlock(t *testing.T) {
	streams, _ := terminal.StreamsForTesting(t)
	view := views.NewView(streams)

	l := NewLocker(0, views.NewStateLocker(arguments.ViewOptions{ViewType: arguments.ViewHuman}, view))
	l.Lock(statemgr.NewUnlockErrorFull(nil, nil), "test-lock")

	diags := l.Unlock()
	if diags.HasErrors() {
		t.Log(diags.Err().Error())
	} else {
		t.Error("expected error")
	}
}

// TestUnlockWithCancelledContext verifies that Unlock succeeds even when the
// locker's context has been cancelled (e.g., due to SIGINT during apply).
// This is a regression test for https://github.com/opentofu/opentofu/issues/3624
func TestUnlockWithCancelledContext(t *testing.T) {
	streams, _ := terminal.StreamsForTesting(t)
	view := views.NewView(streams)

	// Create a context that is already cancelled (simulates Ctrl+C)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	l := NewLocker(0, views.NewStateLocker(arguments.ViewOptions{ViewType: arguments.ViewHuman}, view))
	l = l.WithContext(ctx)

	mgr := statemgr.NewFullFake(nil, nil)
	l.Lock(mgr, "test-lock")

	// Unlock should succeed despite the cancelled context,
	// This test isn't super because it's using the fake state manager,
	// but it acts as a canary for the regression.
	diags := l.Unlock()
	if diags.HasErrors() {
		t.Errorf("Unlock failed with cancelled context: %s", diags.Err())
	}
}
