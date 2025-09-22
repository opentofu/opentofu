// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/go-test/deep"
	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty-debug/ctydebug"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/checks"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestNodeModuleVariablePath(t *testing.T) {
	n := &nodeModuleVariable{
		Addr: addrs.RootModuleInstance.InputVariable("foo"),
		Config: &configs.Variable{
			Name:           "foo",
			Type:           cty.String,
			ConstraintType: cty.String,
		},
	}

	want := addrs.RootModuleInstance
	got := n.Path()
	if got.String() != want.String() {
		t.Fatalf("wrong module address %s; want %s", got, want)
	}
}

func TestNodeModuleVariableReferenceableName(t *testing.T) {
	n := &nodeExpandModuleVariable{
		Addr: addrs.InputVariable{Name: "foo"},
		Config: &configs.Variable{
			Name:           "foo",
			Type:           cty.String,
			ConstraintType: cty.String,
		},
	}

	{
		expected := []addrs.Referenceable{
			addrs.InputVariable{Name: "foo"},
		}
		actual := n.ReferenceableAddrs()
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("%#v != %#v", actual, expected)
		}
	}

	{
		gotSelfPath, gotReferencePath := n.ReferenceOutside()
		wantSelfPath := addrs.RootModuleInstance
		wantReferencePath := addrs.RootModuleInstance
		if got, want := gotSelfPath.String(), wantSelfPath.String(); got != want {
			t.Errorf("wrong self path\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := gotReferencePath.String(), wantReferencePath.String(); got != want {
			t.Errorf("wrong reference path\ngot:  %s\nwant: %s", got, want)
		}
	}

}

func TestNodeModuleVariableReference(t *testing.T) {
	n := &nodeExpandModuleVariable{
		Addr:   addrs.InputVariable{Name: "foo"},
		Module: addrs.RootModule.Child("bar"),
		Config: &configs.Variable{
			Name:           "foo",
			Type:           cty.String,
			ConstraintType: cty.String,
		},
		Expr: &hclsyntax.ScopeTraversalExpr{
			Traversal: hcl.Traversal{
				hcl.TraverseRoot{Name: "var"},
				hcl.TraverseAttr{Name: "foo"},
			},
		},
	}

	want := []*addrs.Reference{
		{
			Subject: addrs.InputVariable{Name: "foo"},
		},
	}
	got := n.References()
	for _, problem := range deep.Equal(got, want) {
		t.Error(problem)
	}
}

func TestNodeModuleVariableReference_grandchild(t *testing.T) {
	n := &nodeExpandModuleVariable{
		Addr:   addrs.InputVariable{Name: "foo"},
		Module: addrs.RootModule.Child("bar"),
		Config: &configs.Variable{
			Name:           "foo",
			Type:           cty.String,
			ConstraintType: cty.String,
		},
		Expr: &hclsyntax.ScopeTraversalExpr{
			Traversal: hcl.Traversal{
				hcl.TraverseRoot{Name: "var"},
				hcl.TraverseAttr{Name: "foo"},
			},
		},
	}

	want := []*addrs.Reference{
		{
			Subject: addrs.InputVariable{Name: "foo"},
		},
	}
	got := n.References()
	for _, problem := range deep.Equal(got, want) {
		t.Error(problem)
	}
}

func TestNodeModuleVariableConstraints(t *testing.T) {
	// This is a little extra convoluted to poke at some edge cases that have cropped up in the past around
	// evaluating dependent nodes between the plan -> apply and destroy cycle.
	m := testModuleInline(t, map[string]string{
		"main.tf": `
			variable "input" {
				type = string
				validation {
					condition = var.input != ""
					error_message = "Input must not be empty."
				}
			}

			module "child" {
				source = "./child"
				input = var.input
			}

			provider "test" {
				alias = "secondary"
				test_string = module.child.output
			}

			resource "test_object" "resource" {
				provider = test.secondary
				test_string = "test string"
			}

		`,
		"child/main.tf": `
			variable "input" {
				type = string
				validation {
					condition = var.input != ""
					error_message = "Input must not be empty."
				}
			}
			provider "test" {
				test_string = "foo"
			}
			resource "test_object" "resource" {
				test_string = var.input
			}
			output "output" {
				value = test_object.resource.id
			}
		`,
	})

	checkableObjects := []addrs.Checkable{
		addrs.InputVariable{Name: "input"}.Absolute(addrs.RootModuleInstance),
		addrs.InputVariable{Name: "input"}.Absolute(addrs.RootModuleInstance.Child("child", addrs.NoKey)),
	}

	p := &MockProvider{
		GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
			Provider: providers.Schema{Block: simpleTestSchema()},
			ResourceTypes: map[string]providers.Schema{
				"test_object": providers.Schema{Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"id": {
							Type:     cty.String,
							Computed: true,
						},
						"test_string": {
							Type:     cty.String,
							Required: true,
						},
					},
				}},
			},
		},
	}
	p.ConfigureProviderFn = func(req providers.ConfigureProviderRequest) (resp providers.ConfigureProviderResponse) {
		if req.Config.GetAttr("test_string").IsNull() {
			resp.Diagnostics = resp.Diagnostics.Append(errors.New("missing test_string value"))
		}
		return resp
	}

	ctxOpts := &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): testProviderFuncFixed(p),
		},
	}

	t.Run("pass", func(t *testing.T) {
		ctx := testContext2(t, ctxOpts)
		plan, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
			Mode: plans.NormalMode,
			SetVariables: InputValues{
				"input": &InputValue{
					Value:      cty.StringVal("beep"),
					SourceType: ValueFromCLIArg,
				},
			},
		})
		assertNoDiagnostics(t, diags)

		for _, addr := range checkableObjects {
			result := plan.Checks.GetObjectResult(addr)
			if result == nil {
				t.Fatalf("no check result for %s in the plan", addr)
			}
			if got, want := result.Status, checks.StatusPass; got != want {
				t.Fatalf("wrong check status for %s during planning\ngot:  %s\nwant: %s", addr, got, want)
			}
		}

		state, diags := ctx.Apply(context.Background(), plan, m, nil)
		assertNoDiagnostics(t, diags)
		for _, addr := range checkableObjects {
			result := state.CheckResults.GetObjectResult(addr)
			if result == nil {
				t.Fatalf("no check result for %s in the final state", addr)
			}
			if got, want := result.Status, checks.StatusPass; got != want {
				t.Errorf("wrong check status for %s after apply\ngot:  %s\nwant: %s", addr, got, want)
			}
		}

		plan, diags = ctx.Plan(context.Background(), m, state, &PlanOpts{
			Mode: plans.DestroyMode,
			SetVariables: InputValues{
				"input": &InputValue{
					Value:      cty.StringVal("beep"),
					SourceType: ValueFromCLIArg,
				},
			},
		})
		assertNoDiagnostics(t, diags)

		state, diags = ctx.Apply(context.Background(), plan, m, nil)
		assertNoDiagnostics(t, diags)
		for _, addr := range checkableObjects {
			result := state.CheckResults.GetObjectResult(addr)
			if result == nil {
				t.Fatalf("no check result for %s in the final state", addr)
			}
			if got, want := result.Status, checks.StatusPass; got != want {
				t.Errorf("wrong check status for %s after apply\ngot:  %s\nwant: %s", addr, got, want)
			}
		}
	})

	t.Run("fail", func(t *testing.T) {
		ctx := testContext2(t, ctxOpts)
		_, diags := ctx.Plan(context.Background(), m, states.NewState(), &PlanOpts{
			Mode: plans.NormalMode,
			SetVariables: InputValues{
				"input": &InputValue{
					Value:      cty.StringVal(""),
					SourceType: ValueFromCLIArg,
				},
			},
		})
		if !diags.HasErrors() {
			t.Fatalf("succeeded; want error")
		}

		const wantSummary = "Invalid value for variable"
		found := false
		for _, diag := range diags {
			if diag.Severity() == tfdiags.Error && diag.Description().Summary == wantSummary {
				found = true
				break
			}
		}

		if !found {
			t.Fatalf("missing expected error\nwant summary: %s\ngot: %s", wantSummary, diags.Err().Error())
		}
	})
}

