package hcl2shim

import (
	"testing"

	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/zclconf/go-cty/cty"
)

// TestComposeMockValueBySchema ensures different configschema.Block values
// processed correctly (lists, maps, objects, etc). Also, it should ensure that
// the resulting values are equal given the same set of inputs (seed, schema, etc).
func TestComposeMockValueBySchema(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		schema    *configschema.Block
		config    cty.Value
		defaults  map[string]cty.Value
		wantVal   cty.Value
		wantError bool
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
				"required-computed":  cty.StringVal("xNmGyAVmNkB4"),
				"optional":           cty.NullVal(cty.String),
				"optional-computed":  cty.StringVal("6zQu0"),
				"computed-only":      cty.StringVal("l3INvNSQT"),
				"sensitive-optional": cty.NullVal(cty.String),
				"sensitive-required": cty.NullVal(cty.String),
				"sensitive-computed": cty.StringVal("ionwj3qrsh4xyC9"),
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
					"required-computed":  cty.StringVal("xNmGyAVmNkB4"),
					"optional":           cty.NullVal(cty.String),
					"optional-computed":  cty.StringVal("6zQu0"),
					"computed-only":      cty.StringVal("l3INvNSQT"),
					"sensitive-optional": cty.NullVal(cty.String),
					"sensitive-required": cty.NullVal(cty.String),
					"sensitive-computed": cty.StringVal("ionwj3qrsh4xyC9"),
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
								"num": {
									Type:     cty.Number,
									Computed: true,
								},
								"str1": {
									Type:     cty.String,
									Computed: true,
								},
								"str2": {
									Type:     cty.String,
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
						"num":  cty.NumberIntVal(0),
						"str1": cty.StringVal("l3INvNSQT"),
						"str2": cty.StringVal("6zQu0"),
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
								"num": {
									Type:     cty.Number,
									Computed: true,
								},
								"str1": {
									Type:     cty.String,
									Computed: true,
								},
								"str2": {
									Type:     cty.String,
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
						"num":  cty.NumberIntVal(0),
						"str1": cty.StringVal("l3INvNSQT"),
						"str2": cty.StringVal("6zQu0"),
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
								"num": {
									Type:     cty.Number,
									Computed: true,
								},
								"str1": {
									Type:     cty.String,
									Computed: true,
								},
								"str2": {
									Type:     cty.String,
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
						"num":  cty.NumberIntVal(0),
						"str1": cty.StringVal("l3INvNSQT"),
						"str2": cty.StringVal("6zQu0"),
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
							"fieldNum":  cty.Number,
							"fieldStr1": cty.String,
							"fieldStr2": cty.String,
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
								"str1": {
									Type:     cty.String,
									Computed: true,
									Optional: true,
								},
								"str2": {
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
				"str":  cty.StringVal("xNmGyAVmNkB4"),
				"bool": cty.False,
				"obj": cty.ObjectVal(map[string]cty.Value{
					"fieldNum":  cty.NumberIntVal(0),
					"fieldStr1": cty.StringVal("l3INvNSQT"),
					"fieldStr2": cty.StringVal("6zQu0"),
				}),
				"list": cty.ListValEmpty(cty.String),
				"nested": cty.ListVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"num":  cty.NumberIntVal(0),
						"str1": cty.StringVal("mCp2gObD"),
						"str2": cty.StringVal("iOtQNQsLiFD5"),
						"bool": cty.False,
						"obj": cty.ObjectVal(map[string]cty.Value{
							"fieldNum": cty.NumberIntVal(0),
							"fieldStr": cty.StringVal("ionwj3qrsh4xyC9"),
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
				"useDefaultsValue": cty.StringVal("iAmFromDefaults"),
				"nested": cty.ObjectVal(map[string]cty.Value{
					"useDefaultsValue": cty.StringVal("iAmFromDefaults"),
				}),
			},
			wantVal: cty.ObjectVal(map[string]cty.Value{
				"useConfigValue":    cty.StringVal("iAmFromConfig"),
				"useDefaultsValue":  cty.StringVal("iAmFromDefaults"),
				"generateMockValue": cty.StringVal("l3INvNSQT"),
				"nested": cty.ListVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"useConfigValue":    cty.StringVal("iAmFromConfig"),
						"useDefaultsValue":  cty.StringVal("iAmFromDefaults"),
						"generateMockValue": cty.StringVal("6zQu0"),
					}),
				}),
			}),
		},
		"type-conversion": {
			schema: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"computed-list": {
						Type:     cty.List(cty.String),
						Computed: true,
					},
				},
			},
			defaults: map[string]cty.Value{
				"computed-list": cty.TupleVal([]cty.Value{
					cty.StringVal("str"),
				}),
			},
			wantVal: cty.ObjectVal(map[string]cty.Value{
				"computed-list": cty.ListVal([]cty.Value{
					cty.StringVal("str"),
				}),
			}),
		},
		"config-override": {
			schema: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"config-field": {
						Type:     cty.String,
						Optional: true,
						Computed: true,
					},
				},
			},
			config: cty.ObjectVal(map[string]cty.Value{
				"config-field": cty.StringVal("iAmFromConfig"),
			}),
			defaults: map[string]cty.Value{
				"config-field": cty.StringVal("str"),
			},
			wantError: true,
		},
		"dynamically-typed-values": {
			schema: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"dynamic-field": {
						Type:     cty.DynamicPseudoType,
						Optional: true,
					},
				},
			},
			wantVal: cty.ObjectVal(map[string]cty.Value{
				"dynamic-field": cty.NullVal(cty.DynamicPseudoType),
			}),
		},
	}

	for name, test := range tests {
		test := test

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			gotVal, gotDiags := NewMockValueComposer(42).ComposeBySchema(test.schema, test.config, test.defaults)
			switch {
			case test.wantError && !gotDiags.HasErrors():
				t.Fatalf("Expected error in diags, but none returned")

			case !test.wantError && gotDiags.HasErrors():
				t.Fatalf("Got unexpected error diags: %v", gotDiags.ErrWithWarnings())

			case !test.wantVal.RawEquals(gotVal):
				t.Fatalf("Got unexpected value: %v", gotVal.GoString())
			}
		})
	}
}
