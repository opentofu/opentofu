// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/checks"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
)

func TestNodeOutputValue_knownValue(t *testing.T) {
	evalCtx := new(MockEvalContext)
	evalCtx.StateState = states.NewState().SyncWrapper()
	evalCtx.RefreshStateState = states.NewState().SyncWrapper()
	evalCtx.ChangesChanges = plans.NewChanges().SyncWrapper()
	evalCtx.ChecksState = checks.NewState(nil)
	evalCtx.withPaths = &mockEvalContextPathTracking{
		// We need to keep track of the separate path-specific
		// MockEvalContexts that the node logic will create
		// as a side-effect of its work so that we can check
		// their calls afterwards.
	}

	config := &configs.Output{Name: "map-output"}
	addr := addrs.OutputValue{Name: config.Name}.Absolute(addrs.RootModuleInstance)
	node := &nodeOutputValue{
		Config: config,
		Module: addrs.RootModule,
		Addr:   addr.OutputValue,
	}
	val := cty.MapVal(map[string]cty.Value{
		"a": cty.StringVal("b"),
	})
	evalCtx.EvaluateExprResult = val

	err := node.Execute(evalCtx, walkApply)
	if err != nil {
		t.Fatalf("unexpected execute error: %s", err)
	}

	outputVal := evalCtx.StateState.OutputValue(addr)
	if got, want := outputVal.Value, val; !got.RawEquals(want) {
		t.Errorf("wrong output value in state\n got: %#v\nwant: %#v", got, want)
	}

	// During execution the node should've asked for a module-instance-specific
	// EvalContext for the root module (where our output value is declared)
	// and then done its main work using that separate EvalContext.
	moduleEvalCtx := evalCtx.withPaths.perPath.Get(addrs.RootModuleInstance)
	if moduleEvalCtx == nil {
		t.Fatal("no MockEvalContext was created for the root module")
	}
	if !moduleEvalCtx.RefreshStateCalled {
		t.Fatal("should have called RefreshState on the root module EvalContext, but didn't")
	}
	refreshOutputVal := evalCtx.RefreshStateState.OutputValue(addr)
	if got, want := refreshOutputVal.Value, val; !got.RawEquals(want) {
		t.Fatalf("wrong output value in refresh state\n got: %#v\nwant: %#v", got, want)
	}
}

func TestNodeOutputValue_noState(t *testing.T) {
	ctx := new(MockEvalContext)
	ctx.ChangesChanges = plans.NewChanges().SyncWrapper()

	config := &configs.Output{Name: "map-output"}
	addr := addrs.OutputValue{Name: config.Name}.Absolute(addrs.RootModuleInstance)
	node := &nodeOutputValue{
		Config: config,
		Module: addrs.RootModule,
		Addr:   addr.OutputValue,
	}
	val := cty.MapVal(map[string]cty.Value{
		"a": cty.StringVal("b"),
	})
	ctx.EvaluateExprResult = val

	err := node.Execute(ctx, walkApply)
	if err != nil {
		t.Fatalf("unexpected execute error: %s", err)
	}
}

func TestNodeOutputValue_invalidDependsOn(t *testing.T) {
	ctx := new(MockEvalContext)
	ctx.StateState = states.NewState().SyncWrapper()
	ctx.ChangesChanges = plans.NewChanges().SyncWrapper()
	ctx.ChecksState = checks.NewState(nil)

	config := &configs.Output{
		Name: "map-output",
		DependsOn: []hcl.Traversal{
			{
				hcl.TraverseRoot{Name: "test_instance"},
				hcl.TraverseAttr{Name: "foo"},
				hcl.TraverseAttr{Name: "bar"},
			},
		},
	}
	addr := addrs.OutputValue{Name: config.Name}.Absolute(addrs.RootModuleInstance)
	node := &nodeOutputValue{
		Config: config,
		Module: addrs.RootModule,
		Addr:   addr.OutputValue,
	}
	val := cty.MapVal(map[string]cty.Value{
		"a": cty.StringVal("b"),
	})
	ctx.EvaluateExprResult = val

	diags := node.Execute(ctx, walkApply)
	if !diags.HasErrors() {
		t.Fatal("expected execute error, but there was none")
	}
	if got, want := diags.Err().Error(), "Invalid depends_on reference"; !strings.Contains(got, want) {
		t.Errorf("expected error to include %q, but was: %s", want, got)
	}
}

