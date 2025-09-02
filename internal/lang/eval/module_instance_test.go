// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty-debug/ctydebug"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
)

func TestCompileModuleInstance_valuesOnly(t *testing.T) {
	// This is a relatively straightforward test of compiling a module
	// instance containing only inert values (no resources, etc) and
	// then pulling values out of it to make sure that it's wired together
	// correctly. This is far from exhaustive but covers some of
	// the fundamentals that more complex situations rely on.

	ctx := grapheval.ContextWithNewWorker(t.Context())
	module := configs.ModuleFromStringForTesting(t, `
		variable "a" {
			type = string
		}
		locals {
			b = "${var.a}:${var.a}"
		}
		output "c" {
			value = "${local.b}/${local.b}"
		}
	`)
	evalCtx := &EvalContext{}
	evalCtx.init()
	call := &moduleInstanceCall{
		inputValues: InputValuesForTesting(map[string]cty.Value{
			"a": cty.True,
		}),
		evalContext: evalCtx,
	}
	inst := compileModuleInstance(ctx, module, addrs.ModuleSourceLocal("."), call)

	got, diags := inst.Value(ctx)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err().Error())
	}
	want := cty.ObjectVal(map[string]cty.Value{
		"c": cty.StringVal("true:true/true:true"),
	})
	if diff := cmp.Diff(want, got, ctydebug.CmpOptions); diff != "" {
		t.Error("wrong result\n" + diff)
	}
}
