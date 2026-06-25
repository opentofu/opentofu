// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"context"
	"maps"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcltest"
	"github.com/zclconf/go-cty-debug/ctydebug"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestCompileInstanceSelectorSingleton(t *testing.T) {
	ctx := grapheval.ContextWithNewWorker(t.Context())
	selector := compileInstanceSelector(ctx, exprs.FlatScopeForTesting(nil), nil, nil, nil, dependsOn{})
	instsSeq, marks, diags := selector.Instances(ctx)
	insts := configgraph.MapMaybe(instsSeq, func(s configgraph.InstancesSeq) map[addrs.InstanceKey]instances.RepetitionData {
		return maps.Collect(s)
	})

	// There should always be exactly one instance with no instance key and
	// no per-instance values.
	wantInsts := configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{
		addrs.NoKey: {},
	})
	if diff := cmp.Diff(wantInsts, insts, ctydebug.CmpOptions); diff != "" {
		t.Error("wrong instances:\n" + diff)
	}
	if len(marks) != 0 {
		t.Errorf("unexpected marks: %#v", marks)
	}
	if len(diags) != 0 {
		t.Errorf("unexpected diagnostics: %s", diags.ErrWithWarnings().Error())
	}
}

func TestCompileInstanceSelectorForEach(t *testing.T) {
	// We have a small number of tests that use this scope just to prove that
	// the compileInstanceSelector function is making use of the scope we pass
	// into it, but the main logic we're testing here only cares about the final
	// value the expression evaluates to and so most of the test cases just use
	// constant-valued expressions for simplicity and readability.
	scope := exprs.FlatScopeForTesting(map[string]cty.Value{
		"empty_map": cty.MapValEmpty(cty.String),
		"empty_set": cty.MapValEmpty(cty.String),
		"map_with_a": cty.MapVal(map[string]cty.Value{
			"a": cty.StringVal("value of a"),
		}),
		"set_with_a": cty.SetVal([]cty.Value{cty.StringVal("a")}),
	})
	rng := hcl.Range{
		Start: hcl.InitialPos,
		End:   hcl.InitialPos,
	}
	diagsHasError := func(want string) func(*testing.T, tfdiags.Diagnostics) {
		return func(t *testing.T, diags tfdiags.Diagnostics) {
			if !diags.HasErrors() {
				t.Fatalf("unexpected success")
			}
			s := diags.Err().Error()
			if !strings.Contains(s, want) {
				t.Errorf("missing expected error\ngot:  %s\nwant: %s", s, want)
			}
		}
	}
	testCompileInstanceSelector(t,
		map[string]compileInstanceSelectorTest{
			// Maps
			"empty map inline": {
				hcl.StaticExpr(cty.MapValEmpty(cty.String), rng),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{}),
				nil,
				nil,
			},
			"empty map from scope": {
				hcltest.MockExprTraversalSrc(`empty_map`),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{}),
				nil,
				nil,
			},
			// This test covers what would be produced by:
			//    for_each = tomap({})
			// ...because in that case we don't have enough information to
			// predict the element type, so we just leave it unspecified.
			"empty map of unknown type inline": {
				hcl.StaticExpr(cty.MapValEmpty(cty.DynamicPseudoType), rng),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{}),
				nil,
				nil,
			},
			"map with one element from scope": {
				hcltest.MockExprTraversalSrc(`map_with_a`),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{
					addrs.StringKey("a"): {
						EachKey:   cty.StringVal("a"),
						EachValue: cty.StringVal("value of a"),
					},
				}),
				nil,
				nil,
			},
			"map with two elements": {
				hcl.StaticExpr(cty.MapVal(map[string]cty.Value{
					"a": cty.StringVal("value of a"),
					"b": cty.StringVal("value of b"),
				}), rng),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{
					addrs.StringKey("a"): {
						EachKey:   cty.StringVal("a"),
						EachValue: cty.StringVal("value of a"),
					},
					addrs.StringKey("b"): {
						EachKey:   cty.StringVal("b"),
						EachValue: cty.StringVal("value of b"),
					},
				}),
				nil,
				nil,
			},
			"empty map marked": {
				hcl.StaticExpr(cty.MapValEmpty(cty.String).Mark("!"), rng),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{}),
				// For this layer of the system we just have general-purpose
				// preservation of whatever marks were present. It's the caller's
				// responsibility to decide how to react to these marks, such
				// as e.g. enforcing a rule that the set of instances must not
				// be decided based on a sensitive value, because rules like
				// that ought to be consistent regardless of which language
				// edition is being used.
				cty.NewValueMarks("!"),
				nil,
			},
			"map that is marked with one element": {
				hcl.StaticExpr(cty.MapVal(map[string]cty.Value{"a": cty.True}).Mark("!"), rng),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{
					addrs.StringKey("a"): {
						// TODO: Should we transfer the marks onto these nested values automatically?
						EachKey:   cty.StringVal("a"),
						EachValue: cty.True,
					},
				}),
				cty.NewValueMarks("!"),
				nil,
			},
			"map that is unmarked with one marked element": {
				hcl.StaticExpr(cty.MapVal(map[string]cty.Value{"a": cty.True.Mark("!")}), rng),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{
					addrs.StringKey("a"): {
						EachKey:   cty.StringVal("a"),
						EachValue: cty.True.Mark("!"),
					},
				}),
				nil,
				nil,
			},
			"unknown map": {
				hcl.StaticExpr(cty.UnknownVal(cty.Map(cty.String)), rng),
				dependsOn{},
				nil, // instances are unknown
				nil,
				nil,
			},
			"null map": {
				hcl.StaticExpr(cty.NullVal(cty.Map(cty.String)), rng),
				dependsOn{},
				nil, // instances are unknown
				nil,
				diagsHasError("The for_each value must not be null."),
			},

			// Objects
			"empty object": {
				hcl.StaticExpr(cty.EmptyObjectVal, rng),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{}),
				nil,
				nil,
			},
			"object with one attribute": {
				hcl.StaticExpr(cty.ObjectVal(map[string]cty.Value{
					"a": cty.StringVal("value of a"),
				}), rng),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{
					addrs.StringKey("a"): {
						EachKey:   cty.StringVal("a"),
						EachValue: cty.StringVal("value of a"),
					},
				}),
				nil,
				nil,
			},
			"object with two attributes": {
				hcl.StaticExpr(cty.ObjectVal(map[string]cty.Value{
					"a": cty.StringVal("value of a"),
					"b": cty.StringVal("value of b"),
				}), rng),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{
					addrs.StringKey("a"): {
						EachKey:   cty.StringVal("a"),
						EachValue: cty.StringVal("value of a"),
					},
					addrs.StringKey("b"): {
						EachKey:   cty.StringVal("b"),
						EachValue: cty.StringVal("value of b"),
					},
				}),
				nil,
				nil,
			},
			"empty object marked": {
				hcl.StaticExpr(cty.EmptyObjectVal.Mark("!"), rng),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{}),
				// For this layer of the system we just have general-purpose
				// preservation of whatever marks were present. It's the caller's
				// responsibility to decide how to react to these marks, such
				// as e.g. enforcing a rule that the set of instances must not
				// be decided based on a sensitive value, because rules like
				// that ought to be consistent regardless of which language
				// edition is being used.
				cty.NewValueMarks("!"),
				nil,
			},
			"object that is marked with one attribute": {
				hcl.StaticExpr(cty.ObjectVal(map[string]cty.Value{"a": cty.True}).Mark("!"), rng),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{
					addrs.StringKey("a"): {
						// TODO: Should we transfer the marks onto these nested values automatically?
						EachKey:   cty.StringVal("a"),
						EachValue: cty.True,
					},
				}),
				cty.NewValueMarks("!"),
				nil,
			},
			"object that is unmarked with one marked attribute": {
				hcl.StaticExpr(cty.ObjectVal(map[string]cty.Value{"a": cty.True.Mark("!")}), rng),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{
					addrs.StringKey("a"): {
						EachKey:   cty.StringVal("a"),
						EachValue: cty.True.Mark("!"),
					},
				}),
				nil,
				nil,
			},
			"unknown empty object": {
				hcl.StaticExpr(cty.UnknownVal(cty.EmptyObject), rng),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{}),
				nil,
				nil,
			},
			"unknown object with two attributes": {
				hcl.StaticExpr(cty.UnknownVal(cty.Object(map[string]cty.Type{
					"a": cty.String,
					"b": cty.Bool,
				})), rng),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{
					addrs.StringKey("a"): {
						EachKey:   cty.StringVal("a"),
						EachValue: cty.UnknownVal(cty.String),
					},
					addrs.StringKey("b"): {
						EachKey:   cty.StringVal("b"),
						EachValue: cty.UnknownVal(cty.Bool),
					},
				}),
				nil,
				nil,
			},
			"null object": {
				hcl.StaticExpr(cty.NullVal(cty.EmptyObject), rng),
				dependsOn{},
				nil, // instances are unknown
				nil,
				diagsHasError("The for_each value must not be null."),
			},

			// Sets
			"empty set inline": {
				hcl.StaticExpr(cty.SetValEmpty(cty.String), rng),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{}),
				nil,
				nil,
			},
			"empty set from scope": {
				hcltest.MockExprTraversalSrc(`empty_set`),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{}),
				nil,
				nil,
			},
			"empty set of unknown type inline": {
				// This test covers what would be produced by:
				//    for_each = toset([])
				// ...because in that case we don't have enough information to
				// predict the element type, so we just leave it unspecified.
				hcl.StaticExpr(cty.SetValEmpty(cty.DynamicPseudoType), rng),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{}),
				nil,
				nil,
			},
			"set with one element from scope": {
				hcltest.MockExprTraversalSrc(`set_with_a`),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{
					addrs.StringKey("a"): {
						EachKey:   cty.StringVal("a"),
						EachValue: cty.StringVal("a"),
					},
				}),
				nil,
				nil,
			},
			"unknown set": {
				hcl.StaticExpr(cty.UnknownVal(cty.Set(cty.String)), rng),
				dependsOn{},
				nil, // instances are unknown
				nil,
				nil,
			},
			"null set": {
				hcl.StaticExpr(cty.NullVal(cty.Set(cty.String)), rng),
				dependsOn{},
				nil, // instances are unknown
				nil,
				diagsHasError("The for_each value must not be null."),
			},
			"set with null in it": {
				hcl.StaticExpr(cty.SetVal([]cty.Value{
					cty.StringVal("not null"),
					cty.NullVal(cty.String),
				}), rng),
				dependsOn{},
				nil, // instances are unknown
				nil,
				diagsHasError("a null element is not allowed"),
			},
			"set of non-string values": {
				hcl.StaticExpr(cty.SetVal([]cty.Value{cty.True}), rng),
				dependsOn{},
				nil,
				nil,
				diagsHasError("When using a set with for_each, the element type must be string because the element values will be used as instance keys."),
			},

			// Various other weird situations
			"empty list": {
				hcl.StaticExpr(cty.ListValEmpty(cty.String), rng),
				dependsOn{},
				nil,
				nil,
				diagsHasError("The for_each value must be either a mapping or a set of strings."),
			},
			"string": {
				hcl.StaticExpr(cty.StringVal("nope"), rng),
				dependsOn{},
				nil,
				nil,
				diagsHasError("The for_each value must be either a mapping or a set of strings."),
			},
			"unknown string": {
				hcl.StaticExpr(cty.UnknownVal(cty.String), rng),
				dependsOn{},
				nil,
				nil,
				// Value should be type-checked even when it's unknown
				diagsHasError("The for_each value must be either a mapping or a set of strings."),
			},
			"unknown type": {
				hcl.StaticExpr(cty.DynamicVal, rng),
				dependsOn{},
				nil, // instances are unknown
				nil,
				nil,
			},
			"marks from depends_on": {
				hcl.StaticExpr(cty.SetVal([]cty.Value{cty.StringVal("...")}), rng),
				dependsOnForTesting("marked"),
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{
					addrs.StringKey("..."): {
						// Note that neither of these is marked, but once these
						// results pass through the configgraph instance
						// compilation code _it_ will mark them both with the
						// same marks as the instance expander returned.
						EachKey:   cty.StringVal("..."),
						EachValue: cty.StringVal("..."),
					},
				}),
				cty.NewValueMarks("marked"),
				nil,
			},
		},
		func(ctx context.Context, e hcl.Expression, deps dependsOn) configgraph.InstanceSelector {
			return compileInstanceSelector(ctx, scope, e, nil, nil, deps)
		},
	)
}

