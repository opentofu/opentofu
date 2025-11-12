// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hcltest"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/zclconf/go-cty/cty"
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
		input         *hcl.Block
		want          *Removed
		err           string
		wantWarnDiags hcl.Diagnostics
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
			hcl.Diagnostics{}.Append(&hcl.Diagnostic{
				Severity: hcl.DiagWarning,
				Summary:  "Missing lifecycle from the removed block",
				Detail:   "It is recommended for each 'removed' block configured to have also the 'lifecycle' block defined. By not specifying if the resource should be destroyed or not, could lead to unwanted behavior.",
				Subject:  &blockRange,
			}),
		},
		"success-with-lifecycle-and-provisioner": {
			&hcl.Block{
				Type: "removed",
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"from": {
							Name: "from",
							Expr: foo_expr,
						},
					},
					Blocks: hcl.Blocks{
						{
							Type: "lifecycle",
							Body: hcltest.MockBody(&hcl.BodyContent{
								Attributes: hcl.Attributes{
									"destroy": &hcl.Attribute{Expr: &hclsyntax.LiteralValueExpr{Val: cty.BoolVal(true)}},
								},
							}),
						},
						{
							Type:   "provisioner",
							Labels: []string{"local-exec"},
							LabelRanges: []hcl.Range{
								{
									Filename: "file",
								},
							},
							Body: hcltest.MockBody(&hcl.BodyContent{
								Attributes: hcl.Attributes{
									"command": &hcl.Attribute{Expr: &hclsyntax.LiteralValueExpr{Val: cty.StringVal("echo 'test'")}},
									"when":    &hcl.Attribute{Expr: &hclsyntax.ScopeTraversalExpr{Traversal: hcl.Traversal{hcl.TraverseRoot{Name: "destroy"}}}},
								},
							}),
						},
					},
				}),
				DefRange: blockRange,
			},
			&Removed{
				From:       mustRemoveEndpointFromExpr(foo_expr),
				DeclRange:  blockRange,
				Destroy:    true,
				DestroySet: true,
				Provisioners: []*Provisioner{
					{
						Type:      "local-exec",
						When:      ProvisionerWhenDestroy,
						OnFailure: ProvisionerOnFailureFail,
						TypeRange: hcl.Range{Filename: "file"},
						Config: hcltest.MockBody(&hcl.BodyContent{
							Attributes: hcl.Attributes{
								"command": &hcl.Attribute{Expr: &hclsyntax.LiteralValueExpr{Val: cty.StringVal("echo 'test'")}},
							},
							Blocks: make(hcl.Blocks, 0),
						}),
					},
				},
			},
			``,
			hcl.Diagnostics{},
		},
		"success-with-lifecycle-not-destroy-and-provisioner": {
			&hcl.Block{
				Type: "removed",
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"from": {
							Name: "from",
							Expr: foo_expr,
						},
					},
					Blocks: hcl.Blocks{
						{
							Type: "lifecycle",
							Body: hcltest.MockBody(&hcl.BodyContent{
								Attributes: hcl.Attributes{
									"destroy": &hcl.Attribute{Expr: &hclsyntax.LiteralValueExpr{Val: cty.BoolVal(false)}},
								},
							}),
						},
						{
							Type:   "provisioner",
							Labels: []string{"local-exec"},
							LabelRanges: []hcl.Range{
								{
									Filename: "file",
								},
							},
							Body: hcltest.MockBody(&hcl.BodyContent{
								Attributes: hcl.Attributes{
									"command": &hcl.Attribute{Expr: &hclsyntax.LiteralValueExpr{Val: cty.StringVal("echo 'test'")}},
									"when":    &hcl.Attribute{Expr: &hclsyntax.ScopeTraversalExpr{Traversal: hcl.Traversal{hcl.TraverseRoot{Name: "destroy"}}}},
								},
							}),
						},
					},
				}),
				DefRange: blockRange,
			},
			&Removed{
				From:       mustRemoveEndpointFromExpr(foo_expr),
				DeclRange:  blockRange,
				Destroy:    false,
				DestroySet: true,
				Provisioners: []*Provisioner{
					{
						Type:      "local-exec",
						When:      ProvisionerWhenDestroy,
						OnFailure: ProvisionerOnFailureFail,
						TypeRange: hcl.Range{Filename: "file"},
						Config: hcltest.MockBody(&hcl.BodyContent{
							Attributes: hcl.Attributes{
								"command": &hcl.Attribute{Expr: &hclsyntax.LiteralValueExpr{Val: cty.StringVal("echo 'test'")}},
							},
							Blocks: make(hcl.Blocks, 0),
						}),
					},
				},
			},
			``,
			hcl.Diagnostics{}.Append(&hcl.Diagnostic{
				Severity: hcl.DiagWarning,
				Summary:  "Removed block provisioners will not be executed",
				Detail:   "The 'removed' block has marked the resource to be forgotten and not destroyed. Therefore, the provisioners configured for it will not be executed.",
				Subject:  &blockRange,
			}),
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
			hcl.Diagnostics{}.Append(&hcl.Diagnostic{
				Severity: hcl.DiagWarning,
				Summary:  "Missing lifecycle from the removed block",
				Detail:   "It is recommended for each 'removed' block configured to have also the 'lifecycle' block defined. By not specifying if the resource should be destroyed or not, could lead to unwanted behavior.",
				Subject:  &blockRange,
			}),
		},
		"error-removed-module-with-provisioner": {
			&hcl.Block{
				Type: "removed",
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"from": {
							Name: "from",
							Expr: mod_foo_expr,
						},
					},
					Blocks: hcl.Blocks{
						{
							Type:   "provisioner",
							Labels: []string{"local-exec"},
							LabelRanges: []hcl.Range{
								{
									Filename: "file",
								},
							},
							Body: hcltest.MockBody(&hcl.BodyContent{
								Attributes: hcl.Attributes{
									"command": &hcl.Attribute{Expr: &hclsyntax.LiteralValueExpr{Val: cty.StringVal("echo 'test'")}},
									"when":    &hcl.Attribute{Expr: hcltest.MockExprTraversalSrc("destroy")},
								},
							}),
						},
					},
				}),
				DefRange: blockRange,
			},
			&Removed{
				From:      mustRemoveEndpointFromExpr(mod_foo_expr),
				DeclRange: blockRange,
			},
			`Invalid "removed" block`,
			hcl.Diagnostics{},
		},
		"error-non-destroy-provisioner": {
			&hcl.Block{
				Type: "removed",
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"from": {
							Name: "from",
							Expr: foo_expr,
						},
					},
					Blocks: hcl.Blocks{
						{
							Type:   "provisioner",
							Labels: []string{"local-exec"},
							LabelRanges: []hcl.Range{
								{
									Filename: "file",
								},
							},
							Body: hcltest.MockBody(&hcl.BodyContent{
								Attributes: hcl.Attributes{
									"command": &hcl.Attribute{Expr: &hclsyntax.LiteralValueExpr{Val: cty.StringVal("echo 'test'")}},
									// Not configuring "when" attribute is defaulting to when=create so it needs to fail
								},
							}),
						},
					},
				}),
				DefRange: blockRange,
			},
			&Removed{
				From:      mustRemoveEndpointFromExpr(foo_expr),
				DeclRange: blockRange,
			},
			`Invalid "removed.provisioner" block`,
			hcl.Diagnostics{},
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
			hcl.Diagnostics{},
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
			hcl.Diagnostics{},
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
			hcl.Diagnostics{},
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
			hcl.Diagnostics{},
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

			if len(diags) > 0 && !diags.HasErrors() {
				if diff := cmp.Diff(diags, test.wantWarnDiags); diff != "" {
					t.Fatalf("unexpected diags: %s", diff)
				}
			} else if len(test.wantWarnDiags) > 0 {
				t.Fatalf("wanted diagnostics but got nothing")
			}

			if diff := cmp.Diff(test.want, got, cmp.AllowUnexported(addrs.MoveEndpoint{}), cmpopts.IgnoreUnexported(cty.Value{})); diff != "" {
				t.Fatalf("wrong result: %s", diff)
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
