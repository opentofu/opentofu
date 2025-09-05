// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"testing"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/marks"
)

func TestModuleInstance_Value(t *testing.T) {
	// We don't actually evaluate these two other nodes during the test. They
	// are only used in pointer comparisons comparing got vs. want values.
	resourceInstancePlaceholder := &ResourceInstance{}
	providerInstancePlaceholder := &ProviderInstance{
		ProviderAddr: addrs.NewBuiltInProvider("test"),
	}

	testValuer(t, map[string]valuerTest[*ModuleInstance]{
		"no output values": {
			&ModuleInstance{
				OutputValuers: nil,
			},
			cty.EmptyObjectVal,
			nil,
		},
		"mixed types": {
			&ModuleInstance{
				OutputValuers: constantOnceValuerMap(map[addrs.OutputValue]cty.Value{
					{Name: "string"}:       cty.StringVal("hello"),
					{Name: "number"}:       cty.Zero,
					{Name: "unknown_list"}: cty.UnknownVal(cty.List(cty.String)),
				}),
			},
			cty.ObjectVal(map[string]cty.Value{
				"string":       cty.StringVal("hello"),
				"number":       cty.Zero,
				"unknown_list": cty.UnknownVal(cty.List(cty.String)),
			}),
			nil,
		},
		"refined unknowns": {
			&ModuleInstance{
				OutputValuers: constantOnceValuerMap(map[addrs.OutputValue]cty.Value{
					{Name: "string"}: cty.UnknownVal(cty.String).Refine().
						NotNull().
						StringPrefix("ami-").
						NewValue(),
					{Name: "number"}: cty.UnknownVal(cty.Number).Refine().
						NumberRangeInclusive(cty.Zero, cty.NumberIntVal(1)).
						NewValue(),
					{Name: "list"}: cty.UnknownVal(cty.List(cty.String)).Refine().
						CollectionLength(4).
						NewValue(),
				}),
			},
			cty.ObjectVal(map[string]cty.Value{
				"string": cty.UnknownVal(cty.String).Refine().
					NotNull().
					StringPrefix("ami-").
					NewValue(),
				"number": cty.UnknownVal(cty.Number).Refine().
					NumberRangeInclusive(cty.Zero, cty.NumberIntVal(1)).
					NewValue(),
				"list": cty.UnknownVal(cty.List(cty.String)).Refine().
					CollectionLength(4).
					NewValue(),
			}),
			nil,
		},
		"marked values": {
			&ModuleInstance{
				OutputValuers: constantOnceValuerMap(map[addrs.OutputValue]cty.Value{
					{Name: "sensitive"}:              cty.StringVal("boop").Mark(marks.Sensitive),
					{Name: "from_resource_instance"}: cty.StringVal("beep").Mark(ResourceInstanceMark{resourceInstancePlaceholder}),
					{Name: "eval_error"}:             exprs.AsEvalError(cty.StringVal("oops")),
				}),
			},
			cty.ObjectVal(map[string]cty.Value{
				"sensitive":              cty.StringVal("boop").Mark(marks.Sensitive),
				"from_resource_instance": cty.StringVal("beep").Mark(ResourceInstanceMark{resourceInstancePlaceholder}),
				"eval_error":             exprs.AsEvalError(cty.StringVal("oops")),
			}),
			nil,
		},
		"provider instance reference": {
			&ModuleInstance{
				OutputValuers: constantOnceValuerMap(map[addrs.OutputValue]cty.Value{
					{Name: "provider_inst"}: ProviderInstanceRefValue(providerInstancePlaceholder),
				}),
			},
			cty.ObjectVal(map[string]cty.Value{
				"provider_inst": ProviderInstanceRefValue(providerInstancePlaceholder),
			}),
			nil,
		},
	})
}
