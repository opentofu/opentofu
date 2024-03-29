// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package gohcl

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/hashicorp/hcl/v2"
)

func TestImpliedBodySchema(t *testing.T) {
	tests := []struct {
		val         interface{}
		wantSchema  *hcl.BodySchema
		wantPartial bool
	}{
		{
			struct{}{},
			&hcl.BodySchema{},
			false,
		},
		{
			struct {
				Ignored bool
			}{},
			&hcl.BodySchema{},
			false,
		},
		{
			struct {
				Attr1 bool `hcl:"attr1"`
				Attr2 bool `hcl:"attr2"`
			}{},
			&hcl.BodySchema{
				Attributes: []hcl.AttributeSchema{
					{
						Name:     "attr1",
						Required: true,
					},
					{
						Name:     "attr2",
						Required: true,
					},
				},
			},
			false,
		},
		{
			struct {
				Attr *bool `hcl:"attr,attr"`
			}{},
			&hcl.BodySchema{
				Attributes: []hcl.AttributeSchema{
					{
						Name:     "attr",
						Required: false,
					},
				},
			},
			false,
		},
		{
			struct {
				Thing struct{} `hcl:"thing,block"`
			}{},
			&hcl.BodySchema{
				Blocks: []hcl.BlockHeaderSchema{
					{
						Type: "thing",
					},
				},
			},
			false,
		},
		{
			struct {
				Thing struct {
					Type string `hcl:"type,label"`
					Name string `hcl:"name,label"`
				} `hcl:"thing,block"`
			}{},
			&hcl.BodySchema{
				Blocks: []hcl.BlockHeaderSchema{
					{
						Type:       "thing",
						LabelNames: []string{"type", "name"},
					},
				},
			},
			false,
		},
		{
			struct {
				Thing []struct {
					Type string `hcl:"type,label"`
					Name string `hcl:"name,label"`
				} `hcl:"thing,block"`
			}{},
			&hcl.BodySchema{
				Blocks: []hcl.BlockHeaderSchema{
					{
						Type:       "thing",
						LabelNames: []string{"type", "name"},
					},
				},
			},
			false,
		},
		{
			struct {
				Thing *struct {
					Type string `hcl:"type,label"`
					Name string `hcl:"name,label"`
				} `hcl:"thing,block"`
			}{},
			&hcl.BodySchema{
				Blocks: []hcl.BlockHeaderSchema{
					{
						Type:       "thing",
						LabelNames: []string{"type", "name"},
					},
				},
			},
			false,
		},
		{
			struct {
				Thing struct {
					Name      string `hcl:"name,label"`
					Something string `hcl:"something"`
				} `hcl:"thing,block"`
			}{},
			&hcl.BodySchema{
				Blocks: []hcl.BlockHeaderSchema{
					{
						Type:       "thing",
						LabelNames: []string{"name"},
					},
				},
			},
			false,
		},
		{
			struct {
				Doodad string `hcl:"doodad"`
				Thing  struct {
					Name string `hcl:"name,label"`
				} `hcl:"thing,block"`
			}{},
			&hcl.BodySchema{
				Attributes: []hcl.AttributeSchema{
					{
						Name:     "doodad",
						Required: true,
					},
				},
				Blocks: []hcl.BlockHeaderSchema{
					{
						Type:       "thing",
						LabelNames: []string{"name"},
					},
				},
			},
			false,
		},
		{
			struct {
				Doodad string `hcl:"doodad"`
				Config string `hcl:",remain"`
			}{},
			&hcl.BodySchema{
				Attributes: []hcl.AttributeSchema{
					{
						Name:     "doodad",
						Required: true,
					},
				},
			},
			true,
		},
		{
			struct {
				Expr hcl.Expression `hcl:"expr"`
			}{},
			&hcl.BodySchema{
				Attributes: []hcl.AttributeSchema{
					{
						Name:     "expr",
						Required: false,
					},
				},
			},
			false,
		},
		{
			struct {
				Meh string `hcl:"meh,optional"`
			}{},
			&hcl.BodySchema{
				Attributes: []hcl.AttributeSchema{
					{
						Name:     "meh",
						Required: false,
					},
				},
			},
			false,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%#v", test.val), func(t *testing.T) {
			schema, partial := ImpliedBodySchema(test.val)
			if !reflect.DeepEqual(schema, test.wantSchema) {
				t.Errorf(
					"wrong schema\ngot:  %s\nwant: %s",
					spew.Sdump(schema), spew.Sdump(test.wantSchema),
				)
			}

			if partial != test.wantPartial {
				t.Errorf(
					"wrong partial flag\ngot:  %#v\nwant: %#v",
					partial, test.wantPartial,
				)
			}
		})
	}
}
