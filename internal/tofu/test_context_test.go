// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	ctymsgpack "github.com/zclconf/go-cty/cty/msgpack"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/moduletest"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/plugins"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestTestContext_EvaluateAgainstState(t *testing.T) {
	tcs := map[string]struct {
		configs   map[string]string
		state     *states.State
		variables InputValues
		provider  *MockProvider

		expectedDiags  []tfdiags.Description
		expectedStatus moduletest.Status
	}{
		"basic_passing": {
			configs: map[string]string{
				"main.tf": `
resource "test_resource" "a" {
	value = "Hello, world!"
}
`,
				"main.tftest.hcl": `
run "test_case" {
	assert {
		condition = test_resource.a.value == "Hello, world!"
		error_message = "invalid value"
	}
}
`,
			},
			state: states.BuildState(func(state *states.SyncState) {
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test_resource",
						Name: "a",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
						AttrsJSON: encodeCtyValue(t, cty.ObjectVal(map[string]cty.Value{
							"value": cty.StringVal("Hello, world!"),
						})),
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					}, addrs.NoKey)
			}),
			provider: &MockProvider{
				GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
					ResourceTypes: map[string]providers.Schema{
						"test_resource": {
							Block: &configschema.Block{
								Attributes: map[string]*configschema.Attribute{
									"value": {
										Type:     cty.String,
										Required: true,
									},
								},
							},
						},
					},
				},
			},
			expectedStatus: moduletest.Pass,
		},
		"basic_passing_with_sensitive_value": {
			configs: map[string]string{
				"main.tf": `
resource "test_resource" "a" {
	sensitive_value = "Shhhhh!"
}
`,
				"main.tftest.hcl": `
run "test_case" {
	assert {
		condition = test_resource.a.sensitive_value == "Shhhhh!"
		error_message = "invalid value"
	}
}
`,
			},
			state: states.BuildState(func(state *states.SyncState) {
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test_resource",
						Name: "a",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
						AttrsJSON: encodeCtyValue(t, cty.ObjectVal(map[string]cty.Value{
							"sensitive_value": cty.StringVal("Shhhhh!"),
						})),
						AttrSensitivePaths: []cty.PathValueMarks{
							{
								Path:  cty.GetAttrPath("sensitive_value"),
								Marks: cty.NewValueMarks(marks.Sensitive),
							},
						},
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					}, addrs.NoKey)
			}),
			provider: &MockProvider{
				GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
					ResourceTypes: map[string]providers.Schema{
						"test_resource": {
							Block: &configschema.Block{
								Attributes: map[string]*configschema.Attribute{
									"sensitive_value": {
										Type:     cty.String,
										Required: true,
									},
								},
							},
						},
					},
				},
			},
			expectedStatus: moduletest.Pass,
		},
		"module_call_with_deprecated_output": {
			configs: map[string]string{
				"./mod/main.tf": `
output "a" {
  value      = "a"
  deprecated = "Don't use me"
}
				`,
				"main.tf": `
module "mod" {
  source = "./mod"
}
`,
				"main.tftest.hcl": `
run "test_case" {
	assert {
		condition = module.mod.a == "a"
		error_message = "invalid value"
	}
}
`,
			},
			state: states.BuildState(func(state *states.SyncState) {
				outputAddr, _ := addrs.ParseAbsOutputValueStr("module.mod.output.a")
				state.SetOutputValue(outputAddr, cty.StringVal("a"), false, "Don't use me")
			}),
			provider:       &MockProvider{},
			expectedStatus: moduletest.Pass,
			expectedDiags: []tfdiags.Description{
				{
					Summary: "Value derived from a deprecated source",
					Detail:  "This value is derived from module.mod.a, which is deprecated with the following message:\n\nDon't use me",
				},
			},
		},
		"with_variables": {
			configs: map[string]string{
				"main.tf": `
variable "value" {
	type = string
}

resource "test_resource" "a" {
	value = var.value
}
`,
				"main.tftest.hcl": `
variables {
	value = "Hello, world!"
}

run "test_case" {
	assert {
		condition = test_resource.a.value == var.value
		error_message = "invalid value"
	}
}
`,
			},
			state: states.BuildState(func(state *states.SyncState) {
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test_resource",
						Name: "a",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
						AttrsJSON: encodeCtyValue(t, cty.ObjectVal(map[string]cty.Value{
							"value": cty.StringVal("Hello, world!"),
						})),
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					}, addrs.NoKey)
			}),
			variables: InputValues{
				"value": {
					Value: cty.StringVal("Hello, world!"),
				},
			},
			provider: &MockProvider{
				GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
					ResourceTypes: map[string]providers.Schema{
						"test_resource": {
							Block: &configschema.Block{
								Attributes: map[string]*configschema.Attribute{
									"value": {
										Type:     cty.String,
										Required: true,
									},
								},
							},
						},
					},
				},
			},
			expectedStatus: moduletest.Pass,
		},
		"basic_failing": {
			configs: map[string]string{
				"main.tf": `
resource "test_resource" "a" {
	value = "Hello, world!"
}
`,
				"main.tftest.hcl": `
run "test_case" {
	assert {
		condition = test_resource.a.value == "incorrect!"
		error_message = "invalid value"
	}
}
`,
			},
			state: states.BuildState(func(state *states.SyncState) {
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test_resource",
						Name: "a",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
						AttrsJSON: encodeCtyValue(t, cty.ObjectVal(map[string]cty.Value{
							"value": cty.StringVal("Hello, world!"),
						})),
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					}, addrs.NoKey)
			}),
			provider: &MockProvider{
				GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
					ResourceTypes: map[string]providers.Schema{
						"test_resource": {
							Block: &configschema.Block{
								Attributes: map[string]*configschema.Attribute{
									"value": {
										Type:     cty.String,
										Required: true,
									},
								},
							},
						},
					},
				},
			},
			expectedStatus: moduletest.Fail,
			expectedDiags: []tfdiags.Description{
				{
					Summary: "Test assertion failed",
					Detail:  "invalid value",
				},
			},
		},
		"two_failing_assertions": {
			configs: map[string]string{
				"main.tf": `
resource "test_resource" "a" {
	value = "Hello, world!"
}
`,
				"main.tftest.hcl": `
run "test_case" {
	assert {
		condition = test_resource.a.value == "incorrect!"
		error_message = "invalid value"
	}

    assert {
        condition = test_resource.a.value == "also incorrect!"
        error_message = "still invalid"
    }
}
`,
			},
			state: states.BuildState(func(state *states.SyncState) {
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test_resource",
						Name: "a",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
						AttrsJSON: encodeCtyValue(t, cty.ObjectVal(map[string]cty.Value{
							"value": cty.StringVal("Hello, world!"),
						})),
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					}, addrs.NoKey)
			}),
			provider: &MockProvider{
				GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
					ResourceTypes: map[string]providers.Schema{
						"test_resource": {
							Block: &configschema.Block{
								Attributes: map[string]*configschema.Attribute{
									"value": {
										Type:     cty.String,
										Required: true,
									},
								},
							},
						},
					},
				},
			},
			expectedStatus: moduletest.Fail,
			expectedDiags: []tfdiags.Description{
				{
					Summary: "Test assertion failed",
					Detail:  "invalid value",
				},
				{
					Summary: "Test assertion failed",
					Detail:  "still invalid",
				},
			},
		},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			config := testModuleInline(t, tc.configs)
			ctx := testContext2(t, &ContextOpts{
				Plugins: plugins.NewLibrary(map[addrs.Provider]providers.Factory{
					addrs.NewDefaultProvider("test"): testProviderFuncFixed(tc.provider),
				}, nil),
			})

			run := moduletest.Run{
				Config: config.Module.Tests["main.tftest.hcl"].Runs[0],
				Name:   "test_case",
			}

			tctx := ctx.TestContext(config, tc.state, &plans.Plan{}, tc.variables)
			tctx.EvaluateAgainstState(&run)

			if expected, actual := tc.expectedStatus, run.Status; expected != actual {
				t.Errorf("expected status \"%s\" but got \"%s\"", expected, actual)
			}

			compareDiagnosticsFromTestResult(t, tc.expectedDiags, run.Diagnostics)
		})
	}
}

