// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"sync"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestEvaluatorGetTerraformAttr(t *testing.T) {
	evaluator := &Evaluator{
		Meta: &ContextMeta{
			Env: "foo",
		},
	}
	data := &evaluationStateData{
		Evaluator: evaluator,
	}
	scope := evaluator.Scope(data, nil, nil, nil)

	t.Run("terraform.workspace", func(t *testing.T) {
		want := cty.StringVal("foo")
		got, diags := scope.Data.GetTerraformAttr(t.Context(), addrs.NewTerraformAttr("terraform", "workspace"), tfdiags.SourceRange{})
		if len(diags) != 0 {
			t.Errorf("unexpected diagnostics %s", spew.Sdump(diags))
		}
		if !got.RawEquals(want) {
			t.Errorf("wrong result %q; want %q", got, want)
		}
	})

	t.Run("tofu.workspace", func(t *testing.T) {
		want := cty.StringVal("foo")
		got, diags := scope.Data.GetTerraformAttr(t.Context(), addrs.NewTerraformAttr("tofu", "workspace"), tfdiags.SourceRange{})
		if len(diags) != 0 {
			t.Errorf("unexpected diagnostics %s", spew.Sdump(diags))
		}
		if !got.RawEquals(want) {
			t.Errorf("wrong result %q; want %q", got, want)
		}
	})
}

func TestEvaluatorGetPathAttr(t *testing.T) {
	evaluator := &Evaluator{
		Meta: &ContextMeta{
			Env: "foo",
		},
		Config: &configs.Config{
			Module: &configs.Module{
				SourceDir: "bar/baz",
			},
		},
	}
	data := &evaluationStateData{
		Evaluator: evaluator,
	}
	scope := evaluator.Scope(data, nil, nil, nil)

	t.Run("module", func(t *testing.T) {
		want := cty.StringVal("bar/baz")
		got, diags := scope.Data.GetPathAttr(t.Context(), addrs.PathAttr{
			Name: "module",
		}, tfdiags.SourceRange{})
		if len(diags) != 0 {
			t.Errorf("unexpected diagnostics %s", spew.Sdump(diags))
		}
		if !got.RawEquals(want) {
			t.Errorf("wrong result %#v; want %#v", got, want)
		}
	})

	t.Run("root", func(t *testing.T) {
		want := cty.StringVal("bar/baz")
		got, diags := scope.Data.GetPathAttr(t.Context(), addrs.PathAttr{
			Name: "root",
		}, tfdiags.SourceRange{})
		if len(diags) != 0 {
			t.Errorf("unexpected diagnostics %s", spew.Sdump(diags))
		}
		if !got.RawEquals(want) {
			t.Errorf("wrong result %#v; want %#v", got, want)
		}
	})
}

func TestEvaluatorGetOutputValue(t *testing.T) {
	evaluator := &Evaluator{
		Meta: &ContextMeta{
			Env: "foo",
		},
		Config: &configs.Config{
			Module: &configs.Module{
				Outputs: map[string]*configs.Output{
					"some_output": {
						Name:      "some_output",
						Sensitive: true,
					},
					"some_other_output": {
						Name: "some_other_output",
					},
				},
			},
		},
		State: states.BuildState(func(state *states.SyncState) {
			state.SetOutputValue(addrs.AbsOutputValue{
				Module: addrs.RootModuleInstance,
				OutputValue: addrs.OutputValue{
					Name: "some_output",
				},
			}, cty.StringVal("first"), true, "")
			state.SetOutputValue(addrs.AbsOutputValue{
				Module: addrs.RootModuleInstance,
				OutputValue: addrs.OutputValue{
					Name: "some_other_output",
				},
			}, cty.StringVal("second"), false, "")
		}).SyncWrapper(),
	}

	data := &evaluationStateData{
		Evaluator: evaluator,
	}
	scope := evaluator.Scope(data, nil, nil, nil)

	want := cty.StringVal("first").Mark(marks.Sensitive)
	got, diags := scope.Data.GetOutput(t.Context(), addrs.OutputValue{
		Name: "some_output",
	}, tfdiags.SourceRange{})

	if len(diags) != 0 {
		t.Errorf("unexpected diagnostics %s", spew.Sdump(diags))
	}
	if !got.RawEquals(want) {
		t.Errorf("wrong result %#v; want %#v", got, want)
	}

	want = cty.StringVal("second")
	got, diags = scope.Data.GetOutput(t.Context(), addrs.OutputValue{
		Name: "some_other_output",
	}, tfdiags.SourceRange{})

	if len(diags) != 0 {
		t.Errorf("unexpected diagnostics %s", spew.Sdump(diags))
	}
	if !got.RawEquals(want) {
		t.Errorf("wrong result %#v; want %#v", got, want)
	}
}

