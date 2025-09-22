// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package lint

import (
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty-debug/ctydebug"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestDiscardedObjectConstructorAttrs(t *testing.T) {
	tests := map[string]struct {
		exprSrc  string
		targetTy cty.Type

		want []DiscardedObjectConstructorAttr
	}{
		// Simple, shallow cases
		"unexpected attr for empty object type": {
			`{
				foo = "bar"
			}`,
			cty.EmptyObject,
			[]DiscardedObjectConstructorAttr{
				{
					Path: cty.GetAttrPath("foo"),
					NameRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 2, Column: 5, Byte: 6},
						End:   tfdiags.SourcePos{Line: 2, Column: 8, Byte: 9},
					},
					ContextRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 3, Column: 5, Byte: 22},
					},
					TargetType: cty.EmptyObject,
				},
			},
		},
		"unexpected attr for non-empty object type": {
			`{
				foo = "bar"
			}`,
			cty.Object(map[string]cty.Type{
				"bar": cty.String,
			}),
			[]DiscardedObjectConstructorAttr{
				{
					Path: cty.GetAttrPath("foo"),
					NameRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 2, Column: 5, Byte: 6},
						End:   tfdiags.SourcePos{Line: 2, Column: 8, Byte: 9},
					},
					ContextRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 3, Column: 5, Byte: 22},
					},
					TargetType: cty.Object(map[string]cty.Type{
						"bar": cty.String,
					}),
				},
			},
		},
		"empty constructor for empty object type": {
			`{}`,
			cty.EmptyObject,
			nil,
		},
		"primitive type": {
			// This case is irrelevant to this particular lint, so we're just
			// testing that it successfully returns nothing rather than doing
			// something undesirable like panicking.
			`"hello"`,
			cty.String,
			nil,
		},

		// Nested in list
		"nested in list, tuple-cons": {
			`[
				{foo = "bar"},
				{baz = "beep"},
			]`,
			cty.List(cty.EmptyObject),
			[]DiscardedObjectConstructorAttr{
				{
					Path: cty.IndexIntPath(0).GetAttr("foo"),
					NameRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 2, Column: 6, Byte: 7},
						End:   tfdiags.SourcePos{Line: 2, Column: 9, Byte: 10},
					},
					ContextRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 2, Column: 5, Byte: 6},
						End:   tfdiags.SourcePos{Line: 2, Column: 18, Byte: 19},
					},
					TargetType: cty.EmptyObject,
				},
				{
					Path: cty.IndexIntPath(1).GetAttr("baz"),
					NameRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 3, Column: 6, Byte: 26},
						End:   tfdiags.SourcePos{Line: 3, Column: 9, Byte: 29},
					},
					ContextRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 3, Column: 5, Byte: 25},
						End:   tfdiags.SourcePos{Line: 3, Column: 19, Byte: 39},
					},
					TargetType: cty.EmptyObject,
				},
			},
		},
		"nested in list, tuple-for": {
			`[
				for x in [] : {
					foo = "bar"
				}
			]`,
			cty.List(cty.EmptyObject),
			[]DiscardedObjectConstructorAttr{
				{
					Path: cty.IndexPath(cty.UnknownVal(cty.Number)).GetAttr("foo"),
					NameRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 3, Column: 6, Byte: 27},
						End:   tfdiags.SourcePos{Line: 3, Column: 9, Byte: 30},
					},
					ContextRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 2, Column: 19, Byte: 20},
						End:   tfdiags.SourcePos{Line: 4, Column: 6, Byte: 44},
					},
					TargetType: cty.EmptyObject,
				},
			},
		},
		"nested in list, object-for": {
			`{
				for x in [] : x => {
					foo = "bar"
				}
			}`,
			cty.List(cty.EmptyObject),
			// Can't define a list value using object-for, so no results here
			nil,
		},

		// Nested in set
		"nested in set, tuple-cons": {
			`[
				{foo = "bar"},
				{baz = "beep"},
			]`,
			cty.Set(cty.EmptyObject),
			[]DiscardedObjectConstructorAttr{
				{
					Path: cty.IndexIntPath(0).GetAttr("foo"),
					NameRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 2, Column: 6, Byte: 7},
						End:   tfdiags.SourcePos{Line: 2, Column: 9, Byte: 10},
					},
					ContextRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 2, Column: 5, Byte: 6},
						End:   tfdiags.SourcePos{Line: 2, Column: 18, Byte: 19},
					},
					TargetType: cty.EmptyObject,
				},
				{
					Path: cty.IndexIntPath(1).GetAttr("baz"),
					NameRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 3, Column: 6, Byte: 26},
						End:   tfdiags.SourcePos{Line: 3, Column: 9, Byte: 29},
					},
					ContextRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 3, Column: 5, Byte: 25},
						End:   tfdiags.SourcePos{Line: 3, Column: 19, Byte: 39},
					},
					TargetType: cty.EmptyObject,
				},
			},
		},
		"nested in set, tuple-for": {
			`[
				for x in [] : {
					foo = "bar"
				}
			]`,
			cty.Set(cty.EmptyObject),
			[]DiscardedObjectConstructorAttr{
				{
					Path: cty.IndexPath(cty.UnknownVal(cty.Number)).GetAttr("foo"),
					NameRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 3, Column: 6, Byte: 27},
						End:   tfdiags.SourcePos{Line: 3, Column: 9, Byte: 30},
					},
					ContextRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 2, Column: 19, Byte: 20},
						End:   tfdiags.SourcePos{Line: 4, Column: 6, Byte: 44},
					},
					TargetType: cty.EmptyObject,
				},
			},
		},
		"nested in set, object-for": {
			`{
				for x in [] : x => {
					foo = "bar"
				}
			}`,
			cty.Set(cty.EmptyObject),
			// Can't define a list value using object-for, so no results here
			nil,
		},

		// Nested in map
		"nested in map, object-cons": {
			`{
				a = {foo = "bar"},
				b = {baz = "beep"},
			}`,
			cty.Map(cty.EmptyObject),
			[]DiscardedObjectConstructorAttr{
				{
					Path: cty.IndexStringPath("a").GetAttr("foo"),
					NameRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 2, Column: 10, Byte: 11},
						End:   tfdiags.SourcePos{Line: 2, Column: 13, Byte: 14},
					},
					ContextRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 2, Column: 9, Byte: 10},
						End:   tfdiags.SourcePos{Line: 2, Column: 22, Byte: 23},
					},
					TargetType: cty.EmptyObject,
				},
				{
					Path: cty.IndexStringPath("b").GetAttr("baz"),
					NameRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 3, Column: 10, Byte: 34},
						End:   tfdiags.SourcePos{Line: 3, Column: 13, Byte: 37},
					},
					ContextRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 3, Column: 9, Byte: 33},
						End:   tfdiags.SourcePos{Line: 3, Column: 23, Byte: 47},
					},
					TargetType: cty.EmptyObject,
				},
			},
		},
		"nested in map, object-for": {
			`{
				for x in [] : x => {
					foo = "bar"
				}
			}`,
			cty.Map(cty.EmptyObject),
			[]DiscardedObjectConstructorAttr{
				{
					Path: cty.IndexPath(cty.UnknownVal(cty.String)).GetAttr("foo"),
					NameRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 3, Column: 6, Byte: 32},
						End:   tfdiags.SourcePos{Line: 3, Column: 9, Byte: 35},
					},
					ContextRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 2, Column: 24, Byte: 25},
						End:   tfdiags.SourcePos{Line: 4, Column: 6, Byte: 49},
					},
					TargetType: cty.EmptyObject,
				},
			},
		},
		"nested in map, tuple-for": {
			`[
				for x in [] : {
					foo = "bar"
				}
			]`,
			cty.Map(cty.EmptyObject),
			// Can't define a map value using tuple-for, so no results here
			nil,
		},

		// Nested in tuple
		"nested in tuple, tuple-cons": {
			`[
				{foo = "bar"},
				{baz = "beep"},
			]`,
			cty.Tuple([]cty.Type{
				cty.EmptyObject,
				cty.Object(map[string]cty.Type{"not_baz": cty.String}),
			}),
			[]DiscardedObjectConstructorAttr{
				{
					Path: cty.IndexIntPath(0).GetAttr("foo"),
					NameRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 2, Column: 6, Byte: 7},
						End:   tfdiags.SourcePos{Line: 2, Column: 9, Byte: 10},
					},
					ContextRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 2, Column: 5, Byte: 6},
						End:   tfdiags.SourcePos{Line: 2, Column: 18, Byte: 19},
					},
					TargetType: cty.EmptyObject,
				},
				{
					Path: cty.IndexIntPath(1).GetAttr("baz"),
					NameRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 3, Column: 6, Byte: 26},
						End:   tfdiags.SourcePos{Line: 3, Column: 9, Byte: 29},
					},
					ContextRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 3, Column: 5, Byte: 25},
						End:   tfdiags.SourcePos{Line: 3, Column: 19, Byte: 39},
					},
					TargetType: cty.Object(map[string]cty.Type{"not_baz": cty.String}),
				},
			},
		},
		"nested in tuple, tuple-for": {
			`[
				for x in [] : {
					foo = "bar"
				}
			]`,
			cty.Tuple([]cty.Type{
				cty.EmptyObject,
				cty.Object(map[string]cty.Type{"not_foo": cty.String}),
			}),
			// We don't do any recursive analysis of a tuple type defined
			// by a tuple-for expression, because we can't correlate the
			// dynamic elements of a for expression with the static elements
			// of a tuple type.
			nil,
		},

		// Nested in another object
		"nested in object-cons": {
			`{
				a = {
					foo = "bar"
				}
				b = {
					baz = "beep"
				}
				c = {}
			}`,
			cty.Object(map[string]cty.Type{
				"a": cty.EmptyObject,
				"b": cty.Object(map[string]cty.Type{"not_baz": cty.String}),
				// "c" intentionally omitted; we're testing a mixture of
				// both nested and non-nested at the same time.
			}),
			[]DiscardedObjectConstructorAttr{
				{
					Path: cty.GetAttrPath("a").GetAttr("foo"),
					NameRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 3, Column: 6, Byte: 17},
						End:   tfdiags.SourcePos{Line: 3, Column: 9, Byte: 20},
					},
					ContextRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 2, Column: 9, Byte: 10},
						End:   tfdiags.SourcePos{Line: 4, Column: 6, Byte: 34},
					},
					TargetType: cty.EmptyObject,
				},
				{
					Path: cty.GetAttrPath("b").GetAttr("baz"),
					NameRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 6, Column: 6, Byte: 50},
						End:   tfdiags.SourcePos{Line: 6, Column: 9, Byte: 53},
					},
					ContextRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 5, Column: 9, Byte: 43},
						End:   tfdiags.SourcePos{Line: 7, Column: 6, Byte: 68},
					},
					TargetType: cty.Object(map[string]cty.Type{"not_baz": cty.String}),
				},
				{
					Path: cty.GetAttrPath("c"),
					NameRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 8, Column: 5, Byte: 73},
						End:   tfdiags.SourcePos{Line: 8, Column: 6, Byte: 74},
					},
					ContextRange: tfdiags.SourceRange{
						Start: tfdiags.SourcePos{Line: 1, Column: 1, Byte: 0},
						End:   tfdiags.SourcePos{Line: 9, Column: 5, Byte: 84},
					},
					TargetType: cty.Object(map[string]cty.Type{
						"a": cty.EmptyObject,
						"b": cty.Object(map[string]cty.Type{"not_baz": cty.String}),
					}),
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			expr, hclDiags := hclsyntax.ParseExpression([]byte(test.exprSrc), "", hcl.InitialPos)
			if hclDiags.HasErrors() {
				t.Fatal("unexpected syntax errors: " + hclDiags.Error())
			}

			got := slices.Collect(DiscardedObjectConstructorAttrs(expr, test.targetTy))
			if diff := cmp.Diff(test.want, got, ctydebug.CmpOptions); diff != "" {
				t.Error("wrong result\n" + diff)
			}
		})
	}
}
