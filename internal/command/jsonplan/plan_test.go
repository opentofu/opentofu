// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsonplan

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/plans"
)

func TestOmitUnknowns(t *testing.T) {
	tests := []struct {
		Input cty.Value
		Want  cty.Value
	}{
		{
			cty.StringVal("hello"),
			cty.StringVal("hello"),
		},
		{
			cty.NullVal(cty.String),
			cty.NullVal(cty.String),
		},
		{
			cty.UnknownVal(cty.String),
			cty.NilVal,
		},
		{
			cty.ListValEmpty(cty.String),
			cty.EmptyTupleVal,
		},
		{
			cty.ListVal([]cty.Value{cty.StringVal("hello")}),
			cty.TupleVal([]cty.Value{cty.StringVal("hello")}),
		},
		{
			cty.ListVal([]cty.Value{cty.NullVal(cty.String)}),
			cty.TupleVal([]cty.Value{cty.NullVal(cty.String)}),
		},
		{
			cty.ListVal([]cty.Value{cty.UnknownVal(cty.String)}),
			cty.TupleVal([]cty.Value{cty.NullVal(cty.String)}),
		},
		{
			cty.ListVal([]cty.Value{cty.StringVal("hello")}),
			cty.TupleVal([]cty.Value{cty.StringVal("hello")}),
		},
		//
		{
			cty.ListVal([]cty.Value{
				cty.StringVal("hello"),
				cty.UnknownVal(cty.String)}),
			cty.TupleVal([]cty.Value{
				cty.StringVal("hello"),
				cty.NullVal(cty.String),
			}),
		},
		{
			cty.MapVal(map[string]cty.Value{
				"hello": cty.True,
				"world": cty.UnknownVal(cty.Bool),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"hello": cty.True,
			}),
		},
		{
			cty.TupleVal([]cty.Value{
				cty.StringVal("alpha"),
				cty.UnknownVal(cty.String),
				cty.StringVal("charlie"),
			}),
			cty.TupleVal([]cty.Value{
				cty.StringVal("alpha"),
				cty.NullVal(cty.String),
				cty.StringVal("charlie"),
			}),
		},
		{
			cty.SetVal([]cty.Value{
				cty.StringVal("dev"),
				cty.StringVal("foo"),
				cty.StringVal("stg"),
				cty.UnknownVal(cty.String),
			}),
			cty.TupleVal([]cty.Value{
				cty.StringVal("dev"),
				cty.StringVal("foo"),
				cty.StringVal("stg"),
				cty.NullVal(cty.String),
			}),
		},
		{
			cty.SetVal([]cty.Value{
				cty.ObjectVal(map[string]cty.Value{
					"a": cty.UnknownVal(cty.String),
				}),
				cty.ObjectVal(map[string]cty.Value{
					"a": cty.StringVal("known"),
				}),
			}),
			cty.TupleVal([]cty.Value{
				cty.ObjectVal(map[string]cty.Value{
					"a": cty.StringVal("known"),
				}),
				cty.EmptyObjectVal,
			}),
		},
	}

	for _, test := range tests {
		got := omitUnknowns(test.Input)
		if !reflect.DeepEqual(got, test.Want) {
			t.Errorf(
				"wrong result\ninput: %#v\ngot:   %#v\nwant:  %#v",
				test.Input, got, test.Want,
			)
		}
	}
}