// This particularly tests that a sensitive attribute in config
// results in a value that has a "sensitive" cty Mark
func TestEvaluatorGetInputVariable(t *testing.T) {
	evaluator := &Evaluator{
		Meta: &ContextMeta{
			Env: "foo",
		},
		Config: &configs.Config{
			Module: &configs.Module{
				Variables: map[string]*configs.Variable{
					"some_var": {
						Name:           "some_var",
						Sensitive:      true,
						Default:        cty.StringVal("foo"),
						Type:           cty.String,
						ConstraintType: cty.String,
					},
					// Avoid double marking a value
					"some_other_var": {
						Name:           "some_other_var",
						Sensitive:      true,
						Default:        cty.StringVal("bar"),
						Type:           cty.String,
						ConstraintType: cty.String,
					},
				},
			},
		},
		VariableValues: map[string]map[string]cty.Value{
			"": {
				"some_var":       cty.StringVal("bar"),
				"some_other_var": cty.StringVal("boop").Mark(marks.Sensitive),
			},
		},
		VariableValuesLock: &sync.Mutex{},
	}

	data := &evaluationStateData{
		Evaluator: evaluator,
	}
	scope := evaluator.Scope(data, nil, nil, nil)

	want := cty.StringVal("bar").Mark(marks.Sensitive)
	got, diags := scope.Data.GetInputVariable(t.Context(), addrs.InputVariable{
		Name: "some_var",
	}, tfdiags.SourceRange{})

	if len(diags) != 0 {
		t.Errorf("unexpected diagnostics %s", spew.Sdump(diags))
	}
	if !got.RawEquals(want) {
		t.Errorf("wrong result %#v; want %#v", got, want)
	}

	want = cty.StringVal("boop").Mark(marks.Sensitive)
	got, diags = scope.Data.GetInputVariable(t.Context(), addrs.InputVariable{
		Name: "some_other_var",
	}, tfdiags.SourceRange{})

	if len(diags) != 0 {
		t.Errorf("unexpected diagnostics %s", spew.Sdump(diags))
	}
	if !got.RawEquals(want) {
		t.Errorf("wrong result %#v; want %#v", got, want)
	}
}

