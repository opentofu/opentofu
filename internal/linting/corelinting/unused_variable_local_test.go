// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package corelinting

import (
	"context"
	"fmt"
	"iter"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/linting"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestUnusedVariables(t *testing.T) {
	var1 := &configs.Variable{
		Name:      "var1",
		DeclRange: hcl.Range{Filename: "variables.tf", Start: hcl.Pos{Line: 0, Column: 0, Byte: 0}, End: hcl.Pos{Line: 0, Column: 15, Byte: 15}},
	}
	var2 := &configs.Variable{
		Name:      "var2",
		DeclRange: hcl.Range{Filename: "variables.tf", Start: hcl.Pos{Line: 3, Column: 0, Byte: 18}, End: hcl.Pos{Line: 3, Column: 15, Byte: 32}},
	}
	withVars := iter.Seq[*configs.Variable](func(yield func(*configs.Variable) bool) {
		if !yield(var1) {
			return
		}
		yield(var2)
	})
	withoutVars := iter.Seq[*configs.Variable](func(yield func(*configs.Variable) bool) {})
	cases := map[string]struct {
		provider  iter.Seq[*configs.Variable]
		wantDiags tfdiags.Diagnostics
	}{
		"no variables provided": {
			provider:  withoutVars,
			wantDiags: tfdiags.New(),
		},
		"variables provided": {
			provider: withVars,
			wantDiags: tfdiags.New(
				tfdiags.LintMessage(ruleIDVariableNotUsed, []linting.RuleAddr{GroupIDImprovement}, "Variable not used", fmt.Sprintf("Found no usage of the variable %q", var1.Name), new(tfdiags.SourceRangeFromHCL(var1.DeclRange)), nil),
				tfdiags.LintMessage(ruleIDVariableNotUsed, []linting.RuleAddr{GroupIDImprovement}, "Variable not used", fmt.Sprintf("Found no usage of the variable %q", var2.Name), new(tfdiags.SourceRangeFromHCL(var2.DeclRange)), nil),
			),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			runAgainstMultipleIdentifiers(t, func(t *testing.T, ctx context.Context) {
				diags := UnusedVariables(ctx, tc.provider)
				compareDiagnostics(t, tc.wantDiags, diags)
			}, []linting.RuleAddr{ruleIDVariableNotUsed}, []linting.RuleAddr{GroupIDImprovement})
		})
	}
}

func TestUnusedLocals(t *testing.T) {
	local1 := &configs.Local{
		Name:      "local1",
		DeclRange: hcl.Range{Filename: "variables.tf", Start: hcl.Pos{Line: 7, Column: 2, Byte: 0}, End: hcl.Pos{Line: 0, Column: 15, Byte: 48}},
	}
	local2 := &configs.Local{
		Name:      "local2",
		DeclRange: hcl.Range{Filename: "variables.tf", Start: hcl.Pos{Line: 8, Column: 2, Byte: 0}, End: hcl.Pos{Line: 0, Column: 15, Byte: 65}},
	}
	cases := map[string]struct {
		provider  iter.Seq[*configs.Local]
		wantDiags tfdiags.Diagnostics
	}{
		"no locals provided": {
			provider:  iter.Seq[*configs.Local](func(yield func(local *configs.Local) bool) {}),
			wantDiags: tfdiags.New(),
		},
		"locals provided": {
			provider: iter.Seq[*configs.Local](func(yield func(local *configs.Local) bool) {
				if !yield(local1) {
					return
				}
				yield(local2)
			}),
			wantDiags: tfdiags.New(
				tfdiags.LintMessage(ruleIDLocalNotUsed, []linting.RuleAddr{GroupIDImprovement}, "Local not used", fmt.Sprintf("Found no usage of the local %q", local1.Name), new(tfdiags.SourceRangeFromHCL(local1.DeclRange)), nil),
				tfdiags.LintMessage(ruleIDLocalNotUsed, []linting.RuleAddr{GroupIDImprovement}, "Local not used", fmt.Sprintf("Found no usage of the local %q", local2.Name), new(tfdiags.SourceRangeFromHCL(local2.DeclRange)), nil),
			),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			runAgainstMultipleIdentifiers(t, func(t *testing.T, ctx context.Context) {
				diags := UnusedLocal(ctx, tc.provider)
				compareDiagnostics(t, tc.wantDiags, diags)
			}, []linting.RuleAddr{ruleIDLocalNotUsed}, []linting.RuleAddr{GroupIDImprovement})
		})
	}
}
