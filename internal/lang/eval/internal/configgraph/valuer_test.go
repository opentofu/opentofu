// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty-debug/ctydebug"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// testValuer is a reusable helper for testing the evaluation behavior of
// [exprs.Valuer] implementations.
//
// For each test it calls [exprs.Valuer.Value] and then compares the returned
// value and diagnostics to those given in the test description. It uses
// t.Run to start a subtest for each [valuerTest] so that each can fail
// independently of each other and of the parent test. In particular, a
// call to testValuer does not prevent subsequent caller code from running when
// it detects failures.
//
// Refer to the callers of this function elsewhere in this package for examples
// of how to use this.
func testValuer[T exprs.Valuer](t *testing.T, tests map[string]valuerTest[T]) {
	t.Helper()

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := grapheval.ContextWithNewWorker(t.Context())
			gotValue, gotDiags := test.object.Value(ctx)

			if !test.wantValue.RawEquals(gotValue) {
				t.Errorf(
					"wrong value\ngot:  %swant: %sdiff:\n%s",
					ctydebug.ValueString(gotValue),
					ctydebug.ValueString(test.wantValue),
					ctydebug.DiffValues(test.wantValue, gotValue),
				)
			}

			if len(gotDiags) == 0 && len(test.wantDiags) == 0 {
				return // two empty diags is equivalent enough
			}
			// We'll use the RPC representations both because it's a convenient
			// way to be agnostic about diagnostic implementation and because
			// this always returns implementations of [tfdiags.Diagnostic] that
			// are usually friendly to diffing with [cmp.Diff].
			gotDiags = gotDiags.ForRPC()
			gotDiags.Sort()
			wantDiags := test.wantDiags.ForRPC()
			wantDiags.Sort()
			if diff := cmp.Diff(wantDiags, gotDiags); diff != "" {
				t.Error("wrong diagnostics\n" + diff)
			}
		})
	}
}

// valuerTest is part of the input to [testValuer]
type valuerTest[T exprs.Valuer] struct {
	object    T
	wantValue cty.Value

	// wantDiags describes diagnostics that the Valuer implementation should
	// return.
	//
	// [testValuer] performs some normalization of both the wanted and actual
	// diagnostics for easier test authoring:
	// - It calls ForRPC on both so that they are normalized to use the
	//   "RPC-friendly" diagnostic type, which also happens to be useful for
	//   nice diffing. However, this means that Expression and EvalContext
	//   metadata on the diagnostics are not considered in the comparison.
	// - It sorts the two sets of diagnostics before comparing them, using
	//   our standard diagnostics ordering. However, the standard diagnostics
	//   ordering prefers to order by source location and many tests use
	//   constant values without source ranges and so the ordering might not
	//   be effective in all cases. To control ordering when needed, use
	//   [exprs.ConstantValuerWithSourceRange] to give each valuer a
	//   different fake source range to use so that the diagnostics can be
	//   ordered by the source positions.
	// - It treats an empty [tfdiags.Diagnostics] as equivalent to a nil one.
	wantDiags tfdiags.Diagnostics
}

// constantOnceValuer wraps the given value in a [OnceValuer], for convenient
// construction of values in test cases.
func constantOnceValuer(v cty.Value) *OnceValuer {
	return ValuerOnce(exprs.ConstantValuer(v))
}

// constantOnceValuerWithSource wraps the given value in a [OnceValuer], for
// convenient construction of values in test cases.
func constantOnceValuerWithSource(v cty.Value, rng tfdiags.SourceRange) *OnceValuer {
	return ValuerOnce(exprs.ConstantValuerWithSourceRange(v, rng))
}

// constantOnceValuerMap wraps each of the values in the given map into
// [OnceValuer]s and returns a new map, for convenient construction of
// maps of values in test cases.
func constantOnceValuerMap[T comparable](vs map[T]cty.Value) map[T]*OnceValuer {
	if len(vs) == 0 {
		return nil
	}
	ret := make(map[T]*OnceValuer, len(vs))
	for k, v := range vs {
		ret[k] = constantOnceValuer(v)
	}
	return ret
}

// diagsForTest is a helper for constructing a [tfdiags.Diagnostics] with
// a fixed set of values appended to it, which is intended mainly for
// constructing values for [valuerTest.wantDiags].
func diagsForTest(toAppend ...any) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	for _, v := range toAppend {
		diags = diags.Append(v)
	}
	return diags
}
