// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/states"
)

func TestUpdateStateHook(t *testing.T) {
	mockHook := new(MockHook)

	ctx := new(MockEvalContext)
	ctx.HookHook = mockHook
	ctx.StateState = states.NewState().SyncWrapper()

	localAddr := addrs.LocalValue{Name: "foo"}.Absolute(addrs.RootModuleInstance)

	if err := updateState(ctx, func(state *states.SyncState) {
		state.SetLocalValue(localAddr, cty.StringVal("hello"))
	}); err != nil {
		t.Fatalf("err: %s", err)
	}

	if !mockHook.PostStateUpdateCalled {
		t.Fatal("should call PostStateUpdate")
	}
	if ctx.StateState.LocalValue(localAddr) != cty.StringVal("hello") {
		t.Fatalf("wrong state passed to hook: %s", spew.Sdump(ctx.StateState))
	}
}