func TestEvaluatorGetResource(t *testing.T) {
	stateSync := states.BuildState(func(ss *states.SyncState) {
		ss.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_resource",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				Status:    states.ObjectReady,
				AttrsJSON: []byte(`{"id":"foo", "nesting_list": [{"sensitive_value":"abc"}], "nesting_map": {"foo":{"foo":"x"}}, "nesting_set": [{"baz":"abc"}], "nesting_single": {"boop":"abc"}, "nesting_nesting": {"nesting_list":[{"sensitive_value":"abc"}]}, "value":"hello"}`),
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
	}).SyncWrapper()

	rc := &configs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "test_resource",
		Name: "foo",
		Config: configs.SynthBody("", map[string]cty.Value{
			"id": cty.StringVal("foo"),
		}),
		Provider: addrs.Provider{
			Hostname:  addrs.DefaultProviderRegistryHost,
			Namespace: "hashicorp",
			Type:      "test",
		},
	}

	evaluator := &Evaluator{
		Meta: &ContextMeta{
			Env: "foo",
		},
		Changes: plans.NewChanges().SyncWrapper(),
		Config: &configs.Config{
			Module: &configs.Module{
				ManagedResources: map[string]*configs.Resource{
					"test_resource.foo": rc,
				},
			},
		},
		State: stateSync,
		Plugins: schemaOnlyProvidersForTesting(map[addrs.Provider]providers.ProviderSchema{
			addrs.NewDefaultProvider("test"): {
				ResourceTypes: map[string]providers.Schema{
					"test_resource": {
						Block: &configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"id": {
									Type:     cty.String,
									Computed: true,
								},
								"value": {
									Type:      cty.String,
									Computed:  true,
									Sensitive: true,
								},
							},
							BlockTypes: map[string]*configschema.NestedBlock{
								"nesting_list": {
									Block: configschema.Block{
										Attributes: map[string]*configschema.Attribute{
											"value":           {Type: cty.String, Optional: true},
											"sensitive_value": {Type: cty.String, Optional: true, Sensitive: true},
										},
									},
									Nesting: configschema.NestingList,
								},
								"nesting_map": {
									Block: configschema.Block{
										Attributes: map[string]*configschema.Attribute{
											"foo": {Type: cty.String, Optional: true, Sensitive: true},
										},
									},
									Nesting: configschema.NestingMap,
								},
								"nesting_set": {
									Block: configschema.Block{
										Attributes: map[string]*configschema.Attribute{
											"baz": {Type: cty.String, Optional: true, Sensitive: true},
										},
									},
									Nesting: configschema.NestingSet,
								},
								"nesting_single": {
									Block: configschema.Block{
										Attributes: map[string]*configschema.Attribute{
											"boop": {Type: cty.String, Optional: true, Sensitive: true},
										},
									},
									Nesting: configschema.NestingSingle,
								},
								"nesting_nesting": {
									Block: configschema.Block{
										BlockTypes: map[string]*configschema.NestedBlock{
											"nesting_list": {
												Block: configschema.Block{
													Attributes: map[string]*configschema.Attribute{
														"value":           {Type: cty.String, Optional: true},
														"sensitive_value": {Type: cty.String, Optional: true, Sensitive: true},
													},
												},
												Nesting: configschema.NestingList,
											},
										},
									},
									Nesting: configschema.NestingSingle,
								},
							},
						},
					},
				},
			},
		}, t),
	}

	data := &evaluationStateData{
		Evaluator: evaluator,
	}
	scope := evaluator.Scope(data, nil, nil, nil)

	want := cty.ObjectVal(map[string]cty.Value{
		"id": cty.StringVal("foo"),
		"nesting_list": cty.ListVal([]cty.Value{
			cty.ObjectVal(map[string]cty.Value{
				"sensitive_value": cty.StringVal("abc").Mark(marks.Sensitive),
				"value":           cty.NullVal(cty.String),
			}),
		}),
		"nesting_map": cty.MapVal(map[string]cty.Value{
			"foo": cty.ObjectVal(map[string]cty.Value{"foo": cty.StringVal("x").Mark(marks.Sensitive)}),
		}),
		"nesting_nesting": cty.ObjectVal(map[string]cty.Value{
			"nesting_list": cty.ListVal([]cty.Value{
				cty.ObjectVal(map[string]cty.Value{
					"sensitive_value": cty.StringVal("abc").Mark(marks.Sensitive),
					"value":           cty.NullVal(cty.String),
				}),
			}),
		}),
		"nesting_set": cty.SetVal([]cty.Value{
			cty.ObjectVal(map[string]cty.Value{
				"baz": cty.StringVal("abc").Mark(marks.Sensitive),
			}),
		}),
		"nesting_single": cty.ObjectVal(map[string]cty.Value{
			"boop": cty.StringVal("abc").Mark(marks.Sensitive),
		}),
		"value": cty.StringVal("hello").Mark(marks.Sensitive),
	})

	addr := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "test_resource",
		Name: "foo",
	}
	got, diags := scope.Data.GetResource(t.Context(), addr, tfdiags.SourceRange{})

	if len(diags) != 0 {
		t.Errorf("unexpected diagnostics %s", spew.Sdump(diags))
	}

	if !got.RawEquals(want) {
		t.Errorf("wrong result:\ngot: %#v\nwant: %#v", got, want)
	}
}

