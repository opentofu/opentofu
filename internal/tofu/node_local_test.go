// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/hcl2shim"
	"github.com/opentofu/opentofu/internal/states"
)

func TestNodeLocalValueExecute(t *testing.T) {
	tests := []struct {
		Value string
		Want  interface{}
		Err   bool
	}{
		{
			"hello!",
			"hello!",
			false,
		},
		{
			"",
			"",
			false,
		},
		{
			"Hello, ${local.foo}",
			nil,
			true, // self-referencing
		},
	}

	for _, test := range tests {
		t.Run(test.Value, func(t *testing.T) {
			expr, diags := hclsyntax.ParseTemplate([]byte(test.Value), "", hcl.Pos{Line: 1, Column: 1})
			if diags.HasErrors() {
				t.Fatal(diags.Error())
			}

			n := &nodeLocalValue{
				Addr:   addrs.LocalValue{Name: "foo"},
				Module: addrs.RootModule,
				Config: &configs.Local{
					Expr: expr,
				},
			}
			ctx := &MockEvalContext{
				StateState: states.NewState().SyncWrapper(),

				EvaluateExprResult: hcl2shim.HCL2ValueFromConfigValue(test.Want),
			}

			err := n.Execute(ctx, walkApply)
			if (err != nil) != test.Err {
				if err != nil {
					t.Errorf("unexpected error: %s", err)
				} else {
					t.Errorf("successful Eval; want error")
				}
			}

			ms := ctx.StateState.Module(addrs.RootModuleInstance)
			gotLocals := ms.LocalValues
			wantLocals := map[string]cty.Value{}
			if test.Want != nil {
				wantLocals["foo"] = hcl2shim.HCL2ValueFromConfigValue(test.Want)
			}

			if !reflect.DeepEqual(gotLocals, wantLocals) {
				t.Errorf(
					"wrong locals after Eval\ngot:  %swant: %s",
					spew.Sdump(gotLocals), spew.Sdump(wantLocals),
				)
			}
		})
	}
}
