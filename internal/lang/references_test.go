// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package lang

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/opentofu/opentofu/internal/addrs"
)

func TestReferencesInExpr(t *testing.T) {
	// Note for developers, this list is non-exhaustive right now
	// and there are many more expression types. This test is mainly to ensure that provider
	// defined function references are returned from references with parentheses
	// to resolve issue opentofu/opentofu#3401
	tests := map[string]struct {
		exprSrc      string
		wantFuncRefs []string // List of expected provider function references
	}{
		"no functions": {
			exprSrc:      `"literal"`,
			wantFuncRefs: nil,
		},
		"provider function without parentheses": {
			exprSrc:      `provider::testing::echo("hello")`,
			wantFuncRefs: []string{"provider::testing::echo"},
		},
		"provider function with parentheses": {
			exprSrc:      `(provider::testing::echo("hello"))`,
			wantFuncRefs: []string{"provider::testing::echo"},
		},
		"provider function in binary expression": {
			exprSrc:      `(provider::testing::add(1, 2)) + 3`,
			wantFuncRefs: []string{"provider::testing::add"},
		},
		"nested parentheses": {
			exprSrc:      `((provider::testing::echo("hello")))`,
			wantFuncRefs: []string{"provider::testing::echo"},
		},
		"provider function in conditional true": {
			exprSrc:      `true ? provider::testing::echo("yes") : "no"`,
			wantFuncRefs: []string{"provider::testing::echo"},
		},
		"provider function in conditional false": {
			exprSrc:      `false ? "yes" : provider::testing::echo("no")`,
			wantFuncRefs: []string{"provider::testing::echo"},
		},
		"provider function in conditional condition": {
			exprSrc:      `provider::testing::is_true() ? "yes" : "no"`,
			wantFuncRefs: []string{"provider::testing::is_true"},
		},
		"multiple provider functions": {
			exprSrc:      `(provider::testing::add(1, 2)) + (provider::testing::mul(3, 4))`,
			wantFuncRefs: []string{"provider::testing::add", "provider::testing::mul"},
		},
		"provider function with alias": {
			exprSrc:      `(provider::testing::custom::echo("hello"))`,
			wantFuncRefs: []string{"provider::testing::custom::echo"},
		},
		"core function not included": {
			exprSrc:      `(core::max(1, 2))`,
			wantFuncRefs: nil, // core functions are not provider functions
		},
		"builtin function not included": {
			exprSrc:      `(length([1, 2, 3]))`,
			wantFuncRefs: nil, // builtin functions are not provider functions
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			expr, diags := hclsyntax.ParseExpression([]byte(test.exprSrc), "test.tf", hcl.Pos{Line: 1, Column: 1})
			if diags.HasErrors() {
				t.Fatalf("Failed to parse expression: %s", diags.Error())
			}

			refs, refDiags := ReferencesInExpr(addrs.ParseRef, expr)
			if refDiags.HasErrors() {
				t.Errorf("Unexpected diagnostics: %s", refDiags.Err())
			}

			// Extract provider function references
			var gotFuncRefs []string
			for _, ref := range refs {
				if provFunc, ok := ref.Subject.(addrs.ProviderFunction); ok {
					gotFuncRefs = append(gotFuncRefs, provFunc.String())
				}
			}

			if diff := cmp.Diff(test.wantFuncRefs, gotFuncRefs); diff != "" {
				t.Errorf("Wrong provider function references (-want +got):\n%s", diff)
			}
		})
	}
}