// GetResource will return a planned object's After value
// if there is a change for that resource instance.
func TestEvaluatorGetResource_changes(t *testing.T) {
	// Set up existing state
	stateSync := states.BuildState(func(ss *states.SyncState) {
		ss.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_resource",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				Status:    states.ObjectPlanned,
				AttrsJSON: []byte(`{"id":"foo", "to_mark_val":"tacos", "sensitive_value":"abc"}`),
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
	}).SyncWrapper()

	// Create a change for the existing state resource,
	// to exercise retrieving the After value of the change
	changesSync := plans.NewChanges().SyncWrapper()
	change := &plans.ResourceInstanceChange{
		Addr: mustResourceInstanceAddr("test_resource.foo"),
		ProviderAddr: addrs.AbsProviderConfig{
			Module:   addrs.RootModule,
			Provider: addrs.NewDefaultProvider("test"),
		},
		Change: plans.Change{
			Action: plans.Update,
			// Provide an After value that contains a marked value
			After: cty.ObjectVal(map[string]cty.Value{
				"id":              cty.StringVal("foo"),
				"to_mark_val":     cty.StringVal("pizza").Mark(marks.Sensitive),
				"sensitive_value": cty.StringVal("abc"),
				"sensitive_collection": cty.MapVal(map[string]cty.Value{
					"boop": cty.StringVal("beep"),
				}),
			}),
		},
	}

	// Set up our schemas
	schemas := &Schemas{
		Providers: map[addrs.Provider]providers.ProviderSchema{
			addrs.NewDefaultProvider("test"): {
				ResourceTypes: map[string]providers.Schema{
					"test_resource": {
						Block: &configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"id": {
									Type:     cty.String,
									Computed: true,
								},
								"to_mark_val": {
									Type:     cty.String,
									Computed: true,
								},
								"sensitive_value": {
									Type:      cty.String,
									Computed:  true,
									Sensitive: true,
								},
								"sensitive_collection": {
									Type:      cty.Map(cty.String),
									Computed:  true,
									Sensitive: true,
								},
							},
						},
					},
				},
			},
		},
	}

	// The resource we'll inspect
	addr := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "test_resource",
		Name: "foo",
	}
	schema, _ := schemas.ResourceTypeConfig(addrs.NewDefaultProvider("test"), addr.Mode, addr.Type)
	// This encoding separates out the After's marks into its AfterValMarks
	csrc, _ := change.Encode(schema.ImpliedType())
	changesSync.AppendResourceInstanceChange(csrc)

	evaluator := &Evaluator{
		Meta: &ContextMeta{
			Env: "foo",
		},
		Changes: changesSync,
		Config: &configs.Config{
			Module: &configs.Module{
				ManagedResources: map[string]*configs.Resource{
					"test_resource.foo": {
						Mode: addrs.ManagedResourceMode,
						Type: "test_resource",
						Name: "foo",
						Provider: addrs.Provider{
							Hostname:  addrs.DefaultProviderRegistryHost,
							Namespace: "hashicorp",
							Type:      "test",
						},
					},
				},
			},
		},
		State:   stateSync,
		Plugins: schemaOnlyProvidersForTesting(schemas.Providers, t),
	}

	data := &evaluationStateData{
		Evaluator: evaluator,
	}
	scope := evaluator.Scope(data, nil, nil, nil)

	want := cty.ObjectVal(map[string]cty.Value{
		"id":              cty.StringVal("foo"),
		"to_mark_val":     cty.StringVal("pizza").Mark(marks.Sensitive),
		"sensitive_value": cty.StringVal("abc").Mark(marks.Sensitive),
		"sensitive_collection": cty.MapVal(map[string]cty.Value{
			"boop": cty.StringVal("beep"),
		}).Mark(marks.Sensitive),
	})

	got, diags := scope.Data.GetResource(t.Context(), addr, tfdiags.SourceRange{})

	if len(diags) != 0 {
		t.Errorf("unexpected diagnostics %s", spew.Sdump(diags))
	}

	if !got.RawEquals(want) {
		t.Errorf("wrong result:\ngot: %#v\nwant: %#v", got, want)
	}
}

