// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plans

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty/cty"
	ctymsgpack "github.com/zclconf/go-cty/cty/msgpack"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
)

func TestProviderAddrs(t *testing.T) {

	plan := &Plan{
		VariableValues: map[string]DynamicValue{},
		Changes: &Changes{
			Resources: []*ResourceInstanceChangeSrc{
				{
					Addr: addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test_thing",
						Name: "woot",
					}.Instance(addrs.IntKey(0)).Absolute(addrs.RootModuleInstance),
					ProviderAddr: addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					},
				},
				{
					Addr: addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test_thing",
						Name: "woot",
					}.Instance(addrs.IntKey(0)).Absolute(addrs.RootModuleInstance),
					DeposedKey: "foodface",
					ProviderAddr: addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					},
				},
				{
					Addr: addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test_thing",
						Name: "what",
					}.Instance(addrs.IntKey(0)).Absolute(addrs.RootModuleInstance),
					ProviderAddr: addrs.AbsProviderConfig{
						Module:   addrs.RootModule.Child("foo"),
						Provider: addrs.NewDefaultProvider("test"),
					},
				},
			},
		},
	}

	got := plan.ProviderAddrs()
	want := []addrs.AbsProviderConfig{
		{
			Module:   addrs.RootModule.Child("foo"),
			Provider: addrs.NewDefaultProvider("test"),
		},
		{
			Module:   addrs.RootModule,
			Provider: addrs.NewDefaultProvider("test"),
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Error("wrong result:\n" + diff)
	}
}

// Module outputs should not effect the result of Empty
func TestModuleOutputChangesEmpty(t *testing.T) {
	changes := &Changes{
		Outputs: []*OutputChangeSrc{
			{
				Addr: addrs.AbsOutputValue{
					Module: addrs.RootModuleInstance.Child("child", addrs.NoKey),
					OutputValue: addrs.OutputValue{
						Name: "output",
					},
				},
				ChangeSrc: ChangeSrc{
					Action: Update,
					Before: []byte("a"),
					After:  []byte("b"),
				},
			},
		},
	}

	if !changes.Empty() {
		t.Fatal("plan has no visible changes")
	}
}

// TestVariableMapper checks that the mapper is decoding types correctly from the plan
func TestVariableMapper(t *testing.T) {
	val1 := cty.StringVal("string value")
	val2 := cty.ObjectVal(map[string]cty.Value{"foo": cty.StringVal("bar")})
	val3 := cty.MapVal(map[string]cty.Value{
		"inner": cty.SetVal([]cty.Value{cty.StringVal("baz")}),
	})
	val4 := cty.ListVal([]cty.Value{cty.BoolVal(false)})
	val5 := cty.SetVal([]cty.Value{
		cty.ObjectVal(
			map[string]cty.Value{
				"inner": cty.ObjectVal(map[string]cty.Value{"foo": cty.NumberIntVal(25)}),
			},
		),
	})
	p := Plan{VariableValues: map[string]DynamicValue{
		"raw_string":                  encodeDynamicValueWithType(t, val1, cty.DynamicPseudoType),
		"object_of_strings":           encodeDynamicValueWithType(t, val2, cty.DynamicPseudoType),
		"map_of_sets_of_strings":      encodeDynamicValueWithType(t, val3, cty.DynamicPseudoType),
		"list_of_bools":               encodeDynamicValueWithType(t, val4, cty.DynamicPseudoType),
		"set_of_obj_of_obj_of_number": encodeDynamicValueWithType(t, val5, cty.DynamicPseudoType),
	}}

	vm := p.VariableMapper()

	cases := map[string]cty.Value{
		"raw_string":                  val1,
		"object_of_strings":           val2,
		"map_of_sets_of_strings":      val3,
		"list_of_bools":               val4,
		"set_of_obj_of_obj_of_number": val5,
	}
	for varName, wantVal := range cases {
		t.Run(varName, func(t *testing.T) {
			val, diag := vm(&configs.Variable{Name: varName})
			if diag.HasErrors() {
				t.Fatalf("unexpected diagnostics from the variable mapper: %s", diag)
			}
			if !val.RawEquals(wantVal) {
				t.Fatalf("returned value is not equal with the expected one.\n\twant:%s\n\tgot:%s\n", wantVal, val)
			}
		})
	}
}

func encodeDynamicValueWithType(t *testing.T, value cty.Value, ty cty.Type) []byte {
	data, err := ctymsgpack.Marshal(value, ty)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %s", err)
	}
	return data
}
