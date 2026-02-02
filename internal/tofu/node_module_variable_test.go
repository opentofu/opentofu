package tofu

import (
	"context"
	"errors"
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/checks"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu/testhelpers"
	"github.com/opentofu/opentofu/internal/tofu/variables"
	"github.com/zclconf/go-cty/cty"
)

func TestNodeModuleVariableConstraints(t *testing.T) {
	// This is a little extra convoluted to poke at some edge cases that have cropped up in the past around
	// evaluating dependent nodes between the plan -> apply and destroy cycle.
	m := testhelpers.TestModuleInline(t, map[string]string{
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

	p := &testhelpers.MockProvider{
		GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
			Provider: providers.Schema{Block: testhelpers.SimpleTestSchema()},
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
			SetVariables: variables.InputValues{
				"input": &variables.InputValue{
					Value:      cty.StringVal("beep"),
					SourceType: variables.ValueFromCLIArg,
				},
			},
		})
		testhelpers.AssertNoDiagnostics(t, diags)

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
		testhelpers.AssertNoDiagnostics(t, diags)
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
			SetVariables: variables.InputValues{
				"input": &variables.InputValue{
					Value:      cty.StringVal("beep"),
					SourceType: variables.ValueFromCLIArg,
				},
			},
		})
		testhelpers.AssertNoDiagnostics(t, diags)

		state, diags = ctx.Apply(context.Background(), plan, m, nil)
		testhelpers.AssertNoDiagnostics(t, diags)
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
			SetVariables: variables.InputValues{
				"input": &variables.InputValue{
					Value:      cty.StringVal(""),
					SourceType: variables.ValueFromCLIArg,
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
