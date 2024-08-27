// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cloud

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/terramate-io/opentofulib/internal/backend"
	"github.com/terramate-io/opentofulib/internal/configs"
	"github.com/terramate-io/opentofulib/internal/tfdiags"
	"github.com/terramate-io/opentofulib/internal/tofu"
	"github.com/zclconf/go-cty/cty"
)

func TestParseCloudRunVariables(t *testing.T) {
	t.Run("populates variables from allowed sources", func(t *testing.T) {
		vv := map[string]backend.UnparsedVariableValue{
			"undeclared":                      testUnparsedVariableValue{source: tofu.ValueFromCLIArg, value: cty.StringVal("0")},
			"declaredFromConfig":              testUnparsedVariableValue{source: tofu.ValueFromConfig, value: cty.StringVal("1")},
			"declaredFromNamedFileMapString":  testUnparsedVariableValue{source: tofu.ValueFromNamedFile, value: cty.MapVal(map[string]cty.Value{"foo": cty.StringVal("bar")})},
			"declaredFromNamedFileBool":       testUnparsedVariableValue{source: tofu.ValueFromNamedFile, value: cty.BoolVal(true)},
			"declaredFromNamedFileNumber":     testUnparsedVariableValue{source: tofu.ValueFromNamedFile, value: cty.NumberIntVal(2)},
			"declaredFromNamedFileListString": testUnparsedVariableValue{source: tofu.ValueFromNamedFile, value: cty.ListVal([]cty.Value{cty.StringVal("2a"), cty.StringVal("2b")})},
			"declaredFromNamedFileNull":       testUnparsedVariableValue{source: tofu.ValueFromNamedFile, value: cty.NullVal(cty.String)},
			"declaredFromNamedMapComplex":     testUnparsedVariableValue{source: tofu.ValueFromNamedFile, value: cty.MapVal(map[string]cty.Value{"foo": cty.ObjectVal(map[string]cty.Value{"qux": cty.ListVal([]cty.Value{cty.BoolVal(true), cty.BoolVal(false)})})})},
			"declaredFromCLIArg":              testUnparsedVariableValue{source: tofu.ValueFromCLIArg, value: cty.StringVal("3")},
			"declaredFromEnvVar":              testUnparsedVariableValue{source: tofu.ValueFromEnvVar, value: cty.StringVal("4")},
		}

		decls := map[string]*configs.Variable{
			"declaredFromConfig": {
				Name:           "declaredFromConfig",
				Type:           cty.String,
				ConstraintType: cty.String,
				ParsingMode:    configs.VariableParseLiteral,
				DeclRange: hcl.Range{
					Filename: "fake.tf",
					Start:    hcl.Pos{Line: 2, Column: 1, Byte: 0},
					End:      hcl.Pos{Line: 2, Column: 1, Byte: 0},
				},
			},
			"declaredFromNamedFileMapString": {
				Name:           "declaredFromNamedFileMapString",
				Type:           cty.Map(cty.String),
				ConstraintType: cty.Map(cty.String),
				ParsingMode:    configs.VariableParseHCL,
				DeclRange: hcl.Range{
					Filename: "fake.tf",
					Start:    hcl.Pos{Line: 2, Column: 1, Byte: 0},
					End:      hcl.Pos{Line: 2, Column: 1, Byte: 0},
				},
			},
			"declaredFromNamedFileBool": {
				Name:           "declaredFromNamedFileBool",
				Type:           cty.Bool,
				ConstraintType: cty.Bool,
				ParsingMode:    configs.VariableParseLiteral,
				DeclRange: hcl.Range{
					Filename: "fake.tf",
					Start:    hcl.Pos{Line: 2, Column: 1, Byte: 0},
					End:      hcl.Pos{Line: 2, Column: 1, Byte: 0},
				},
			},
			"declaredFromNamedFileNumber": {
				Name:           "declaredFromNamedFileNumber",
				Type:           cty.Number,
				ConstraintType: cty.Number,
				ParsingMode:    configs.VariableParseLiteral,
				DeclRange: hcl.Range{
					Filename: "fake.tf",
					Start:    hcl.Pos{Line: 2, Column: 1, Byte: 0},
					End:      hcl.Pos{Line: 2, Column: 1, Byte: 0},
				},
			},
			"declaredFromNamedFileListString": {
				Name:           "declaredFromNamedFileListString",
				Type:           cty.List(cty.String),
				ConstraintType: cty.List(cty.String),
				ParsingMode:    configs.VariableParseHCL,
				DeclRange: hcl.Range{
					Filename: "fake.tf",
					Start:    hcl.Pos{Line: 2, Column: 1, Byte: 0},
					End:      hcl.Pos{Line: 2, Column: 1, Byte: 0},
				},
			},
			"declaredFromNamedFileNull": {
				Name:           "declaredFromNamedFileNull",
				Type:           cty.String,
				ConstraintType: cty.String,
				ParsingMode:    configs.VariableParseHCL,
				DeclRange: hcl.Range{
					Filename: "fake.tf",
					Start:    hcl.Pos{Line: 2, Column: 1, Byte: 0},
					End:      hcl.Pos{Line: 2, Column: 1, Byte: 0},
				},
			},
			"declaredFromNamedMapComplex": {
				Name:           "declaredFromNamedMapComplex",
				Type:           cty.DynamicPseudoType,
				ConstraintType: cty.DynamicPseudoType,
				ParsingMode:    configs.VariableParseHCL,
				DeclRange: hcl.Range{
					Filename: "fake.tf",
					Start:    hcl.Pos{Line: 2, Column: 1, Byte: 0},
					End:      hcl.Pos{Line: 2, Column: 1, Byte: 0},
				},
			},
			"declaredFromCLIArg": {
				Name:           "declaredFromCLIArg",
				Type:           cty.String,
				ConstraintType: cty.String,
				ParsingMode:    configs.VariableParseLiteral,
				DeclRange: hcl.Range{
					Filename: "fake.tf",
					Start:    hcl.Pos{Line: 2, Column: 1, Byte: 0},
					End:      hcl.Pos{Line: 2, Column: 1, Byte: 0},
				},
			},
			"declaredFromEnvVar": {
				Name:           "declaredFromEnvVar",
				Type:           cty.String,
				ConstraintType: cty.String,
				ParsingMode:    configs.VariableParseLiteral,
				DeclRange: hcl.Range{
					Filename: "fake.tf",
					Start:    hcl.Pos{Line: 2, Column: 1, Byte: 0},
					End:      hcl.Pos{Line: 2, Column: 1, Byte: 0},
				},
			},
			"missing": {
				Name:           "missing",
				Type:           cty.String,
				ConstraintType: cty.String,
				Default:        cty.StringVal("2"),
				ParsingMode:    configs.VariableParseLiteral,
				DeclRange: hcl.Range{
					Filename: "fake.tf",
					Start:    hcl.Pos{Line: 2, Column: 1, Byte: 0},
					End:      hcl.Pos{Line: 2, Column: 1, Byte: 0},
				},
			},
		}
		wantVals := make(map[string]string)
		wantVals["declaredFromNamedFileBool"] = "true"
		wantVals["declaredFromNamedFileNumber"] = "2"
		wantVals["declaredFromNamedFileListString"] = `["2a", "2b"]`
		wantVals["declaredFromNamedFileNull"] = "null"
		wantVals["declaredFromNamedFileMapString"] = "{\n  foo = \"bar\"\n}"
		wantVals["declaredFromNamedMapComplex"] = "{\n  foo = {\n    qux = [true, false]\n  }\n}"
		wantVals["declaredFromCLIArg"] = `"3"`
		wantVals["declaredFromEnvVar"] = `"4"`

		gotVals, diags := ParseCloudRunVariables(vv, decls)
		if diff := cmp.Diff(wantVals, gotVals, cmp.Comparer(cty.Value.RawEquals)); diff != "" {
			t.Errorf("wrong result\n%s", diff)
		}

		if got, want := len(diags), 1; got != want {
			t.Fatalf("expected 1 variable error: %v, got %v", diags.Err(), want)
		}

		if got, want := diags[0].Description().Summary, "Value for undeclared variable"; got != want {
			t.Errorf("wrong summary for diagnostic 0\ngot:  %s\nwant: %s", got, want)
		}
	})
}

type testUnparsedVariableValue struct {
	source tofu.ValueSourceType
	value  cty.Value
}

func (v testUnparsedVariableValue) ParseVariableValue(mode configs.VariableParsingMode) (*tofu.InputValue, tfdiags.Diagnostics) {
	return &tofu.InputValue{
		Value:      v.value,
		SourceType: v.source,
		SourceRange: tfdiags.SourceRange{
			Filename: "fake.tfvars",
			Start:    tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
			End:      tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
		},
	}, nil
}
