// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package corelinting

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/collections"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/linting"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestUsedVarsAndLocalsCollector(t *testing.T) {
	t.Run("nil configuration", func(t *testing.T) {
		ctx := tfdiags.ContextWithLintFilterHints(t.Context(), collections.NewSet(ruleIDVariableNotUsed, ruleIDLocalNotUsed), collections.NewSet[linting.RuleAddr]())
		c := UsedVarsAndLocalsCollector(ctx, nil)
		if !isNoopCollector(c) {
			t.Fatalf("expected a noOpUsedVarsAndLocals to be returned but got %s", reflect.TypeOf(c))
		}
	})
	t.Run("no lint rule enabled", func(t *testing.T) {
		ctx := tfdiags.ContextWithLintFilterHints(t.Context(), collections.NewSet[linting.RuleAddr](), collections.NewSet[linting.RuleAddr]())
		c := UsedVarsAndLocalsCollector(ctx, &configs.Config{})
		if !isNoopCollector(c) {
			t.Fatalf("expected a noOpUsedVarsAndLocals to be returned but got %s", reflect.TypeOf(c))
		}
	})
	t.Run("only not used variables linting enabled", func(t *testing.T) {
		ctx := tfdiags.ContextWithLintFilterHints(t.Context(), collections.NewSet[linting.RuleAddr](ruleIDVariableNotUsed), collections.NewSet[linting.RuleAddr]())
		c := UsedVarsAndLocalsCollector(ctx, &configs.Config{})
		if isNoopCollector(c) {
			t.Fatalf("expected a functional collector to be returned but got %s", reflect.TypeOf(c))
		}
	})
	t.Run("only not used locals linting enabled", func(t *testing.T) {
		ctx := tfdiags.ContextWithLintFilterHints(t.Context(), collections.NewSet[linting.RuleAddr](ruleIDLocalNotUsed), collections.NewSet[linting.RuleAddr]())
		c := UsedVarsAndLocalsCollector(ctx, &configs.Config{})
		if isNoopCollector(c) {
			t.Fatalf("expected a functional collector to be returned but got %s", reflect.TypeOf(c))
		}
	})
	t.Run("both linting rules enabled", func(t *testing.T) {
		ctx := tfdiags.ContextWithLintFilterHints(t.Context(), collections.NewSet[linting.RuleAddr](ruleIDVariableNotUsed, ruleIDLocalNotUsed), collections.NewSet[linting.RuleAddr]())
		c := UsedVarsAndLocalsCollector(ctx, &configs.Config{})
		if isNoopCollector(c) {
			t.Fatalf("expected a functional collector to be returned but got %s", reflect.TypeOf(c))
		}
	})
	t.Run("group id enabled", func(t *testing.T) {
		ctx := tfdiags.ContextWithLintFilterHints(t.Context(), collections.NewSet[linting.RuleAddr](GroupIDImprovement), collections.NewSet[linting.RuleAddr]())
		c := UsedVarsAndLocalsCollector(ctx, &configs.Config{})
		if isNoopCollector(c) {
			t.Fatalf("expected a functional collector to be returned but got %s", reflect.TypeOf(c))
		}
	})
}

