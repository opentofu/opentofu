// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configschema

import (
	"testing"

	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/zclconf/go-cty/cty"
)

func TestBlockValueMarks(t *testing.T) {
	schema := &Block{
		Attributes: map[string]*Attribute{
			"unsensitive": {
				Type:     cty.String,
				Optional: true,
			},
			"sensitive": {
				Type:      cty.String,
				Sensitive: true,
			},
			"nested": {
				NestedType: &Object{
					Attributes: map[string]*Attribute{
						"boop": {
							Type: cty.String,
						},
						"honk": {
							Type:      cty.String,
							Sensitive: true,
						},
					},
					Nesting: NestingList,
				},
			},
		},

		BlockTypes: map[string]*NestedBlock{
			"list": {
				Nesting: NestingList,
				Block: Block{
					Attributes: map[string]*Attribute{
						"unsensitive": {
							Type:     cty.String,
							Optional: true,
						},
						"sensitive": {
							Type:      cty.String,
							Sensitive: true,
						},
					},
				},
			},
		},
	}
	ephemeralSchema := func(s *Block) *Block {
		cp := *s
		cp.Ephemeral = true
		return &cp
	}(schema)

	testCases := map[string]struct {
		schema *Block
		given  cty.Value
		expect cty.Value
	}{
		"unknown object": {
			schema,
			cty.UnknownVal(schema.ImpliedType()),
			cty.UnknownVal(schema.ImpliedType()),
		},
		"null object": {
			schema,
			cty.NullVal(schema.ImpliedType()),
			cty.NullVal(schema.ImpliedType()),
		},
		"object with unknown attributes and blocks": {
			schema,
			cty.ObjectVal(map[string]cty.Value{
				"sensitive":   cty.UnknownVal(cty.String),
				"unsensitive": cty.UnknownVal(cty.String),
				"nested": cty.NullVal(cty.List(cty.Object(map[string]cty.Type{
					"boop": cty.String,
					"honk": cty.String,
				}))),
				"list": cty.UnknownVal(schema.BlockTypes["list"].ImpliedType()),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"sensitive":   cty.UnknownVal(cty.String).Mark(marks.Sensitive),
				"unsensitive": cty.UnknownVal(cty.String),
				"nested": cty.NullVal(cty.List(cty.Object(map[string]cty.Type{
					"boop": cty.String,
					"honk": cty.String,
				}))),
				"list": cty.UnknownVal(schema.BlockTypes["list"].ImpliedType()),
			}),
		},
		"object with block value": {
			schema,
			cty.ObjectVal(map[string]cty.Value{
				"sensitive":   cty.NullVal(cty.String),
				"unsensitive": cty.UnknownVal(cty.String),
				"nested": cty.NullVal(cty.List(cty.Object(map[string]cty.Type{
					"boop": cty.String,
					"honk": cty.String,
				}))),
				"list": cty.ListVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"sensitive":   cty.UnknownVal(cty.String),
						"unsensitive": cty.UnknownVal(cty.String),
					}),
					cty.ObjectVal(map[string]cty.Value{
						"sensitive":   cty.NullVal(cty.String),
						"unsensitive": cty.NullVal(cty.String),
					}),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"sensitive":   cty.NullVal(cty.String).Mark(marks.Sensitive),
				"unsensitive": cty.UnknownVal(cty.String),
				"nested": cty.NullVal(cty.List(cty.Object(map[string]cty.Type{
					"boop": cty.String,
					"honk": cty.String,
				}))),
				"list": cty.ListVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"sensitive":   cty.UnknownVal(cty.String).Mark(marks.Sensitive),
						"unsensitive": cty.UnknownVal(cty.String),
					}),
					cty.ObjectVal(map[string]cty.Value{
						"sensitive":   cty.NullVal(cty.String).Mark(marks.Sensitive),
						"unsensitive": cty.NullVal(cty.String),
					}),
				}),
			}),
		},
		"object with known values and nested attribute": {
			schema,
			cty.ObjectVal(map[string]cty.Value{
				"sensitive":   cty.StringVal("foo"),
				"unsensitive": cty.StringVal("bar"),
				"nested": cty.ListVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"boop": cty.StringVal("foo"),
						"honk": cty.StringVal("bar"),
					}),
					cty.ObjectVal(map[string]cty.Value{
						"boop": cty.NullVal(cty.String),
						"honk": cty.NullVal(cty.String),
					}),
					cty.ObjectVal(map[string]cty.Value{
						"boop": cty.UnknownVal(cty.String),
						"honk": cty.UnknownVal(cty.String),
					}),
				}),
				"list": cty.NullVal(cty.List(cty.Object(map[string]cty.Type{
					"sensitive":   cty.String,
					"unsensitive": cty.String,
				}))),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"sensitive":   cty.StringVal("foo").Mark(marks.Sensitive),
				"unsensitive": cty.StringVal("bar"),
				"nested": cty.ListVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"boop": cty.StringVal("foo"),
						"honk": cty.StringVal("bar").Mark(marks.Sensitive),
					}),
					cty.ObjectVal(map[string]cty.Value{
						"boop": cty.NullVal(cty.String),
						"honk": cty.NullVal(cty.String).Mark(marks.Sensitive),
					}),
					cty.ObjectVal(map[string]cty.Value{
						"boop": cty.UnknownVal(cty.String),
						"honk": cty.UnknownVal(cty.String).Mark(marks.Sensitive),
					}),
				}),
				"list": cty.NullVal(cty.List(cty.Object(map[string]cty.Type{
					"sensitive":   cty.String,
					"unsensitive": cty.String,
				}))),
			}),
		},
		"object with known values and nested attribute for an ephemeral schema": {
			ephemeralSchema,
			cty.ObjectVal(map[string]cty.Value{
				"sensitive":   cty.StringVal("foo"),
				"unsensitive": cty.StringVal("bar"),
				"nested": cty.ListVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"boop": cty.StringVal("foo"),
						"honk": cty.StringVal("bar"),
					}),
					cty.ObjectVal(map[string]cty.Value{
						"boop": cty.NullVal(cty.String),
						"honk": cty.NullVal(cty.String),
					}),
					cty.ObjectVal(map[string]cty.Value{
						"boop": cty.UnknownVal(cty.String),
						"honk": cty.UnknownVal(cty.String),
					}),
				}),
				"list": cty.NullVal(cty.List(cty.Object(map[string]cty.Type{
					"sensitive":   cty.String,
					"unsensitive": cty.String,
				}))),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"sensitive":   cty.StringVal("foo").Mark(marks.Sensitive),
				"unsensitive": cty.StringVal("bar"),
				"nested": cty.ListVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"boop": cty.StringVal("foo"),
						"honk": cty.StringVal("bar").Mark(marks.Sensitive),
					}),
					cty.ObjectVal(map[string]cty.Value{
						"boop": cty.NullVal(cty.String),
						"honk": cty.NullVal(cty.String).Mark(marks.Sensitive),
					}),
					cty.ObjectVal(map[string]cty.Value{
						"boop": cty.UnknownVal(cty.String),
						"honk": cty.UnknownVal(cty.String).Mark(marks.Sensitive),
					}),
				}),
				"list": cty.NullVal(cty.List(cty.Object(map[string]cty.Type{
					"sensitive":   cty.String,
					"unsensitive": cty.String,
				}))),
			}).Mark(marks.Ephemeral),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got := tc.given.MarkWithPaths(tc.schema.ValueMarks(tc.given, nil))
			if !got.RawEquals(tc.expect) {
				t.Fatalf("\nexpected: %#v\ngot:      %#v\n", tc.expect, got)
			}
		})
	}
}
