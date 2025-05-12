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
	evalCtx := new(MockEvalContext)
	evalCtx.StateState = states.NewState().SyncWrapper()
	evalCtx.RefreshStateState = states.NewState().SyncWrapper()
	evalCtx.ChecksState = checks.NewState(nil)

	config := &configs.Output{Name: "map-output"}
	addr := addrs.OutputValue{Name: config.Name}.Absolute(addrs.RootModuleInstance)
	node := &NodeApplyableOutput{Config: config, Addr: addr}
	val := cty.MapVal(map[string]cty.Value{
		"a": cty.StringVal("b"),
	})
	evalCtx.EvaluateExprResult = val

	err := node.Execute(t.Context(), evalCtx, walkApply)
	if err != nil {
		t.Fatalf("unexpected execute error: %s", err)
	}

	outputVal := evalCtx.StateState.OutputValue(addr)
	if got, want := outputVal.Value, val; !got.RawEquals(want) {
		t.Errorf("wrong output value in state\n got: %#v\nwant: %#v", got, want)
	}

	if !evalCtx.RefreshStateCalled {
		t.Fatal("should have called RefreshState, but didn't")
	}
	refreshOutputVal := evalCtx.RefreshStateState.OutputValue(addr)
	if got, want := refreshOutputVal.Value, val; !got.RawEquals(want) {
		t.Fatalf("wrong output value in refresh state\n got: %#v\nwant: %#v", got, want)
	}
}

func TestNodeApplyableOutputExecute_noState(t *testing.T) {
	evalCtx := new(MockEvalContext)

	config := &configs.Output{Name: "map-output"}
	addr := addrs.OutputValue{Name: config.Name}.Absolute(addrs.RootModuleInstance)
	node := &NodeApplyableOutput{Config: config, Addr: addr}
	val := cty.MapVal(map[string]cty.Value{
		"a": cty.StringVal("b"),
	})
	evalCtx.EvaluateExprResult = val

	err := node.Execute(t.Context(), evalCtx, walkApply)
	if err != nil {
		t.Fatalf("unexpected execute error: %s", err)
	}
}

func TestNodeApplyableOutputExecute_invalidDependsOn(t *testing.T) {
	evalCtx := new(MockEvalContext)
	evalCtx.StateState = states.NewState().SyncWrapper()
	evalCtx.ChecksState = checks.NewState(nil)

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
	evalCtx.EvaluateExprResult = val

	diags := node.Execute(t.Context(), evalCtx, walkApply)
	if !diags.HasErrors() {
		t.Fatal("expected execute error, but there was none")
	}
	if got, want := diags.Err().Error(), "Invalid depends_on reference"; !strings.Contains(got, want) {
		t.Errorf("expected error to include %q, but was: %s", want, got)
	}
}

func TestNodeApplyableOutputExecute_sensitiveValueNotOutput(t *testing.T) {
	evalCtx := new(MockEvalContext)
	evalCtx.StateState = states.NewState().SyncWrapper()
	evalCtx.ChecksState = checks.NewState(nil)

	config := &configs.Output{Name: "map-output"}
	addr := addrs.OutputValue{Name: config.Name}.Absolute(addrs.RootModuleInstance)
	node := &NodeApplyableOutput{Config: config, Addr: addr}
	val := cty.MapVal(map[string]cty.Value{
		"a": cty.StringVal("b").Mark(marks.Sensitive),
	})
	evalCtx.EvaluateExprResult = val

	diags := node.Execute(t.Context(), evalCtx, walkApply)
	if !diags.HasErrors() {
		t.Fatal("expected execute error, but there was none")
	}
	if got, want := diags.Err().Error(), "Output refers to sensitive values"; !strings.Contains(got, want) {
		t.Errorf("expected error to include %q, but was: %s", want, got)
	}
}

func TestNodeApplyableOutputExecute_deprecatedOutput(t *testing.T) {
	evalCtx := new(MockEvalContext)
	evalCtx.StateState = states.NewState().SyncWrapper()
	evalCtx.ChecksState = checks.NewState(nil)

	config := &configs.Output{Name: "map-output"}
	addr := addrs.OutputValue{Name: config.Name}.Absolute(addrs.RootModuleInstance)
	node := &NodeApplyableOutput{Config: config, Addr: addr}
	val := cty.MapVal(map[string]cty.Value{
		"a": marks.Deprecated(cty.StringVal("b"), marks.DeprecationCause{}),
	})
	evalCtx.EvaluateExprResult = val

	// We set a value with no deprecation marks to check if the marks
	// will be updated in the state.
	evalCtx.StateState.SetOutputValue(addr, marks.RemoveDeepDeprecated(val), false, "")

	diags := node.Execute(t.Context(), evalCtx, walkApply)
	if diags.HasErrors() {
		t.Fatalf("Got unexpected error: %v", diags)
	}

	modOutputAddr, diags := addrs.ParseAbsOutputValueStr("output.map-output")
	if diags.HasErrors() {
		t.Fatalf("Invalid mod addr in test: %v", diags)
	}

	stateVal := evalCtx.StateState.OutputValue(modOutputAddr)

	_, pvms := stateVal.Value.UnmarkDeepWithPaths()
	if len(pvms) != 1 {
		t.Fatalf("Expected a single mark to be present, got: %v", pvms)
	}

	if !marks.HasDeprecated(stateVal.Value.AsValueMap()["a"]) {
		t.Fatalf("No deprecated mark found")
	}
}