func TestNodeOutputValue_sensitiveValueNotOutput(t *testing.T) {
	ctx := new(MockEvalContext)
	ctx.StateState = states.NewState().SyncWrapper()
	ctx.ChangesChanges = plans.NewChanges().SyncWrapper()
	ctx.ChecksState = checks.NewState(nil)

	config := &configs.Output{Name: "map-output"}
	addr := addrs.OutputValue{Name: config.Name}.Absolute(addrs.RootModuleInstance)
	node := &nodeOutputValue{
		Config: config,
		Module: addrs.RootModule,
		Addr:   addr.OutputValue,
	}
	val := cty.MapVal(map[string]cty.Value{
		"a": cty.StringVal("b").Mark(marks.Sensitive),
	})
	ctx.EvaluateExprResult = val

	diags := node.Execute(ctx, walkApply)
	if !diags.HasErrors() {
		t.Fatal("expected execute error, but there was none")
	}
	if got, want := diags.Err().Error(), "Output refers to sensitive values"; !strings.Contains(got, want) {
		t.Errorf("expected error to include %q, but was: %s", want, got)
	}
}

func TestNodeOutputValue_sensitiveValueAndOutput(t *testing.T) {
	ctx := new(MockEvalContext)
	ctx.StateState = states.NewState().SyncWrapper()
	ctx.ChangesChanges = plans.NewChanges().SyncWrapper()
	ctx.ChecksState = checks.NewState(nil)

	config := &configs.Output{
		Name:      "map-output",
		Sensitive: true,
	}
	addr := addrs.OutputValue{Name: config.Name}.Absolute(addrs.RootModuleInstance)
	node := &nodeOutputValue{
		Config: config,
		Module: addrs.RootModule,
		Addr:   addr.OutputValue,
	}
	val := cty.MapVal(map[string]cty.Value{
		"a": cty.StringVal("b").Mark(marks.Sensitive),
	})
	ctx.EvaluateExprResult = val

	err := node.Execute(ctx, walkApply)
	if err != nil {
		t.Fatalf("unexpected execute error: %s", err)
	}

	// Unmarked value should be stored in state
	outputVal := ctx.StateState.OutputValue(addr)
	want, _ := val.UnmarkDeep()
	if got := outputVal.Value; !got.RawEquals(want) {
		t.Errorf("wrong output value in state\n got: %#v\nwant: %#v", got, want)
	}
}

func TestNodeOutputValue_executeRootDestroy(t *testing.T) {
	outputAddr := addrs.OutputValue{Name: "foo"}.Absolute(addrs.RootModuleInstance)

	state := states.NewState()
	state.Module(addrs.RootModuleInstance).SetOutputValue("foo", cty.StringVal("bar"), false)
	state.OutputValue(outputAddr)

	ctx := &MockEvalContext{
		StateState:     state.SyncWrapper(),
		ChangesChanges: plans.NewChanges().SyncWrapper(),
	}
	node := &nodeOutputValue{
		Module:     addrs.RootModule,
		Addr:       outputAddr.OutputValue,
		Destroying: true,
	}

	diags := node.Execute(ctx, walkApply)
	if diags.HasErrors() {
		t.Fatalf("Unexpected error: %s", diags.Err())
	}
	if state.OutputValue(outputAddr) != nil {
		t.Fatal("Unexpected outputs in state after removal")
	}
}

func TestNodeOutputValue_destroyNotInState(t *testing.T) {
	outputAddr := addrs.OutputValue{Name: "foo"}.Absolute(addrs.RootModuleInstance)

	state := states.NewState()

	ctx := &MockEvalContext{
		StateState:     state.SyncWrapper(),
		ChangesChanges: plans.NewChanges().SyncWrapper(),
	}
	node := &nodeOutputValue{
		Module:     addrs.RootModule,
		Addr:       outputAddr.OutputValue,
		Destroying: true,
	}

	diags := node.Execute(ctx, walkApply)
	if diags.HasErrors() {
		t.Fatalf("Unexpected error: %s", diags.Err())
	}
	if state.OutputValue(outputAddr) != nil {
		t.Fatal("Unexpected outputs in state after removal")
	}
}
