// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcltest"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/lang/marks"
)

func TestCompileModuleInstanceOutputValuesValue(t *testing.T) {
	// This test is focused on whether individual output values get
	// compiled and evaluated correctly, testing the overall compilation
	// process only as a side-effect.
	// [TestCompileModuleInstanceOutputValues] is the opposite, testing whether
	// the overall compilation process is working without testing all of the
	// possible variations of the result.

	// This test also covers some of the behaviors of
	// [configgraph.OutputValue.Value] because that's where the effects of
	// most of the output value configuration options are actually implemented,
	// but the main goal of this test is to make sure that all of the
	// configuration arguments get correctly mapped to the fields of
	// the configgraph type, and not to test the configgraph type itself.
	//
	// configgraph.OutputValue behaviors that are not directly related to
	// arguments in an "output" block should NOT be tested here. Test them
	// in package configgraph instead.

	tests := map[string]struct {
		Config    *configs.Output
		WantValue cty.Value
		WantError string
	}{
		"minimal": {
			Config: &configs.Output{
				Name:           "foo",
				Expr:           hcltest.MockExprLiteral(cty.StringVal("value of foo")),
				ConstraintType: cty.DynamicPseudoType,
			},
			WantValue: cty.StringVal("value of foo"),
		},
		"marked value": {
			Config: &configs.Output{
				Name:           "foo",
				Expr:           hcltest.MockExprLiteral(cty.StringVal("value of foo").Mark("jovial")),
				ConstraintType: cty.DynamicPseudoType,
			},
			WantValue: cty.StringVal("value of foo").Mark("jovial"),
		},
		"explicit type conversion": {
			Config: &configs.Output{
				Name:           "foo",
				Expr:           hcltest.MockExprLiteral(cty.True.Mark("jovial")),
				TypeExpr:       hcltest.MockExprTraversalSrc(`string`),
				ConstraintType: cty.String,
			},
			WantValue: cty.StringVal("true").Mark("jovial"),
		},
		"sensitive": {
			Config: &configs.Output{
				Name:           "foo",
				Expr:           hcltest.MockExprLiteral(cty.StringVal("value of foo").Mark("melancholy")),
				ConstraintType: cty.DynamicPseudoType,
				Sensitive:      true,
				SensitiveSet:   true,
			},
			WantValue: cty.StringVal("value of foo").Mark("melancholy").Mark(marks.Sensitive),
		},
		"explicit type conversion sensitive": {
			Config: &configs.Output{
				Name:           "foo",
				Expr:           hcltest.MockExprLiteral(cty.False.Mark("ambivalent")),
				TypeExpr:       hcltest.MockExprTraversalSrc(`string`),
				ConstraintType: cty.String,
				Sensitive:      true,
				SensitiveSet:   true,
			},
			WantValue: cty.StringVal("false").Mark("ambivalent").Mark(marks.Sensitive),
		},
		"ephemeral": {
			Config: &configs.Output{
				Name:           "foo",
				Expr:           hcltest.MockExprLiteral(cty.StringVal("value of foo").Mark("impelled")),
				ConstraintType: cty.DynamicPseudoType,
				Ephemeral:      true,
				EphemeralSet:   true,
			},
			WantValue: cty.StringVal("value of foo").Mark("impelled").Mark(marks.Ephemeral),
		},
		"explicit type conversion ephemeral": {
			Config: &configs.Output{
				Name:           "foo",
				Expr:           hcltest.MockExprLiteral(cty.False.Mark("tempestuous")),
				TypeExpr:       hcltest.MockExprTraversalSrc(`string`),
				ConstraintType: cty.String,
				Ephemeral:      true,
				EphemeralSet:   true,
			},
			WantValue: cty.StringVal("false").Mark("tempestuous").Mark(marks.Ephemeral),
		},
		"sensitive and ephemeral": {
			Config: &configs.Output{
				Name:           "foo",
				Expr:           hcltest.MockExprLiteral(cty.StringVal("value of foo").Mark("sanguine")),
				ConstraintType: cty.DynamicPseudoType,
				Sensitive:      true,
				SensitiveSet:   true,
				Ephemeral:      true,
				EphemeralSet:   true,
			},
			WantValue: cty.StringVal("value of foo").Mark("sanguine").Mark(marks.Sensitive).Mark(marks.Ephemeral),
		},
		"explicit type conversion fails": {
			Config: &configs.Output{
				Name:           "foo",
				Expr:           hcltest.MockExprLiteral(cty.EmptyObjectVal),
				TypeExpr:       hcltest.MockExprTraversalSrc(`string`),
				ConstraintType: cty.String,
			},
			WantError: `string required, but have object`,
			WantValue: cty.UnknownVal(cty.String).Mark(exprs.EvalError),
		},
		"precondition passes": {
			Config: &configs.Output{
				Name:           "foo",
				Expr:           hcltest.MockExprLiteral(cty.EmptyObjectVal),
				ConstraintType: cty.DynamicPseudoType,
				Preconditions: []*configs.CheckRule{
					{
						Condition:    hcltest.MockExprLiteral(cty.True),
						ErrorMessage: hcltest.MockExprLiteral(cty.StringVal("Elephants are not permitted on the balcony.")),
					},
				},
			},
			WantValue: cty.EmptyObjectVal,
		},
		"precondition fails": {
			Config: &configs.Output{
				Name:           "foo",
				Expr:           hcltest.MockExprLiteral(cty.EmptyObjectVal),
				ConstraintType: cty.DynamicPseudoType,
				Preconditions: []*configs.CheckRule{
					{
						Condition:    hcltest.MockExprLiteral(cty.False),
						ErrorMessage: hcltest.MockExprLiteral(cty.StringVal("Capybaras are not permitted in the conservatory.")),
					},
				},
			},
			WantError: `Output value precondition failed`,
			WantValue: cty.DynamicVal.Mark(exprs.EvalError),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// The compile function is designed to compile all values from
			// a module at the same time, so for this test we'll just pretend
			// we're compiling a module with only a single output value.
			configs := map[string]*configs.Output{
				test.Config.Name: test.Config,
			}
			// No symbols are available for evaluation here, because we're
			// primarily testing what the implementation does with the resulting
			// value rather than the evaluation itself. Expression evaluation
			// is already well-tested elsewhere, and scope binding in particular
			// is tested in [TestCompileModuleInstanceOutputValues].
			declScope := exprs.FlatScopeForTesting(nil)
			gotOutputs := compileModuleInstanceOutputValues(t.Context(), configs, declScope, addrs.RootModuleInstance, nil)
			gotOutput := gotOutputs[test.Config.Addr()]
			if gotOutput == nil {
				t.Fatalf("no compiled version of %s in result", test.Config.Addr())
			}

			ctx := grapheval.ContextWithNewWorker(t.Context())
			ctx = grapheval.ContextWithRequestTracker(ctx, grapheval.NewRequestTracker())
			gotValue, gotDiags := gotOutput.Value(ctx)
			if test.WantError != "" {
				if !gotDiags.HasErrors() {
					t.Fatalf("unexpected success\nwant error substring: %s", test.WantError)
				}
				if gotErr := gotDiags.Err().Error(); !strings.Contains(gotErr, test.WantError) {
					t.Errorf("wrong error\ngot:\n%s\nwant substring: %s", gotErr, test.WantError)
				}
			} else if gotDiags.HasErrors() {
				t.Errorf("unexpected failure: %s", gotDiags.Err())
			}
			if !test.WantValue.RawEquals(gotValue) {
				t.Errorf("wrong result\ngot:  %#v\nwant: %#v", gotValue, test.WantValue)
			}
		})
	}
}

