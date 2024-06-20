// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcltest"
	"github.com/opentofu/opentofu/internal/addrs"
)

func TestRemovedBlock_decode(t *testing.T) {
	blockRange := hcl.Range{
		Filename: "mock.tf",
		Start:    hcl.Pos{Line: 3, Column: 12, Byte: 27},
		End:      hcl.Pos{Line: 3, Column: 19, Byte: 34},
	}

	foo_expr := hcltest.MockExprTraversalSrc("test_instance.foo")
	mod_foo_expr := hcltest.MockExprTraversalSrc("module.foo")
	foo_index_expr := hcltest.MockExprTraversalSrc("test_instance.foo[1]")
	mod_boop_index_foo_expr := hcltest.MockExprTraversalSrc("module.boop[1].test_instance.foo")
	data_foo_expr := hcltest.MockExprTraversalSrc("data.test_instance.foo")

	tests := map[string]struct {
		input *hcl.Block
		want  *Removed
		err   string
	}{
		"success": {
			&hcl.Block{
				Type: "removed",
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"from": {
							Name: "from",
							Expr: foo_expr,
						},
					},
				}),
				DefRange: blockRange,
			},
			&Removed{
				From:      mustRemoveEndpointFromExpr(foo_expr),
				DeclRange: blockRange,
			},
			``,
		},
		"modules": {
			&hcl.Block{
				Type: "removed",
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"from": {
							Name: "from",
							Expr: mod_foo_expr,
						},
					},
				}),
				DefRange: blockRange,
			},
			&Removed{
				From:      mustRemoveEndpointFromExpr(mod_foo_expr),
				DeclRange: blockRange,
			},
			``,
		},
		"error: missing argument": {
			&hcl.Block{
				Type: "removed",
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{},
				}),
				DefRange: blockRange,
			},
			&Removed{
				DeclRange: blockRange,
			},
			"Missing required argument",
		},
		"error: indexed resources": {
			&hcl.Block{
				Type: "removed",
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"from": {
							Name: "from",
							Expr: foo_index_expr,
						},
					},
				}),
				DefRange: blockRange,
			},
			&Removed{
				DeclRange: blockRange,
			},
			"Resource instance address with keys is not allowed",
		},
		"error: indexed modules": {
			&hcl.Block{
				Type: "removed",
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"from": {
							Name: "from",
							Expr: mod_boop_index_foo_expr,
						},
					},
				}),
				DefRange: blockRange,
			},
			&Removed{
				DeclRange: blockRange,
			},
			"Module instance address with keys is not allowed",
		},
		"error: data address": {
			&hcl.Block{
				Type: "moved",
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"from": {
							Name: "from",
							Expr: data_foo_expr,
						},
					},
				}),
				DefRange: blockRange,
			},
			&Removed{
				DeclRange: blockRange,
			},
			"Data source address is not allowed",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, diags := decodeRemovedBlock(test.input)

			if diags.HasErrors() {
				if test.err == "" {
					t.Fatalf("unexpected error: %s", diags.Errs())
				}
				if gotErr := diags[0].Summary; gotErr != test.err {
					t.Errorf("wrong error, got %q, want %q", gotErr, test.err)
				}
			} else if test.err != "" {
				t.Fatal("expected error")
			}

			if !cmp.Equal(got, test.want, cmp.AllowUnexported(addrs.MoveEndpoint{})) {
				t.Fatalf("wrong result: %s", cmp.Diff(got, test.want))
			}
		})
	}
}

func TestRemovedBlock_inModule(t *testing.T) {
	parser := NewParser(nil)
	mod, diags := parser.LoadConfigDir("testdata/valid-modules/removed-blocks", RootModuleCallForTesting())
	if diags.HasErrors() {
		t.Errorf("unexpected error: %s", diags.Error())
	}

	var got []string
	for _, mc := range mod.Removed {
		got = append(got, mc.From.RelSubject.String())
	}
	want := []string{
		`test.foo`,
		`test.foo`,
		`module.a`,
		`module.a`,
		`test.foo`,
		`test.boop`,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("wrong addresses\n%s", diff)
	}
}

func mustRemoveEndpointFromExpr(expr hcl.Expression) *addrs.RemoveEndpoint {
	traversal, hcldiags := hcl.AbsTraversalForExpr(expr)
	if hcldiags.HasErrors() {
		panic(hcldiags.Errs())
	}

	ep, diags := addrs.ParseRemoveEndpoint(traversal)
	if diags.HasErrors() {
		panic(diags.Err())
	}

	return ep
}
