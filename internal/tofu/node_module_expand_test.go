// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"testing"

	"github.com/hashicorp/hcl/v2/hcltest"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/zclconf/go-cty/cty"
)

func TestNodeExpandModuleExecute(t *testing.T) {
	evalCtx := &MockEvalContext{
		InstanceExpanderExpander: instances.NewExpander(),
	}
	evalCtx.installSimpleEval()

	node := nodeExpandModule{
		Addr: addrs.Module{"child"},
		ModuleCall: &configs.ModuleCall{
			Count: hcltest.MockExprLiteral(cty.NumberIntVal(2)),
		},
	}

	err := node.Execute(t.Context(), evalCtx, walkApply)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if !evalCtx.InstanceExpanderCalled {
		t.Fatal("did not expand")
	}
}

func TestNodeCloseModuleExecute(t *testing.T) {
	t.Run("walkApply", func(t *testing.T) {
		state := states.NewState()
		state.EnsureModule(addrs.RootModuleInstance.Child("child", addrs.NoKey))
		evalCtx := &MockEvalContext{
			StateState: state.SyncWrapper(),
		}
		node := nodeCloseModule{Addr: addrs.Module{"child"}}
		diags := node.Execute(t.Context(), evalCtx, walkApply)
		if diags.HasErrors() {
			t.Fatalf("unexpected error: %s", diags.Err())
		}

		// Since module.child has no resources, it should be removed
		if _, ok := state.Modules["module.child"]; !ok {
			t.Fatal("module.child should not be removed from state yet")
		}

		// the root module should do all the module cleanup
		node = nodeCloseModule{Addr: addrs.RootModule}
		diags = node.Execute(t.Context(), evalCtx, walkApply)
		if diags.HasErrors() {
			t.Fatalf("unexpected error: %s", diags.Err())
		}

		// Since module.child has no resources, it should be removed
		if _, ok := state.Modules["module.child"]; ok {
			t.Fatal("module.child was not removed from state")
		}
	})

	// walkImport is a no-op
	t.Run("walkImport", func(t *testing.T) {
		state := states.NewState()
		state.EnsureModule(addrs.RootModuleInstance.Child("child", addrs.NoKey))
		evalCtx := &MockEvalContext{
			StateState: state.SyncWrapper(),
		}
		node := nodeCloseModule{Addr: addrs.Module{"child"}}

		diags := node.Execute(t.Context(), evalCtx, walkImport)
		if diags.HasErrors() {
			t.Fatalf("unexpected error: %s", diags.Err())
		}
		if _, ok := state.Modules["module.child"]; !ok {
			t.Fatal("module.child was removed from state, expected no-op")
		}
	})
}

func TestNodeValidateModuleExecute(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		evalCtx := &MockEvalContext{
			InstanceExpanderExpander: instances.NewExpander(),
		}
		evalCtx.installSimpleEval()
		node := nodeValidateModule{
			nodeExpandModule{
				Addr: addrs.Module{"child"},
				ModuleCall: &configs.ModuleCall{
					Count: hcltest.MockExprLiteral(cty.NumberIntVal(2)),
				},
			},
		}

		diags := node.Execute(t.Context(), evalCtx, walkApply)
		if diags.HasErrors() {
			t.Fatalf("unexpected error: %v", diags.Err())
		}
	})

	t.Run("invalid count", func(t *testing.T) {
		evalCtx := &MockEvalContext{
			InstanceExpanderExpander: instances.NewExpander(),
		}
		evalCtx.installSimpleEval()
		node := nodeValidateModule{
			nodeExpandModule{
				Addr: addrs.Module{"child"},
				ModuleCall: &configs.ModuleCall{
					Count: hcltest.MockExprLiteral(cty.StringVal("invalid")),
				},
			},
		}

		err := node.Execute(t.Context(), evalCtx, walkApply)
		if err == nil {
			t.Fatal("expected error, got success")
		}
	})

}