func TestTestContext_EvaluateAgainstPlan(t *testing.T) {
	tcs := map[string]struct {
		configs   map[string]string
		state     *states.State
		plan      *plans.Plan
		variables InputValues
		provider  *MockProvider

		expectedDiags  []tfdiags.Description
		expectedStatus moduletest.Status
	}{
		"basic_passing": {
			configs: map[string]string{
				"main.tf": `
resource "test_resource" "a" {
	value = "Hello, world!"
}
`,
				"main.tftest.hcl": `
run "test_case" {
	assert {
		condition = test_resource.a.value == "Hello, world!"
		error_message = "invalid value"
	}
}
`,
			},
			state: states.BuildState(func(state *states.SyncState) {
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test_resource",
						Name: "a",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectPlanned,
						AttrsJSON: encodeCtyValue(t, cty.NullVal(cty.Object(map[string]cty.Type{
							"value": cty.String,
						}))),
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					}, addrs.NoKey)
			}),
			plan: &plans.Plan{
				Changes: &plans.Changes{
					Resources: []*plans.ResourceInstanceChangeSrc{
						{
							Addr: addrs.Resource{
								Mode: addrs.ManagedResourceMode,
								Type: "test_resource",
								Name: "a",
							}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
							ProviderAddr: addrs.AbsProviderConfig{
								Module:   addrs.RootModule,
								Provider: addrs.NewDefaultProvider("test"),
							},
							ChangeSrc: plans.ChangeSrc{
								Action: plans.Create,
								Before: nil,
								After: encodeDynamicValue(t, cty.ObjectVal(map[string]cty.Value{
									"value": cty.StringVal("Hello, world!"),
								})),
							},
						},
					},
				},
			},
			provider: &MockProvider{
				GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
					ResourceTypes: map[string]providers.Schema{
						"test_resource": {
							Block: &configschema.Block{
								Attributes: map[string]*configschema.Attribute{
									"value": {
										Type:     cty.String,
										Required: true,
									},
								},
							},
						},
					},
				},
			},
			expectedStatus: moduletest.Pass,
		},
		"basic_failing": {
			configs: map[string]string{
				"main.tf": `
resource "test_resource" "a" {
	value = "Hello, world!"
}
`,
				"main.tftest.hcl": `
run "test_case" {
	assert {
		condition = test_resource.a.value == "incorrect!"
		error_message = "invalid value"
	}
}
`,
			},
			state: states.BuildState(func(state *states.SyncState) {
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test_resource",
						Name: "a",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectPlanned,
						AttrsJSON: encodeCtyValue(t, cty.NullVal(cty.Object(map[string]cty.Type{
							"value": cty.String,
						}))),
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					}, addrs.NoKey)
			}),
			plan: &plans.Plan{
				Changes: &plans.Changes{
					Resources: []*plans.ResourceInstanceChangeSrc{
						{
							Addr: addrs.Resource{
								Mode: addrs.ManagedResourceMode,
								Type: "test_resource",
								Name: "a",
							}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
							ProviderAddr: addrs.AbsProviderConfig{
								Module:   addrs.RootModule,
								Provider: addrs.NewDefaultProvider("test"),
							},
							ChangeSrc: plans.ChangeSrc{
								Action: plans.Create,
								Before: nil,
								After: encodeDynamicValue(t, cty.ObjectVal(map[string]cty.Value{
									"value": cty.StringVal("Hello, world!"),
								})),
							},
						},
					},
				},
			},
			provider: &MockProvider{
				GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
					ResourceTypes: map[string]providers.Schema{
						"test_resource": {
							Block: &configschema.Block{
								Attributes: map[string]*configschema.Attribute{
									"value": {
										Type:     cty.String,
										Required: true,
									},
								},
							},
						},
					},
				},
			},
			expectedStatus: moduletest.Fail,
			expectedDiags: []tfdiags.Description{
				{
					Summary: "Test assertion failed",
					Detail:  "invalid value",
				},
			},
		},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			config := testModuleInline(t, tc.configs)
			ctx := testContext2(t, &ContextOpts{
				Plugins: plugins.NewLibrary(map[addrs.Provider]providers.Factory{
					addrs.NewDefaultProvider("test"): testProviderFuncFixed(tc.provider),
				}, nil),
			})

			run := moduletest.Run{
				Config: config.Module.Tests["main.tftest.hcl"].Runs[0],
				Name:   "test_case",
			}

			tctx := ctx.TestContext(config, tc.state, tc.plan, tc.variables)
			tctx.EvaluateAgainstPlan(&run)

			if expected, actual := tc.expectedStatus, run.Status; expected != actual {
				t.Errorf("expected status \"%s\" but got \"%s\"", expected, actual)
			}

			compareDiagnosticsFromTestResult(t, tc.expectedDiags, run.Diagnostics)
		})
	}
}