func TestEvaluatorGetResource_Ephemeral(t *testing.T) {
	rc := &configs.Resource{
		Mode: addrs.EphemeralResourceMode,
		Type: "test_resource",
		Name: "foo",
		Config: configs.SynthBody("", map[string]cty.Value{
			"secret_name": cty.StringVal("foo"),
		}),
		Provider: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/test"]`).Provider,
	}
	ephemeralSchema := providers.Schema{
		Block: &configschema.Block{
			Attributes: map[string]*configschema.Attribute{
				"id": {
					Type:     cty.String,
					Computed: true,
				},
				"value": {
					Type:     cty.String,
					Computed: true,
				},
			},
			BlockTypes: map[string]*configschema.NestedBlock{
				"nesting_map": {
					Block: configschema.Block{
						Attributes: map[string]*configschema.Attribute{
							"foo": {
								Type:     cty.String,
								Optional: true,
								// Sensitive is added here to ensure that the mark is kept after processing the ephemeral ones
								Sensitive: true,
							},
						},
					},
					Nesting: configschema.NestingSet,
				},
			},
		},
	}
	tests := map[string]struct {
		changes *plans.ChangesSync
		state   *states.SyncState
		want    cty.Value
	}{
		"no changes and no state": {
			plans.NewChanges().SyncWrapper(),
			states.NewState().SyncWrapper(),
			cty.DynamicVal.Mark(marks.Ephemeral),
		},
		"with state and planned changes": {
			plans.BuildChanges(func(sync *plans.ChangesSync) {
				sync.AppendResourceInstanceChange(
					&plans.ResourceInstanceChangeSrc{
						Addr:        rc.Addr().Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
						PrevRunAddr: rc.Addr().Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
						DeposedKey:  states.NotDeposed,
						ProviderAddr: addrs.AbsProviderConfig{
							Provider: rc.Provider,
							Module:   addrs.RootModule,
						},
						ChangeSrc: plans.ChangeSrc{
							After: encodeDynamicValue(t, cty.ObjectVal(map[string]cty.Value{
								"id":    cty.StringVal("foo"),
								"value": cty.StringVal("tacos"),
								"nesting_map": cty.SetVal([]cty.Value{
									cty.ObjectVal(map[string]cty.Value{
										"foo": cty.StringVal("test"),
									}),
								}),
							})),
							AfterValMarks: []cty.PathValueMarks{
								{
									Path: cty.GetAttrPath("nesting_map").Index(cty.ObjectVal(map[string]cty.Value{"foo": cty.StringVal("test")})).GetAttr("foo"),
									Marks: map[interface{}]struct{}{
										// added the ephemeral mark here to validate that it is removed and the
										// sensitive one is added based on the schema
										marks.Ephemeral: {},
									},
								},
							},
						},
					},
				)
			}).SyncWrapper(),
			states.BuildState(func(state *states.SyncState) {
				state.SetResourceInstanceCurrent(
					rc.Addr().Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status:    states.ObjectPlanned,
						AttrsJSON: []byte(`{"id":"foo", "val":"tacos"}`),
					},
					addrs.AbsProviderConfig{
						Provider: rc.Provider,
						Module:   addrs.RootModule,
					},
					addrs.NoKey,
				)
			}).SyncWrapper(),
			cty.ObjectVal(map[string]cty.Value{
				"id":    cty.StringVal("foo"),
				"value": cty.StringVal("tacos"),
				"nesting_map": cty.SetVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						// expected to have this attribute marked as sensitive but not as ephemeral
						// since the ephemeral one is meant to be only at the root block level.
						"foo": cty.StringVal("test").Mark(marks.Sensitive),
					}),
				}),
			}).Mark(marks.Ephemeral),
		},
		"with object ready state and no changes": {
			plans.BuildChanges(func(sync *plans.ChangesSync) {}).SyncWrapper(),
			states.BuildState(func(state *states.SyncState) {
				state.SetResourceInstanceCurrent(
					rc.Addr().Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status:    states.ObjectReady,
						AttrsJSON: []byte(`{"id":"foo", "value":"tacos", "nesting_map": [{"foo": "test"}]}`),
					},
					addrs.AbsProviderConfig{
						Provider: rc.Provider,
						Module:   addrs.RootModule,
					},
					addrs.NoKey,
				)
			}).SyncWrapper(),
			cty.ObjectVal(map[string]cty.Value{
				"id":    cty.StringVal("foo"),
				"value": cty.StringVal("tacos"),
				"nesting_map": cty.SetVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						// expected to have this attribute marked as sensitive but not as ephemeral
						// since the ephemeral one is meant to be only at the root block level.
						"foo": cty.StringVal("test").Mark(marks.Sensitive),
					}),
				}),
			}).Mark(marks.Ephemeral),
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// having these here for easier reference in the test body
			state := tt.state
			changes := tt.changes
			want := tt.want

			evaluator := &Evaluator{
				Meta: &ContextMeta{
					Env: "foo",
				},
				Changes: changes,
				Config: &configs.Config{
					Module: &configs.Module{
						EphemeralResources: map[string]*configs.Resource{
							rc.Addr().String(): rc,
						},
					},
				},
				State: state,
				Plugins: schemaOnlyProvidersForTesting(map[addrs.Provider]providers.ProviderSchema{
					addrs.NewDefaultProvider("test"): {
						EphemeralResources: map[string]providers.Schema{
							"test_resource": ephemeralSchema,
						},
					},
				}, t),
			}
			data := &evaluationStateData{
				Evaluator: evaluator,
			}
			scope := evaluator.Scope(data, nil, nil, nil)

			got, diags := scope.Data.GetResource(t.Context(), rc.Addr(), tfdiags.SourceRange{})

			if len(diags) != 0 {
				t.Errorf("unexpected diagnostics %s", spew.Sdump(diags))
			}

			if !got.RawEquals(want) {
				t.Errorf("wrong result:\ngot: %#v\nwant: %#v", got, want)
			}
		})
	}
}

