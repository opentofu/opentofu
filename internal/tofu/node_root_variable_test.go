// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcltest"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/checks"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/lang"
)

func TestNodeRootVariableExecute(t *testing.T) {
	t.Run("type conversion", func(t *testing.T) {
		evalCtx := new(MockEvalContext)

		n := &NodeRootVariable{
			Addr: addrs.InputVariable{Name: "foo"},
			Config: &configs.Variable{
				Name:           "foo",
				Type:           cty.String,
				ConstraintType: cty.String,
			},
			RawValue: &InputValue{
				Value:      cty.True,
				SourceType: ValueFromUnknown,
			},
		}

		evalCtx.ChecksState = checks.NewState(&configs.Config{
			Module: &configs.Module{
				Variables: map[string]*configs.Variable{
					"foo": n.Config,
				},
			},
		})

		diags := n.Execute(t.Context(), evalCtx, walkApply)
		if diags.HasErrors() {
			t.Fatalf("unexpected error: %s", diags.Err())
		}

		if !evalCtx.SetRootModuleArgumentCalled {
			t.Fatalf("ctx.SetRootModuleArgument wasn't called")
		}
		if got, want := evalCtx.SetRootModuleArgumentAddr.String(), "var.foo"; got != want {
			t.Errorf("wrong address for ctx.SetRootModuleArgument\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := evalCtx.SetRootModuleArgumentValue, cty.StringVal("true"); !want.RawEquals(got) {
			// NOTE: The given value was cty.Bool but the type constraint was
			// cty.String, so it was NodeRootVariable's responsibility to convert
			// as part of preparing the "final value".
			t.Errorf("wrong value for ctx.SetRootModuleArgument\ngot:  %#v\nwant: %#v", got, want)
		}
	})
	t.Run("validation", func(t *testing.T) {
		evalCtx := new(MockEvalContext)

		// The variable validation function gets called with OpenTofu's
		// built-in functions available, so we need a minimal scope just for
		// it to get the functions from.
		evalCtx.EvaluationScopeScope = &lang.Scope{}

		// We need to reimplement a _little_ bit of EvalContextBuiltin logic
		// here to get a similar effect with EvalContextMock just to get the
		// value to flow through here in a realistic way that'll make this test
		// useful.
		var finalVal cty.Value
		evalCtx.SetRootModuleArgumentFunc = func(addr addrs.InputVariable, v cty.Value) {
			if addr.Name == "foo" {
				t.Logf("set %s to %#v", addr.String(), v)
				finalVal = v
			}
		}
		evalCtx.GetVariableValueFunc = func(addr addrs.AbsInputVariableInstance) cty.Value {
			if addr.String() != "var.foo" {
				return cty.NilVal
			}
			t.Logf("reading final val for %s (%#v)", addr.String(), finalVal)
			return finalVal
		}

		n := &NodeRootVariable{
			Addr: addrs.InputVariable{Name: "foo"},
			Config: &configs.Variable{
				Name:           "foo",
				Type:           cty.Number,
				ConstraintType: cty.Number,
				Validations: []*configs.CheckRule{
					{
						Condition: fakeHCLExpressionFunc(func(ctx *hcl.EvalContext) (cty.Value, hcl.Diagnostics) {
							// This returns true only if the given variable value
							// is exactly cty.Number, which allows us to verify
							// that we were given the value _after_ type
							// conversion.
							// This had previously not been handled correctly,
							// as reported in:
							//     https://github.com/hashicorp/terraform/issues/29899
							vars := ctx.Variables["var"]
							if vars == cty.NilVal || !vars.Type().IsObjectType() || !vars.Type().HasAttribute("foo") {
								t.Logf("var.foo isn't available")
								return cty.False, nil
							}
							val := vars.GetAttr("foo")
							if val == cty.NilVal || val.Type() != cty.Number {
								t.Logf("var.foo is %#v; want a number", val)
								return cty.False, nil
							}
							return cty.True, nil
						}),
						ErrorMessage: hcltest.MockExprLiteral(cty.StringVal("Must be a number.")),
					},
				},
			},
			RawValue: &InputValue{
				// Note: This is a string, but the variable's type constraint
				// is number so it should be converted before use.
				Value:      cty.StringVal("5"),
				SourceType: ValueFromUnknown,
			},
		}

		ref := &nodeVariableReference{
			Addr:   n.Addr,
			Config: n.Config,
		}

		evalCtx.ChecksState = checks.NewState(&configs.Config{
			Module: &configs.Module{
				Variables: map[string]*configs.Variable{
					"foo": n.Config,
				},
			},
		})

		diags := n.Execute(t.Context(), evalCtx, walkApply)
		if diags.HasErrors() {
			t.Fatalf("unexpected error: %s", diags.Err())
		}

		g, err := ref.DynamicExpand(evalCtx)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		for _, v := range g.Vertices() {
			if ev, ok := v.(GraphNodeExecutable); ok {
				diags = ev.Execute(t.Context(), evalCtx, walkApply)
				if diags.HasErrors() {
					t.Fatalf("unexpected error: %s", diags.Err())
				}
			}
		}

		if !evalCtx.SetRootModuleArgumentCalled {
			t.Fatalf("ctx.SetRootModuleArgument wasn't called")
		}
		if got, want := evalCtx.SetRootModuleArgumentAddr.String(), "var.foo"; got != want {
			t.Errorf("wrong address for ctx.SetRootModuleArgument\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := evalCtx.SetRootModuleArgumentValue, cty.NumberIntVal(5); !want.RawEquals(got) {
			// NOTE: The given value was cty.Bool but the type constraint was
			// cty.String, so it was NodeRootVariable's responsibility to convert
			// as part of preparing the "final value".
			t.Errorf("wrong value for ctx.SetRootModuleArgument\ngot:  %#v\nwant: %#v", got, want)
		}
		if status := evalCtx.Checks().ObjectCheckStatus(n.Addr.Absolute(addrs.RootModuleInstance)); status != checks.StatusPass {
			t.Errorf("expected checks to pass but go %s instead", status)
		}
	})
}

// fakeHCLExpressionFunc is a fake implementation of hcl.Expression that just
// directly produces a value with direct Go code.
//
// An expression of this type has no references and so it cannot access any
// variables from the EvalContext unless something else arranges for them
// to be guaranteed available. For example, custom variable validations just
// unconditionally have access to the variable they are validating regardless
// of references.
type fakeHCLExpressionFunc func(*hcl.EvalContext) (cty.Value, hcl.Diagnostics)

var _ hcl.Expression = fakeHCLExpressionFunc(nil)

func (f fakeHCLExpressionFunc) Value(ctx *hcl.EvalContext) (cty.Value, hcl.Diagnostics) {
	return f(ctx)
}

func (f fakeHCLExpressionFunc) Variables() []hcl.Traversal {
	return nil
}

func (f fakeHCLExpressionFunc) Functions() []hcl.Traversal {
	return nil
}

func (f fakeHCLExpressionFunc) Range() hcl.Range {
	return hcl.Range{
		Filename: "fake",
		Start:    hcl.InitialPos,
		End:      hcl.InitialPos,
	}
}

func (f fakeHCLExpressionFunc) StartRange() hcl.Range {
	return f.Range()
}
