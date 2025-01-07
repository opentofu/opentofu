// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/states"
)

func TestNodeExpandApplyableResourceExecute(t *testing.T) {
	state := states.NewState()
	t.Run("no config", func(t *testing.T) {
		ctx := &MockEvalContext{
			StateState:               state.SyncWrapper(),
			InstanceExpanderExpander: instances.NewExpander(),
		}

		node := &nodeExpandApplyableResource{
			NodeAbstractResource: &NodeAbstractResource{
				Addr:   mustConfigResourceAddr("test_instance.foo"),
				Config: nil,
			},
		}
		diags := node.Execute(ctx, walkApply)
		if diags.HasErrors() {
			t.Fatalf("unexpected error: %s", diags.Err())
		}

		state.PruneResourceHusks()
		if !state.Empty() {
			t.Fatalf("expected no state, got:\n %s", state.String())
		}
	})

	t.Run("simple", func(t *testing.T) {
		ctx := &MockEvalContext{
			StateState:               state.SyncWrapper(),
			InstanceExpanderExpander: instances.NewExpander(),
		}

		node := &nodeExpandApplyableResource{
			NodeAbstractResource: &NodeAbstractResource{
				Addr: mustConfigResourceAddr("test_instance.foo"),
				Config: &configs.Resource{
					Mode: addrs.ManagedResourceMode,
					Type: "test_instance",
					Name: "foo",
				},
				ResolvedProvider: ResolvedProvider{ProviderConfig: addrs.AbsProviderConfig{
					Provider: addrs.NewDefaultProvider("test"),
					Module:   addrs.RootModule,
				}},
			},
		}
		diags := node.Execute(ctx, walkApply)
		if diags.HasErrors() {
			t.Fatalf("unexpected error: %s", diags.Err())
		}
		if state.Empty() {
			t.Fatal("expected resources in state, got empty state")
		}
		r := state.Resource(mustAbsResourceAddr("test_instance.foo"))
		if r == nil {
			t.Fatal("test_instance.foo not found in state")
		}
	})
}