func TestCompileInstanceSelectorCount(t *testing.T) {
	// We have a small number of tests that use this scope just to prove that
	// the compileInstanceSelector function is making use of the scope we pass
	// into it, but the main logic we're testing here only cares about the final
	// value the expression evaluates to and so most of the test cases just use
	// constant-valued expressions for simplicity and readability.
	scope := exprs.FlatScopeForTesting(map[string]cty.Value{
		"zero": cty.Zero,
		"one":  cty.NumberIntVal(1),
	})
	rng := hcl.Range{
		Start: hcl.InitialPos,
		End:   hcl.InitialPos,
	}
	diagsHasError := func(want string) func(*testing.T, tfdiags.Diagnostics) {
		return func(t *testing.T, diags tfdiags.Diagnostics) {
			if !diags.HasErrors() {
				t.Fatalf("unexpected success")
			}
			s := diags.Err().Error()
			if !strings.Contains(s, want) {
				t.Errorf("missing expected error\ngot:  %s\nwant: %s", s, want)
			}
		}
	}
	testCompileInstanceSelector(t,
		map[string]compileInstanceSelectorTest{
			"zero inline": {
				hcl.StaticExpr(cty.Zero, rng),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{}),
				nil,
				nil,
			},
			"zero from scope": {
				hcltest.MockExprTraversalSrc(`zero`),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{}),
				nil,
				nil,
			},
			"one inline": {
				hcl.StaticExpr(cty.NumberIntVal(1), rng),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{
					addrs.IntKey(0): {
						CountIndex: cty.Zero,
					},
				}),
				nil,
				nil,
			},
			"one from scope": {
				hcltest.MockExprTraversalSrc(`one`),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{
					addrs.IntKey(0): {
						CountIndex: cty.Zero,
					},
				}),
				nil,
				nil,
			},
			"three": {
				hcl.StaticExpr(cty.NumberIntVal(3), rng),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{
					addrs.IntKey(0): {
						CountIndex: cty.Zero,
					},
					addrs.IntKey(1): {
						CountIndex: cty.NumberIntVal(1),
					},
					addrs.IntKey(2): {
						CountIndex: cty.NumberIntVal(2),
					},
				}),
				nil,
				nil,
			},
			"three marked": {
				hcl.StaticExpr(cty.NumberIntVal(3).Mark("!"), rng),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{
					// TODO: Should we automatically propagate the mark to the
					// CountIndex values in here too?
					addrs.IntKey(0): {
						CountIndex: cty.Zero,
					},
					addrs.IntKey(1): {
						CountIndex: cty.NumberIntVal(1),
					},
					addrs.IntKey(2): {
						CountIndex: cty.NumberIntVal(2),
					},
				}),
				cty.NewValueMarks("!"),
				nil,
			},
			"unknown number": {
				hcl.StaticExpr(cty.UnknownVal(cty.Number), rng),
				dependsOn{},
				nil, // instances are unknown
				nil,
				nil,
			},
			"unknown type": {
				hcl.StaticExpr(cty.DynamicVal, rng),
				dependsOn{},
				nil, // instances are unknown
				nil,
				nil,
			},
			"not a number": {
				hcl.StaticExpr(cty.EmptyObjectVal, rng),
				dependsOn{},
				nil,
				nil,
				diagsHasError("number required, but have object."),
			},
			"unknown and not a number": {
				hcl.StaticExpr(cty.UnknownVal(cty.Bool), rng),
				dependsOn{},
				nil,
				nil,
				diagsHasError("number required, but have bool."),
			},
			"null number": {
				hcl.StaticExpr(cty.NullVal(cty.Number), rng),
				dependsOn{},
				nil,
				nil,
				diagsHasError("must not be null."),
			},
			"negative number": {
				hcl.StaticExpr(cty.NumberIntVal(-1), rng),
				dependsOn{},
				nil,
				nil,
				diagsHasError("must not be a negative number."),
			},
			"fractional number": {
				hcl.StaticExpr(cty.NumberFloatVal(0.5), rng),
				dependsOn{},
				nil,
				nil,
				diagsHasError("must be a whole number."),
			},
			"very large number": {
				hcl.StaticExpr(cty.MustParseNumberVal("99999999999999999999"), rng),
				dependsOn{},
				nil,
				nil,
				diagsHasError("must be between 0 and 2147483647, inclusive."),
			},
			"larger than maximum count": {
				hcl.StaticExpr(cty.NumberIntVal(maxCount+1), rng),
				dependsOn{},
				nil,
				nil,
				diagsHasError("must be between 0 and 2147483647, inclusive."),
			},
			"marks from depends_on": {
				hcl.StaticExpr(cty.NumberIntVal(0), rng),
				dependsOnForTesting("marked"),
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{}),
				cty.NewValueMarks("marked"),
				nil,
			},
		},
		func(ctx context.Context, e hcl.Expression, deps dependsOn) configgraph.InstanceSelector {
			return compileInstanceSelector(ctx, scope, nil, e, nil, deps)
		},
	)
}

