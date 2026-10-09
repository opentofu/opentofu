// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package symlib

import (
	"math/big"
	"testing"

	"github.com/zclconf/go-cty/cty"
)

// TestFunction_variadicNull verifies that passing literal null (cty.NullVal(cty.DynamicPseudoType))
// to a variadic or any-typed parameter in a symbols function does not cause cty to skip
// evaluation or return null regardless of the return expression (Issue #4630).
func TestFunction_variadicNull(t *testing.T) {
	files := map[string]string{
		"./functions.sym.hcl": `
function "test_variadic_any" {
  type = number

  parameter "value" {
    type = any
    variadic = true
  }

  return = 0
}

function "test_any_param" {
  type = number

  parameter "value" {
    type = any
  }

  return = 42
}

function "test_untyped_param" {
  type = number

  parameter "value" {}

  return = 100
}
`,
	}

	lib, diags := testCompile(t, files)
	assertNoDiags(t, diags)

	tests := []struct {
		name     string
		funcName string
		args     []cty.Value
		want     int64
	}{
		{
			name:     "variadic any parameter with null and other args",
			funcName: "test_variadic_any",
			args:     []cty.Value{cty.NullVal(cty.DynamicPseudoType), cty.NumberIntVal(1)},
			want:     0,
		},
		{
			name:     "any parameter with null",
			funcName: "test_any_param",
			args:     []cty.Value{cty.NullVal(cty.DynamicPseudoType)},
			want:     42,
		},
		{
			name:     "untyped parameter with null",
			funcName: "test_untyped_param",
			args:     []cty.Value{cty.NullVal(cty.DynamicPseudoType)},
			want:     100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, ok := lib.functions[tt.funcName]
			if !ok {
				t.Fatalf("Failed to find function %s", tt.funcName)
			}

			got, err := f.Call(tt.args)
			if err != nil {
				t.Fatalf("unexpected error calling function: %s", err)
			}
			if got.IsNull() {
				t.Fatalf("expected %d, got null", tt.want)
			}
			if got.AsBigFloat().Cmp(new(big.Float).SetInt64(tt.want)) != 0 {
				t.Fatalf("expected %d, got %s", tt.want, got.GoString())
			}
		})
	}
}
