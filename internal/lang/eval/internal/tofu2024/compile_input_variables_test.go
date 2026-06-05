// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/zclconf/go-cty-debug/ctydebug"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestCompileInputVariable(t *testing.T) {
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
				Type: cty.String,
			},
			wantValue: cty.StringVal("Timothy"),
		},
		"required, nullable variable that is null": {
			values: map[string]cty.Value{
				"name": cty.NullVal(cty.String),
			},
			config: &configs.Variable{
				Name:        "name",
				Type:        cty.String,
				Nullable:    true,
				NullableSet: true,
			},
			wantValue: cty.NullVal(cty.String),
		},
		"required variable that is not set": {
			values: map[string]cty.Value{},
			config: &configs.Variable{
				Name: "name",
				Type: cty.String,
			},
			wantValue: exprs.AsEvalError(cty.UnknownVal(cty.String)),
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
				Type:     cty.String,
				Nullable: false,
			},
			wantValue: exprs.AsEvalError(cty.UnknownVal(cty.String)),
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
				Type:    cty.String,
				Default: cty.StringVal("not Timothy"),
			},
			wantValue: cty.StringVal("Timothy"),
		},
		"optional variable that is not set": {
			values: map[string]cty.Value{},
			config: &configs.Variable{
				Name:    "name",
				Type:    cty.String,
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
				Type:     cty.String,
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
				Type:     cty.String,
				Default:  cty.StringVal("not Timothy"),
				Nullable: false,
			},
			wantValue: cty.StringVal("not Timothy"),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if test.config.Type == cty.NilType {
				t.Fatal("input variable declaration has no type") // omitting this causes confusing downstream errors, and never occurs in practice
			}
			if test.config.ConstraintType == cty.NilType {
				// This emulates some fixup that the config loader would normally do
				// in the simple case where no optional attributes are present,
				// since we're not actually running the config loader here.
				test.config.ConstraintType = test.config.Type
				test.config.TypeDefaults = &typeexpr.Defaults{
					Type: test.config.Type,
				}
			}

			// This test is oriented around testing one variable at a time,
			// but it uses the function that compiles all of the variables
			// in a module at once so we'll pretend like we're compiling
			// a module that only has this one variable declaration.
			configs := map[string]*configs.Variable{
				test.config.Name: test.config,
			}
			valuesValuer := exprs.ConstantValuerWithSourceRange(cty.ObjectVal(test.values), presentDefRange)
			emptyScope := exprs.FlatScopeForTesting(nil)

			compiled := compileModuleInstanceInputVariables(t.Context(), configs, valuesValuer, emptyScope, addrs.RootModuleInstance, &missingDefRange)
			if compiled == nil {
				t.Fatal("compileModuleInstanceInputVariables returned nil")
			}
			vn, ok := compiled[test.config.Addr()]
			if !ok {
				t.Fatalf("compileModuleInstanceInputVariables result does not include entry for %s", test.config.Addr())
			}

			ctx := grapheval.ContextWithNewWorker(t.Context())
			gotValue, gotDiags := vn.Value(ctx)
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
