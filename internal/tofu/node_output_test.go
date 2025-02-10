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
	"github.com/opentofu/opentofu/internal/states"
)

func TestNodeApplyableOutputExecute_knownValue(t *testing.T) {
	ctx := new(MockEvalContext)
	ctx.StateState = states.NewState().SyncWrapper()
	ctx.RefreshStateState = states.NewState().SyncWrapper()
	ctx.ChecksState = checks.NewState(nil)

	config := &configs.Output{Name: "map-output"}
	addr := addrs.OutputValue{Name: config.Name}.Absolute(addrs.RootModuleInstance)
	node := &NodeApplyableOutput{Config: config, Addr: addr}
	val := cty.MapVal(map[string]cty.Value{
		"a": cty.StringVal("b"),
	})
	ctx.EvaluateExprResult = val

	err := node.Execute(ctx, walkApply)
	if err != nil {
		t.Fatalf("unexpected execute error: %s", err)
	}

	outputVal := ctx.StateState.OutputValue(addr)
	if got, want := outputVal.Value, val; !got.RawEquals(want) {
		t.Errorf("wrong output value in state\n got: %#v\nwant: %#v", got, want)
	}

	if !ctx.RefreshStateCalled {
		t.Fatal("should have called RefreshState, but didn't")
	}
	refreshOutputVal := ctx.RefreshStateState.OutputValue(addr)
	if got, want := refreshOutputVal.Value, val; !got.RawEquals(want) {
		t.Fatalf("wrong output value in refresh state\n got: %#v\nwant: %#v", got, want)
	}
}

func TestNodeApplyableOutputExecute_noState(t *testing.T) {
	ctx := new(MockEvalContext)

	config := &configs.Output{Name: "map-output"}
	addr := addrs.OutputValue{Name: config.Name}.Absolute(addrs.RootModuleInstance)
	node := &NodeApplyableOutput{Config: config, Addr: addr}
	val := cty.MapVal(map[string]cty.Value{
		"a": cty.StringVal("b"),
	})
	ctx.EvaluateExprResult = val

	err := node.Execute(ctx, walkApply)
	if err != nil {
		t.Fatalf("unexpected execute error: %s", err)
	}
}

func TestNodeApplyableOutputExecute_invalidDependsOn(t *testing.T) {
	ctx := new(MockEvalContext)
	ctx.StateState = states.NewState().SyncWrapper()
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
	node := &NodeApplyableOutput{Config: config, Addr: addr}
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

func TestNodeApplyableOutputExecute_sensitiveValueNotOutput(t *testing.T) {
	ctx := new(MockEvalContext)
	ctx.StateState = states.NewState().SyncWrapper()
	ctx.ChecksState = checks.NewState(nil)

	config := &configs.Output{Name: "map-output"}
	addr := addrs.OutputValue{Name: config.Name}.Absolute(addrs.RootModuleInstance)
	node := &NodeApplyableOutput{Config: config, Addr: addr}
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

func TestNodeApplyableOutputExecute_alternativelyMarkedValue(t *testing.T) {
	ctx := new(MockEvalContext)
	ctx.StateState = states.NewState().SyncWrapper()
	ctx.ChecksState = checks.NewState(nil)

	config := &configs.Output{Name: "map-output"}
	addr := addrs.OutputValue{Name: config.Name}.Absolute(addrs.RootModuleInstance)
	node := &NodeApplyableOutput{Config: config, Addr: addr}
	val := cty.MapVal(map[string]cty.Value{
		"a": cty.StringVal("b").Mark("alternative-mark"),
	})
	ctx.EvaluateExprResult = val

	diags := node.Execute(ctx, walkApply)
	if diags.HasErrors() {
		t.Fatalf("Got unexpected error: %v", diags)
	}

	modOutputAddr, diags := addrs.ParseAbsOutputValueStr("output.map-output")
	if diags.HasErrors() {
		t.Fatalf("Invalid mod addr in test: %v", diags)
	}

	stateVal := ctx.StateState.OutputValue(modOutputAddr)

	if !stateVal.Value.HasMark("alternative-mark") {
		t.Fatalf("Non-sensitive mark has been erased")
	}
}

func TestNodeApplyableOutputExecute_sensitiveValueAndOutput(t *testing.T) {
	ctx := new(MockEvalContext)
	ctx.StateState = states.NewState().SyncWrapper()
	ctx.ChecksState = checks.NewState(nil)

	config := &configs.Output{
		Name:      "map-output",
		Sensitive: true,
	}
	addr := addrs.OutputValue{Name: config.Name}.Absolute(addrs.RootModuleInstance)
	node := &NodeApplyableOutput{Config: config, Addr: addr}
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

func TestNodeDestroyableOutputExecute(t *testing.T) {
	outputAddr := addrs.OutputValue{Name: "foo"}.Absolute(addrs.RootModuleInstance)

	state := states.NewState()
	state.Module(addrs.RootModuleInstance).SetOutputValue("foo", cty.StringVal("bar"), false)
	state.OutputValue(outputAddr)

	ctx := &MockEvalContext{
		StateState: state.SyncWrapper(),
	}
	node := NodeDestroyableOutput{Addr: outputAddr}

	diags := node.Execute(ctx, walkApply)
	if diags.HasErrors() {
		t.Fatalf("Unexpected error: %s", diags.Err())
	}
	if state.OutputValue(outputAddr) != nil {
		t.Fatal("Unexpected outputs in state after removal")
	}
}

func TestNodeDestroyableOutputExecute_notInState(t *testing.T) {
	outputAddr := addrs.OutputValue{Name: "foo"}.Absolute(addrs.RootModuleInstance)

	state := states.NewState()

	ctx := &MockEvalContext{
		StateState: state.SyncWrapper(),
	}
	node := NodeDestroyableOutput{Addr: outputAddr}

	diags := node.Execute(ctx, walkApply)
	if diags.HasErrors() {
		t.Fatalf("Unexpected error: %s", diags.Err())
	}
	if state.OutputValue(outputAddr) != nil {
		t.Fatal("Unexpected outputs in state after removal")
	}
}
