// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package symlib

import (
	"math/big"
	"testing"

	"github.com/zclconf/go-cty/cty"
)

// TestFunction_variadicNull verifies that passing literal null (cty.NullVal(cty.DynamicPseudoType))
// or unknown values to a variadic, any-typed, or untyped parameter in a symbols function does not
// cause cty to skip evaluation or return null regardless of the return expression (Issue #4630).
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

	t.Run("variadic any parameter with null and other args", func(t *testing.T) {
		f, ok := lib.functions["test_variadic_any"]
		if !ok {
			t.Fatal("Failed to find function test_variadic_any")
		}

		got, err := f.Call([]cty.Value{cty.NullVal(cty.DynamicPseudoType), cty.NumberIntVal(1)})
		if err != nil {
			t.Fatalf("unexpected error calling function: %s", err)
		}
		if got.IsNull() {
			t.Fatalf("expected 0, got null")
		}
		if got.AsBigFloat().Cmp(new(big.Float).SetInt64(0)) != 0 {
			t.Fatalf("expected 0, got %s", got.GoString())
		}
	})

	t.Run("any parameter with null", func(t *testing.T) {
		f, ok := lib.functions["test_any_param"]
		if !ok {
			t.Fatal("Failed to find function test_any_param")
		}

		got, err := f.Call([]cty.Value{cty.NullVal(cty.DynamicPseudoType)})
		if err != nil {
			t.Fatalf("unexpected error calling function: %s", err)
		}
		if got.IsNull() {
			t.Fatalf("expected 42, got null")
		}
		if got.AsBigFloat().Cmp(new(big.Float).SetInt64(42)) != 0 {
			t.Fatalf("expected 42, got %s", got.GoString())
		}
	})

	t.Run("untyped parameter with null", func(t *testing.T) {
		f, ok := lib.functions["test_untyped_param"]
		if !ok {
			t.Fatal("Failed to find function test_untyped_param")
		}

		got, err := f.Call([]cty.Value{cty.NullVal(cty.DynamicPseudoType)})
		if err != nil {
			t.Fatalf("unexpected error calling function: %s", err)
		}
		if got.IsNull() {
			t.Fatalf("expected 100, got null")
		}
		if got.AsBigFloat().Cmp(new(big.Float).SetInt64(100)) != 0 {
			t.Fatalf("expected 100, got %s", got.GoString())
		}
	})

	t.Run("any parameter with unknown", func(t *testing.T) {
		f, ok := lib.functions["test_any_param"]
		if !ok {
			t.Fatal("Failed to find function test_any_param")
		}

		got, err := f.Call([]cty.Value{cty.UnknownVal(cty.DynamicPseudoType)})
		if err != nil {
			t.Fatalf("unexpected error calling function: %s", err)
		}
		if got.IsNull() {
			t.Fatalf("expected 42, got null")
		}
		if got.AsBigFloat().Cmp(new(big.Float).SetInt64(42)) != 0 {
			t.Fatalf("expected 42, got %s", got.GoString())
		}
	})
}
