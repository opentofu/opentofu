// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"testing"

	"github.com/davecgh/go-spew/spew"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/states"
)

func TestUpdateStateHook(t *testing.T) {
	mockHook := new(MockHook)

	resAddr := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "foo",
		Name: "bar",
	}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance)
	providerAddr, _ := addrs.ParseAbsProviderConfigStr(`provider["registry.opentofu.org/org/foo"]`)
	resData := &states.ResourceInstanceObjectSrc{
		SchemaVersion: 42,
	}

	state := states.NewState()
	state.Module(addrs.RootModuleInstance).SetResourceInstanceCurrent(resAddr.Resource, resData, providerAddr, addrs.NoKey)

	ctx := new(MockEvalContext)
	ctx.HookHook = mockHook
	ctx.StateState = state.SyncWrapper()

	if err := updateStateHook(ctx, resAddr); err != nil {
		t.Fatalf("err: %s", err)
	}

	if !mockHook.PostStateUpdateCalled {
		t.Fatal("should call PostStateUpdate")
	}

	target := states.NewState()
	mockHook.PostStateUpdateFn(target.SyncWrapper())

	if !state.ManagedResourcesEqual(target) {
		t.Fatalf("wrong state passed to hook: %s", spew.Sdump(target))
	}
}

func TestUpdateStateHookRemoved(t *testing.T) {
	mockHook := new(MockHook)

	resAddr0 := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "foo",
		Name: "bar",
	}.Instance(addrs.IntKey(0)).Absolute(addrs.RootModuleInstance)
	resAddr1 := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "foo",
		Name: "bar",
	}.Instance(addrs.IntKey(1)).Absolute(addrs.RootModuleInstance)

	providerAddr, _ := addrs.ParseAbsProviderConfigStr(`provider["registry.opentofu.org/org/foo"]`)
	resData := &states.ResourceInstanceObjectSrc{
		SchemaVersion: 42,
	}

	state := states.NewState()
	state.Module(addrs.RootModuleInstance).SetResourceInstanceCurrent(resAddr0.Resource, resData, providerAddr, addrs.NoKey)

	ctx := new(MockEvalContext)
	ctx.HookHook = mockHook
	ctx.StateState = state.SyncWrapper()

	target := states.NewState()

	// Write resource instance 0
	if err := updateStateHook(ctx, resAddr0); err != nil {
		t.Fatalf("err: %s", err)
	}

	// Flush
	if !mockHook.PostStateUpdateCalled {
		t.Fatal("should call PostStateUpdate")
	}
	mockHook.PostStateUpdateCalled = false
	mockHook.PostStateUpdateFn(target.SyncWrapper())

	// Will remove the entry if it exists
	if err := updateStateHook(ctx, resAddr1); err != nil {
		t.Fatalf("err: %s", err)
	}

	// Flush
	if !mockHook.PostStateUpdateCalled {
		t.Fatal("should call PostStateUpdate")
	}
	mockHook.PostStateUpdateFn(target.SyncWrapper())

	// Comparison
	if !state.ManagedResourcesEqual(target) {
		t.Fatalf("wrong state passed to hook: %s \nExpected:\n %s", spew.Sdump(target), spew.Sdump(state))
	}
}
