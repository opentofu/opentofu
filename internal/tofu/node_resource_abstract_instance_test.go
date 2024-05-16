// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/zclconf/go-cty/cty"
)

func TestNodeAbstractResourceInstanceProvider(t *testing.T) {
	tests := []struct {
		Addr                 addrs.AbsResourceInstance
		Config               *configs.Resource
		StoredProviderConfig addrs.AbsProviderConfig
		Want                 addrs.Provider
	}{
		{
			Addr: addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "null_resource",
				Name: "baz",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			Want: addrs.Provider{
				Hostname:  addrs.DefaultProviderRegistryHost,
				Namespace: "hashicorp",
				Type:      "null",
			},
		},
		{
			Addr: addrs.Resource{
				Mode: addrs.DataResourceMode,
				Type: "terraform_remote_state",
				Name: "baz",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			Want: addrs.Provider{
				// As a special case, the type prefix "terraform_" maps to
				// the builtin provider, not the default one.
				Hostname:  addrs.BuiltInProviderHost,
				Namespace: addrs.BuiltInProviderNamespace,
				Type:      "terraform",
			},
		},
		{
			Addr: addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "null_resource",
				Name: "baz",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			Config: &configs.Resource{
				// Just enough configs.Resource for the Provider method. Not
				// actually valid for general use.
				Provider: addrs.Provider{
					Hostname:  addrs.DefaultProviderRegistryHost,
					Namespace: "awesomecorp",
					Type:      "happycloud",
				},
			},
			// The config overrides the default behavior.
			Want: addrs.Provider{
				Hostname:  addrs.DefaultProviderRegistryHost,
				Namespace: "awesomecorp",
				Type:      "happycloud",
			},
		},
		{
			Addr: addrs.Resource{
				Mode: addrs.DataResourceMode,
				Type: "terraform_remote_state",
				Name: "baz",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			Config: &configs.Resource{
				// Just enough configs.Resource for the Provider method. Not
				// actually valid for general use.
				Provider: addrs.Provider{
					Hostname:  addrs.DefaultProviderRegistryHost,
					Namespace: "awesomecorp",
					Type:      "happycloud",
				},
			},
			// The config overrides the default behavior.
			Want: addrs.Provider{
				Hostname:  addrs.DefaultProviderRegistryHost,
				Namespace: "awesomecorp",
				Type:      "happycloud",
			},
		},
		{
			Addr: addrs.Resource{
				Mode: addrs.DataResourceMode,
				Type: "null_resource",
				Name: "baz",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			Config: nil,
			StoredProviderConfig: addrs.AbsProviderConfig{
				Module: addrs.RootModule,
				Provider: addrs.Provider{
					Hostname:  addrs.DefaultProviderRegistryHost,
					Namespace: "awesomecorp",
					Type:      "null",
				},
			},
			// The stored provider config overrides the default behavior.
			Want: addrs.Provider{
				Hostname:  addrs.DefaultProviderRegistryHost,
				Namespace: "awesomecorp",
				Type:      "null",
			},
		},
	}

	for _, test := range tests {
		var name string
		if test.Config != nil {
			name = fmt.Sprintf("%s with configured %s", test.Addr, test.Config.Provider)
		} else {
			name = fmt.Sprintf("%s with no configuration", test.Addr)
		}
		t.Run(name, func(t *testing.T) {
			node := &NodeAbstractResourceInstance{
				// Just enough NodeAbstractResourceInstance for the Provider
				// function. (This would not be valid for some other functions.)
				Addr: test.Addr,
				NodeAbstractResource: NodeAbstractResource{
					Addr:                 test.Addr.ConfigResource(),
					Config:               test.Config,
					storedProviderConfig: test.StoredProviderConfig,
				},
			}
			got := node.Provider()
			if got != test.Want {
				t.Errorf("wrong result\naddr:  %s\nconfig: %#v\ngot:   %s\nwant:  %s", test.Addr, test.Config, got, test.Want)
			}
		})
	}
}

func TestNodeAbstractResourceInstance_WriteResourceInstanceState(t *testing.T) {
	state := states.NewState()
	ctx := new(MockEvalContext)
	ctx.StateState = state.SyncWrapper()
	ctx.PathPath = addrs.RootModuleInstance

	mockProvider := mockProviderWithResourceTypeSchema("aws_instance", &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"id": {
				Type:     cty.String,
				Optional: true,
			},
		},
	})

	obj := &states.ResourceInstanceObject{
		Value: cty.ObjectVal(map[string]cty.Value{
			"id": cty.StringVal("i-abc123"),
		}),
		Status: states.ObjectReady,
	}

	node := &NodeAbstractResourceInstance{
		Addr: mustResourceInstanceAddr("aws_instance.foo"),
		// instanceState:        obj,
		NodeAbstractResource: NodeAbstractResource{
			ResolvedProvider: mustProviderConfig(`provider["registry.opentofu.org/hashicorp/aws"]`),
		},
	}
	ctx.ProviderProvider = mockProvider
	ctx.ProviderSchemaSchema = mockProvider.GetProviderSchema()

	err := node.writeResourceInstanceState(ctx, obj, workingState)
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	checkStateString(t, state, `
aws_instance.foo:
  ID = i-abc123
  provider = provider["registry.opentofu.org/hashicorp/aws"]
	`)
}

//nolint:dupl,funlen,cyclop // This test doesn't need to be short nor reusing test cases.
func TestComposeMockValueBySchema(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		schema      *configschema.Block
		config      cty.Value
		defaults    map[string]cty.Value
		wantVal     cty.Value
		wantWarning bool
		wantError   bool
	}{
		"diff-props-in-root-attributes": {
			schema: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"required-only": {
						Type:      cty.String,
						Required:  true,
						Optional:  false,
						Computed:  false,
						Sensitive: false,
					},
					"required-computed": {
						Type:      cty.String,
						Required:  true,
						Optional:  false,
						Computed:  true,
						Sensitive: false,
					},
					"optional": {
						Type:      cty.String,
						Required:  false,
						Optional:  true,
						Computed:  false,
						Sensitive: false,
					},
					"optional-computed": {
						Type:      cty.String,
						Required:  false,
						Optional:  true,
						Computed:  true,
						Sensitive: false,
					},
					"computed-only": {
						Type:      cty.String,
						Required:  false,
						Optional:  false,
						Computed:  true,
						Sensitive: false,
					},
					"sensitive-optional": {
						Type:      cty.String,
						Required:  false,
						Optional:  true,
						Computed:  false,
						Sensitive: true,
					},
					"sensitive-required": {
						Type:      cty.String,
						Required:  true,
						Optional:  false,
						Computed:  false,
						Sensitive: true,
					},
					"sensitive-computed": {
						Type:      cty.String,
						Required:  true,
						Optional:  false,
						Computed:  true,
						Sensitive: true,
					},
				},
			},
			config: cty.NilVal,
			wantVal: cty.ObjectVal(map[string]cty.Value{
				"required-only":      cty.NullVal(cty.String),
				"required-computed":  cty.StringVal("aaaaaaaa"),
				"optional":           cty.NullVal(cty.String),
				"optional-computed":  cty.StringVal("aaaaaaaa"),
				"computed-only":      cty.StringVal("aaaaaaaa"),
				"sensitive-optional": cty.NullVal(cty.String),
				"sensitive-required": cty.NullVal(cty.String),
				"sensitive-computed": cty.StringVal("aaaaaaaa"),
			}),
		},
		"diff-props-in-single-block-attributes": {
			schema: &configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"nested": {
						Nesting: configschema.NestingSingle,
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"required-only": {
									Type:      cty.String,
									Required:  true,
									Optional:  false,
									Computed:  false,
									Sensitive: false,
								},
								"required-computed": {
									Type:      cty.String,
									Required:  true,
									Optional:  false,
									Computed:  true,
									Sensitive: false,
								},
								"optional": {
									Type:      cty.String,
									Required:  false,
									Optional:  true,
									Computed:  false,
									Sensitive: false,
								},
								"optional-computed": {
									Type:      cty.String,
									Required:  false,
									Optional:  true,
									Computed:  true,
									Sensitive: false,
								},
								"computed-only": {
									Type:      cty.String,
									Required:  false,
									Optional:  false,
									Computed:  true,
									Sensitive: false,
								},
								"sensitive-optional": {
									Type:      cty.String,
									Required:  false,
									Optional:  true,
									Computed:  false,
									Sensitive: true,
								},
								"sensitive-required": {
									Type:      cty.String,
									Required:  true,
									Optional:  false,
									Computed:  false,
									Sensitive: true,
								},
								"sensitive-computed": {
									Type:      cty.String,
									Required:  true,
									Optional:  false,
									Computed:  true,
									Sensitive: true,
								},
							},
						},
					},
				},
			},
			config: cty.ObjectVal(map[string]cty.Value{
				"nested": cty.ObjectVal(map[string]cty.Value{}),
			}),
			wantVal: cty.ObjectVal(map[string]cty.Value{
				"nested": cty.ObjectVal(map[string]cty.Value{
					"required-only":      cty.NullVal(cty.String),
					"required-computed":  cty.StringVal("aaaaaaaa"),
					"optional":           cty.NullVal(cty.String),
					"optional-computed":  cty.StringVal("aaaaaaaa"),
					"computed-only":      cty.StringVal("aaaaaaaa"),
					"sensitive-optional": cty.NullVal(cty.String),
					"sensitive-required": cty.NullVal(cty.String),
					"sensitive-computed": cty.StringVal("aaaaaaaa"),
				}),
			}),
		},
		"basic-group-block": {
			schema: &configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"nested": {
						Nesting: configschema.NestingGroup,
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"field": {
									Type:     cty.Number,
									Computed: true,
								},
							},
						},
					},
				},
			},
			config: cty.ObjectVal(map[string]cty.Value{
				"nested": cty.ObjectVal(map[string]cty.Value{}),
			}),
			wantVal: cty.ObjectVal(map[string]cty.Value{
				"nested": cty.ObjectVal(map[string]cty.Value{
					"field": cty.NumberIntVal(0),
				}),
			}),
		},
		"basic-list-block": {
			schema: &configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"nested": {
						Nesting: configschema.NestingList,
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"field": {
									Type:     cty.Number,
									Computed: true,
								},
							},
						},
					},
				},
			},
			config: cty.ObjectVal(map[string]cty.Value{
				"nested": cty.ListVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{})}),
			}),
			wantVal: cty.ObjectVal(map[string]cty.Value{
				"nested": cty.ListVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"field": cty.NumberIntVal(0),
					}),
				}),
			}),
		},
		"basic-set-block": {
			schema: &configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"nested": {
						Nesting: configschema.NestingSet,
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"field": {
									Type:     cty.Number,
									Computed: true,
								},
							},
						},
					},
				},
			},
			config: cty.ObjectVal(map[string]cty.Value{
				"nested": cty.SetVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{})}),
			}),
			wantVal: cty.ObjectVal(map[string]cty.Value{
				"nested": cty.SetVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"field": cty.NumberIntVal(0),
					}),
				}),
			}),
		},
		"basic-map-block": {
			schema: &configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"nested": {
						Nesting: configschema.NestingMap,
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"field": {
									Type:     cty.Number,
									Computed: true,
								},
							},
						},
					},
				},
			},
			config: cty.ObjectVal(map[string]cty.Value{
				"nested": cty.MapVal(map[string]cty.Value{
					"somelabel": cty.ObjectVal(map[string]cty.Value{}),
				}),
			}),
			wantVal: cty.ObjectVal(map[string]cty.Value{
				"nested": cty.MapVal(map[string]cty.Value{
					"somelabel": cty.ObjectVal(map[string]cty.Value{
						"field": cty.NumberIntVal(0),
					}),
				}),
			}),
		},
		"basic-mocked-attributes": {
			schema: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"num": {
						Type:     cty.Number,
						Computed: true,
						Optional: true,
					},
					"str": {
						Type:     cty.String,
						Computed: true,
						Optional: true,
					},
					"bool": {
						Type:     cty.Bool,
						Computed: true,
						Optional: true,
					},
					"obj": {
						Type: cty.Object(map[string]cty.Type{
							"fieldNum": cty.Number,
							"fieldStr": cty.String,
						}),
						Computed: true,
						Optional: true,
					},
					"list": {
						Type:     cty.List(cty.String),
						Computed: true,
						Optional: true,
					},
				},
				BlockTypes: map[string]*configschema.NestedBlock{
					"nested": {
						Nesting: configschema.NestingList,
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"num": {
									Type:     cty.Number,
									Computed: true,
									Optional: true,
								},
								"str": {
									Type:     cty.String,
									Computed: true,
									Optional: true,
								},
								"bool": {
									Type:     cty.Bool,
									Computed: true,
									Optional: true,
								},
								"obj": {
									Type: cty.Object(map[string]cty.Type{
										"fieldNum": cty.Number,
										"fieldStr": cty.String,
									}),
									Computed: true,
									Optional: true,
								},
								"list": {
									Type:     cty.List(cty.String),
									Computed: true,
									Optional: true,
								},
							},
						},
					},
				},
			},
			config: cty.ObjectVal(map[string]cty.Value{
				"nested": cty.ListVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{})}),
			}),
			wantVal: cty.ObjectVal(map[string]cty.Value{
				"num":  cty.NumberIntVal(0),
				"str":  cty.StringVal("aaaaaaaa"),
				"bool": cty.False,
				"obj": cty.ObjectVal(map[string]cty.Value{
					"fieldNum": cty.NumberIntVal(0),
					"fieldStr": cty.StringVal("aaaaaaaa"),
				}),
				"list": cty.ListValEmpty(cty.String),
				"nested": cty.ListVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"num":  cty.NumberIntVal(0),
						"str":  cty.StringVal("aaaaaaaa"),
						"bool": cty.False,
						"obj": cty.ObjectVal(map[string]cty.Value{
							"fieldNum": cty.NumberIntVal(0),
							"fieldStr": cty.StringVal("aaaaaaaa"),
						}),
						"list": cty.ListValEmpty(cty.String),
					}),
				}),
			}),
		},
		"source-priority": {
			schema: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"useConfigValue": {
						Type:     cty.String,
						Computed: true,
						Optional: true,
					},
					"useDefaultsValue": {
						Type:     cty.String,
						Computed: true,
						Optional: true,
					},
					"generateMockValue": {
						Type:     cty.String,
						Computed: true,
						Optional: true,
					},
				},
				BlockTypes: map[string]*configschema.NestedBlock{
					"nested": {
						Nesting: configschema.NestingList,
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"useConfigValue": {
									Type:     cty.String,
									Computed: true,
									Optional: true,
								},
								"useDefaultsValue": {
									Type:     cty.String,
									Computed: true,
									Optional: true,
								},
								"generateMockValue": {
									Type:     cty.String,
									Computed: true,
									Optional: true,
								},
							},
						},
					},
				},
			},
			config: cty.ObjectVal(map[string]cty.Value{
				"useConfigValue": cty.StringVal("iAmFromConfig"),
				"nested": cty.ListVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"useConfigValue": cty.StringVal("iAmFromConfig"),
					}),
				}),
			}),
			defaults: map[string]cty.Value{
				"useConfigValue":   cty.StringVal("iAmFromDefaults"),
				"useDefaultsValue": cty.StringVal("iAmFromDefaults"),
				"nested": cty.ObjectVal(map[string]cty.Value{
					"useConfigValue":   cty.StringVal("iAmFromDefaults"),
					"useDefaultsValue": cty.StringVal("iAmFromDefaults"),
				}),
			},
			wantVal: cty.ObjectVal(map[string]cty.Value{
				"useConfigValue":    cty.StringVal("iAmFromConfig"),
				"useDefaultsValue":  cty.StringVal("iAmFromDefaults"),
				"generateMockValue": cty.StringVal("aaaaaaaa"),
				"nested": cty.ListVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"useConfigValue":    cty.StringVal("iAmFromConfig"),
						"useDefaultsValue":  cty.StringVal("iAmFromDefaults"),
						"generateMockValue": cty.StringVal("aaaaaaaa"),
					}),
				}),
			}),
			wantWarning: true, // ignored value in defaults
		},
	}

	mvc := mockValueComposer{
		getMockStringOverride: func(length int) string {
			return strings.Repeat("a", length)
		},
	}

	for name, test := range tests {
		test := test

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			gotVal, gotDiags := mvc.composeMockValueBySchema(test.schema, test.config, test.defaults)
			switch {
			case test.wantError && !gotDiags.HasErrors():
				t.Fatalf("Expected error in diags, but none returned")

			case !test.wantError && gotDiags.HasErrors():
				t.Fatalf("Got unexpected error diags: %v", gotDiags.ErrWithWarnings())

			case test.wantWarning && len(gotDiags) == 0:
				t.Fatalf("Expected warning in diags, but none returned")

			case !test.wantWarning && len(gotDiags) != 0:
				t.Fatalf("Got unexpected diags: %v", gotDiags.ErrWithWarnings())

			case !test.wantVal.RawEquals(gotVal):
				t.Fatalf("Got unexpected value: %v", gotVal.GoString())
			}
		})
	}
}
