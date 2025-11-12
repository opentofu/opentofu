// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package funcs

import (
	"reflect"
	"testing"

	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/zclconf/go-cty/cty"
)

func TestEphemeralAsNullFunc(t *testing.T) {
	// This is a bit overkill, but it is better to be explicit
	cases := []struct {
		Name     string
		Input    cty.Value
		Expected cty.Value
	}{
		{"null",
			cty.NullVal(cty.String),
			cty.NullVal(cty.String)},
		{"null_sensitive",
			cty.NullVal(cty.String).Mark(marks.Sensitive),
			cty.NullVal(cty.String).Mark(marks.Sensitive)},
		{"null_ephemeral",
			cty.NullVal(cty.String).Mark(marks.Ephemeral),
			cty.NullVal(cty.String)},
		{"null_complex",
			cty.NullVal(cty.String).Mark(marks.Ephemeral).Mark(marks.Sensitive),
			cty.NullVal(cty.String).Mark(marks.Sensitive)},
		{"unknown",
			cty.UnknownVal(cty.String),
			cty.UnknownVal(cty.String)},
		{"unknown_sensitive",
			cty.UnknownVal(cty.String).Mark(marks.Sensitive),
			cty.UnknownVal(cty.String).Mark(marks.Sensitive)},
		{"unknown_ephemeral",
			cty.UnknownVal(cty.String).Mark(marks.Ephemeral),
			cty.UnknownVal(cty.String)},
		{"unknown_complex",
			cty.UnknownVal(cty.String).Mark(marks.Ephemeral).Mark(marks.Sensitive),
			cty.UnknownVal(cty.String).Mark(marks.Sensitive)},
		{"primitive",
			cty.StringVal("myprimitive"),
			cty.StringVal("myprimitive")},
		{"primitive_sensitive",
			cty.StringVal("mysensitive").Mark(marks.Sensitive),
			cty.StringVal("mysensitive").Mark(marks.Sensitive)},
		{"primitive_ephemeral",
			cty.StringVal("myephemeral").Mark(marks.Ephemeral),
			cty.NullVal(cty.String)},
		{"list",
			cty.ListVal([]cty.Value{cty.StringVal("val")}),
			cty.ListVal([]cty.Value{cty.StringVal("val")})},
		{"list_sensitive",
			cty.ListVal([]cty.Value{cty.StringVal("val")}).Mark(marks.Sensitive),
			cty.ListVal([]cty.Value{cty.StringVal("val")}).Mark(marks.Sensitive)},
		{"list_ephemeral",
			cty.ListVal([]cty.Value{cty.StringVal("val")}).Mark(marks.Ephemeral),
			cty.NullVal(cty.List(cty.String))},
		{"list_complex",
			cty.ListVal([]cty.Value{cty.StringVal("val")}).Mark(marks.Ephemeral).Mark(marks.Sensitive),
			cty.NullVal(cty.List(cty.String)).Mark(marks.Sensitive)},
		{"listcontents_sensitive",
			cty.ListVal([]cty.Value{cty.StringVal("val").Mark(marks.Sensitive)}),
			cty.ListVal([]cty.Value{cty.StringVal("val").Mark(marks.Sensitive)})},
		{"listcontents_ephemeral",
			cty.ListVal([]cty.Value{cty.StringVal("val").Mark(marks.Ephemeral)}),
			cty.ListVal([]cty.Value{cty.NullVal(cty.String)})},
		{"listcontents_complex",
			cty.ListVal([]cty.Value{cty.StringVal("val").Mark(marks.Ephemeral).Mark(marks.Sensitive)}),
			cty.ListVal([]cty.Value{cty.NullVal(cty.String).Mark(marks.Sensitive)})},
		{"listcontents_multiple",
			cty.ListVal([]cty.Value{cty.StringVal("val").Mark(marks.Ephemeral).Mark(marks.Sensitive), cty.StringVal("other")}),
			cty.ListVal([]cty.Value{cty.NullVal(cty.String).Mark(marks.Sensitive), cty.StringVal("other")})},
		{"listempty",
			cty.ListValEmpty(cty.String),
			cty.ListValEmpty(cty.String)},
		{"listempty_sensitive",
			cty.ListValEmpty(cty.String).Mark(marks.Sensitive),
			cty.ListValEmpty(cty.String).Mark(marks.Sensitive)},
		{"listempty_ephemeral",
			cty.ListValEmpty(cty.String).Mark(marks.Ephemeral),
			cty.NullVal(cty.List(cty.String))},
		{"listempty_complex",
			cty.ListValEmpty(cty.String).Mark(marks.Ephemeral).Mark(marks.Sensitive),
			cty.NullVal(cty.List(cty.String)).Mark(marks.Sensitive)},
		{"map",
			cty.MapVal(map[string]cty.Value{"key": cty.StringVal("val")}),
			cty.MapVal(map[string]cty.Value{"key": cty.StringVal("val")})},
		{"map_sensitive",
			cty.MapVal(map[string]cty.Value{"key": cty.StringVal("val")}).Mark(marks.Sensitive),
			cty.MapVal(map[string]cty.Value{"key": cty.StringVal("val")}).Mark(marks.Sensitive)},
		{"map_ephemeral",
			cty.MapVal(map[string]cty.Value{"key": cty.StringVal("val")}).Mark(marks.Ephemeral),
			cty.NullVal(cty.Map(cty.String))},
		{"map_complex",
			cty.MapVal(map[string]cty.Value{"key": cty.StringVal("val")}).Mark(marks.Ephemeral).Mark(marks.Sensitive),
			cty.NullVal(cty.Map(cty.String)).Mark(marks.Sensitive)},
		{"mapcontents_sensitive",
			cty.MapVal(map[string]cty.Value{"key": cty.StringVal("val").Mark(marks.Sensitive)}),
			cty.MapVal(map[string]cty.Value{"key": cty.StringVal("val").Mark(marks.Sensitive)})},
		{"mapcontents_ephemeral",
			cty.MapVal(map[string]cty.Value{"key": cty.StringVal("val").Mark(marks.Ephemeral)}),
			cty.MapVal(map[string]cty.Value{"key": cty.NullVal(cty.String)})},
		{"mapcontents_complex",
			cty.MapVal(map[string]cty.Value{"key": cty.StringVal("val").Mark(marks.Ephemeral).Mark(marks.Sensitive)}),
			cty.MapVal(map[string]cty.Value{"key": cty.NullVal(cty.String).Mark(marks.Sensitive)})},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			out, err := EphemeralAsNullFunc.Call([]cty.Value{tc.Input})
			if err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(tc.Expected, out) {
				t.Fatalf("Expected %#v, Got %#v", tc.Expected, out)
			}
		})
	}
}
