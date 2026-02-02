// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"testing"

	"github.com/opentofu/opentofu/internal/tofu/testhelpers"
	"github.com/opentofu/opentofu/internal/tofu/variables"
	"github.com/zclconf/go-cty/cty"
)

func TestCheckInputVariables(t *testing.T) {
	c := testhelpers.TestModule(t, "input-variables")

	t.Run("No variables set", func(t *testing.T) {
		// No variables set
		diags := checkInputVariables(c.Module.Variables, nil)
		if !diags.HasErrors() {
			t.Fatal("check succeeded, but want errors")
		}

		// Required variables set, optional variables unset
		// This is still an error at this layer, since it's the caller's
		// responsibility to have already merged in any default values.
		diags = checkInputVariables(c.Module.Variables, variables.InputValues{
			"foo": &variables.InputValue{
				Value:      cty.StringVal("bar"),
				SourceType: variables.ValueFromCLIArg,
			},
		})
		if !diags.HasErrors() {
			t.Fatal("check succeeded, but want errors")
		}
	})

	t.Run("All variables set", func(t *testing.T) {
		diags := checkInputVariables(c.Module.Variables, variables.InputValues{
			"foo": &variables.InputValue{
				Value:      cty.StringVal("bar"),
				SourceType: variables.ValueFromCLIArg,
			},
			"bar": &variables.InputValue{
				Value:      cty.StringVal("baz"),
				SourceType: variables.ValueFromCLIArg,
			},
			"map": &variables.InputValue{
				Value:      cty.StringVal("baz"), // okay because config has no type constraint
				SourceType: variables.ValueFromCLIArg,
			},
			"object_map": &variables.InputValue{
				Value: cty.MapVal(map[string]cty.Value{
					"uno": cty.ObjectVal(map[string]cty.Value{
						"foo": cty.StringVal("baz"),
						"bar": cty.NumberIntVal(2), // type = any
					}),
					"dos": cty.ObjectVal(map[string]cty.Value{
						"foo": cty.StringVal("bat"),
						"bar": cty.NumberIntVal(99), // type = any
					}),
				}),
				SourceType: variables.ValueFromCLIArg,
			},
			"object_list": &variables.InputValue{
				Value: cty.ListVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"foo": cty.StringVal("baz"),
						"bar": cty.NumberIntVal(2), // type = any
					}),
					cty.ObjectVal(map[string]cty.Value{
						"foo": cty.StringVal("bang"),
						"bar": cty.NumberIntVal(42), // type = any
					}),
				}),
				SourceType: variables.ValueFromCLIArg,
			},
		})
		if diags.HasErrors() {
			t.Fatalf("unexpected errors: %s", diags.Err())
		}
	})

	t.Run("Invalid Complex Types", func(t *testing.T) {
		diags := checkInputVariables(c.Module.Variables, variables.InputValues{
			"foo": &variables.InputValue{
				Value:      cty.StringVal("bar"),
				SourceType: variables.ValueFromCLIArg,
			},
			"bar": &variables.InputValue{
				Value:      cty.StringVal("baz"),
				SourceType: variables.ValueFromCLIArg,
			},
			"map": &variables.InputValue{
				Value:      cty.StringVal("baz"), // okay because config has no type constraint
				SourceType: variables.ValueFromCLIArg,
			},
			"object_map": &variables.InputValue{
				Value: cty.MapVal(map[string]cty.Value{
					"uno": cty.ObjectVal(map[string]cty.Value{
						"foo": cty.StringVal("baz"),
						"bar": cty.NumberIntVal(2), // type = any
					}),
					"dos": cty.ObjectVal(map[string]cty.Value{
						"foo": cty.StringVal("bat"),
						"bar": cty.NumberIntVal(99), // type = any
					}),
				}),
				SourceType: variables.ValueFromCLIArg,
			},
			"object_list": &variables.InputValue{
				Value: cty.TupleVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"foo": cty.StringVal("baz"),
						"bar": cty.NumberIntVal(2), // type = any
					}),
					cty.ObjectVal(map[string]cty.Value{
						"foo": cty.StringVal("bang"),
						"bar": cty.StringVal("42"), // type = any, but mismatch with the first list item
					}),
				}),
				SourceType: variables.ValueFromCLIArg,
			},
		})

		if diags.HasErrors() {
			t.Fatalf("unexpected errors: %s", diags.Err())
		}
	})
}