func TestCompileModuleInstanceOutputValues(t *testing.T) {
	// This test is for the overall compilation loop of
	// [compileModuleInstanceOutputValues], verifying that it produces something
	// reasonable for each of the configuration objects that it's given but
	// not trying to test all of the different behaviors an individual output
	// value can potentially have.
	// [TestCompileModuleInstanceOutputValuesValue] is the opposite, testing
	// the various details of the handling of single output values with
	// assorted settings.

	configs := map[string]*configs.Output{
		"literal": {
			Name:           "literal",
			Expr:           hcltest.MockExprLiteral(cty.StringVal("literal value")),
			TypeExpr:       hcltest.MockExprTraversalSrc(`string`),
			ConstraintType: cty.String,
		},
		"reference": {
			Name:           "reference",
			Expr:           hcltest.MockExprTraversalSrc(`a`),
			TypeExpr:       hcltest.MockExprTraversalSrc(`string`),
			ConstraintType: cty.String,
		},

		// We also test an explicit DependsOn in here because unfortunately
		// that mechanism (at the time of writing) requires the expression to
		// be something that [addrs.ParseRef] would accept and is resolvable
		// in the scope, and that's easier to arrange in this test where
		// we're already testing scope binding anyway.
		//
		// We're considering a more flexible interpretation of depends_on in
		// future where arbitrary expressions are allowed and we just copy
		// all of the dependency-related marks from the result onto the
		// final object. If we do that in future then we'll probably need to
		// take a slightly different approach in this test, and that's okay.
		"depends_on": {
			Name:           "depends_on",
			Expr:           hcltest.MockExprLiteral(cty.StringVal("depends_on value")),
			ConstraintType: cty.DynamicPseudoType,
			DependsOn: []hcl.Traversal{
				{
					hcl.TraverseRoot{Name: "local"},
					hcl.TraverseAttr{Name: "foo"},
				},
			},
		},
	}
	stubResourceInstMark := configgraph.NewResourceInstanceMark(&configgraph.ResourceInstance{
		Addr: addrs.MustParseResourceAddr("foo.bar").Absolute(addrs.RootModuleInstance).Instance(addrs.NoKey),
	})
	declScope := exprs.FlatScopeForTesting(map[string]cty.Value{
		"a": cty.StringVal("value of a"),
		"local": cty.ObjectVal(map[string]cty.Value{
			"foo": cty.StringVal("value of local.foo").Mark(stubResourceInstMark),
		}),
	})
	moduleInstAddr := addrs.RootModuleInstance.Child("foo", addrs.NoKey)
	extraMarks := cty.NewValueMarks("extra") // all result values should be annotated with these
	wantValues := map[string]cty.Value{
		"literal":    cty.StringVal("literal value").Mark("extra"),
		"reference":  cty.StringVal("value of a").Mark("extra"),
		"depends_on": cty.StringVal("depends_on value").Mark(stubResourceInstMark).Mark("extra"),
	}

	got := compileModuleInstanceOutputValues(t.Context(), configs, declScope, moduleInstAddr, extraMarks)
	if got, want := len(got), len(wantValues); got != want {
		t.Errorf("unexpected number of compiled results %d; want %d", got, want)
	}

	for name, wantValue := range wantValues {
		t.Run(name, func(t *testing.T) {
			lv := got[addrs.OutputValue{Name: name}]
			if lv == nil {
				t.Fatalf("missing %q value in result", name)
			}
			ctx := grapheval.ContextWithNewWorker(t.Context())
			ctx = grapheval.ContextWithRequestTracker(ctx, grapheval.NewRequestTracker())
			val, diags := lv.Value(ctx)
			if diags.HasErrors() {
				t.Errorf("unexpected errors from %q: %s", name, diags.Err())
			}
			if got, want := val, wantValue; !want.RawEquals(got) {
				t.Errorf("wrong value for %q\ngot:  %#v\nwant: %#v", name, got, want)
			}
			if got, want := lv.Addr, moduleInstAddr.OutputValue(name); !want.Equal(got) {
				t.Errorf("wrong address for %q\ngot:  %s\nwant: %s", name, got, want)
			}
		})
	}

}
