// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"fmt"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/zclconf/go-cty/cty"
)

// This exercises most of the logic in StaticEvaluator and staticScopeData
//
//nolint:gocognit // it's a test
func TestStaticEvaluator_Evaluate(t *testing.T) {
	// Synthetic file for building test components
	testData := `
variable "str" {
	type = string
}
variable "str_map" {
	type = map(string)
}
variable "str_def" {
	type = string
	default = "sane default"
}
variable "str_map_def" {
	type = map(string)
	default = {
		keyA = "A"
	}
}
# Should/can variable checks be performed during static evaluation?

locals {
	# Simple static cases
	static = "static"
	static_ref = local.static
	static_fn = md5(local.static)
	path_root = path.root
	path_module = path.module

	# Variable References with Defaults
	var_str_def_ref = var.str_def
	var_map_def_access = var.str_map_def["keyA"]

	# Variable References without Defaults
	var_str_ref = var.str
	var_map_access = var.str_map["keyA"]

	# Bad References
	invalid_ref = invalid.attribute
	unavailable_ref = foo.bar.attribute

	# Circular References
	circular = local.circular
	circular_ref = local.circular
	circular_a = local.circular_b
	circular_b = local.circular_a

	# Dependency chain
	ref_a = var.str
	ref_b = local.ref_a
	ref_c = local.ref_b

	# Missing References
	local_missing = local.missing
	var_missing = var.missing
}

resource "foo" "bar" {}
	`

	parser := testParser(map[string]string{"eval.tf": testData})

	file, fileDiags := parser.LoadConfigFile("eval.tf")
	if fileDiags.HasErrors() {
		t.Fatal(fileDiags)
	}

	t.Run("Empty Eval", func(t *testing.T) {
		mod, _ := NewModule([]*File{file}, nil, RootModuleCallForTesting, "dir")
		emptyEval := StaticEvaluator{}

		// Expr with no traversals shouldn't access any fields
		value, diags := emptyEval.Evaluate(mod.Locals["static"].Expr, StaticIdentifier{})
		if diags.HasErrors() {
			t.Error(diags)
		}
		if value.AsString() != "static" {
			t.Errorf("Expected %s got %s", "static", value.AsString())
		}

		// Expr with traversals should panic, indicating a programming error
		defer func() {
			r := recover()
			if r == nil {
				t.Fatalf("should panic")
			}
		}()
		_, _ = emptyEval.Evaluate(mod.Locals["static_ref"].Expr, StaticIdentifier{})
	})

	t.Run("Simple static cases", func(t *testing.T) {
		mod, _ := NewModule([]*File{file}, nil, RootModuleCallForTesting, "dir")
		eval := NewStaticEvaluator(mod, RootModuleCallForTesting)

		locals := []struct {
			ident string
			value string
		}{
			{"static", "static"},
			{"static_ref", "static"},
			{"static_fn", "a81259cef8e959c624df1d456e5d3297"},
			{"path_root", "<testing>"},
			{"path_module", "dir"},
		}
		for _, local := range locals {
			t.Run(local.ident, func(t *testing.T) {
				value, diags := eval.Evaluate(mod.Locals[local.ident].Expr, StaticIdentifier{})
				if diags.HasErrors() {
					t.Error(diags)
				}
				if value.AsString() != local.value {
					t.Errorf("Expected %s got %s", local.value, value.AsString())
				}
			})
		}
	})

	t.Run("Valid Variables", func(t *testing.T) {
		input := map[string]cty.Value{
			"str":     cty.StringVal("vara"),
			"str_map": cty.MapVal(map[string]cty.Value{"keyA": cty.StringVal("mapa")}),
		}
		call := NewStaticModuleCall(nil, func(v *Variable) (cty.Value, hcl.Diagnostics) {
			if in, ok := input[v.Name]; ok {
				return in, nil
			}
			return v.Default, nil
		}, "<testing>")
		mod, _ := NewModule([]*File{file}, nil, call, "dir")
		eval := NewStaticEvaluator(mod, call)

		locals := []struct {
			ident string
			value string
		}{
			{"var_str_def_ref", "sane default"},
			{"var_map_def_access", "A"},
			{"var_str_ref", "vara"},
			{"var_map_access", "mapa"},
		}
		for _, local := range locals {
			t.Run(local.ident, func(t *testing.T) {
				value, diags := eval.Evaluate(mod.Locals[local.ident].Expr, StaticIdentifier{})
				if diags.HasErrors() {
					t.Error(diags)
				}
				if value.AsString() != local.value {
					t.Errorf("Expected %s got %s", local.value, value.AsString())
				}
			})
		}
	})

	t.Run("Bad References", func(t *testing.T) {
		mod, _ := NewModule([]*File{file}, nil, RootModuleCallForTesting, "dir")
		eval := NewStaticEvaluator(mod, RootModuleCallForTesting)

		locals := []struct {
			ident string
			diag  string
		}{
			{"invalid_ref", "eval.tf:37,16-33: Dynamic value in static context; Unable to use invalid.attribute in static context"},
			{"unavailable_ref", "eval.tf:38,20-27: Dynamic value in static context; Unable to use foo.bar in static context"},
		}
		for _, local := range locals {
			t.Run(local.ident, func(t *testing.T) {
				badref := mod.Locals[local.ident]
				_, diags := eval.Evaluate(badref.Expr, StaticIdentifier{Subject: addrs.LocalValue{Name: badref.Name}, DeclRange: badref.DeclRange})
				assertExactDiagnostics(t, diags, []string{local.diag})
			})
		}
	})

	t.Run("Circular References", func(t *testing.T) {
		mod, _ := NewModule([]*File{file}, nil, RootModuleCallForTesting, "dir")
		eval := NewStaticEvaluator(mod, RootModuleCallForTesting)

		locals := []struct {
			ident string
			diags []string
		}{
			{"circular", []string{
				"eval.tf:41,2-27: Circular reference; local.circular is self referential",
				"eval.tf:41,2-27: Unable to compute static value; local.circular depends on local.circular which is not available",
			}},
			{"circular_ref", []string{
				"eval.tf:41,2-27: Circular reference; local.circular is self referential",
				"eval.tf:41,2-27: Unable to compute static value; local.circular_ref depends on local.circular which is not available",
			}},
			{"circular_a", []string{
				"eval.tf:44,2-31: Circular reference; local.circular_b is self referential",
				"eval.tf:43,2-31: Unable to compute static value; local.circular_b depends on local.circular_a which is not available",
				"eval.tf:44,2-31: Unable to compute static value; local.circular_a depends on local.circular_b which is not available",
			}},
		}
		for _, local := range locals {
			t.Run(local.ident, func(t *testing.T) {
				badref := mod.Locals[local.ident]
				_, diags := eval.Evaluate(badref.Expr, StaticIdentifier{Subject: addrs.LocalValue{Name: badref.Name}, DeclRange: badref.DeclRange})
				assertExactDiagnostics(t, diags, local.diags)
			})
		}
	})

	t.Run("Dependency chain", func(t *testing.T) {
		call := NewStaticModuleCall(nil, func(v *Variable) (cty.Value, hcl.Diagnostics) {
			return cty.DynamicVal, hcl.Diagnostics{&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Variable value not provided",
				Detail:   fmt.Sprintf("var.%s not included", v.Name),
				Subject:  v.DeclRange.Ptr(),
			}}
		}, "<testing>")
		mod, _ := NewModule([]*File{file}, nil, call, "dir")
		eval := NewStaticEvaluator(mod, call)

		badref := mod.Locals["ref_c"]
		_, diags := eval.Evaluate(badref.Expr, StaticIdentifier{Subject: addrs.LocalValue{Name: badref.Name}, DeclRange: badref.DeclRange})
		assertExactDiagnostics(t, diags, []string{
			"eval.tf:2,1-15: Variable value not provided; var.str not included",
			"eval.tf:2,1-15: Unable to compute static value; local.ref_a depends on var.str which is not available",
			"eval.tf:47,2-17: Unable to compute static value; local.ref_b depends on local.ref_a which is not available",
			"eval.tf:48,2-21: Unable to compute static value; local.ref_c depends on local.ref_b which is not available",
		})
	})

	t.Run("Missing References", func(t *testing.T) {
		mod, _ := NewModule([]*File{file}, nil, RootModuleCallForTesting, "dir")
		eval := NewStaticEvaluator(mod, RootModuleCallForTesting)

		locals := []struct {
			ident string
			diag  string
		}{
			{"local_missing", "eval.tf:52,18-31: Undefined local; Undefined local local.missing"},
			{"var_missing", "eval.tf:53,16-27: Undefined variable; Undefined variable var.missing"},
		}
		for _, local := range locals {
			t.Run(local.ident, func(t *testing.T) {
				badref := mod.Locals[local.ident]
				_, diags := eval.Evaluate(badref.Expr, StaticIdentifier{Subject: addrs.LocalValue{Name: badref.Name}, DeclRange: badref.DeclRange})
				assertExactDiagnostics(t, diags, []string{local.diag})
			})
		}
	})
}

