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

	state := states.NewState()
	state.Module(addrs.RootModuleInstance).SetLocalValue("foo", cty.StringVal("hello"))

	ctx := new(MockEvalContext)
	ctx.HookHook = mockHook
	ctx.StateState = state.SyncWrapper()

	if err := updateStateHook(ctx, nil); err != nil {
		t.Fatalf("err: %s", err)
	}

	if !mockHook.PostStateUpdateCalled {
		t.Fatal("should call PostStateUpdate")
	}
	if mockHook.PostStateUpdateState.LocalValue(addrs.LocalValue{Name: "foo"}.Absolute(addrs.RootModuleInstance)) != cty.StringVal("hello") {
		t.Fatalf("wrong state passed to hook: %s", spew.Sdump(mockHook.PostStateUpdateState))
	}
}