func TestNodeModuleVariable_warningDiags(t *testing.T) {
	t.Run("unused object attribute", func(t *testing.T) {
		n := &nodeModuleVariable{
			Addr: addrs.InputVariable{Name: "foo"}.Absolute(addrs.RootModuleInstance),
			Config: &configs.Variable{
				Name: "foo",
				ConstraintType: cty.Object(map[string]cty.Type{
					"foo": cty.String,
					"bar": cty.Object(map[string]cty.Type{"nested": cty.EmptyObject}),
				}),
			},
			Expr: &hclsyntax.ObjectConsExpr{
				SrcRange: hcl.Range{Filename: "context1.tofu"},
				Items: []hclsyntax.ObjectConsItem{
					{
						KeyExpr: &hclsyntax.LiteralValueExpr{
							Val: cty.StringVal("baz"),
							SrcRange: hcl.Range{
								Filename: "test1.tofu",
							},
						},
						ValueExpr: &hclsyntax.LiteralValueExpr{
							Val: cty.StringVal("..."),
						},
					},
					{
						KeyExpr: &hclsyntax.LiteralValueExpr{
							Val: cty.StringVal("bar"),
							SrcRange: hcl.Range{
								Filename: "test.tofu",
							},
						},
						ValueExpr: &hclsyntax.ObjectConsExpr{
							SrcRange: hcl.Range{Filename: "context2.tofu"},
							Items: []hclsyntax.ObjectConsItem{
								{
									KeyExpr: &hclsyntax.LiteralValueExpr{
										Val: cty.StringVal("beep"),
										SrcRange: hcl.Range{
											Filename: "test2.tofu",
										},
									},
									ValueExpr: &hclsyntax.LiteralValueExpr{
										Val: cty.StringVal("..."),
									},
								},
							},
						},
					},
				},
			},
			ModuleInstance: addrs.RootModuleInstance,
		}
		// We use the "ForRPC" representation of the diagnostics just because
		// it's more friendly for comparison and we care only about the
		// user-facing information in the diagnostics, not their concrete types.
		gotDiags := n.warningDiags().ForRPC()
		var wantDiags tfdiags.Diagnostics
		wantDiags = wantDiags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagWarning,
			Summary:  "Object attribute is ignored",
			Detail:   `The object type for input variable "foo" does not include an attribute named "baz", so this definition is unused. Did you mean to set attribute "bar" instead?`,
			Subject: &hcl.Range{
				Filename: "test1.tofu", // from synthetic source range in constructed expression above
			},
			Context: &hcl.Range{
				Filename: "context1.tofu", // from synthetic source range in constructed expression above
			},
		})
		wantDiags = wantDiags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagWarning,
			Summary:  "Object attribute is ignored",
			Detail:   `The object type for input variable "foo" nested value .bar does not include an attribute named "beep", so this definition is unused.`,
			Subject: &hcl.Range{
				Filename: "test2.tofu", // from synthetic source range in constructed expression above
			},
			Context: &hcl.Range{
				Filename: "context2.tofu", // from synthetic source range in constructed expression above
			},
		})
		wantDiags = wantDiags.ForRPC()
		if diff := cmp.Diff(wantDiags, gotDiags, ctydebug.CmpOptions); diff != "" {
			t.Error("wrong diagnostics\n" + diff)
		}
	})
}
