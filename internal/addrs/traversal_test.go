// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"fmt"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

func TestTraversalsEquivalent(t *testing.T) {
	tests := []struct {
		A, B       string
		Equivalent bool
	}{
		{
			`foo`,
			`foo`,
			true,
		},
		{
			`foo`,
			`foo.bar`,
			false,
		},
		{
			`foo.bar`,
			`foo`,
			false,
		},
		{
			`foo`,
			`bar`,
			false,
		},
		{
			`foo.bar`,
			`foo.bar`,
			true,
		},
		{
			`foo.bar`,
			`foo.baz`,
			false,
		},
		{
			`foo["bar"]`,
			`foo["bar"]`,
			true,
		},
		{
			`foo["bar"]`,
			`foo["baz"]`,
			false,
		},
		{
			`foo[0]`,
			`foo[0]`,
			true,
		},
		{
			`foo[0]`,
			`foo[1]`,
			false,
		},
		{
			`foo[0]`,
			`foo["0"]`,
			false,
		},
		{
			`foo["0"]`,
			`foo[0]`,
			false,
		},
		{
			// HCL considers these distinct syntactically but considers them
			// equivalent during expression evaluation, so whether to consider
			// these equivalent is unfortunately context-dependent. We take
			// the more conservative interpretation of considering them to
			// be distinct.
			`foo["bar"]`,
			`foo.bar`,
			false,
		},
		{
			// The following strings differ only in the level of unicode
			// normalization. HCL considers two strings to be equal if they
			// have identical unicode normalization.
			`foo["ba\u0301z"]`,
			`foo["b\u00e1z"]`,
			true,
		},
		{
			`foo[1.0]`,
			`foo[1]`,
			true,
		},
		{
			`foo[01]`,
			`foo[1]`,
			true,
		},
		{
			// A traversal with a non-integral numeric index is strange, but
			// is permitted by HCL syntactically. It would be rejected during
			// expression evaluation.
			`foo[1.2]`,
			`foo[1]`,
			false,
		},
		{
			// A traversal with a non-integral numeric index is strange, but
			// is permitted by HCL syntactically. It would be rejected during
			// expression evaluation.
			`foo[1.2]`,
			`foo[1.2]`,
			true,
		},
		{
			// Integers too large to fit into the significand of a float64
			// historically caused some grief for HCL and cty, but this should
			// be fixed now and so the following should compare as different.
			`foo[9223372036854775807]`,
			`foo[9223372036854775808]`,
			false,
		},
		{
			// As above, but these two _equal_ large integers should compare
			// as equivalent.
			`foo[9223372036854775807]`,
			`foo[9223372036854775807]`,
			true,
		},
		{
			`foo[3.14159265358979323846264338327950288419716939937510582097494459]`,
			`foo[3.14159265358979323846264338327950288419716939937510582097494459]`,
			true,
		},
		// HCL and cty also have some numeric comparison quirks with floats
		// that lack an exact base-2 representation and zero vs. negative zero,
		// but those quirks can't arise from parsing a traversal -- only from
		// dynamic expression evaluation -- so we don't need to (and cannot)
		// check them here.
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s ≡ %s", test.A, test.B), func(t *testing.T) {
			a, diags := hclsyntax.ParseTraversalAbs([]byte(test.A), "", hcl.InitialPos)
			if diags.HasErrors() {
				t.Fatalf("input A has invalid syntax: %s", diags.Error())
			}
			b, diags := hclsyntax.ParseTraversalAbs([]byte(test.B), "", hcl.InitialPos)
			if diags.HasErrors() {
				t.Fatalf("input B has invalid syntax: %s", diags.Error())
			}

			got := TraversalsEquivalent(a, b)
			if want := test.Equivalent; got != want {
				t.Errorf("wrong result\ninput A: %s\ninput B: %s\ngot:     %t\nwant:    %t", test.A, test.B, got, want)
			}
		})
	}
}
