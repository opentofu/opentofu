package configs

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/json"
	"github.com/zclconf/go-cty-debug/ctydebug"
	"github.com/zclconf/go-cty/cty"
)

func TestDecodeIgnoreChangesPatterns(t *testing.T) {
	// This is a narrow test covering only the unexported decodeIgnoreChangesPatterns
	// helper function. This function is used only when ignore_changes has a static
	// list expression assigned to it. This function does not deal with other
	// ignore_changes situations such as the special "all" keyword.

	type TestCase struct {
		inputExpr string
		want      []IgnoreChangesPattern
	}

	nativeSyntaxTests := map[string]TestCase{
		"empty": {
			`[]`,
			nil,
		},
		"just root attribute": {
			`[
				foo,
			]`,
			[]IgnoreChangesPattern{
				{
					traversal: hcl.Traversal{
						hcl.TraverseAttr{
							Name: "foo",
							SrcRange: hcl.Range{
								Start: hcl.Pos{Line: 2, Column: 5, Byte: 6},
								End:   hcl.Pos{Line: 2, Column: 8, Byte: 9},
							},
						},
					},
				},
			},
		},
		"two root attributes": {
			`[
				foo,
				bar,
			]`,
			[]IgnoreChangesPattern{
				{
					traversal: hcl.Traversal{
						hcl.TraverseAttr{
							Name: "foo",
							SrcRange: hcl.Range{
								Start: hcl.Pos{Line: 2, Column: 5, Byte: 6},
								End:   hcl.Pos{Line: 2, Column: 8, Byte: 9},
							},
						},
					},
				},
				{
					traversal: hcl.Traversal{
						hcl.TraverseAttr{
							Name: "bar",
							SrcRange: hcl.Range{
								Start: hcl.Pos{Line: 3, Column: 5, Byte: 15},
								End:   hcl.Pos{Line: 3, Column: 8, Byte: 18},
							},
						},
					},
				},
			},
		},
		"attribute in nested object": {
			`[
				foo.bar,
			]`,
			[]IgnoreChangesPattern{
				{
					traversal: hcl.Traversal{
						hcl.TraverseAttr{
							Name: "foo",
							SrcRange: hcl.Range{
								Start: hcl.Pos{Line: 2, Column: 5, Byte: 6},
								End:   hcl.Pos{Line: 2, Column: 8, Byte: 9},
							},
						},
						hcl.TraverseAttr{
							Name: "bar",
							SrcRange: hcl.Range{
								Start: hcl.Pos{Line: 2, Column: 8, Byte: 9},
								End:   hcl.Pos{Line: 2, Column: 12, Byte: 13},
							},
						},
					},
				},
			},
		},
		"element in nested list": {
			`[
				foo[0],
			]`,
			[]IgnoreChangesPattern{
				{
					traversal: hcl.Traversal{
						hcl.TraverseAttr{
							Name: "foo",
							SrcRange: hcl.Range{
								Start: hcl.Pos{Line: 2, Column: 5, Byte: 6},
								End:   hcl.Pos{Line: 2, Column: 8, Byte: 9},
							},
						},
						hcl.TraverseIndex{
							Key: cty.Zero,
							SrcRange: hcl.Range{
								Start: hcl.Pos{Line: 2, Column: 8, Byte: 9},
								End:   hcl.Pos{Line: 2, Column: 11, Byte: 12},
							},
						},
					},
				},
			},
		},
		"element in nested map": {
			`[
				foo["bar"],
			]`,
			[]IgnoreChangesPattern{
				{
					traversal: hcl.Traversal{
						hcl.TraverseAttr{
							Name: "foo",
							SrcRange: hcl.Range{
								Start: hcl.Pos{Line: 2, Column: 5, Byte: 6},
								End:   hcl.Pos{Line: 2, Column: 8, Byte: 9},
							},
						},
						hcl.TraverseIndex{
							Key: cty.StringVal("bar"),
							SrcRange: hcl.Range{
								Start: hcl.Pos{Line: 2, Column: 8, Byte: 9},
								End:   hcl.Pos{Line: 2, Column: 15, Byte: 16},
							},
						},
					},
				},
			},
		},
		"attribute of element in nested list": {
			`[
				foo[0].bar,
			]`,
			[]IgnoreChangesPattern{
				{
					traversal: hcl.Traversal{
						hcl.TraverseAttr{
							Name: "foo",
							SrcRange: hcl.Range{
								Start: hcl.Pos{Line: 2, Column: 5, Byte: 6},
								End:   hcl.Pos{Line: 2, Column: 8, Byte: 9},
							},
						},
						hcl.TraverseIndex{
							Key: cty.Zero,
							SrcRange: hcl.Range{
								Start: hcl.Pos{Line: 2, Column: 8, Byte: 9},
								End:   hcl.Pos{Line: 2, Column: 11, Byte: 12},
							},
						},
						hcl.TraverseAttr{
							Name: "bar",
							SrcRange: hcl.Range{
								Start: hcl.Pos{Line: 2, Column: 11, Byte: 12},
								End:   hcl.Pos{Line: 2, Column: 15, Byte: 16},
							},
						},
					},
				},
			},
		},
		"attribute of all elements in nested collection": {
			`[
				foo[*].bar,
			]`,
			[]IgnoreChangesPattern{
				{
					traversal: hcl.Traversal{
						hcl.TraverseAttr{
							Name: "foo",
							SrcRange: hcl.Range{
								Start: hcl.Pos{Line: 2, Column: 5, Byte: 6},
								End:   hcl.Pos{Line: 2, Column: 8, Byte: 9},
							},
						},
						hcl.TraverseSplat{
							SrcRange: hcl.Range{
								Start: hcl.Pos{Line: 2, Column: 8, Byte: 9},
								End:   hcl.Pos{Line: 2, Column: 11, Byte: 12},
							},
						},
						hcl.TraverseAttr{
							Name: "bar",
							SrcRange: hcl.Range{
								Start: hcl.Pos{Line: 2, Column: 11, Byte: 12},
								End:   hcl.Pos{Line: 2, Column: 15, Byte: 16},
							},
						},
					},
				},
			},
		},
		// TODO: If we decide to move forward with this prototype, there are
		// plenty more situations to test.
	}
	jsonSyntaxTests := map[string]TestCase{
		"empty": {
			`[]`,
			nil,
		},
		"attribute of all elements in nested collection": {
			`[
				"foo[*].bar"
			]`,
			[]IgnoreChangesPattern{
				{
					traversal: hcl.Traversal{
						hcl.TraverseAttr{
							Name: "foo",
							SrcRange: hcl.Range{
								Start: hcl.Pos{Line: 2, Column: 9, Byte: 6},
								End:   hcl.Pos{Line: 2, Column: 12, Byte: 9},
							},
						},
						hcl.TraverseSplat{
							SrcRange: hcl.Range{
								Start: hcl.Pos{Line: 2, Column: 12, Byte: 9},
								End:   hcl.Pos{Line: 2, Column: 15, Byte: 12},
							},
						},
						hcl.TraverseAttr{
							Name: "bar",
							SrcRange: hcl.Range{
								Start: hcl.Pos{Line: 2, Column: 15, Byte: 12},
								End:   hcl.Pos{Line: 2, Column: 19, Byte: 16},
							},
						},
					},
				},
			},
		},
		// TODO: If we decide to move forward with this prototype, there are
		// plenty more situations to test.
	}

	runTests := func(t *testing.T, tests map[string]TestCase, parse func(*testing.T, string) hcl.Expression) {
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				fullExpr := parse(t, test.inputExpr)
				exprs, diags := hcl.ExprList(fullExpr)
				if diags.HasErrors() {
					t.Fatalf("cannot interpret expression as static list:\n%s", diags.Error())
				}

				got, diags := decodeIgnoreChangesPatterns(exprs)
				if diags.HasErrors() {
					t.Fatalf("invalid patterns:\n%s", diags.Error())
				}

				cmpOpts := cmp.Options{
					cmp.AllowUnexported(IgnoreChangesPattern{}),
					cmpopts.IgnoreUnexported(
						hcl.TraverseAttr{},
						hcl.TraverseIndex{},
						hcl.TraverseSplat{},
					),
					ctydebug.CmpOptions,
				}
				if diff := cmp.Diff(test.want, got, cmpOpts); diff != "" {
					t.Errorf("wrong result\n%s", diff)
				}
			})
		}
	}
	parseNativeSyntax := func(t *testing.T, inputExpr string) hcl.Expression {
		fullExpr, diags := hclsyntax.ParseExpression([]byte(inputExpr), "", hcl.InitialPos)
		if diags.HasErrors() {
			t.Fatalf("invalid expression syntax:\n%s", diags.Error())
		}
		return fullExpr
	}
	parseJSONSyntax := func(t *testing.T, inputExpr string) hcl.Expression {
		fullExpr, diags := json.ParseExpression([]byte(inputExpr), "")
		if diags.HasErrors() {
			t.Fatalf("invalid expression syntax:\n%s", diags.Error())
		}
		return fullExpr
	}

	t.Run("native_syntax", func(t *testing.T) {
		runTests(t, nativeSyntaxTests, parseNativeSyntax)
	})
	t.Run("json_syntax", func(t *testing.T) {
		runTests(t, jsonSyntaxTests, parseJSONSyntax)
	})
}