func compareDiagnosticsFromTestResult(t *testing.T, expected []tfdiags.Description, actual tfdiags.Diagnostics) {
	if len(expected) != len(actual) {
		t.Errorf("found invalid number of diagnostics, expected %d but found %d", len(expected), len(actual))
	}

	length := len(expected)
	if len(actual) > length {
		length = len(actual)
	}

	for ix := 0; ix < length; ix++ {
		if ix >= len(expected) {
			t.Errorf("found extra diagnostic at %d:\n%v", ix, actual[ix].Description())
		} else if ix >= len(actual) {
			t.Errorf("missing diagnostic at %d:\n%v", ix, expected[ix])
		} else {
			expected := expected[ix]
			actual := actual[ix].Description()
			if diff := cmp.Diff(expected, actual); len(diff) > 0 {
				t.Errorf("found different diagnostics at %d:\nexpected:\n%s\nactual:\n%s\ndiff:%s", ix, expected, actual, diff)
			}
		}
	}
}

func encodeDynamicValue(t *testing.T, value cty.Value) []byte {
	data, err := ctymsgpack.Marshal(value, value.Type())
	if err != nil {
		t.Fatalf("failed to marshal JSON: %s", err)
	}
	return data
}

func encodeCtyValue(t *testing.T, value cty.Value) []byte {
	data, err := ctyjson.Marshal(value, value.Type())
	if err != nil {
		t.Fatalf("failed to marshal JSON: %s", err)
	}
	return data
}

