// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configschema

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/zclconf/go-cty/cty"
)

func TestBlock_WriteOnlyPaths(t *testing.T) {
	complexBlock := &Block{
		Attributes: map[string]*Attribute{
			"regular_attr":   {},
			"sensitive_attr": {Sensitive: true},
			"wo_attr":        {WriteOnly: true},
			"nested_single_attribute": {
				NestedType: &Object{
					Attributes: map[string]*Attribute{
						"regular_attr":   {},
						"sensitive_attr": {Sensitive: true},
						"wo_attr":        {WriteOnly: true},
					},
					Nesting: NestingSingle,
				},
			},
			"nested_set_attribute": {
				NestedType: &Object{
					Attributes: map[string]*Attribute{
						"regular_attr":   {},
						"sensitive_attr": {Sensitive: true},
						"wo_attr":        {WriteOnly: true},
					},
					Nesting: NestingSet,
				},
			},
			"nested_map_attribute": {
				NestedType: &Object{
					Attributes: map[string]*Attribute{
						"regular_attr":   {},
						"sensitive_attr": {Sensitive: true},
						"wo_attr":        {WriteOnly: true},
					},
					Nesting: NestingMap,
				},
			},
		},
		BlockTypes: map[string]*NestedBlock{
			"nested_single_block": {
				Block: Block{
					Attributes: map[string]*Attribute{
						"regular_attr":   {},
						"sensitive_attr": {Sensitive: true},
						"wo_attr":        {WriteOnly: true},
					},
				},
				Nesting: NestingSingle,
			},
			"nested_set_block": {
				Block: Block{
					Attributes: map[string]*Attribute{
						"wo_attr": {WriteOnly: true},
					},
				},
				Nesting: NestingSet,
			},
			"nested_map_block": {
				Block: Block{
					Attributes: map[string]*Attribute{
						"wo_attr": {WriteOnly: true},
					},
				},
				Nesting: NestingMap,
			},
		},
	}
	cases := map[string]struct {
		block *Block
		val   cty.Value
		want  []cty.Path
	}{
		"only attributes": {
			block: complexBlock,
			val: cty.ObjectVal(map[string]cty.Value{
				"regular_attr":            cty.StringVal("foo"),
				"sensitive_attr":          cty.StringVal("bar"),
				"wo_attr":                 cty.StringVal("baz"),
				"nested_single_attribute": cty.NullVal(cty.Object(map[string]cty.Type{})),
				"nested_set_attribute":    cty.NullVal(cty.Set(cty.String)),
				"nested_map_attribute":    cty.NullVal(cty.Map(cty.String)),
				"nested_single_block":     cty.NullVal(cty.Object(map[string]cty.Type{})),
				"nested_set_block":        cty.NullVal(cty.Set(cty.String)),
				"nested_map_block":        cty.NullVal(cty.Map(cty.String)),
			}),
			want: []cty.Path{
				cty.GetAttrPath("wo_attr"),
			},
		},
		"single nested attribute": {
			block: complexBlock,
			val: cty.ObjectVal(map[string]cty.Value{
				"regular_attr":   cty.NullVal(cty.String),
				"sensitive_attr": cty.NullVal(cty.String),
				"wo_attr":        cty.StringVal("baz"),
				"nested_single_attribute": cty.ObjectVal(map[string]cty.Value{
					"regular_attr":   cty.StringVal("foo"),
					"sensitive_attr": cty.StringVal("bar"),
					"wo_attr":        cty.StringVal("baz"),
				}),
				"nested_set_attribute": cty.NullVal(cty.Set(cty.String)),
				"nested_map_attribute": cty.NullVal(cty.Map(cty.String)),
				"nested_single_block":  cty.NullVal(cty.Object(map[string]cty.Type{})),
				"nested_set_block":     cty.NullVal(cty.Set(cty.String)),
				"nested_map_block":     cty.NullVal(cty.Map(cty.String)),
			}),
			want: []cty.Path{
				cty.GetAttrPath("wo_attr"),
				cty.GetAttrPath("nested_single_attribute").GetAttr("wo_attr"),
			},
		},
		"set nested attribute": {
			block: complexBlock,
			val: cty.ObjectVal(map[string]cty.Value{
				"regular_attr":            cty.NullVal(cty.String),
				"sensitive_attr":          cty.NullVal(cty.String),
				"wo_attr":                 cty.StringVal("baz"),
				"nested_single_attribute": cty.NullVal(cty.Object(map[string]cty.Type{})),
				"nested_set_attribute": cty.SetVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{"wo_attr": cty.StringVal("foo")}),
				}),
				"nested_map_attribute": cty.NullVal(cty.Map(cty.String)),
				"nested_single_block":  cty.NullVal(cty.Object(map[string]cty.Type{})),
				"nested_set_block":     cty.NullVal(cty.Set(cty.String)),
				"nested_map_block":     cty.NullVal(cty.Map(cty.String)),
			}),
			want: []cty.Path{
				cty.GetAttrPath("wo_attr"),
				cty.GetAttrPath("nested_set_attribute").Index(cty.ObjectVal(map[string]cty.Value{"wo_attr": cty.StringVal("foo")})).GetAttr("wo_attr"),
			},
		},
		"map nested attribute": {
			block: complexBlock,
			val: cty.ObjectVal(map[string]cty.Value{
				"regular_attr":            cty.NullVal(cty.String),
				"sensitive_attr":          cty.NullVal(cty.String),
				"wo_attr":                 cty.StringVal("baz"),
				"nested_single_attribute": cty.NullVal(cty.Object(map[string]cty.Type{})),
				"nested_set_attribute":    cty.NullVal(cty.Set(cty.String)),
				"nested_map_attribute": cty.MapVal(map[string]cty.Value{
					"wo_attr": cty.ObjectVal(map[string]cty.Value{"wo_attr": cty.StringVal("foo")}),
				}),
				"nested_single_block": cty.NullVal(cty.Object(map[string]cty.Type{})),
				"nested_set_block":    cty.NullVal(cty.Set(cty.String)),
				"nested_map_block":    cty.NullVal(cty.Map(cty.String)),
			}),
			want: []cty.Path{
				cty.GetAttrPath("wo_attr"),
				cty.GetAttrPath("nested_map_attribute").Index(cty.StringVal("wo_attr")).GetAttr("wo_attr"),
			},
		},
		"single nested block": {
			block: complexBlock,
			val: cty.ObjectVal(map[string]cty.Value{
				"regular_attr":            cty.NullVal(cty.String),
				"sensitive_attr":          cty.NullVal(cty.String),
				"wo_attr":                 cty.StringVal("baz"),
				"nested_single_attribute": cty.NullVal(cty.Object(map[string]cty.Type{})),
				"nested_set_attribute":    cty.NullVal(cty.Set(cty.String)),
				"nested_map_attribute":    cty.NullVal(cty.Map(cty.String)),
				"nested_single_block": cty.ObjectVal(map[string]cty.Value{
					"regular_attr":   cty.StringVal("foo"),
					"sensitive_attr": cty.StringVal("bar"),
					"wo_attr":        cty.StringVal("baz"),
				}),
				"nested_set_block": cty.NullVal(cty.Set(cty.String)),
				"nested_map_block": cty.NullVal(cty.Map(cty.String)),
			}),
			want: []cty.Path{
				cty.GetAttrPath("wo_attr"),
				cty.GetAttrPath("nested_single_block").GetAttr("wo_attr"),
			},
		},
		"set nested block": {
			block: complexBlock,
			val: cty.ObjectVal(map[string]cty.Value{
				"regular_attr":            cty.NullVal(cty.String),
				"sensitive_attr":          cty.NullVal(cty.String),
				"wo_attr":                 cty.StringVal("baz"),
				"nested_single_attribute": cty.NullVal(cty.Object(map[string]cty.Type{})),
				"nested_set_attribute":    cty.NullVal(cty.Set(cty.String)),
				"nested_map_attribute":    cty.NullVal(cty.Map(cty.String)),
				"nested_single_block":     cty.NullVal(cty.Object(map[string]cty.Type{})),
				"nested_set_block": cty.SetVal([]cty.Value{
					cty.StringVal("foo"),
				}),
				"nested_map_block": cty.NullVal(cty.Map(cty.String)),
			}),
			want: []cty.Path{
				cty.GetAttrPath("wo_attr"),
				cty.GetAttrPath("nested_set_block").IndexString("foo").GetAttr("wo_attr"),
			},
		},
		"map nested block": {
			block: complexBlock,
			val: cty.ObjectVal(map[string]cty.Value{
				"regular_attr":            cty.NullVal(cty.String),
				"sensitive_attr":          cty.NullVal(cty.String),
				"wo_attr":                 cty.StringVal("baz"),
				"nested_single_attribute": cty.NullVal(cty.Object(map[string]cty.Type{})),
				"nested_set_attribute":    cty.NullVal(cty.Set(cty.String)),
				"nested_map_attribute":    cty.NullVal(cty.Map(cty.String)),
				"nested_single_block":     cty.NullVal(cty.Object(map[string]cty.Type{})),
				"nested_set_block":        cty.NullVal(cty.Set(cty.String)),
				"nested_map_block": cty.MapVal(map[string]cty.Value{
					"wo_attr": cty.StringVal("foo"),
				}),
			}),
			want: []cty.Path{
				cty.GetAttrPath("wo_attr"),
				cty.GetAttrPath("nested_map_block").Index(cty.StringVal("wo_attr")).GetAttr("wo_attr"),
			},
		},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			got := tt.block.WriteOnlyPaths(tt.val, nil)
			gotPs := cty.NewPathSet(got...)
			wantPs := cty.NewPathSet(tt.want...)
			if !gotPs.Equal(wantPs) {
				diff := cmp.Diff(wantPs.List(), gotPs.List(), cmpopts.EquateComparable(cty.GetAttrStep{}, cty.IndexStep{}))
				t.Errorf("paths returned are not as expected:\n%s", diff)
			}
		})
	}
}