func TestUnknownAsBool(t *testing.T) {
	tests := []struct {
		Input cty.Value
		Want  cty.Value
	}{
		{
			cty.StringVal("hello"),
			cty.False,
		},
		{
			cty.NullVal(cty.String),
			cty.False,
		},
		{
			cty.UnknownVal(cty.String),
			cty.True,
		},

		{
			cty.NullVal(cty.DynamicPseudoType),
			cty.False,
		},
		{
			cty.NullVal(cty.Object(map[string]cty.Type{"test": cty.String})),
			cty.False,
		},
		{
			cty.DynamicVal,
			cty.True,
		},

		{
			cty.ListValEmpty(cty.String),
			cty.EmptyTupleVal,
		},
		{
			cty.ListVal([]cty.Value{cty.StringVal("hello")}),
			cty.TupleVal([]cty.Value{cty.False}),
		},
		{
			cty.ListVal([]cty.Value{cty.NullVal(cty.String)}),
			cty.TupleVal([]cty.Value{cty.False}),
		},
		{
			cty.ListVal([]cty.Value{cty.UnknownVal(cty.String)}),
			cty.TupleVal([]cty.Value{cty.True}),
		},
		{
			cty.SetValEmpty(cty.String),
			cty.EmptyTupleVal,
		},
		{
			cty.SetVal([]cty.Value{cty.StringVal("hello")}),
			cty.TupleVal([]cty.Value{cty.False}),
		},
		{
			cty.SetVal([]cty.Value{cty.NullVal(cty.String)}),
			cty.TupleVal([]cty.Value{cty.False}),
		},
		{
			cty.SetVal([]cty.Value{cty.UnknownVal(cty.String)}),
			cty.TupleVal([]cty.Value{cty.True}),
		},
		{
			cty.EmptyTupleVal,
			cty.EmptyTupleVal,
		},
		{
			cty.TupleVal([]cty.Value{cty.StringVal("hello")}),
			cty.TupleVal([]cty.Value{cty.False}),
		},
		{
			cty.TupleVal([]cty.Value{cty.NullVal(cty.String)}),
			cty.TupleVal([]cty.Value{cty.False}),
		},
		{
			cty.TupleVal([]cty.Value{cty.UnknownVal(cty.String)}),
			cty.TupleVal([]cty.Value{cty.True}),
		},
		{
			cty.MapValEmpty(cty.String),
			cty.EmptyObjectVal,
		},
		{
			cty.MapVal(map[string]cty.Value{"greeting": cty.StringVal("hello")}),
			cty.EmptyObjectVal,
		},
		{
			cty.MapVal(map[string]cty.Value{"greeting": cty.NullVal(cty.String)}),
			cty.EmptyObjectVal,
		},
		{
			cty.MapVal(map[string]cty.Value{"greeting": cty.UnknownVal(cty.String)}),
			cty.ObjectVal(map[string]cty.Value{"greeting": cty.True}),
		},
		{
			cty.EmptyObjectVal,
			cty.EmptyObjectVal,
		},
		{
			cty.ObjectVal(map[string]cty.Value{"greeting": cty.StringVal("hello")}),
			cty.EmptyObjectVal,
		},
		{
			cty.ObjectVal(map[string]cty.Value{"greeting": cty.NullVal(cty.String)}),
			cty.EmptyObjectVal,
		},
		{
			cty.ObjectVal(map[string]cty.Value{"greeting": cty.UnknownVal(cty.String)}),
			cty.ObjectVal(map[string]cty.Value{"greeting": cty.True}),
		},
		{
			cty.SetVal([]cty.Value{
				cty.ObjectVal(map[string]cty.Value{
					"a": cty.UnknownVal(cty.String),
				}),
				cty.ObjectVal(map[string]cty.Value{
					"a": cty.StringVal("known"),
				}),
			}),
			cty.TupleVal([]cty.Value{
				cty.EmptyObjectVal,
				cty.ObjectVal(map[string]cty.Value{
					"a": cty.True,
				}),
			}),
		},
		{
			cty.SetVal([]cty.Value{
				cty.MapValEmpty(cty.String),
				cty.MapVal(map[string]cty.Value{
					"a": cty.StringVal("known"),
				}),
				cty.MapVal(map[string]cty.Value{
					"a": cty.UnknownVal(cty.String),
				}),
			}),
			cty.TupleVal([]cty.Value{
				cty.EmptyObjectVal,
				cty.ObjectVal(map[string]cty.Value{
					"a": cty.True,
				}),
				cty.EmptyObjectVal,
			}),
		},
	}

	for _, test := range tests {
		got := unknownAsBool(test.Input)
		if !reflect.DeepEqual(got, test.Want) {
			t.Errorf(
				"wrong result\ninput: %#v\ngot:   %#v\nwant:  %#v",
				test.Input, got, test.Want,
			)
		}
	}
}

