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
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/zclconf/go-cty/cty"
)

// This exercises most of the logic in StaticEvaluator and staticScopeData
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

	# Terraform 
	ws = terraform.workspace

	# Functions
	func = md5("my-string")
	missing_func = missing_fn("my-string")
	provider_func = provider::type::fn("my-string")
}

resource "foo" "bar" {}
	`

	parser := testParser(map[string]string{"eval.tf": testData})

	file, fileDiags := parser.LoadConfigFile("eval.tf")
	if fileDiags.HasErrors() {
		t.Fatal(fileDiags)
	}

	dummyIdentifier := StaticIdentifier{Subject: "local.test"}

	t.Run("Empty Eval", func(t *testing.T) {
		mod, _ := NewModule([]*File{file}, nil, RootModuleCallForTesting(), "dir", SelectiveLoadAll)
		emptyEval := StaticEvaluator{}

		// Expr with no traversals shouldn't access any fields
		value, diags := emptyEval.Evaluate(t.Context(), mod.Locals["static"].Expr, dummyIdentifier)
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
		_, _ = emptyEval.Evaluate(t.Context(), mod.Locals["static_ref"].Expr, dummyIdentifier)
	})

	t.Run("Simple static cases", func(t *testing.T) {
		mod, _ := NewModule([]*File{file}, nil, RootModuleCallForTesting(), "dir", SelectiveLoadAll)
		eval := NewStaticEvaluator(mod, RootModuleCallForTesting())

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
				value, diags := eval.Evaluate(t.Context(), mod.Locals[local.ident].Expr, dummyIdentifier)
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
		}, "<testing>", "")
		mod, _ := NewModule([]*File{file}, nil, call, "dir", SelectiveLoadAll)
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
				value, diags := eval.Evaluate(t.Context(), mod.Locals[local.ident].Expr, dummyIdentifier)
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
		mod, _ := NewModule([]*File{file}, nil, RootModuleCallForTesting(), "dir", SelectiveLoadAll)
		eval := NewStaticEvaluator(mod, RootModuleCallForTesting())

		locals := []struct {
			ident string
			diag  string
		}{
			{"invalid_ref", "eval.tf:37,16-33: Dynamic value in static context; Unable to use invalid.attribute in static context, which is required by local.invalid_ref"},
			{"unavailable_ref", "eval.tf:38,20-27: Dynamic value in static context; Unable to use foo.bar in static context, which is required by local.unavailable_ref"},
		}
		for _, local := range locals {
			t.Run(local.ident, func(t *testing.T) {
				badref := mod.Locals[local.ident]
				_, diags := eval.Evaluate(t.Context(), badref.Expr, StaticIdentifier{Subject: fmt.Sprintf("local.%s", badref.Name), DeclRange: badref.DeclRange})
				assertExactDiagnostics(t, diags, []string{local.diag})
			})
		}
	})

	t.Run("Circular References", func(t *testing.T) {
		mod, _ := NewModule([]*File{file}, nil, RootModuleCallForTesting(), "dir", SelectiveLoadAll)
		eval := NewStaticEvaluator(mod, RootModuleCallForTesting())

		locals := []struct {
			ident string
			diags []string
		}{
			{"circular", []string{
				"eval.tf:41,2-27: Circular reference; local.circular is self referential",
			}},
			{"circular_ref", []string{
				"eval.tf:41,2-27: Circular reference; local.circular is self referential",
				"eval.tf:42,2-31: Unable to compute static value; local.circular_ref depends on local.circular which is not available",
			}},
			{"circular_a", []string{
				"eval.tf:43,2-31: Unable to compute static value; local.circular_a depends on local.circular_b which is not available",
				"eval.tf:43,2-31: Circular reference; local.circular_a is self referential",
			}},
		}
		for _, local := range locals {
			t.Run(local.ident, func(t *testing.T) {
				badref := mod.Locals[local.ident]
				_, diags := eval.Evaluate(t.Context(), badref.Expr, StaticIdentifier{Subject: fmt.Sprintf("local.%s", badref.Name), DeclRange: badref.DeclRange})
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
		}, "<testing>", "")
		mod, _ := NewModule([]*File{file}, nil, call, "dir", SelectiveLoadAll)
		eval := NewStaticEvaluator(mod, call)

		badref := mod.Locals["ref_c"]
		_, diags := eval.Evaluate(t.Context(), badref.Expr, StaticIdentifier{Subject: fmt.Sprintf("local.%s", badref.Name), DeclRange: badref.DeclRange})
		assertExactDiagnostics(t, diags, []string{
			"eval.tf:2,1-15: Variable value not provided; var.str not included",
			"eval.tf:47,2-17: Unable to compute static value; local.ref_a depends on var.str which is not available",
			"eval.tf:48,2-21: Unable to compute static value; local.ref_b depends on local.ref_a which is not available",
			"eval.tf:49,2-21: Unable to compute static value; local.ref_c depends on local.ref_b which is not available",
		})
	})

	t.Run("Missing References", func(t *testing.T) {
		mod, _ := NewModule([]*File{file}, nil, RootModuleCallForTesting(), "dir", SelectiveLoadAll)
		eval := NewStaticEvaluator(mod, RootModuleCallForTesting())

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
				_, diags := eval.Evaluate(t.Context(), badref.Expr, StaticIdentifier{Subject: fmt.Sprintf("local.%s", badref.Name), DeclRange: badref.DeclRange})
				assertExactDiagnostics(t, diags, []string{local.diag})
			})
		}
	})

	t.Run("Workspace", func(t *testing.T) {
		call := NewStaticModuleCall(nil, nil, "<testing>", "my-workspace")
		mod, _ := NewModule([]*File{file}, nil, call, "dir", SelectiveLoadAll)
		eval := NewStaticEvaluator(mod, call)

		value, diags := eval.Evaluate(t.Context(), mod.Locals["ws"].Expr, dummyIdentifier)
		if diags.HasErrors() {
			t.Error(diags)
		}
		if value.AsString() != "my-workspace" {
			t.Errorf("Expected %s got %s", "my-workspace", value.AsString())
		}
	})

	t.Run("Functions", func(t *testing.T) {
		mod, _ := NewModule([]*File{file}, nil, RootModuleCallForTesting(), "dir", SelectiveLoadAll)
		eval := NewStaticEvaluator(mod, RootModuleCallForTesting())

		value, diags := eval.Evaluate(t.Context(), mod.Locals["func"].Expr, dummyIdentifier)
		if diags.HasErrors() {
			t.Error(diags)
		}
		if value.AsString() != "f887f41a53a46e2d40a3f8f86cacaaa2" {
			t.Errorf("Expected %s got %s", "f887f41a53a46e2d40a3f8f86cacaaa2", value.AsString())
		}

		_, diags = eval.Evaluate(t.Context(), mod.Locals["missing_func"].Expr, StaticIdentifier{Subject: fmt.Sprintf("local.%s", mod.Locals["missing_func"].Name), DeclRange: mod.Locals["missing_func"].DeclRange})
		assertExactDiagnostics(t, diags, []string{`eval.tf:60,17-27: Call to unknown function; There is no function named "missing_fn".`})
		_, diags = eval.Evaluate(t.Context(), mod.Locals["provider_func"].Expr, StaticIdentifier{Subject: fmt.Sprintf("local.%s", mod.Locals["provider_func"].Name), DeclRange: mod.Locals["provider_func"].DeclRange})
		assertExactDiagnostics(t, diags, []string{`eval.tf:61,18-36: Provider function in static context; Unable to use provider::type::fn in static context, which is required by local.provider_func`})
	})
}

func TestStaticEvaluator_DecodeExpression(t *testing.T) {
	dummyIdentifier := StaticIdentifier{Subject: "local.test"}
	parser := testParser(map[string]string{"eval.tf": ""})
	file, fileDiags := parser.LoadConfigFile("eval.tf")
	if fileDiags.HasErrors() {
		t.Fatal(fileDiags)
	}
	mod, _ := NewModule([]*File{file}, nil, RootModuleCallForTesting(), "dir", SelectiveLoadAll)
	eval := NewStaticEvaluator(mod, RootModuleCallForTesting())

	cases := []struct {
		expr  string
		diags []string
	}{{
		expr: `"static"`,
	}, {
		expr: `count`,
		diags: []string{
			`eval.tf:1,1-6: Invalid reference; The "count" object cannot be accessed directly. Instead, access one of its attributes.`,
			`:0,0-0: Dynamic value in static context; Unable to use count. in static context, which is required by local.test`,
		},
	}, {
		expr:  `module.foo.bar`,
		diags: []string{`eval.tf:1,1-15: Module output not supported in static context; Unable to use module.foo.bar in static context, which is required by local.test`},
	}}

	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			expr, _ := hclsyntax.ParseExpression([]byte(tc.expr), "eval.tf", hcl.InitialPos)
			var str string
			diags := eval.DecodeExpression(t.Context(), expr, dummyIdentifier, &str)
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
		diags: []string{`eval.tf:4,11-25: Module output not supported in static context; Unable to use module.foo.bar in static context, which is required by backend.badeval`},
	}, {
		ident: "sensitive",
		body: `
locals {
	sens = sensitive("magic")
}
terraform {
	backend "sensitive" {
		thing = local.sens
	}
}`,
		diags: []string{`eval.tf:6,2-21: Backend config contains sensitive values; The backend configuration is stored in .terraform/terraform.tfstate as well as plan files. It is recommended to instead supply sensitive credentials via backend specific environment variables`},
	}}

	schema := &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"thing": &configschema.Attribute{
				Type: cty.String,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.ident, func(t *testing.T) {
			parser := testParser(map[string]string{"eval.tf": tc.body})
			file, fileDiags := parser.LoadConfigFile("eval.tf")
			if fileDiags.HasErrors() {
				t.Fatal(fileDiags)
			}

			mod, _ := NewModule([]*File{file}, nil, RootModuleCallForTesting(), "dir", SelectiveLoadAll)

			_, diags := mod.Backend.Hash(schema)
			if diags.HasErrors() {
				assertExactDiagnostics(t, diags, tc.diags)
				return
			}

			_, diags = mod.Backend.Decode(schema)

			assertExactDiagnostics(t, diags, tc.diags)
		})
	}
}
