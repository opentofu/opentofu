// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty-debug/ctydebug"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestCompileInputVariableValuer(t *testing.T) {
	missingDefRange := tfdiags.SourceRange{
		Filename: "<missing-def-range>",
		Start:    tfdiags.SourcePos{Line: 1, Column: 1, Byte: 1},
		End:      tfdiags.SourcePos{Line: 1, Column: 1, Byte: 1},
	}
	presentDefRange := tfdiags.SourceRange{
		Filename: "<present-def-range>",
		Start:    tfdiags.SourcePos{Line: 1, Column: 1, Byte: 1},
		End:      tfdiags.SourcePos{Line: 1, Column: 1, Byte: 1},
	}

	tests := map[string]struct {
		values map[string]cty.Value
		config *configs.Variable

		wantValue cty.Value
		wantDiags tfdiags.Diagnostics
	}{
		"required variable that is set": {
			values: map[string]cty.Value{
				"name": cty.StringVal("Timothy"),
			},
			config: &configs.Variable{
				Name: "name",
			},
			wantValue: cty.StringVal("Timothy"),
		},
		"required, nullable variable that is null": {
			values: map[string]cty.Value{
				"name": cty.NullVal(cty.String),
			},
			config: &configs.Variable{
				Name:        "name",
				Nullable:    true,
				NullableSet: true,
			},
			wantValue: cty.NullVal(cty.String),
		},
		"required variable that is not set": {
			values: map[string]cty.Value{},
			config: &configs.Variable{
				Name: "name",
			},
			wantValue: cty.DynamicVal,
			wantDiags: tfdiags.New(
				&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Missing definition for required input variable",
					Detail:   `Input variable "name" is required, and so it must be provided as an argument to this module.`,
					Subject:  presentDefRange.ToHCL().Ptr(),
				},
			),
		},
		"required, non-nullable variable that is null": {
			values: map[string]cty.Value{
				"name": cty.NullVal(cty.String),
			},
			config: &configs.Variable{
				Name:     "name",
				Nullable: false,
			},
			wantValue: cty.DynamicVal,
			wantDiags: tfdiags.New(
				&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid value for input variable",
					Detail:   `Input variable "name" is required and not nullable, and so it cannot be set to null.`,
					Subject:  presentDefRange.ToHCL().Ptr(),
				},
			),
		},

		"optional variable that is set": {
			values: map[string]cty.Value{
				"name": cty.StringVal("Timothy"),
			},
			config: &configs.Variable{
				Name:    "name",
				Default: cty.StringVal("not Timothy"),
			},
			wantValue: cty.StringVal("Timothy"),
		},
		"optional variable that is not set": {
			values: map[string]cty.Value{},
			config: &configs.Variable{
				Name:    "name",
				Default: cty.StringVal("not Timothy"),
			},
			wantValue: cty.StringVal("not Timothy"),
		},
		"optional, nullable variable that is set to null": {
			values: map[string]cty.Value{
				"name": cty.NullVal(cty.String),
			},
			config: &configs.Variable{
				Name:     "name",
				Default:  cty.StringVal("not Timothy"),
				Nullable: true,
			},
			wantValue: cty.NullVal(cty.String),
		},
		"optional, non-nullable variable that is set to null": {
			values: map[string]cty.Value{
				"name": cty.NullVal(cty.String),
			},
			config: &configs.Variable{
				Name:     "name",
				Default:  cty.StringVal("not Timothy"),
				Nullable: false,
			},
			wantValue: cty.StringVal("not Timothy"),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			valuesValuer := exprs.ConstantValuerWithSourceRange(cty.ObjectVal(test.values), presentDefRange)
			resultValuer := compileInputVariableValuer(valuesValuer, test.config, addrs.RootModuleInstance, &missingDefRange)

			gotValue, gotDiags := resultValuer.Value(t.Context())
			if diff := cmp.Diff(test.wantValue, gotValue, ctydebug.CmpOptions); diff != "" {
				t.Error("wrong result value\n" + diff)
			}
			// We'll use the "ForRPC" variant of diagnostics so that we're
			// comparing by content and not by the implementation detail of
			// what types of diagnostic are returned.
			wantDiags := test.wantDiags.ForRPC()
			wantDiags.Sort()
			gotDiags = gotDiags.ForRPC()
			gotDiags.Sort()
			if diff := cmp.Diff(wantDiags, gotDiags, ctydebug.CmpOptions); diff != "" {
				t.Error("wrong diagnostics\n" + diff)
			}
		})
	}
}
