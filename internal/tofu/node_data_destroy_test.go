// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/states"
)

func TestNodeDataDestroyExecute(t *testing.T) {
	state := states.NewState()
	state.Module(addrs.RootModuleInstance).SetResourceInstanceCurrent(
		addrs.Resource{
			Mode: addrs.DataResourceMode,
			Type: "test_instance",
			Name: "foo",
		}.Instance(addrs.NoKey),
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"dynamic":{"type":"string","value":"hello"}}`),
		},
		addrs.AbsProviderConfig{
			Provider: addrs.NewDefaultProvider("test"),
			Module:   addrs.RootModule,
		},
		addrs.NoKey,
	)
	evalCtx := &MockEvalContext{
		StateState: state.SyncWrapper(),
	}

	node := NodeDestroyableDataResourceInstance{&NodeAbstractResourceInstance{
		Addr: addrs.Resource{
			Mode: addrs.DataResourceMode,
			Type: "test_instance",
			Name: "foo",
		}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
	}}

	diags := node.Execute(t.Context(), evalCtx, walkApply)
	if diags.HasErrors() {
		t.Fatalf("unexpected error: %v", diags.Err())
	}

	// verify resource removed from state
	if state.HasManagedResourceInstanceObjects() {
		t.Fatal("resources still in state after NodeDataDestroy.Execute")
	}
}
