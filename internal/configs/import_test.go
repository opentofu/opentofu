// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"reflect"
	"testing"

	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/parser"
	"github.com/zclconf/go-cty/cty"
)

var (
	typeComparer      = cmp.Comparer(cty.Type.Equals)
	valueComparer     = cmp.Comparer(cty.Value.RawEquals)
	traversalComparer = cmp.Comparer(traversalsAreEquivalent)
)

func TestImportBlock_decode(t *testing.T) {
	blockRange := hcl.Range{
		Filename: "mock.tf",
		Start:    hcl.Pos{Line: 3, Column: 12, Byte: 27},
		End:      hcl.Pos{Line: 3, Column: 19, Byte: 34},
	}
	pos := hcl.Pos{Line: 1, Column: 1}

	fooStrExpr, hclDiags := hclsyntax.ParseExpression([]byte("\"foo\""), "", pos)
	if hclDiags.HasErrors() {
		t.Fatal(hclDiags)
	}
	barExpr, hclDiags := hclsyntax.ParseExpression([]byte("test_instance.bar"), "", pos)
	if hclDiags.HasErrors() {
		t.Fatal(hclDiags)
	}

	barIndexExpr, hclDiags := hclsyntax.ParseExpression([]byte("test_instance.bar[\"one\"]"), "", pos)
	if hclDiags.HasErrors() {
		t.Fatal(hclDiags)
	}

	modBarExpr, hclDiags := hclsyntax.ParseExpression([]byte("module.bar.test_instance.bar"), "", pos)
	if hclDiags.HasErrors() {
		t.Fatal(hclDiags)
	}

	dynamicBarExpr, hclDiags := hclsyntax.ParseExpression([]byte("test_instance.bar[var.var1]"), "", pos)
	if hclDiags.HasErrors() {
		t.Fatal(hclDiags)
	}

	invalidExpr, hclDiags := hclsyntax.ParseExpression([]byte("var.var1 ? test_instance.bar : test_instance.foo"), "", pos)
	if hclDiags.HasErrors() {
		t.Fatal(hclDiags)
	}

	barResource := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "test_instance",
		Name: "bar",
	}

	tests := map[string]struct {
		input *parser.Import
		want  *Import
		err   string
	}{
		"success": {
			&parser.Import{
				ID: &hcl.Attribute{
					Name: "id",
					Expr: fooStrExpr,
				},
				To: &hcl.Attribute{
					Name: "to",
					Expr: barExpr,
				},
				DefRange: blockRange,
			},
			&Import{
				To: barExpr,
				ResolvedTo: &addrs.AbsResourceInstance{
					Resource: addrs.ResourceInstance{Resource: barResource},
				},
				StaticTo: addrs.ConfigResource{
					Resource: barResource,
				},
				ID:        fooStrExpr,
				DeclRange: blockRange,
			},
			``,
		},
		"indexed resources": {
			&parser.Import{
				ID: &hcl.Attribute{
					Name: "id",
					Expr: fooStrExpr,
				},
				To: &hcl.Attribute{
					Name: "to",
					Expr: barIndexExpr,
				},
				DefRange: blockRange,
			},
			&Import{
				To: barIndexExpr,
				StaticTo: addrs.ConfigResource{
					Resource: barResource,
				},
				ResolvedTo: &addrs.AbsResourceInstance{
					Resource: addrs.ResourceInstance{
						Resource: barResource,
						Key:      addrs.StringKey("one"),
					},
				},
				ID:        fooStrExpr,
				DeclRange: blockRange,
			},
			``,
		},
		"resource inside module": {
			&parser.Import{
				ID: &hcl.Attribute{
					Name: "id",
					Expr: fooStrExpr,
				},
				To: &hcl.Attribute{
					Name: "to",
					Expr: modBarExpr,
				},
				DefRange: blockRange,
			},
			&Import{
				To: modBarExpr,
				StaticTo: addrs.ConfigResource{
					Module:   addrs.Module{"bar"},
					Resource: barResource,
				},
				ResolvedTo: &addrs.AbsResourceInstance{
					Module: addrs.ModuleInstance{addrs.ModuleInstanceStep{
						Name: "bar",
					}},
					Resource: addrs.ResourceInstance{
						Resource: barResource,
					},
				},
				ID:        fooStrExpr,
				DeclRange: blockRange,
			},
			``,
		},
		"dynamic resource index": {
			&parser.Import{
				ID: &hcl.Attribute{
					Name: "id",
					Expr: fooStrExpr,
				},
				To: &hcl.Attribute{
					Name: "to",
					Expr: dynamicBarExpr,
				},
				DefRange: blockRange,
			},
			&Import{
				To: dynamicBarExpr,
				StaticTo: addrs.ConfigResource{
					Resource: barResource,
				},
				ID:        fooStrExpr,
				DeclRange: blockRange,
			},
			``,
		},
		"error: missing id argument": {
			&parser.Import{
				To: &hcl.Attribute{
					Name: "to",
					Expr: barExpr,
				},
				DefRange: blockRange,
			},
			&Import{
				To: barExpr,
				ResolvedTo: &addrs.AbsResourceInstance{
					Resource: addrs.ResourceInstance{Resource: barResource},
				},
				StaticTo: addrs.ConfigResource{
					Resource: barResource,
				},
				DeclRange: blockRange,
			},
			"Missing required argument",
		},
		"error: missing to argument": {
			&parser.Import{
				ID: &hcl.Attribute{
					Name: "id",
					Expr: fooStrExpr,
				},
				DefRange: blockRange,
			},
			&Import{
				ID:        fooStrExpr,
				DeclRange: blockRange,
			},
			"Missing required argument",
		},
		"error: invalid import address": {
			&parser.Import{
				ID: &hcl.Attribute{
					Name: "id",
					Expr: fooStrExpr,
				},
				To: &hcl.Attribute{
					Name: "to",
					Expr: invalidExpr,
				},
				DefRange: blockRange,
			},
			&Import{
				To:        invalidExpr,
				ID:        fooStrExpr,
				DeclRange: blockRange,
			},
			"Invalid import address expression",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, diags := decodeImportBlock(test.input)

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

			if !cmp.Equal(got, test.want, typeComparer, valueComparer, traversalComparer) {
				t.Fatalf("wrong result: %s", cmp.Diff(got, test.want, typeComparer, valueComparer, traversalComparer))
			}
		})
	}
}

// Taken from traversalsAreEquivalent of hcl/v2
func traversalsAreEquivalent(a, b hcl.Traversal) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		aStep := a[i]
		bStep := b[i]

		if reflect.TypeOf(aStep) != reflect.TypeOf(bStep) {
			return false
		}

		// We can now assume that both are of the same type.
		switch ts := aStep.(type) {

		case hcl.TraverseRoot:
			if bStep.(hcl.TraverseRoot).Name != ts.Name {
				return false
			}

		case hcl.TraverseAttr:
			if bStep.(hcl.TraverseAttr).Name != ts.Name {
				return false
			}

		case hcl.TraverseIndex:
			if !bStep.(hcl.TraverseIndex).Key.RawEquals(ts.Key) {
				return false
			}

		default:
			return false
		}
	}
	return true
}
