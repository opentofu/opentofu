package hcl2shim

import (
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/zclconf/go-cty/cty"
)

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

	const mockStringLength = 8

	mvc := mockValueComposer{
		getMockStringOverride: func() string {
			return strings.Repeat("a", mockStringLength)
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