func TestCompileInstanceSelectorEnabled(t *testing.T) {
	// We have a small number of tests that use this scope just to prove that
	// the compileInstanceSelector function is making use of the scope we pass
	// into it, but the main logic we're testing here only cares about the final
	// value the expression evaluates to and so most of the test cases just use
	// constant-valued expressions for simplicity and readability.
	scope := exprs.FlatScopeForTesting(map[string]cty.Value{
		"t": cty.True,  // Not "true" because that's a keyword in HCL
		"f": cty.False, // Not "false" because that's a keyword in HCL
	})
	rng := hcl.Range{
		Start: hcl.InitialPos,
		End:   hcl.InitialPos,
	}
	diagsHasError := func(want string) func(*testing.T, tfdiags.Diagnostics) {
		return func(t *testing.T, diags tfdiags.Diagnostics) {
			if !diags.HasErrors() {
				t.Fatalf("unexpected success")
			}
			s := diags.Err().Error()
			if !strings.Contains(s, want) {
				t.Errorf("missing expected error\ngot:  %s\nwant: %s", s, want)
			}
		}
	}
	testCompileInstanceSelector(t,
		map[string]compileInstanceSelectorTest{
			"false inline": {
				hcl.StaticExpr(cty.False, rng),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{}),
				nil,
				nil,
			},
			"false from scope": {
				hcltest.MockExprTraversalSrc(`f`),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{}),
				nil,
				nil,
			},
			"true inline": {
				hcl.StaticExpr(cty.True, rng),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{
					addrs.NoKey: {},
				}),
				nil,
				nil,
			},
			"true from scope": {
				hcltest.MockExprTraversalSrc(`t`),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{
					addrs.NoKey: {},
				}),
				nil,
				nil,
			},
			"true marked": {
				hcl.StaticExpr(cty.True.Mark("!"), rng),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{
					addrs.NoKey: {},
				}),
				cty.NewValueMarks("!"),
				nil,
			},
			"false marked": {
				hcl.StaticExpr(cty.False.Mark("!"), rng),
				dependsOn{},
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{}),
				cty.NewValueMarks("!"),
				nil,
			},
			"unknown bool": {
				hcl.StaticExpr(cty.UnknownVal(cty.Bool), rng),
				dependsOn{},
				nil, // instances are unknown
				nil,
				nil,
			},
			"unknown type": {
				hcl.StaticExpr(cty.DynamicVal, rng),
				dependsOn{},
				nil, // instances are unknown
				nil,
				nil,
			},
			"not a bool": {
				hcl.StaticExpr(cty.EmptyObjectVal, rng),
				dependsOn{},
				nil,
				nil,
				diagsHasError("bool required, but have object."),
			},
			"unknown and not a bool": {
				hcl.StaticExpr(cty.UnknownVal(cty.EmptyObject), rng),
				dependsOn{},
				nil,
				nil,
				diagsHasError("bool required, but have object."),
			},
			"null bool": {
				hcl.StaticExpr(cty.NullVal(cty.Bool), rng),
				dependsOn{},
				nil,
				nil,
				diagsHasError("must not be null."),
			},
			"marks from depends_on": {
				hcl.StaticExpr(cty.False, rng),
				dependsOnForTesting("marked"),
				configgraph.Known(map[addrs.InstanceKey]instances.RepetitionData{}),
				cty.NewValueMarks("marked"),
				nil,
			},
		},
		func(ctx context.Context, e hcl.Expression, deps dependsOn) configgraph.InstanceSelector {
			return compileInstanceSelector(ctx, scope, nil, nil, e, deps)
		},
	)
}

