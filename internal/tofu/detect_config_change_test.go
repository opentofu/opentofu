// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"testing"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/configs/configschema"
)

func TestValuesEqualIgnoringNulls(t *testing.T) {
	schema := &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"name": {Type: cty.String, Optional: true},
			"tags": {Type: cty.Map(cty.String), Optional: true},
		},
	}

	tests := []struct {
		name   string
		a      cty.Value
		b      cty.Value
		schema *configschema.Block
		want   bool
	}{
		{
			name: "both null",
			a:    cty.NullVal(cty.String),
			b:    cty.NullVal(cty.String),
			want: true,
		},
		{
			name: "equal strings",
			a:    cty.StringVal("hello"),
			b:    cty.StringVal("hello"),
			want: true,
		},
		{
			name: "different strings",
			a:    cty.StringVal("hello"),
			b:    cty.StringVal("world"),
			want: false,
		},
		{
			name: "null vs empty object",
			a:    cty.NullVal(cty.EmptyObject),
			b:    cty.EmptyObjectVal,
			want: true,
		},
		{
			name: "null vs empty map",
			a:    cty.NullVal(cty.Map(cty.String)),
			b:    cty.MapValEmpty(cty.String),
			want: true,
		},
		{
			name: "null vs non-empty map",
			a:    cty.NullVal(cty.Map(cty.String)),
			b:    cty.MapVal(map[string]cty.Value{"k": cty.StringVal("v")}),
			want: false,
		},
		{
			name: "equal objects",
			a: cty.ObjectVal(map[string]cty.Value{
				"name": cty.StringVal("foo"),
				"tags": cty.MapVal(map[string]cty.Value{"env": cty.StringVal("test")}),
			}),
			b: cty.ObjectVal(map[string]cty.Value{
				"name": cty.StringVal("foo"),
				"tags": cty.MapVal(map[string]cty.Value{"env": cty.StringVal("test")}),
			}),
			schema: schema,
			want:   true,
		},
		{
			name: "different object attribute",
			a: cty.ObjectVal(map[string]cty.Value{
				"name": cty.StringVal("foo"),
				"tags": cty.NullVal(cty.Map(cty.String)),
			}),
			b: cty.ObjectVal(map[string]cty.Value{
				"name": cty.StringVal("bar"),
				"tags": cty.NullVal(cty.Map(cty.String)),
			}),
			schema: schema,
			want:   false,
		},
		{
			name: "object with null vs empty map attribute",
			a: cty.ObjectVal(map[string]cty.Value{
				"name": cty.StringVal("foo"),
				"tags": cty.NullVal(cty.Map(cty.String)),
			}),
			b: cty.ObjectVal(map[string]cty.Value{
				"name": cty.StringVal("foo"),
				"tags": cty.MapValEmpty(cty.String),
			}),
			schema: schema,
			want:   true,
		},
		{
			name:   "equal lists",
			a:      cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}),
			b:      cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}),
			schema: nil,
			want:   true,
		},
		{
			name:   "different lists",
			a:      cty.ListVal([]cty.Value{cty.StringVal("a")}),
			b:      cty.ListVal([]cty.Value{cty.StringVal("b")}),
			schema: nil,
			want:   false,
		},
		{
			name:   "different list lengths",
			a:      cty.ListVal([]cty.Value{cty.StringVal("a")}),
			b:      cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}),
			schema: nil,
			want:   false,
		},
		{
			name: "equal maps",
			a:    cty.MapVal(map[string]cty.Value{"k1": cty.StringVal("v1")}),
			b:    cty.MapVal(map[string]cty.Value{"k1": cty.StringVal("v1")}),
			want: true,
		},
		{
			name: "different map values",
			a:    cty.MapVal(map[string]cty.Value{"k1": cty.StringVal("v1")}),
			b:    cty.MapVal(map[string]cty.Value{"k1": cty.StringVal("v2")}),
			want: false,
		},
		{
			name: "different map keys",
			a:    cty.MapVal(map[string]cty.Value{"k1": cty.StringVal("v1")}),
			b:    cty.MapVal(map[string]cty.Value{"k2": cty.StringVal("v1")}),
			want: false,
		},
		{
			name: "different types",
			a:    cty.StringVal("hello"),
			b:    cty.NumberIntVal(42),
			want: false,
		},
		{
			name: "equal booleans",
			a:    cty.True,
			b:    cty.True,
			want: true,
		},
		{
			name: "different booleans",
			a:    cty.True,
			b:    cty.False,
			want: false,
		},
		{
			name: "equal numbers",
			a:    cty.NumberIntVal(42),
			b:    cty.NumberIntVal(42),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := valuesEqualIgnoringNulls(tt.a, tt.b, tt.schema)
			if got != tt.want {
				t.Errorf("valuesEqualIgnoringNulls() = %v, want %v", got, tt.want)
			}
		})
	}
}