func TestEncodePaths(t *testing.T) {
	tests := map[string]struct {
		Input cty.PathSet
		Want  json.RawMessage
	}{
		"empty set": {
			cty.NewPathSet(),
			json.RawMessage(nil),
		},
		"index path with string and int steps": {
			cty.NewPathSet(cty.IndexStringPath("boop").IndexInt(0)),
			json.RawMessage(`[["boop",0]]`),
		},
		"get attr path with one step": {
			cty.NewPathSet(cty.GetAttrPath("triggers")),
			json.RawMessage(`[["triggers"]]`),
		},
		"multiple paths of different types": {
			// The order of the path sets is not guaranteed, so we sort the
			// result by the number of elements in the path to make the test deterministic.
			cty.NewPathSet(
				cty.GetAttrPath("alpha").GetAttr("beta"),                            // 2 elements
				cty.GetAttrPath("triggers").IndexString("name").IndexString("test"), // 3 elements
				cty.IndexIntPath(0).IndexInt(1).IndexInt(2).IndexInt(3),             // 4 elements
			),
			json.RawMessage(`[[0,1,2,3],["alpha","beta"],["triggers","name","test"]]`),
		},
	}

	// comp is a custom comparator for comparing JSON arrays. It sorts the
	// arrays based on the number of elements in each path before comparing them.
	// this allows our test cases to be more flexible about the order of the
	// paths in the result. and deterministic on both 32 and 64 bit architectures.
	comp := func(a, b json.RawMessage) (bool, error) {
		if a == nil && b == nil {
			return true, nil // Both are nil, they are equal
		}
		if a == nil || b == nil {
			return false, nil // One is nil and the other is not, they are not equal
		}

		var pathsA, pathsB [][]interface{}
		err := json.Unmarshal(a, &pathsA)
		if err != nil {
			return false, fmt.Errorf("error unmarshalling first argument: %w", err)
		}
		err = json.Unmarshal(b, &pathsB)
		if err != nil {
			return false, fmt.Errorf("error unmarshalling second argument: %w", err)
		}

		// Sort the slices based on the number of elements in each path
		sort.Slice(pathsA, func(i, j int) bool {
			return len(pathsA[i]) < len(pathsA[j])
		})
		sort.Slice(pathsB, func(i, j int) bool {
			return len(pathsB[i]) < len(pathsB[j])
		})

		return cmp.Equal(pathsA, pathsB), nil
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := encodePaths(test.Input)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			equal, err := comp(got, test.Want)
			if err != nil {
				t.Fatalf("error comparing JSON slices: %s", err)
			}
			if !equal {
				t.Errorf("paths do not match:\n%s", cmp.Diff(got, test.Want))
			}
		})
	}
}

func TestOutputs(t *testing.T) {
	root := addrs.RootModuleInstance

	child, diags := addrs.ParseModuleInstanceStr("module.child")
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	tests := map[string]struct {
		changes  *plans.Changes
		expected map[string]Change
	}{
		"copies all outputs": {
			changes: &plans.Changes{
				Outputs: []*plans.OutputChangeSrc{
					{
						Addr: root.OutputValue("first"),
						ChangeSrc: plans.ChangeSrc{
							Action: plans.Create,
						},
					},
					{
						Addr: root.OutputValue("second"),
						ChangeSrc: plans.ChangeSrc{
							Action: plans.Create,
						},
					},
				},
			},
			expected: map[string]Change{
				"first": {
					Actions:         []string{"create"},
					Before:          json.RawMessage("null"),
					After:           json.RawMessage("null"),
					AfterUnknown:    json.RawMessage("false"),
					BeforeSensitive: json.RawMessage("false"),
					AfterSensitive:  json.RawMessage("false"),
				},
				"second": {
					Actions:         []string{"create"},
					Before:          json.RawMessage("null"),
					After:           json.RawMessage("null"),
					AfterUnknown:    json.RawMessage("false"),
					BeforeSensitive: json.RawMessage("false"),
					AfterSensitive:  json.RawMessage("false"),
				},
			},
		},
		"skips non root modules": {
			changes: &plans.Changes{
				Outputs: []*plans.OutputChangeSrc{
					{
						Addr: root.OutputValue("first"),
						ChangeSrc: plans.ChangeSrc{
							Action: plans.Create,
						},
					},
					{
						Addr: child.OutputValue("second"),
						ChangeSrc: plans.ChangeSrc{
							Action: plans.Create,
						},
					},
				},
			},
			expected: map[string]Change{
				"first": {
					Actions:         []string{"create"},
					Before:          json.RawMessage("null"),
					After:           json.RawMessage("null"),
					AfterUnknown:    json.RawMessage("false"),
					BeforeSensitive: json.RawMessage("false"),
					AfterSensitive:  json.RawMessage("false"),
				},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			changes, err := MarshalOutputChanges(test.changes)
			if err != nil {
				t.Fatalf("unexpected err: %s", err)
			}

			if !cmp.Equal(changes, test.expected) {
				t.Errorf("wrong result:\n %v\n", cmp.Diff(changes, test.expected))
			}
		})
	}
}