func TestStaticEvaluator_DecodeExpression(t *testing.T) {
	parser := testParser(map[string]string{"eval.tf": ""})
	file, fileDiags := parser.LoadConfigFile("eval.tf")
	if fileDiags.HasErrors() {
		t.Fatal(fileDiags)
	}
	mod, _ := NewModule([]*File{file}, nil, RootModuleCallForTesting, "dir")
	eval := NewStaticEvaluator(mod, RootModuleCallForTesting)

	cases := []struct {
		expr  string
		diags []string
	}{{
		expr: `"static"`,
	}, {
		expr:  `count`,
		diags: []string{`eval.tf:1,1-6: Invalid reference; The "count" object cannot be accessed directly. Instead, access one of its attributes.`},
	}, {
		expr:  `module.foo.bar`,
		diags: []string{`eval.tf:1,1-15: Dynamic value in static context; Unable to use module.foo.bar in static context`},
	}}

	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			expr, _ := hclsyntax.ParseExpression([]byte(tc.expr), "eval.tf", hcl.InitialPos)
			var str string
			diags := eval.DecodeExpression(expr, StaticIdentifier{}, &str)
			assertExactDiagnostics(t, diags, tc.diags)
		})
	}
}
func TestStaticEvaluator_DecodeBlock(t *testing.T) {
	cases := []struct {
		ident string
		body  string
		diags []string
	}{{
		ident: "valid",
		body: `
locals {
	static = "static"
}

terraform {
	backend "valid" {
		thing = local.static
	}
}`,
	}, {
		ident: "badref",
		body: `
terraform {
	backend "badref" {
		thing = count
	}
}`,
		diags: []string{`eval.tf:4,11-16: Invalid reference; The "count" object cannot be accessed directly. Instead, access one of its attributes.`},
	}, {
		ident: "badeval",
		body: `
terraform {
	backend "badeval" {
		thing = module.foo.bar
	}
}`,
		diags: []string{`eval.tf:4,11-25: Dynamic value in static context; Unable to use module.foo.bar in static context`},
	}}

	for _, tc := range cases {
		t.Run(tc.ident, func(t *testing.T) {
			parser := testParser(map[string]string{"eval.tf": tc.body})
			file, fileDiags := parser.LoadConfigFile("eval.tf")
			if fileDiags.HasErrors() {
				t.Fatal(fileDiags)
			}

			mod, _ := NewModule([]*File{file}, nil, RootModuleCallForTesting, "dir")
			_, diags := mod.Backend.Decode(&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"thing": &configschema.Attribute{
						Type: cty.String,
					},
				},
			})

			assertExactDiagnostics(t, diags, tc.diags)
		})
	}
}