func TestEvaluatorGetModule(t *testing.T) {
	// Create a new evaluator with an existing state
	stateSync := states.BuildState(func(ss *states.SyncState) {
		ss.SetOutputValue(
			addrs.OutputValue{Name: "out"}.Absolute(addrs.ModuleInstance{addrs.ModuleInstanceStep{Name: "mod"}}),
			cty.StringVal("bar"),
			true,
			"",
		)
	}).SyncWrapper()
	evaluator := evaluatorForModule(stateSync, plans.NewChanges().SyncWrapper())
	data := &evaluationStateData{
		Evaluator: evaluator,
	}
	scope := evaluator.Scope(data, nil, nil, nil)
	want := cty.ObjectVal(map[string]cty.Value{"out": cty.StringVal("bar").Mark(marks.Sensitive)})
	got, diags := scope.Data.GetModule(t.Context(), addrs.ModuleCall{
		Name: "mod",
	}, tfdiags.SourceRange{})

	if len(diags) != 0 {
		t.Errorf("unexpected diagnostics %s", spew.Sdump(diags))
	}
	if !got.RawEquals(want) {
		t.Errorf("wrong result %#v; want %#v", got, want)
	}

	// Changes should override the state value
	changesSync := plans.NewChanges().SyncWrapper()
	change := &plans.OutputChange{
		Addr:      addrs.OutputValue{Name: "out"}.Absolute(addrs.ModuleInstance{addrs.ModuleInstanceStep{Name: "mod"}}),
		Sensitive: true,
		Change: plans.Change{
			After: cty.StringVal("baz"),
		},
	}
	cs, _ := change.Encode()
	changesSync.AppendOutputChange(cs)
	evaluator = evaluatorForModule(stateSync, changesSync)
	data = &evaluationStateData{
		Evaluator: evaluator,
	}
	scope = evaluator.Scope(data, nil, nil, nil)
	want = cty.ObjectVal(map[string]cty.Value{"out": cty.StringVal("baz").Mark(marks.Sensitive)})
	got, diags = scope.Data.GetModule(t.Context(), addrs.ModuleCall{
		Name: "mod",
	}, tfdiags.SourceRange{})

	if len(diags) != 0 {
		t.Errorf("unexpected diagnostics %s", spew.Sdump(diags))
	}
	if !got.RawEquals(want) {
		t.Errorf("wrong result %#v; want %#v", got, want)
	}

	// Test changes with empty state
	evaluator = evaluatorForModule(states.NewState().SyncWrapper(), changesSync)
	data = &evaluationStateData{
		Evaluator: evaluator,
	}
	scope = evaluator.Scope(data, nil, nil, nil)
	want = cty.ObjectVal(map[string]cty.Value{"out": cty.StringVal("baz").Mark(marks.Sensitive)})
	got, diags = scope.Data.GetModule(t.Context(), addrs.ModuleCall{
		Name: "mod",
	}, tfdiags.SourceRange{})

	if len(diags) != 0 {
		t.Errorf("unexpected diagnostics %s", spew.Sdump(diags))
	}
	if !got.RawEquals(want) {
		t.Errorf("wrong result %#v; want %#v", got, want)
	}
}

func evaluatorForModule(stateSync *states.SyncState, changesSync *plans.ChangesSync) *Evaluator {
	return &Evaluator{
		Meta: &ContextMeta{
			Env: "foo",
		},
		Config: &configs.Config{
			Module: &configs.Module{
				ModuleCalls: map[string]*configs.ModuleCall{
					"mod": {
						Name: "mod",
					},
				},
			},
			Children: map[string]*configs.Config{
				"mod": {
					Path: addrs.Module{"module.mod"},
					Module: &configs.Module{
						Outputs: map[string]*configs.Output{
							"out": {
								Name:      "out",
								Sensitive: true,
							},
						},
					},
				},
			},
		},
		State:   stateSync,
		Changes: changesSync,
	}
}