func TestValidate(t *testing.T) {
	var1 := &configs.Variable{
		Name:      "var1",
		DeclRange: hcl.Range{Filename: "variables.tf", Start: hcl.Pos{Line: 0, Column: 0, Byte: 0}, End: hcl.Pos{Line: 0, Column: 15, Byte: 15}},
	}
	var2 := &configs.Variable{
		Name:      "var2",
		DeclRange: hcl.Range{Filename: "variables.tf", Start: hcl.Pos{Line: 3, Column: 0, Byte: 18}, End: hcl.Pos{Line: 3, Column: 15, Byte: 32}},
	}
	local1 := &configs.Local{
		Name:      "local1",
		DeclRange: hcl.Range{Filename: "variables.tf", Start: hcl.Pos{Line: 7, Column: 2, Byte: 0}, End: hcl.Pos{Line: 0, Column: 15, Byte: 48}},
	}
	local2 := &configs.Local{
		Name:      "local2",
		DeclRange: hcl.Range{Filename: "variables.tf", Start: hcl.Pos{Line: 8, Column: 2, Byte: 0}, End: hcl.Pos{Line: 0, Column: 15, Byte: 65}},
	}
	cfg := &configs.Config{
		Module: &configs.Module{
			Variables: map[string]*configs.Variable{"var1": var1, "var2": var2},
			Locals:    map[string]*configs.Local{"local1": local1, "local2": local2},
		},
	}
	cases := map[string]struct {
		collect   func(collector VarsAndLocalsCollector)
		wantDiags tfdiags.Diagnostics
	}{
		"nothing collected": {
			collect: func(collector VarsAndLocalsCollector) {},
			wantDiags: tfdiags.New(
				tfdiags.LintMessage(ruleIDVariableNotUsed, []linting.RuleAddr{GroupIDImprovement}, "Variable not used", fmt.Sprintf("Found no usage of the variable %q", var1.Name), new(tfdiags.SourceRangeFromHCL(var1.DeclRange)), nil),
				tfdiags.LintMessage(ruleIDVariableNotUsed, []linting.RuleAddr{GroupIDImprovement}, "Variable not used", fmt.Sprintf("Found no usage of the variable %q", var2.Name), new(tfdiags.SourceRangeFromHCL(var2.DeclRange)), nil),
				tfdiags.LintMessage(ruleIDLocalNotUsed, []linting.RuleAddr{GroupIDImprovement}, "Local not used", fmt.Sprintf("Found no usage of the local %q", local1.Name), new(tfdiags.SourceRangeFromHCL(local1.DeclRange)), nil),
				tfdiags.LintMessage(ruleIDLocalNotUsed, []linting.RuleAddr{GroupIDImprovement}, "Local not used", fmt.Sprintf("Found no usage of the local %q", local2.Name), new(tfdiags.SourceRangeFromHCL(local2.DeclRange)), nil),
			),
		},
		"only one var collected": {
			collect: func(collector VarsAndLocalsCollector) {
				collector.CollectVariable(var1)
			},
			wantDiags: tfdiags.New(
				tfdiags.LintMessage(ruleIDVariableNotUsed, []linting.RuleAddr{GroupIDImprovement}, "Variable not used", fmt.Sprintf("Found no usage of the variable %q", var2.Name), new(tfdiags.SourceRangeFromHCL(var2.DeclRange)), nil),
				tfdiags.LintMessage(ruleIDLocalNotUsed, []linting.RuleAddr{GroupIDImprovement}, "Local not used", fmt.Sprintf("Found no usage of the local %q", local1.Name), new(tfdiags.SourceRangeFromHCL(local1.DeclRange)), nil),
				tfdiags.LintMessage(ruleIDLocalNotUsed, []linting.RuleAddr{GroupIDImprovement}, "Local not used", fmt.Sprintf("Found no usage of the local %q", local2.Name), new(tfdiags.SourceRangeFromHCL(local2.DeclRange)), nil),
			),
		},
		"both variables collected": {
			collect: func(collector VarsAndLocalsCollector) {
				collector.CollectVariable(var1)
				collector.CollectVariable(var2)
			},
			wantDiags: tfdiags.New(
				tfdiags.LintMessage(ruleIDLocalNotUsed, []linting.RuleAddr{GroupIDImprovement}, "Local not used", fmt.Sprintf("Found no usage of the local %q", local1.Name), new(tfdiags.SourceRangeFromHCL(local1.DeclRange)), nil),
				tfdiags.LintMessage(ruleIDLocalNotUsed, []linting.RuleAddr{GroupIDImprovement}, "Local not used", fmt.Sprintf("Found no usage of the local %q", local2.Name), new(tfdiags.SourceRangeFromHCL(local2.DeclRange)), nil),
			),
		},
		"only 1 local collected": {
			collect: func(collector VarsAndLocalsCollector) {
				collector.CollectLocal(local1)
			},
			wantDiags: tfdiags.New(
				tfdiags.LintMessage(ruleIDVariableNotUsed, []linting.RuleAddr{GroupIDImprovement}, "Variable not used", fmt.Sprintf("Found no usage of the variable %q", var1.Name), new(tfdiags.SourceRangeFromHCL(var1.DeclRange)), nil),
				tfdiags.LintMessage(ruleIDVariableNotUsed, []linting.RuleAddr{GroupIDImprovement}, "Variable not used", fmt.Sprintf("Found no usage of the variable %q", var2.Name), new(tfdiags.SourceRangeFromHCL(var2.DeclRange)), nil),
				tfdiags.LintMessage(ruleIDLocalNotUsed, []linting.RuleAddr{GroupIDImprovement}, "Local not used", fmt.Sprintf("Found no usage of the local %q", local2.Name), new(tfdiags.SourceRangeFromHCL(local2.DeclRange)), nil),
			),
		},
		"both locals collected": {
			collect: func(collector VarsAndLocalsCollector) {
				collector.CollectLocal(local1)
				collector.CollectLocal(local2)
			},
			wantDiags: tfdiags.New(
				tfdiags.LintMessage(ruleIDVariableNotUsed, []linting.RuleAddr{GroupIDImprovement}, "Variable not used", fmt.Sprintf("Found no usage of the variable %q", var1.Name), new(tfdiags.SourceRangeFromHCL(var1.DeclRange)), nil),
				tfdiags.LintMessage(ruleIDVariableNotUsed, []linting.RuleAddr{GroupIDImprovement}, "Variable not used", fmt.Sprintf("Found no usage of the variable %q", var2.Name), new(tfdiags.SourceRangeFromHCL(var2.DeclRange)), nil),
			),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := tfdiags.ContextWithLintFilterHints(t.Context(), collections.NewSet[linting.RuleAddr](GroupIDImprovement), collections.NewSet[linting.RuleAddr]())
			c := UsedVarsAndLocalsCollector(ctx, cfg)
			tc.collect(c)
			diags := c.Validate(ctx)
			slices.SortFunc(diags, func(a, b tfdiags.Diagnostic) int {
				return strings.Compare(a.Description().Detail, b.Description().Detail)
			})
			slices.SortFunc(tc.wantDiags, func(a, b tfdiags.Diagnostic) int {
				return strings.Compare(a.Description().Detail, b.Description().Detail)
			})
			compareDiagnostics(t, tc.wantDiags, diags)
		})
	}
}