func compareDiagnostics(t *testing.T, want, got tfdiags.Diagnostics) {
	if len(want) != len(got) {
		t.Fatalf("cannot compare the diagnostic slices. want len = %d; got len = %d", len(want), len(got))
	}
	diagsSortF := func(a, b tfdiags.Diagnostic) int {
		if d := int(a.Severity().ToHCL()) - int(b.Severity().ToHCL()); d != 0 {
			return d
		}
		if d := strings.Compare(a.Description().Summary, b.Description().Summary); d != 0 {
			return d
		}
		if d := strings.Compare(a.Description().Detail, b.Description().Detail); d != 0 {
			return d
		}
		aSub := a.Source().Subject
		bSub := b.Source().Subject
		if aSub == bSub {
			return 0
		}
		if aSub != nil && bSub == nil {
			return 1
		}
		if aSub == nil && bSub != nil {
			return -1
		}
		return aSub.Start.Line - bSub.Start.Line
	}
	slices.SortFunc(want, diagsSortF)
	slices.SortFunc(got, diagsSortF)
	for i := range want {
		compareDiagnostic(t, i, want[i], got[i])
	}
}

func compareDiagnostic(t *testing.T, i int, want, got tfdiags.Diagnostic) {
	var prefix string
	if i >= 0 {
		prefix = fmt.Sprintf("[idx %d] ", i)
	}
	if wv, gv := want.Severity(), got.Severity(); wv != gv {
		t.Errorf("%sinvalid severity. want: %q but got %q", prefix, wv.String(), gv.String())
	}
	if wv, gv := want.Description().Summary, got.Description().Summary; wv != gv {
		t.Errorf("%sinvalid summary. want: %q but got %q", prefix, wv, gv)
	}
	if wv, gv := want.Description().Detail, got.Description().Detail; wv != gv {
		t.Errorf("%sinvalid detail. want: %q but got %q", prefix, wv, gv)
	}
	if wv, gv := want.Description().Address, got.Description().Address; wv != gv {
		t.Errorf("%sinvalid address. want: %q but got %q", prefix, wv, gv)
	}
	if wv, gv := want.Source(), got.Source(); !wv.Equal(gv) {
		t.Errorf("%sinvalid source. want: %+v but got %+v", prefix, wv, gv)
	}
	wei, gei := want.ExtraInfo(), got.ExtraInfo()
	if diff := cmp.Diff(wei, gei); diff != "" {
		t.Errorf("%sinvalid extra info (-want,+got):\n%s", prefix, diff)
	}
	wexpr, gexpr := want.FromExpr(), got.FromExpr()
	if diff := cmp.Diff(wexpr, gexpr); diff != "" {
		t.Errorf("%sinvalid fromExpr (-want,+got):\n%s", prefix, diff)
	}
}