func TestNodeApplyableOutputExecute_alternativelyMarkedValue(t *testing.T) {
	evalCtx := new(MockEvalContext)
	evalCtx.StateState = states.NewState().SyncWrapper()
	evalCtx.ChecksState = checks.NewState(nil)

	config := &configs.Output{Name: "map-output"}
	addr := addrs.OutputValue{Name: config.Name}.Absolute(addrs.RootModuleInstance)
	node := &NodeApplyableOutput{Config: config, Addr: addr}
	val := cty.MapVal(map[string]cty.Value{
		"a": cty.StringVal("b").Mark("alternative-mark"),
	})
	evalCtx.EvaluateExprResult = val

	diags := node.Execute(t.Context(), evalCtx, walkApply)
	if diags.HasErrors() {
		t.Fatalf("Got unexpected error: %v", diags)
	}

	modOutputAddr, diags := addrs.ParseAbsOutputValueStr("output.map-output")
	if diags.HasErrors() {
		t.Fatalf("Invalid mod addr in test: %v", diags)
	}

	stateVal := evalCtx.StateState.OutputValue(modOutputAddr)

	_, pvms := stateVal.Value.UnmarkDeepWithPaths()
	if len(pvms) != 1 {
		t.Fatalf("Expected a single mark to be present, got: %v", pvms)
	}

	// We want to check if the mark is still under the same path.
	if !pvms[0].Path.Equals(cty.IndexStringPath("a")) ||
		!pvms[0].Marks.Equal(cty.NewValueMarks("alternative-mark")) {
		t.Fatalf("Expected an alternativeMark with preserved path (a). Got: %v", pvms)
	}
}

func TestNodeApplyableOutputExecute_sensitiveValueAndOutput(t *testing.T) {
	evalCtx := new(MockEvalContext)
	evalCtx.StateState = states.NewState().SyncWrapper()
	evalCtx.ChecksState = checks.NewState(nil)

	config := &configs.Output{
		Name:      "map-output",
		Sensitive: true,
	}
	addr := addrs.OutputValue{Name: config.Name}.Absolute(addrs.RootModuleInstance)
	node := &NodeApplyableOutput{Config: config, Addr: addr}
	val := cty.MapVal(map[string]cty.Value{
		"a": cty.StringVal("b").Mark(marks.Sensitive),
	})
	evalCtx.EvaluateExprResult = val

	err := node.Execute(t.Context(), evalCtx, walkApply)
	if err != nil {
		t.Fatalf("unexpected execute error: %s", err)
	}

	// Unmarked value should be stored in state
	outputVal := evalCtx.StateState.OutputValue(addr)
	want, _ := val.UnmarkDeep()
	if got := outputVal.Value; !got.RawEquals(want) {
		t.Errorf("wrong output value in state\n got: %#v\nwant: %#v", got, want)
	}
}

func TestNodeDestroyableOutputExecute(t *testing.T) {
	outputAddr := addrs.OutputValue{Name: "foo"}.Absolute(addrs.RootModuleInstance)

	state := states.NewState()
	state.Module(addrs.RootModuleInstance).SetOutputValue("foo", cty.StringVal("bar"), false, "")
	state.OutputValue(outputAddr)

	evalCtx := &MockEvalContext{
		StateState: state.SyncWrapper(),
	}
	node := NodeDestroyableOutput{Addr: outputAddr}

	diags := node.Execute(t.Context(), evalCtx, walkApply)
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

	evalCtx := &MockEvalContext{
		StateState: state.SyncWrapper(),
	}
	node := NodeDestroyableOutput{Addr: outputAddr}

	diags := node.Execute(t.Context(), evalCtx, walkApply)
	if diags.HasErrors() {
		t.Fatalf("Unexpected error: %s", diags.Err())
	}
	if state.OutputValue(outputAddr) != nil {
		t.Fatal("Unexpected outputs in state after removal")
	}
}
