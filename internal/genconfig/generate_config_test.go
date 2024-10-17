// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package genconfig

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/lang/marks"
)

func TestConfigGeneration(t *testing.T) {
	sensitiveVal := func(t cty.Type, required, optional, computed bool) *configschema.Attribute {
		return &configschema.Attribute{
			Type:      t,
			Sensitive: true,
			Required:  required,
			Optional:  optional,
			Computed:  computed,
		}
	}

	tcs := map[string]struct {
		schema   *configschema.Block
		addr     addrs.AbsResourceInstance
		provider addrs.LocalProviderConfig
		value    cty.Value
		expected string
	}{
		"simple_resource": {
			schema: &configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"list_block": {
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"nested_value": {
									Type:     cty.String,
									Optional: true,
								},
							},
						},
						Nesting: configschema.NestingSingle,
					},
				},
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"value": {
						Type:     cty.String,
						Optional: true,
					},
				},
			},
			addr: addrs.AbsResourceInstance{
				Module: nil,
				Resource: addrs.ResourceInstance{
					Resource: addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "tfcoremock_simple_resource",
						Name: "empty",
					},
					Key: nil,
				},
			},
			provider: addrs.LocalProviderConfig{
				LocalName: "tfcoremock",
			},
			value: cty.NilVal,
			expected: `
resource "tfcoremock_simple_resource" "empty" {
  value = null          # OPTIONAL string
  list_block {          # OPTIONAL block
    nested_value = null # OPTIONAL string
  }
}`,
		},
		"simple_resource_with_state": {
			schema: &configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"list_block": {
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"nested_value": {
									Type:     cty.String,
									Optional: true,
								},
							},
						},
						Nesting: configschema.NestingSingle,
					},
				},
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"value": {
						Type:     cty.String,
						Optional: true,
					},
				},
			},
			addr: addrs.AbsResourceInstance{
				Module: nil,
				Resource: addrs.ResourceInstance{
					Resource: addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "tfcoremock_simple_resource",
						Name: "empty",
					},
					Key: nil,
				},
			},
			provider: addrs.LocalProviderConfig{
				LocalName: "tfcoremock",
			},
			value: cty.ObjectVal(map[string]cty.Value{
				"id":    cty.StringVal("D2320658"),
				"value": cty.StringVal("Hello, world!"),
				"list_block": cty.ObjectVal(map[string]cty.Value{
					"nested_value": cty.StringVal("Hello, solar system!"),
				}),
			}),
			expected: `
resource "tfcoremock_simple_resource" "empty" {
  value = "Hello, world!"
  list_block {
    nested_value = "Hello, solar system!"
  }
}`,
		},
		"simple_resource_with_partial_state": {
			schema: &configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"list_block": {
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"nested_value": {
									Type:     cty.String,
									Optional: true,
								},
							},
						},
						Nesting: configschema.NestingSingle,
					},
				},
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"value": {
						Type:     cty.String,
						Optional: true,
					},
				},
			},
			addr: addrs.AbsResourceInstance{
				Module: nil,
				Resource: addrs.ResourceInstance{
					Resource: addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "tfcoremock_simple_resource",
						Name: "empty",
					},
					Key: nil,
				},
			},
			provider: addrs.LocalProviderConfig{
				LocalName: "tfcoremock",
			},
			value: cty.ObjectVal(map[string]cty.Value{
				"id": cty.StringVal("D2320658"),
				"list_block": cty.ObjectVal(map[string]cty.Value{
					"nested_value": cty.StringVal("Hello, solar system!"),
				}),
			}),
			expected: `
resource "tfcoremock_simple_resource" "empty" {
  value = null
  list_block {
    nested_value = "Hello, solar system!"
  }
}`,
		},
		"simple_resource_with_alternate_provider": {
			schema: &configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"list_block": {
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"nested_value": {
									Type:     cty.String,
									Optional: true,
								},
							},
						},
						Nesting: configschema.NestingSingle,
					},
				},
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"value": {
						Type:     cty.String,
						Optional: true,
					},
				},
			},
			addr: addrs.AbsResourceInstance{
				Module: nil,
				Resource: addrs.ResourceInstance{
					Resource: addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "tfcoremock_simple_resource",
						Name: "empty",
					},
					Key: nil,
				},
			},
			provider: addrs.LocalProviderConfig{
				LocalName: "mock",
			},
			value: cty.ObjectVal(map[string]cty.Value{
				"id":    cty.StringVal("D2320658"),
				"value": cty.StringVal("Hello, world!"),
				"list_block": cty.ObjectVal(map[string]cty.Value{
					"nested_value": cty.StringVal("Hello, solar system!"),
				}),
			}),
			expected: `
resource "tfcoremock_simple_resource" "empty" {
  provider = mock
  value    = "Hello, world!"
  list_block {
    nested_value = "Hello, solar system!"
  }
}`,
		},
		"simple_resource_with_aliased_provider": {
			schema: &configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"list_block": {
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"nested_value": {
									Type:     cty.String,
									Optional: true,
								},
							},
						},
						Nesting: configschema.NestingSingle,
					},
				},
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"value": {
						Type:     cty.String,
						Optional: true,
					},
				},
			},
			addr: addrs.AbsResourceInstance{
				Module: nil,
				Resource: addrs.ResourceInstance{
					Resource: addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "tfcoremock_simple_resource",
						Name: "empty",
					},
					Key: nil,
				},
			},
			provider: addrs.LocalProviderConfig{
				LocalName: "tfcoremock",
				Alias:     "alternate",
			},
			value: cty.ObjectVal(map[string]cty.Value{
				"id":    cty.StringVal("D2320658"),
				"value": cty.StringVal("Hello, world!"),
				"list_block": cty.ObjectVal(map[string]cty.Value{
					"nested_value": cty.StringVal("Hello, solar system!"),
				}),
			}),
			expected: `
resource "tfcoremock_simple_resource" "empty" {
  provider = tfcoremock.alternate
  value    = "Hello, world!"
  list_block {
    nested_value = "Hello, solar system!"
  }
}`,
		},
		"resource_with_nulls": {
			schema: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"single": {
						NestedType: &configschema.Object{
							Attributes: map[string]*configschema.Attribute{},
							Nesting:    configschema.NestingSingle,
						},
						Required: true,
					},
					"list": {
						NestedType: &configschema.Object{
							Attributes: map[string]*configschema.Attribute{
								"nested_id": {
									Type:     cty.String,
									Optional: true,
								},
							},
							Nesting: configschema.NestingList,
						},
						Required: true,
					},
					"map": {
						NestedType: &configschema.Object{
							Attributes: map[string]*configschema.Attribute{
								"nested_id": {
									Type:     cty.String,
									Optional: true,
								},
							},
							Nesting: configschema.NestingMap,
						},
						Required: true,
					},
				},
				BlockTypes: map[string]*configschema.NestedBlock{
					"nested_single": {
						Nesting: configschema.NestingSingle,
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"nested_id": {
									Type:     cty.String,
									Optional: true,
								},
							},
						},
					},
					// No configschema.NestingGroup example for this test, because this block type can never be null in state.
					"nested_list": {
						Nesting: configschema.NestingList,
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"nested_id": {
									Type:     cty.String,
									Optional: true,
								},
							},
						},
					},
					"nested_set": {
						Nesting: configschema.NestingSet,
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"nested_id": {
									Type:     cty.String,
									Optional: true,
								},
							},
						},
					},
					"nested_map": {
						Nesting: configschema.NestingMap,
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"nested_id": {
									Type:     cty.String,
									Optional: true,
								},
							},
						},
					},
				},
			},
			addr: addrs.AbsResourceInstance{
				Module: nil,
				Resource: addrs.ResourceInstance{
					Resource: addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "tfcoremock_simple_resource",
						Name: "empty",
					},
					Key: nil,
				},
			},
			provider: addrs.LocalProviderConfig{
				LocalName: "tfcoremock",
			},
			value: cty.ObjectVal(map[string]cty.Value{
				"id":     cty.StringVal("D2320658"),
				"single": cty.NullVal(cty.Object(map[string]cty.Type{})),
				"list": cty.NullVal(cty.List(cty.Object(map[string]cty.Type{
					"nested_id": cty.String,
				}))),
				"map": cty.NullVal(cty.Map(cty.Object(map[string]cty.Type{
					"nested_id": cty.String,
				}))),
				"nested_single": cty.NullVal(cty.Object(map[string]cty.Type{
					"nested_id": cty.String,
				})),
				"nested_list": cty.ListValEmpty(cty.Object(map[string]cty.Type{
					"nested_id": cty.String,
				})),
				"nested_set": cty.SetValEmpty(cty.Object(map[string]cty.Type{
					"nested_id": cty.String,
				})),
				"nested_map": cty.MapValEmpty(cty.Object(map[string]cty.Type{
					"nested_id": cty.String,
				})),
			}),
			expected: `
resource "tfcoremock_simple_resource" "empty" {
  list   = null
  map    = null
  single = null
}`,
		},
		"jsonencode_wrapping": {
			schema: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"juststr": {
						Type:     cty.String,
						Optional: true,
					},
					"jsonobj": {
						Type:     cty.String,
						Optional: true,
					},
					"jsonarr": {
						Type:     cty.String,
						Optional: true,
					},
					"sensitivejsonobj": {
						Type:      cty.String,
						Optional:  true,
						Sensitive: true,
					},
					"secrets": {
						Type: cty.Object(map[string]cty.Type{
							"main":      cty.String,
							"secondary": cty.String,
						}),
						Optional:  true,
						Sensitive: true,
					},
				},
			},
			addr: addrs.AbsResourceInstance{
				Module: nil,
				Resource: addrs.ResourceInstance{
					Resource: addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "tfcoremock_simple_resource",
						Name: "example",
					},
					Key: nil,
				},
			},
			provider: addrs.LocalProviderConfig{
				LocalName: "tfcoremock",
			},
			value: cty.ObjectVal(map[string]cty.Value{
				"juststr":          cty.StringVal("{a=b}"),
				"jsonobj":          cty.StringVal(`{"SomeDate":"2012-10-17"}`),
				"jsonarr":          cty.StringVal(`[{"a": 1}]`),
				"sensitivejsonobj": cty.StringVal(`{"SomePassword":"dontstealplease"}`),
				"secrets": cty.ObjectVal(map[string]cty.Value{
					"main":      cty.StringVal(`{"v":"mypass"}`),
					"secondary": cty.StringVal(`{"v":"mybackup"}`),
				}),
			}),
			expected: `
resource "tfcoremock_simple_resource" "example" {
  jsonarr = jsonencode([{
    a = 1
  }])
  jsonobj = jsonencode({
    SomeDate = "2012-10-17"
  })
  juststr          = "{a=b}"
  secrets          = null # sensitive
  sensitivejsonobj = null # sensitive
}`,
		},
		"optional_empty_sensitive_string": {
			schema: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"str": {
						Type:      cty.String,
						Optional:  true,
						Sensitive: true,
					},
				},
			},
			addr: addrs.AbsResourceInstance{
				Module: nil,
				Resource: addrs.ResourceInstance{
					Resource: addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "tfcoremock_simple_resource",
						Name: "example",
					},
					Key: nil,
				},
			},
			provider: addrs.LocalProviderConfig{
				LocalName: "tfcoremock",
			},
			value: cty.ObjectVal(map[string]cty.Value{
				"str": cty.StringVal("").Mark(marks.Sensitive),
			}),
			expected: `
resource "tfcoremock_simple_resource" "example" {
  str = null # sensitive
}`,
		},
		"simple_resource_with_all_sensitive_required_values": {
			// By having all the values being sensitive and required,
			// they should be output as null with a comment
			// indicating that they are sensitive.
			schema: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"sensitive_string": sensitiveVal(cty.String, true, false, false),
					"sensitive_number": sensitiveVal(cty.Number, true, false, false),
					"sensitive_bool":   sensitiveVal(cty.Bool, true, false, false),
					"sensitive_list":   sensitiveVal(cty.List(cty.String), true, false, false),
					"sensitive_map":    sensitiveVal(cty.Map(cty.String), true, false, false),
					"sensitive_object": sensitiveVal(cty.Object(map[string]cty.Type{}), true, false, false),
				},
			},
			addr: addrs.AbsResourceInstance{
				Module: nil,
				Resource: addrs.ResourceInstance{
					Resource: addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "tfcoremock_simple_resource",
						Name: "example",
					},
					Key: nil,
				},
			},
			provider: addrs.LocalProviderConfig{
				LocalName: "tfcoremock",
			},
			value: cty.ObjectVal(map[string]cty.Value{
				"sensitive_string": cty.StringVal("sensitive").Mark(marks.Sensitive),
				"sensitive_number": cty.NumberIntVal(42).Mark(marks.Sensitive),
				"sensitive_bool":   cty.True.Mark(marks.Sensitive),
				"sensitive_list":   cty.ListVal([]cty.Value{cty.StringVal("sensitive")}).Mark(marks.Sensitive),
				"sensitive_map":    cty.MapVal(map[string]cty.Value{"key": cty.StringVal("sensitive")}).Mark(marks.Sensitive),
				"sensitive_object": cty.ObjectVal(map[string]cty.Value{}).Mark(marks.Sensitive),
			}),
			expected: `
resource "tfcoremock_simple_resource" "example" {
  sensitive_bool   = null # sensitive
  sensitive_list   = null # sensitive
  sensitive_map    = null # sensitive
  sensitive_number = null # sensitive
  sensitive_object = null # sensitive
  sensitive_string = null # sensitive
}`,
		},
		"simple_resource_with_all_sensitive_optional_values": {
			// By having all the values being sensitive and optional,
			// they should be output as null with a comment
			// indicating that they are sensitive.

			schema: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"sensitive_string": sensitiveVal(cty.String, false, true, false),
					"sensitive_number": sensitiveVal(cty.Number, false, true, false),
					"sensitive_bool":   sensitiveVal(cty.Bool, false, true, false),
					"sensitive_list":   sensitiveVal(cty.List(cty.String), false, true, false),
					"sensitive_map":    sensitiveVal(cty.Map(cty.String), false, true, false),
					"sensitive_object": sensitiveVal(cty.Object(map[string]cty.Type{}), false, true, false),
				},
			},
			addr: addrs.AbsResourceInstance{
				Module: nil,
				Resource: addrs.ResourceInstance{
					Resource: addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "tfcoremock_simple_resource",
						Name: "example",
					},
					Key: nil,
				},
			},
			provider: addrs.LocalProviderConfig{
				LocalName: "tfcoremock",
			},
			value: cty.ObjectVal(map[string]cty.Value{
				"sensitive_string": cty.StringVal("sensitive").Mark(marks.Sensitive),
				"sensitive_number": cty.NumberIntVal(42).Mark(marks.Sensitive),
				"sensitive_bool":   cty.True.Mark(marks.Sensitive),
				"sensitive_list":   cty.ListVal([]cty.Value{cty.StringVal("sensitive")}).Mark(marks.Sensitive),
				"sensitive_map":    cty.MapVal(map[string]cty.Value{"key": cty.StringVal("sensitive")}).Mark(marks.Sensitive),
				"sensitive_object": cty.ObjectVal(map[string]cty.Value{}).Mark(marks.Sensitive),
			}),
			expected: `
resource "tfcoremock_simple_resource" "example" {
  sensitive_bool   = null # sensitive
  sensitive_list   = null # sensitive
  sensitive_map    = null # sensitive
  sensitive_number = null # sensitive
  sensitive_object = null # sensitive
  sensitive_string = null # sensitive
}`,
		},
		"simple_resource_with_all_sensitive_computed_values": {
			// By having all the values being sensitive and computed,
			// they should be omitted from the output because we are not aware of the
			// actual values.
			schema: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"sensitive_string": sensitiveVal(cty.String, false, false, true),
					"sensitive_number": sensitiveVal(cty.Number, false, false, true),
					"sensitive_bool":   sensitiveVal(cty.Bool, false, false, true),
					"sensitive_list":   sensitiveVal(cty.List(cty.String), false, false, true),
					"sensitive_map":    sensitiveVal(cty.Map(cty.String), false, false, true),
					"sensitive_object": sensitiveVal(cty.Object(map[string]cty.Type{}), false, false, true),
				},
			},
			addr: addrs.AbsResourceInstance{
				Module: nil,
				Resource: addrs.ResourceInstance{
					Resource: addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "tfcoremock_simple_resource",
						Name: "example",
					},
					Key: nil,
				},
			},
			provider: addrs.LocalProviderConfig{
				LocalName: "tfcoremock",
			},
			value: cty.ObjectVal(map[string]cty.Value{
				"sensitive_string": cty.StringVal("sensitive").Mark(marks.Sensitive),
				"sensitive_number": cty.NumberIntVal(42).Mark(marks.Sensitive),
				"sensitive_bool":   cty.True.Mark(marks.Sensitive),
				"sensitive_list":   cty.ListVal([]cty.Value{cty.StringVal("sensitive")}).Mark(marks.Sensitive),
				"sensitive_map":    cty.MapVal(map[string]cty.Value{"key": cty.StringVal("sensitive")}).Mark(marks.Sensitive),
				"sensitive_object": cty.ObjectVal(map[string]cty.Value{}).Mark(marks.Sensitive),
			}),
			expected: `
resource "tfcoremock_simple_resource" "example" {
}`,
		},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			err := tc.schema.InternalValidate()
			if err != nil {
				t.Fatalf("schema failed InternalValidate: %s", err)
			}
			contents, diags := GenerateResourceContents(tc.addr, tc.schema, tc.provider, tc.value)
			if len(diags) > 0 {
				t.Errorf("expected no diagnostics but found %s", diags)
			}

			got := WrapResourceContents(tc.addr, contents)
			want := strings.TrimSpace(tc.expected)
			if diff := cmp.Diff(got, want); len(diff) > 0 {
				t.Errorf("got:\n%s\nwant:\n%s\ndiff:\n%s", got, want, diff)
			}
		})
	}
}
