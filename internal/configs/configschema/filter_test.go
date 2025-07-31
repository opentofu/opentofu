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

func TestFilter(t *testing.T) {
	testCases := map[string]struct {
		schema          *Block
		filterAttribute FilterT[*Attribute]
		filterBlock     FilterT[*NestedBlock]
		want            *Block
	}{
		"empty": {
			schema:          &Block{},
			filterAttribute: FilterDeprecatedAttribute,
			filterBlock:     FilterDeprecatedBlock,
			want:            &Block{},
		},
		"noop": {
			schema: &Block{
				Attributes: map[string]*Attribute{
					"string": {
						Type:     cty.String,
						Required: true,
					},
				},
				BlockTypes: map[string]*NestedBlock{
					"list": {
						Nesting: NestingList,
						Block: Block{
							Attributes: map[string]*Attribute{
								"string": {
									Type:     cty.String,
									Required: true,
								},
							},
						},
					},
				},
			},
			filterAttribute: nil,
			filterBlock:     nil,
			want: &Block{
				Attributes: map[string]*Attribute{
					"string": {
						Type:     cty.String,
						Required: true,
					},
				},
				BlockTypes: map[string]*NestedBlock{
					"list": {
						Nesting: NestingList,
						Block: Block{
							Attributes: map[string]*Attribute{
								"string": {
									Type:     cty.String,
									Required: true,
								},
							},
						},
					},
				},
			},
		},
		"filter_deprecated": {
			schema: &Block{
				Attributes: map[string]*Attribute{
					"string": {
						Type:     cty.String,
						Optional: true,
					},
					"deprecated_string": {
						Type:       cty.String,
						Deprecated: true,
					},
					"nested": {
						NestedType: &Object{
							Attributes: map[string]*Attribute{
								"string": {
									Type: cty.String,
								},
								"deprecated_string": {
									Type:       cty.String,
									Deprecated: true,
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
								"string": {
									Type:     cty.String,
									Optional: true,
								},
							},
							Deprecated: true,
						},
					},
				},
			},
			filterAttribute: FilterDeprecatedAttribute,
			filterBlock:     FilterDeprecatedBlock,
			want: &Block{
				Attributes: map[string]*Attribute{
					"string": {
						Type:     cty.String,
						Optional: true,
					},
					"nested": {
						NestedType: &Object{
							Attributes: map[string]*Attribute{
								"string": {
									Type: cty.String,
								},
							},
							Nesting: NestingList,
						},
					},
				},
			},
		},
		"filter_read_only": {
			schema: &Block{
				Attributes: map[string]*Attribute{
					"string": {
						Type:     cty.String,
						Optional: true,
					},
					"read_only_string": {
						Type:     cty.String,
						Computed: true,
					},
					"nested": {
						NestedType: &Object{
							Attributes: map[string]*Attribute{
								"string": {
									Type:     cty.String,
									Optional: true,
								},
								"read_only_string": {
									Type:     cty.String,
									Computed: true,
								},
								"deeply_nested": {
									NestedType: &Object{
										Attributes: map[string]*Attribute{
											"number": {
												Type:     cty.Number,
												Required: true,
											},
											"read_only_number": {
												Type:     cty.Number,
												Computed: true,
											},
										},
										Nesting: NestingList,
									},
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
								"string": {
									Type:     cty.String,
									Optional: true,
								},
								"read_only_string": {
									Type:     cty.String,
									Computed: true,
								},
							},
						},
					},
				},
			},
			filterAttribute: FilterReadOnlyAttribute,
			filterBlock:     nil,
			want: &Block{
				Attributes: map[string]*Attribute{
					"string": {
						Type:     cty.String,
						Optional: true,
					},
					"nested": {
						NestedType: &Object{
							Attributes: map[string]*Attribute{
								"string": {
									Type:     cty.String,
									Optional: true,
								},
								"deeply_nested": {
									NestedType: &Object{
										Attributes: map[string]*Attribute{
											"number": {
												Type:     cty.Number,
												Required: true,
											},
										},
										Nesting: NestingList,
									},
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
								"string": {
									Type:     cty.String,
									Optional: true,
								},
							},
						},
					},
				},
			},
		},
		"filter_optional_computed_id": {
			schema: &Block{
				Attributes: map[string]*Attribute{
					"id": {
						Type:     cty.String,
						Optional: true,
						Computed: true,
					},
					"string": {
						Type:     cty.String,
						Optional: true,
						Computed: true,
					},
				},
			},
			filterAttribute: FilterHelperSchemaIdAttribute,
			filterBlock:     nil,
			want: &Block{
				Attributes: map[string]*Attribute{
					"string": {
						Type:     cty.String,
						Optional: true,
						Computed: true,
					},
				},
			},
		},
		"filter_computed_from_optional_block": {
			schema: &Block{
				Attributes: map[string]*Attribute{
					"id": {
						Type:     cty.String,
						Computed: true,
					},
					"nested_val": {
						Type:     cty.String,
						Optional: true,
						NestedType: &Object{
							Attributes: map[string]*Attribute{
								"child_computed": {
									Type:     cty.String,
									Computed: true,
								},
							},
						},
					},
				},
			},
			filterAttribute: FilterReadOnlyAttribute,
			filterBlock:     FilterDeprecatedBlock,
			want: &Block{
				Attributes: map[string]*Attribute{
					"nested_val": {
						Type:     cty.String,
						Optional: true,
						NestedType: &Object{
							Attributes: map[string]*Attribute{},
						},
					},
				},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			schemaBeforeFilter := cloneBlock(tc.schema)
			got := tc.schema.Filter(tc.filterAttribute, tc.filterBlock)
			if !cmp.Equal(got, tc.want, cmp.Comparer(cty.Type.Equals), cmpopts.EquateEmpty()) {
				t.Fatal(cmp.Diff(got, tc.want, cmp.Comparer(cty.Type.Equals), cmpopts.EquateEmpty()))
			}
			if !cmp.Equal(schemaBeforeFilter, tc.schema, cmp.Comparer(cty.Type.Equals), cmpopts.EquateEmpty()) {
				t.Fatal("before and after schema differ. the filtering function alters the actual schema", cmp.Diff(schemaBeforeFilter, tc.schema, cmp.Comparer(cty.Type.Equals), cmpopts.EquateEmpty()))
			}
		})
	}
}

func cloneBlock(in *Block) *Block {
	if in == nil {
		return nil
	}
	out := Block{
		Attributes:      make(map[string]*Attribute, len(in.Attributes)),
		BlockTypes:      make(map[string]*NestedBlock, len(in.BlockTypes)),
		Description:     in.Description,
		DescriptionKind: in.DescriptionKind,
		Deprecated:      in.Deprecated,
	}
	for k, v := range in.Attributes {
		out.Attributes[k] = cloneAttribute(v)
	}
	for k, v := range in.BlockTypes {
		out.BlockTypes[k] = cloneNestedBlock(v)
	}
	return &out
}

func cloneNestedBlock(in *NestedBlock) *NestedBlock {
	bl := cloneBlock(&in.Block)
	out := &NestedBlock{
		Block:    *bl,
		Nesting:  in.Nesting,
		MinItems: in.MinItems,
		MaxItems: in.MaxItems,
	}
	return out
}

func cloneAttribute(in *Attribute) *Attribute {
	out := &Attribute{
		Type:            in.Type,
		NestedType:      nil, // handled later
		Description:     in.Description,
		DescriptionKind: in.DescriptionKind,
		Required:        in.Required,
		Optional:        in.Optional,
		Computed:        in.Computed,
		Sensitive:       in.Sensitive,
		Deprecated:      in.Deprecated,
	}
	if in.NestedType != nil {
		out.NestedType = &Object{
			Attributes: make(map[string]*Attribute, len(in.NestedType.Attributes)),
			Nesting:    in.NestedType.Nesting,
		}
		for k, v := range in.NestedType.Attributes {
			out.NestedType.Attributes[k] = cloneAttribute(v)
		}
	}
	return out
}