func deepObjectValue(depth int) cty.Value {
	v := cty.ObjectVal(map[string]cty.Value{
		"a": cty.StringVal("a"),
		"b": cty.NumberIntVal(2),
		"c": cty.True,
		"d": cty.UnknownVal(cty.String),
	})

	result := v

	for i := 0; i < depth; i++ {
		result = cty.ObjectVal(map[string]cty.Value{
			"a": result,
			"b": result,
			"c": result,
		})
	}

	return result
}

func BenchmarkUnknownAsBool_2(b *testing.B) {
	value := deepObjectValue(2)
	for n := 0; n < b.N; n++ {
		unknownAsBool(value)
	}
}

func BenchmarkUnknownAsBool_3(b *testing.B) {
	value := deepObjectValue(3)
	for n := 0; n < b.N; n++ {
		unknownAsBool(value)
	}
}

func BenchmarkUnknownAsBool_5(b *testing.B) {
	value := deepObjectValue(5)
	for n := 0; n < b.N; n++ {
		unknownAsBool(value)
	}
}

func BenchmarkUnknownAsBool_7(b *testing.B) {
	value := deepObjectValue(7)
	for n := 0; n < b.N; n++ {
		unknownAsBool(value)
	}
}

func BenchmarkUnknownAsBool_9(b *testing.B) {
	value := deepObjectValue(9)
	for n := 0; n < b.N; n++ {
		unknownAsBool(value)
	}
}

// TestGenerateChange covers sensitivity tests for GenerateChange.
// TestOutputs test cases covered by outputs, but since is invalid to
// have outputs with sensitivity on the root module, we're creating this test
// to cover the remaining edge cases.
func TestGenerateChange(t *testing.T) {
	tests := map[string]struct {
		val1     cty.Value
		val2     cty.Value
		expected *Change
	}{
		"basic change": {
			val1: cty.StringVal("test0"),
			val2: cty.StringVal("test1"),
			expected: &Change{
				Before:          json.RawMessage("\"test0\""),
				After:           json.RawMessage("\"test1\""),
				AfterUnknown:    json.RawMessage("false"),
				BeforeSensitive: json.RawMessage("false"),
				AfterSensitive:  json.RawMessage("false"),
			},
		},
		"handles sensitivity": {
			val1: cty.NumberIntVal(3).Mark(marks.Sensitive),
			val2: cty.NumberIntVal(5).Mark(marks.Sensitive),
			expected: &Change{
				Before:          json.RawMessage("3"),
				After:           json.RawMessage("5"),
				AfterUnknown:    json.RawMessage("false"),
				BeforeSensitive: json.RawMessage("true"),
				AfterSensitive:  json.RawMessage("true"),
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			change, err := GenerateChange(test.val1, test.val2)
			if err != nil {
				t.Fatalf("unexpected err: %s", err)
			}

			if !cmp.Equal(change, test.expected) {
				t.Errorf("wrong result:\n %v\n", cmp.Diff(change, test.expected))
			}
		})
	}
}
