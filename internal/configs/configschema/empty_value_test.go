// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configschema

import (
	"fmt"
	"testing"

	"github.com/apparentlymart/go-dump/dump"
	"github.com/davecgh/go-spew/spew"
	"github.com/zclconf/go-cty/cty"
)

func TestBlockEmptyValue(t *testing.T) {
	tests := []struct {
		Schema *Block
		Want   cty.Value
	}{
		{
			&Block{},
			cty.EmptyObjectVal,
		},
		{
			&Block{
				Attributes: map[string]*Attribute{
					"str": {Type: cty.String, Required: true},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"str": cty.NullVal(cty.String),
			}),
		},
		{
			&Block{
				BlockTypes: map[string]*NestedBlock{
					"single": {
						Nesting: NestingSingle,
						Block: Block{
							Attributes: map[string]*Attribute{
								"str": {Type: cty.String, Required: true},
							},
						},
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"single": cty.NullVal(cty.Object(map[string]cty.Type{
					"str": cty.String,
				})),
			}),
		},
		{
			&Block{
				BlockTypes: map[string]*NestedBlock{
					"group": {
						Nesting: NestingGroup,
						Block: Block{
							Attributes: map[string]*Attribute{
								"str": {Type: cty.String, Required: true},
							},
						},
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"group": cty.ObjectVal(map[string]cty.Value{
					"str": cty.NullVal(cty.String),
				}),
			}),
		},
		{
			&Block{
				BlockTypes: map[string]*NestedBlock{
					"list": {
						Nesting: NestingList,
						Block: Block{
							Attributes: map[string]*Attribute{
								"str": {Type: cty.String, Required: true},
							},
						},
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"list": cty.ListValEmpty(cty.Object(map[string]cty.Type{
					"str": cty.String,
				})),
			}),
		},
		{
			&Block{
				BlockTypes: map[string]*NestedBlock{
					"list_dynamic": {
						Nesting: NestingList,
						Block: Block{
							Attributes: map[string]*Attribute{
								"str": {Type: cty.DynamicPseudoType, Required: true},
							},
						},
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"list_dynamic": cty.EmptyTupleVal,
			}),
		},
		{
			&Block{
				BlockTypes: map[string]*NestedBlock{
					"map": {
						Nesting: NestingMap,
						Block: Block{
							Attributes: map[string]*Attribute{
								"str": {Type: cty.String, Required: true},
							},
						},
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"map": cty.MapValEmpty(cty.Object(map[string]cty.Type{
					"str": cty.String,
				})),
			}),
		},
		{
			&Block{
				BlockTypes: map[string]*NestedBlock{
					"map_dynamic": {
						Nesting: NestingMap,
						Block: Block{
							Attributes: map[string]*Attribute{
								"str": {Type: cty.DynamicPseudoType, Required: true},
							},
						},
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"map_dynamic": cty.EmptyObjectVal,
			}),
		},
		{
			&Block{
				BlockTypes: map[string]*NestedBlock{
					"set": {
						Nesting: NestingSet,
						Block: Block{
							Attributes: map[string]*Attribute{
								"str": {Type: cty.String, Required: true},
							},
						},
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"set": cty.SetValEmpty(cty.Object(map[string]cty.Type{
					"str": cty.String,
				})),
			}),
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%#v", test.Schema), func(t *testing.T) {
			got := test.Schema.EmptyValue()
			if !test.Want.RawEquals(got) {
				t.Errorf("wrong result\nschema: %s\ngot: %s\nwant: %s", spew.Sdump(test.Schema), dump.Value(got), dump.Value(test.Want))
			}
		})
	}
}

// Attribute EmptyValue() is well covered by the Block tests above; these tests
// focus on the behavior with NestedType field inside an Attribute
func TestAttributeEmptyValue(t *testing.T) {
	tests := []struct {
		Schema *Attribute
		Want   cty.Value
	}{
		{
			&Attribute{},
			cty.NilVal,
		},
		{
			&Attribute{
				Type: cty.String,
			},
			cty.NullVal(cty.String),
		},
		{
			&Attribute{
				NestedType: &Object{
					Nesting: NestingSingle,
					Attributes: map[string]*Attribute{
						"str": {Type: cty.String, Required: true},
					},
				},
			},
			cty.NullVal(cty.Object(map[string]cty.Type{
				"str": cty.String,
			})),
		},
		{
			&Attribute{
				NestedType: &Object{
					Nesting: NestingList,
					Attributes: map[string]*Attribute{
						"str": {Type: cty.String, Required: true},
					},
				},
			},
			cty.NullVal(cty.List(
				cty.Object(map[string]cty.Type{
					"str": cty.String,
				}),
			)),
		},
		{
			&Attribute{
				NestedType: &Object{
					Nesting: NestingMap,
					Attributes: map[string]*Attribute{
						"str": {Type: cty.String, Required: true},
					},
				},
			},
			cty.NullVal(cty.Map(
				cty.Object(map[string]cty.Type{
					"str": cty.String,
				}),
			)),
		},
		{
			&Attribute{
				NestedType: &Object{
					Nesting: NestingSet,
					Attributes: map[string]*Attribute{
						"str": {Type: cty.String, Required: true},
					},
				},
			},
			cty.NullVal(cty.Set(
				cty.Object(map[string]cty.Type{
					"str": cty.String,
				}),
			)),
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%#v", test.Schema), func(t *testing.T) {
			got := test.Schema.EmptyValue()
			if !test.Want.RawEquals(got) {
				t.Errorf("wrong result\nschema: %s\ngot: %s\nwant: %s", spew.Sdump(test.Schema), dump.Value(got), dump.Value(test.Want))
			}
		})
	}
}
