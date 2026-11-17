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
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestOutputValue_Value(t *testing.T) {
	testAddr := addrs.OutputValue{Name: "test"}
	testValuer(t, map[string]valuerTest[*OutputValue]{
		"minimal": {
			&OutputValue{
				Addr:       testAddr.Absolute(addrs.RootModuleInstance),
				RawValue:   constantOnceValuer(cty.StringVal("Hello, world!"), "RawValue %s", testAddr),
				TargetType: cty.DynamicPseudoType,
			},
			cty.StringVal("Hello, world!"),
			nil,
		},
		"explicit type conversion": {
			&OutputValue{
				Addr:       testAddr.Absolute(addrs.RootModuleInstance),
				RawValue:   constantOnceValuer(cty.True, "RawValue %s", testAddr),
				TargetType: cty.String,
			},
			cty.StringVal("true"),
			nil,
		},
		"force sensitive": {
			&OutputValue{
				Addr:           testAddr.Absolute(addrs.RootModuleInstance),
				RawValue:       constantOnceValuer(cty.StringVal("Hello, world!"), "RawValue %s", testAddr),
				TargetType:     cty.DynamicPseudoType,
				ForceSensitive: true,
			},
			cty.StringVal("Hello, world!").Mark(marks.Sensitive),
			nil,
		},
		"force ephemeral": {
			&OutputValue{
				Addr:           testAddr.Absolute(addrs.RootModuleInstance),
				RawValue:       constantOnceValuer(cty.StringVal("Hello, world!"), "RawValue %s", testAddr),
				TargetType:     cty.DynamicPseudoType,
				ForceEphemeral: true,
			},
			cty.StringVal("Hello, world!").Mark(marks.Ephemeral),
			nil,
		},
		"force sensitive and ephemeral": {
			&OutputValue{
				Addr:           testAddr.Absolute(addrs.RootModuleInstance),
				RawValue:       constantOnceValuer(cty.StringVal("Hello, world!"), "RawValue %s", testAddr),
				TargetType:     cty.DynamicPseudoType,
				ForceSensitive: true,
				ForceEphemeral: true,
			},
			cty.StringVal("Hello, world!").Mark(marks.Sensitive).Mark(marks.Ephemeral),
			nil,
		},
		"precondition successful": {
			&OutputValue{
				Addr:       testAddr.Absolute(addrs.RootModuleInstance),
				RawValue:   constantOnceValuer(cty.StringVal("Hello, world!"), "RawValue %s", testAddr),
				TargetType: cty.DynamicPseudoType,
				Preconditions: []*CheckRule{
					{
						ConditionValuer:    constantOnceValuer(cty.True, "condition"),
						ErrorMessageValuer: constantOnceValuer(cty.StringVal("failed"), "error_message"),
					},
				},
			},
			cty.StringVal("Hello, world!"),
			nil,
		},
		"precondition unsuccessful": {
			&OutputValue{
				Addr:       testAddr.Absolute(addrs.RootModuleInstance),
				RawValue:   constantOnceValuer(cty.StringVal("Hello, world!"), "RawValue %s", testAddr),
				TargetType: cty.DynamicPseudoType,
				Preconditions: []*CheckRule{
					{
						ConditionValuer: constantOnceValuerWithSource(
							cty.False,
							tfdiags.SourceRange{Filename: "condition"},
							"condition",
						),
						ErrorMessageValuer: constantOnceValuerWithSource(
							cty.StringVal("Failure Message"),
							tfdiags.SourceRange{Filename: "error_message"},
							"error_message",
						),
						DeclSourceRange: tfdiags.SourceRange{
							Filename: "precondition",
						},
					},
				},
			},
			cty.DynamicVal.Mark(exprs.EvalError),
			tfdiags.New(
				// FIXME: This should actually be reported as a sourceful
				// diagnostic whose subject is the ConditionValuer's source
				// range, but the check-checking codepath doesn't seem to be
				// doing that correctly right now.
				tfdiags.Sourceless(
					tfdiags.Error,
					"Output value precondition failed",
					"Failure Message\n\nThis problem was reported by the precondition at precondition:0,0.",
				),
			),
		},
	})
}