func TestBlock_PathSetContainsWriteOnly(t *testing.T) {
	complexBlock := &Block{
		Attributes: map[string]*Attribute{
			"regular_attr":   {},
			"sensitive_attr": {Sensitive: true},
			"wo_attr":        {WriteOnly: true},
			"nested_set_attribute": {
				NestedType: &Object{
					Attributes: map[string]*Attribute{
						"regular_attr":   {},
						"sensitive_attr": {Sensitive: true},
						"wo_attr":        {WriteOnly: true},
					},
					Nesting: NestingSet,
				},
			},
			"nested_map_attribute": {
				NestedType: &Object{
					Attributes: map[string]*Attribute{
						"regular_attr":   {},
						"sensitive_attr": {Sensitive: true},
						"wo_attr":        {WriteOnly: true},
					},
					Nesting: NestingMap,
				},
			},
		},
		BlockTypes: map[string]*NestedBlock{
			"nested_set_block": {
				Block: Block{
					Attributes: map[string]*Attribute{
						"wo_attr": {WriteOnly: true},
					},
				},
				Nesting: NestingSet,
			},
		},
	}
	cases := map[string]struct {
		block   *Block
		val     cty.Value
		pathSet cty.PathSet
		want    bool
	}{
		"PathSet pointing to a simple root attribute": {
			block: complexBlock,
			val: cty.ObjectVal(map[string]cty.Value{
				"regular_attr":         cty.StringVal("foo"),
				"sensitive_attr":       cty.StringVal("bar"),
				"wo_attr":              cty.StringVal("baz"),
				"nested_set_attribute": cty.NullVal(cty.Set(cty.String)),
				"nested_map_attribute": cty.NullVal(cty.Map(cty.String)),
				"nested_set_block":     cty.NullVal(cty.Set(cty.String)),
			}),
			pathSet: cty.NewPathSet([]cty.Path{
				cty.GetAttrPath("wo_attr"),
			}...),
			want: true,
		},
		"PathSet points to a path in a existing nested object": {
			block: complexBlock,
			val: cty.ObjectVal(map[string]cty.Value{
				"regular_attr":         cty.NullVal(cty.String),
				"sensitive_attr":       cty.NullVal(cty.String),
				"wo_attr":              cty.StringVal("baz"),
				"nested_set_attribute": cty.NullVal(cty.Set(cty.String)),
				"nested_map_attribute": cty.MapVal(map[string]cty.Value{
					"wo_attr": cty.ObjectVal(map[string]cty.Value{"wo_attr": cty.StringVal("foo")}),
				}),
				"nested_single_block": cty.NullVal(cty.Object(map[string]cty.Type{})),
				"nested_set_block":    cty.NullVal(cty.Set(cty.String)),
				"nested_map_block":    cty.NullVal(cty.Map(cty.String)),
			}),
			pathSet: cty.NewPathSet([]cty.Path{
				cty.GetAttrPath("nested_map_attribute").Index(cty.StringVal("wo_attr")).GetAttr("wo_attr"),
			}...),
			want: true,
		},
		"empty PathSet": {
			block: complexBlock,
			val: cty.ObjectVal(map[string]cty.Value{
				"regular_attr":   cty.NullVal(cty.String),
				"sensitive_attr": cty.NullVal(cty.String),
				"wo_attr":        cty.StringVal("baz"),
				"nested_set_attribute": cty.SetVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{"wo_attr": cty.ObjectVal(map[string]cty.Value{"wo_attr": cty.StringVal("foo")})}),
				}),
				"nested_map_attribute": cty.NullVal(cty.Map(cty.String)),
				"nested_single_block":  cty.NullVal(cty.Object(map[string]cty.Type{})),
				"nested_set_block":     cty.NullVal(cty.Set(cty.String)),
				"nested_map_block":     cty.NullVal(cty.Map(cty.String)),
			}),
			pathSet: cty.NewPathSet(),
			want:    false,
		},
		"PathSet points to unset root value": {
			block: complexBlock,
			val: cty.ObjectVal(map[string]cty.Value{
				"regular_attr":            cty.NullVal(cty.String),
				"sensitive_attr":          cty.NullVal(cty.String),
				"wo_attr":                 cty.NullVal(cty.String),
				"nested_single_attribute": cty.NullVal(cty.Object(map[string]cty.Type{})),
				"nested_set_attribute":    cty.NullVal(cty.Set(cty.String)),
				"nested_map_attribute": cty.MapVal(map[string]cty.Value{
					"wo_attr": cty.ObjectVal(map[string]cty.Value{"wo_attr": cty.StringVal("foo")}),
				}),
				"nested_single_block": cty.NullVal(cty.Object(map[string]cty.Type{})),
				"nested_set_block":    cty.NullVal(cty.Set(cty.String)),
				"nested_map_block":    cty.NullVal(cty.Map(cty.String)),
			}),
			pathSet: cty.NewPathSet([]cty.Path{
				cty.GetAttrPath("wo_attr"),
			}...),
			want: true,
		},
		"PathSet points to unset value from a nil block": {
			block: complexBlock,
			val: cty.ObjectVal(map[string]cty.Value{
				"regular_attr":            cty.NullVal(cty.String),
				"sensitive_attr":          cty.NullVal(cty.String),
				"wo_attr":                 cty.StringVal("baz"),
				"nested_single_attribute": cty.NullVal(cty.Object(map[string]cty.Type{})),
				"nested_set_attribute":    cty.NullVal(cty.Set(cty.String)),
				"nested_map_attribute":    cty.NullVal(cty.Map(cty.String)),
				"nested_single_block":     cty.NullVal(cty.Object(map[string]cty.Type{})),
				"nested_set_block":        cty.NullVal(cty.Set(cty.String)),
				"nested_map_block":        cty.NullVal(cty.Map(cty.String)),
			}),
			pathSet: cty.NewPathSet([]cty.Path{
				cty.GetAttrPath("nested_single_block").GetAttr("wo_attr"),
			}...),
			want: false,
		},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			got := tt.block.PathSetContainsWriteOnly(tt.val, tt.pathSet)
			if got != tt.want {
				existingPaths := tt.block.WriteOnlyPaths(tt.val, nil)
				gotPs := cty.NewPathSet(existingPaths...)
				diff := cmp.Diff(tt.pathSet.List(), gotPs.List(), cmpopts.EquateComparable(cty.GetAttrStep{}, cty.IndexStep{}))
				t.Errorf("wanted %t but got %t:\n%s", tt.want, got, diff)
			}
		})
	}
}
