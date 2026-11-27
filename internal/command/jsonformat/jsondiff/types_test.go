// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsondiff

import (
	"encoding/json"
	"testing"
)

func TestEqual(t *testing.T) {
	tests := []struct {
		a, b  interface{}
		equal bool
	}{
		{nil, nil, true},
		{nil, "x", false},
		{"x", "x", true},
		{"x", "y", false},
		{"x", json.Number("1"), false},
		{true, true, true},
		{true, false, false},
		{true, "true", false},
		{json.Number("1"), json.Number("1"), true},
		{json.Number("1"), json.Number("1.0"), false},
		{json.Number("1"), 1.0, false},
		{1.0, 1.0, true},
		{[]interface{}{}, []interface{}{}, true},
		{[]interface{}{json.Number("1")}, []interface{}{json.Number("1")}, true},
		{[]interface{}{json.Number("1")}, []interface{}{json.Number("2")}, false},
		{[]interface{}{json.Number("1")}, []interface{}{json.Number("1"), json.Number("2")}, false},
		{[]interface{}{json.Number("1")}, []interface{}{"1"}, false},
		{[]interface{}{}, map[string]interface{}{}, false},
		// A nil slice or map compares equal to an empty one, which
		// reflect.DeepEqual did not consider equal.
		{[]interface{}(nil), []interface{}{}, true},
		{map[string]interface{}(nil), map[string]interface{}{}, true},
		{[]interface{}{}, nil, false},
		// Types that JSON decoding never produces fall back to
		// reflect.DeepEqual.
		{struct{ A int }{1}, struct{ A int }{1}, true},
		{struct{ A int }{1}, struct{ A int }{2}, false},
		{
			map[string]interface{}{"a": json.Number("1"), "b": []interface{}{"x"}},
			map[string]interface{}{"b": []interface{}{"x"}, "a": json.Number("1")},
			true,
		},
		{
			map[string]interface{}{"a": json.Number("1")},
			map[string]interface{}{"a": json.Number("2")},
			false,
		},
		{
			map[string]interface{}{"a": json.Number("1")},
			map[string]interface{}{"a": json.Number("1"), "b": json.Number("2")},
			false,
		},
		{
			map[string]interface{}{"a": json.Number("1")},
			map[string]interface{}{"b": json.Number("1")},
			false,
		},
		{
			map[string]interface{}{"a": map[string]interface{}{"b": []interface{}{nil, false}}},
			map[string]interface{}{"a": map[string]interface{}{"b": []interface{}{nil, false}}},
			true,
		},
	}

	for _, test := range tests {
		if got := Equal(test.a, test.b); got != test.equal {
			t.Errorf("Equal(%#v, %#v) = %t, want %t", test.a, test.b, got, test.equal)
			// Equality must also be symmetric.
			if got := Equal(test.b, test.a); got != test.equal {
				t.Errorf("Equal(%#v, %#v) = %t, want %t", test.b, test.a, got, test.equal)
			}
		}
	}
}
