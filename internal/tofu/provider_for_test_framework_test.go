// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/zclconf/go-cty/cty"
)

func TestProviderForTest_ReadResource(t *testing.T) {
	mockProvider := &MockProvider{}

	provider, err := newProviderForTestWithSchema(mockProvider, mockProvider.GetProviderSchema(t.Context()), nil)
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	resp := provider.ReadResource(t.Context(), providers.ReadResourceRequest{
		TypeName: "test",
		Private:  []byte{},
	})

	if !resp.Diagnostics.HasErrors() {
		t.Fatalf("expected errors but none were found")
	}

	errMsg := resp.Diagnostics[0].Description().Summary
	if !strings.Contains(errMsg, "Unexpected null value for prior state") {
		t.Fatalf("expected prior state not found error but got: %s", errMsg)
	}
}

func TestFilterComputedOnlyAttributes(t *testing.T) {
	tests := []struct {
		name     string
		schema   *configschema.Block
		value    cty.Value
		expected cty.Value
	}{
		{
			name:     "nil schema returns value unchanged",
			schema:   nil,
			value:    cty.ObjectVal(map[string]cty.Value{"foo": cty.StringVal("bar")}),
			expected: cty.ObjectVal(map[string]cty.Value{"foo": cty.StringVal("bar")}),
		},
		{
			name:     "null value returns null",
			schema:   &configschema.Block{},
			value:    cty.NullVal(cty.Object(map[string]cty.Type{"foo": cty.String})),
			expected: cty.NullVal(cty.Object(map[string]cty.Type{"foo": cty.String})),
		},
		{
			name:     "unknown value returns unknown",
			schema:   &configschema.Block{},
			value:    cty.UnknownVal(cty.Object(map[string]cty.Type{"foo": cty.String})),
			expected: cty.UnknownVal(cty.Object(map[string]cty.Type{"foo": cty.String})),
		},
		{
			name: "computed-only attribute is nulled out",
			schema: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"computed_only": {Computed: true, Optional: false, Type: cty.String},
					"normal":        {Optional: true, Type: cty.String},
				},
			},
			value: cty.ObjectVal(map[string]cty.Value{
				"computed_only": cty.StringVal("should be nulled"),
				"normal":        cty.StringVal("keep me"),
			}),
			expected: cty.ObjectVal(map[string]cty.Value{
				"computed_only": cty.NullVal(cty.String),
				"normal":        cty.StringVal("keep me"),
			}),
		},
		{
			name: "optional+computed attribute is NOT nulled",
			schema: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"optional_computed": {Computed: true, Optional: true, Type: cty.String},
				},
			},
			value: cty.ObjectVal(map[string]cty.Value{
				"optional_computed": cty.StringVal("keep me"),
			}),
			expected: cty.ObjectVal(map[string]cty.Value{
				"optional_computed": cty.StringVal("keep me"),
			}),
		},
		{
			name: "required attribute is NOT nulled",
			schema: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"required": {Required: true, Type: cty.String},
				},
			},
			value: cty.ObjectVal(map[string]cty.Value{
				"required": cty.StringVal("keep me"),
			}),
			expected: cty.ObjectVal(map[string]cty.Value{
				"required": cty.StringVal("keep me"),
			}),
		},
		{
			name: "nested block with computed-only attribute is nulled",
			schema: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"name": {Optional: true, Type: cty.String},
				},
				BlockTypes: map[string]*configschema.NestedBlock{
					"nested": {
						Nesting: configschema.NestingList,
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"id":             {Computed: true, Optional: false, Type: cty.String},
								"user_specified": {Optional: true, Type: cty.String},
							},
						},
					},
				},
			},
			value: cty.ObjectVal(map[string]cty.Value{
				"name": cty.StringVal("test"),
				"nested": cty.ListVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"id":             cty.StringVal("computed-id"),
						"user_specified": cty.StringVal("user-value"),
					}),
				}),
			}),
			expected: cty.ObjectVal(map[string]cty.Value{
				"name": cty.StringVal("test"),
				"nested": cty.ListVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"id":             cty.NullVal(cty.String),
						"user_specified": cty.StringVal("user-value"),
					}),
				}),
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterComputedOnlyAttributes(tt.schema, tt.value)
			if !result.RawEquals(tt.expected) {
				t.Errorf("filterComputedOnlyAttributes() = %v, want %v", result.GoString(), tt.expected.GoString())
			}
		})
	}
}