type compileInstanceSelectorTest struct {
	expr       hcl.Expression
	deps       dependsOn
	wantInsts  configgraph.Maybe[map[addrs.InstanceKey]instances.RepetitionData]
	wantMarks  cty.ValueMarks
	checkDiags func(*testing.T, tfdiags.Diagnostics)
}

func testCompileInstanceSelector(
	t *testing.T,
	tests map[string]compileInstanceSelectorTest,
	compile func(context.Context, hcl.Expression, dependsOn) configgraph.InstanceSelector,
) {
	t.Helper()

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := grapheval.ContextWithNewWorker(t.Context())

			selector := compile(ctx, test.expr, test.deps)
			instsSeq, marks, diags := selector.Instances(ctx)
			insts := configgraph.MapMaybe(instsSeq, func(s configgraph.InstancesSeq) map[addrs.InstanceKey]instances.RepetitionData {
				return maps.Collect(s)
			})

			if test.checkDiags != nil {
				if len(diags) == 0 {
					t.Error("unexpected success; wanted diagnostics")
				}
				// This callback can choose whether it reacts to errors by
				// aborting (using methods like t.Fatal) or if it still allows
				// us to continue to checking the instances and marks below.
				test.checkDiags(t, diags)
			} else if len(diags) != 0 {
				t.Fatalf("unexpected diagnostics: %s", diags.ErrWithWarnings().Error())
			}

			if diff := cmp.Diff(test.wantInsts, insts, ctydebug.CmpOptions); diff != "" {
				t.Error("wrong instances:\n" + diff)
			}
			if diff := cmp.Diff(test.wantMarks, marks, ctydebug.CmpOptions); diff != "" {
				t.Error("wrong marks:\n" + diff)
			}
		})
	}
}