// This checks the thread-safety of the collector implementation.
func TestCollect(t *testing.T) {
	ctx := tfdiags.ContextWithLintFilterHints(t.Context(), collections.NewSet[linting.RuleAddr](GroupIDImprovement), collections.NewSet[linting.RuleAddr]())
	const samples = 1000
	cfg := &configs.Config{Module: &configs.Module{Variables: make(map[string]*configs.Variable), Locals: make(map[string]*configs.Local)}}
	for i := range samples {
		lname := fmt.Sprintf("local%d", i)
		cfg.Module.Locals[lname] = &configs.Local{
			Name: lname,
			DeclRange: hcl.Range{
				Filename: "locals.tf",
				Start:    hcl.Pos{Line: i},
				End:      hcl.Pos{Line: i},
			},
		}
		vname := fmt.Sprintf("var%d", i)
		cfg.Module.Variables[vname] = &configs.Variable{
			Name: vname,
			DeclRange: hcl.Range{
				Filename: "variables.tf",
				Start:    hcl.Pos{Line: i + 1},
				End:      hcl.Pos{Line: i + 1},
			},
		}
	}
	c := UsedVarsAndLocalsCollector(ctx, cfg)
	var wg sync.WaitGroup
	var i int
	for _, vc := range cfg.Module.Variables {
		i++
		if i%100 == 0 {
			continue // to generate some linting warnings
		}
		wg.Go(func() {
			c.CollectVariable(vc)
		})
	}
	i = 0
	for _, lc := range cfg.Module.Locals {
		i++
		if i%100 == 0 {
			continue // to generate some linting warnings
		}
		wg.Go(func() {
			c.CollectLocal(lc)
		})
	}
	wg.Wait()
	diags := c.Validate(ctx)
	if len(diags) != 20 {
		t.Errorf("expected to have %d diagnostics but got %d", 20, len(diags))
	}
}

func isNoopCollector(c VarsAndLocalsCollector) bool {
	_, ok := c.(*noOpUsedVarsAndLocals)
	return ok
}
