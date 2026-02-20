// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package convert

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	proto "github.com/opentofu/opentofu/internal/tfplugin6"
	"github.com/zclconf/go-cty/cty"
	"google.golang.org/protobuf/testing/protocmp"
)

var (
	equateEmpty   = cmpopts.EquateEmpty()
	typeComparer  = cmp.Comparer(cty.Type.Equals)
	valueComparer = cmp.Comparer(cty.Value.RawEquals)
)

// Test that we can convert configschema to protobuf types and back again.
func TestConvertSchemaBlocks(t *testing.T) {
	tests := map[string]struct {
		Block *proto.Schema_Block
		Want  *configschema.Block
	}{
		"attributes": {
			&proto.Schema_Block{
				Attributes: []*proto.Schema_Attribute{
					{
						Name:     "computed",
						Type:     []byte(`["list","bool"]`),
						Computed: true,
					},
					{
						Name:     "optional",
						Type:     []byte(`"string"`),
						Optional: true,
					},
					{
						Name:     "optional_computed",
						Type:     []byte(`["map","bool"]`),
						Optional: true,
						Computed: true,
					},
					{
						Name:     "required",
						Type:     []byte(`"number"`),
						Required: true,
					},
					{
						Name:      "write_only",
						Type:      []byte(`"number"`),
						WriteOnly: true,
					},
					{
						Name: "nested_type",
						NestedType: &proto.Schema_Object{
							Nesting: proto.Schema_Object_SINGLE,
							Attributes: []*proto.Schema_Attribute{
								{
									Name:     "computed",
									Type:     []byte(`["list","bool"]`),
									Computed: true,
								},
								{
									Name:     "optional",
									Type:     []byte(`"string"`),
									Optional: true,
								},
								{
									Name:     "optional_computed",
									Type:     []byte(`["map","bool"]`),
									Optional: true,
									Computed: true,
								},
								{
									Name:     "required",
									Type:     []byte(`"number"`),
									Required: true,
								},
								{
									Name:      "write_only",
									Type:      []byte(`"number"`),
									WriteOnly: true,
								},
							},
						},
						Required: true,
					},
					{
						Name: "deeply_nested_type",
						NestedType: &proto.Schema_Object{
							Nesting: proto.Schema_Object_SINGLE,
							Attributes: []*proto.Schema_Attribute{
								{
									Name: "first_level",
									NestedType: &proto.Schema_Object{
										Nesting: proto.Schema_Object_SINGLE,
										Attributes: []*proto.Schema_Attribute{
											{
												Name:     "computed",
												Type:     []byte(`["list","bool"]`),
												Computed: true,
											},
											{
												Name:     "optional",
												Type:     []byte(`"string"`),
												Optional: true,
											},
											{
												Name:     "optional_computed",
												Type:     []byte(`["map","bool"]`),
												Optional: true,
												Computed: true,
											},
											{
												Name:     "required",
												Type:     []byte(`"number"`),
												Required: true,
											},
											{
												Name:      "write_only",
												Type:      []byte(`"number"`),
												WriteOnly: true,
											},
										},
									},
									Computed: true,
								},
							},
						},
						Required: true,
					},
					{
						Name: "nested_list",
						NestedType: &proto.Schema_Object{
							Nesting: proto.Schema_Object_LIST,
							Attributes: []*proto.Schema_Attribute{
								{
									Name:     "required",
									Type:     []byte(`"string"`),
									Computed: true,
								},
								{
									Name:      "write_only",
									Type:      []byte(`"string"`),
									WriteOnly: true,
								},
							},
						},
						Required: true,
					},
					{
						Name: "nested_set",
						NestedType: &proto.Schema_Object{
							Nesting: proto.Schema_Object_SET,
							Attributes: []*proto.Schema_Attribute{
								{
									Name:     "required",
									Type:     []byte(`"string"`),
									Computed: true,
								},
								{
									// Even though the set types do not accept write-only attributes, we want
									// to test the generic convertion of this.
									Name:      "write_only",
									Type:      []byte(`"string"`),
									WriteOnly: true,
								},
							},
						},
						Required: true,
					},
					{
						Name: "nested_map",
						NestedType: &proto.Schema_Object{
							Nesting: proto.Schema_Object_MAP,
							Attributes: []*proto.Schema_Attribute{
								{
									Name:     "required",
									Type:     []byte(`"string"`),
									Computed: true,
								},
								{
									Name:      "write_only",
									Type:      []byte(`"string"`),
									WriteOnly: true,
								},
							},
						},
						Required: true,
					},
				},
			},
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"computed": {
						Type:     cty.List(cty.Bool),
						Computed: true,
					},
					"optional": {
						Type:     cty.String,
						Optional: true,
					},
					"optional_computed": {
						Type:     cty.Map(cty.Bool),
						Optional: true,
						Computed: true,
					},
					"required": {
						Type:     cty.Number,
						Required: true,
					},
					"write_only": {
						Type:      cty.Number,
						WriteOnly: true,
					},
					"nested_type": {
						NestedType: &configschema.Object{
							Attributes: map[string]*configschema.Attribute{
								"computed": {
									Type:     cty.List(cty.Bool),
									Computed: true,
								},
								"optional": {
									Type:     cty.String,
									Optional: true,
								},
								"optional_computed": {
									Type:     cty.Map(cty.Bool),
									Optional: true,
									Computed: true,
								},
								"required": {
									Type:     cty.Number,
									Required: true,
								},
								"write_only": {
									Type:      cty.Number,
									WriteOnly: true,
								},
							},
							Nesting: configschema.NestingSingle,
						},
						Required: true,
					},
					"deeply_nested_type": {
						NestedType: &configschema.Object{
							Attributes: map[string]*configschema.Attribute{
								"first_level": {
									NestedType: &configschema.Object{
										Nesting: configschema.NestingSingle,
										Attributes: map[string]*configschema.Attribute{
											"computed": {
												Type:     cty.List(cty.Bool),
												Computed: true,
											},
											"optional": {
												Type:     cty.String,
												Optional: true,
											},
											"optional_computed": {
												Type:     cty.Map(cty.Bool),
												Optional: true,
												Computed: true,
											},
											"required": {
												Type:     cty.Number,
												Required: true,
											},
											"write_only": {
												Type:      cty.Number,
												WriteOnly: true,
											},
										},
									},
									Computed: true,
								},
							},
							Nesting: configschema.NestingSingle,
						},
						Required: true,
					},
					"nested_list": {
						NestedType: &configschema.Object{
							Nesting: configschema.NestingList,
							Attributes: map[string]*configschema.Attribute{
								"required": {
									Type:     cty.String,
									Computed: true,
								},
								"write_only": {
									Type:      cty.String,
									WriteOnly: true,
								},
							},
						},
						Required: true,
					},
					"nested_map": {
						NestedType: &configschema.Object{
							Nesting: configschema.NestingMap,
							Attributes: map[string]*configschema.Attribute{
								"required": {
									Type:     cty.String,
									Computed: true,
								},
								"write_only": {
									Type:      cty.String,
									WriteOnly: true,
								},
							},
						},
						Required: true,
					},
					"nested_set": {
						NestedType: &configschema.Object{
							Nesting: configschema.NestingSet,
							Attributes: map[string]*configschema.Attribute{
								"required": {
									Type:     cty.String,
									Computed: true,
								},
								"write_only": {
									Type:      cty.String,
									WriteOnly: true,
								},
							},
						},
						Required: true,
					},
				},
			},
		},
		"blocks": {
			&proto.Schema_Block{
				BlockTypes: []*proto.Schema_NestedBlock{
					{
						TypeName: "list",
						Nesting:  proto.Schema_NestedBlock_LIST,
						Block:    &proto.Schema_Block{},
					},
					{
						TypeName: "map",
						Nesting:  proto.Schema_NestedBlock_MAP,
						Block:    &proto.Schema_Block{},
					},
					{
						TypeName: "set",
						Nesting:  proto.Schema_NestedBlock_SET,
						Block:    &proto.Schema_Block{},
					},
					{
						TypeName: "single",
						Nesting:  proto.Schema_NestedBlock_SINGLE,
						Block: &proto.Schema_Block{
							Attributes: []*proto.Schema_Attribute{
								{
									Name:     "foo",
									Type:     []byte(`"dynamic"`),
									Required: true,
								},
								{
									Name:      "write_only",
									Type:      []byte(`"dynamic"`),
									WriteOnly: true,
								},
							},
						},
					},
				},
			},
			&configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"list": {
						Nesting: configschema.NestingList,
					},
					"map": {
						Nesting: configschema.NestingMap,
					},
					"set": {
						Nesting: configschema.NestingSet,
					},
					"single": {
						Nesting: configschema.NestingSingle,
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"foo": {
									Type:     cty.DynamicPseudoType,
									Required: true,
								},
								"write_only": {
									Type:      cty.DynamicPseudoType,
									WriteOnly: true,
								},
							},
						},
					},
				},
			},
		},
		"deep block nesting": {
			&proto.Schema_Block{
				BlockTypes: []*proto.Schema_NestedBlock{
					{
						TypeName: "single",
						Nesting:  proto.Schema_NestedBlock_SINGLE,
						Block: &proto.Schema_Block{
							BlockTypes: []*proto.Schema_NestedBlock{
								{
									TypeName: "list",
									Nesting:  proto.Schema_NestedBlock_LIST,
									Block: &proto.Schema_Block{
										BlockTypes: []*proto.Schema_NestedBlock{
											{
												TypeName: "set",
												Nesting:  proto.Schema_NestedBlock_SET,
												Block:    &proto.Schema_Block{},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			&configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"single": {
						Nesting: configschema.NestingSingle,
						Block: configschema.Block{
							BlockTypes: map[string]*configschema.NestedBlock{
								"list": {
									Nesting: configschema.NestingList,
									Block: configschema.Block{
										BlockTypes: map[string]*configschema.NestedBlock{
											"set": {
												Nesting: configschema.NestingSet,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			converted := ProtoToConfigSchema(tc.Block)
			if !cmp.Equal(converted, tc.Want, typeComparer, valueComparer, equateEmpty) {
				t.Fatal(cmp.Diff(converted, tc.Want, typeComparer, valueComparer, equateEmpty))
			}
		})
	}
}

// Test that we can convert configschema to protobuf types and back again.
func TestConvertProtoSchemaBlocks(t *testing.T) {
	tests := map[string]struct {
		Want  *proto.Schema_Block
		Block *configschema.Block
	}{
		"attributes": {
			&proto.Schema_Block{
				Attributes: []*proto.Schema_Attribute{
					{
						Name:     "computed",
						Type:     []byte(`["list","bool"]`),
						Computed: true,
					},
					{
						Name:     "optional",
						Type:     []byte(`"string"`),
						Optional: true,
					},
					{
						Name:     "optional_computed",
						Type:     []byte(`["map","bool"]`),
						Optional: true,
						Computed: true,
					},
					{
						Name:     "required",
						Type:     []byte(`"number"`),
						Required: true,
					},
					{
						Name:      "write_only",
						Type:      []byte(`"number"`),
						WriteOnly: true,
					},
				},
			},
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"computed": {
						Type:     cty.List(cty.Bool),
						Computed: true,
					},
					"optional": {
						Type:     cty.String,
						Optional: true,
					},
					"optional_computed": {
						Type:     cty.Map(cty.Bool),
						Optional: true,
						Computed: true,
					},
					"required": {
						Type:     cty.Number,
						Required: true,
					},
					"write_only": {
						Type:      cty.Number,
						WriteOnly: true,
					},
				},
			},
		},
		"blocks": {
			&proto.Schema_Block{
				BlockTypes: []*proto.Schema_NestedBlock{
					{
						TypeName: "list",
						Nesting:  proto.Schema_NestedBlock_LIST,
						Block:    &proto.Schema_Block{},
					},
					{
						TypeName: "map",
						Nesting:  proto.Schema_NestedBlock_MAP,
						Block:    &proto.Schema_Block{},
					},
					{
						TypeName: "set",
						Nesting:  proto.Schema_NestedBlock_SET,
						Block:    &proto.Schema_Block{},
					},
					{
						TypeName: "single",
						Nesting:  proto.Schema_NestedBlock_SINGLE,
						Block: &proto.Schema_Block{
							Attributes: []*proto.Schema_Attribute{
								{
									Name:     "foo",
									Type:     []byte(`"dynamic"`),
									Required: true,
								},
								{
									Name:      "write_only",
									Type:      []byte(`"dynamic"`),
									WriteOnly: true,
								},
							},
						},
					},
				},
			},
			&configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"list": {
						Nesting: configschema.NestingList,
					},
					"map": {
						Nesting: configschema.NestingMap,
					},
					"set": {
						Nesting: configschema.NestingSet,
					},
					"single": {
						Nesting: configschema.NestingSingle,
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"foo": {
									Type:     cty.DynamicPseudoType,
									Required: true,
								},
								"write_only": {
									Type:      cty.DynamicPseudoType,
									WriteOnly: true,
								},
							},
						},
					},
				},
			},
		},
		"deep block nesting": {
			&proto.Schema_Block{
				BlockTypes: []*proto.Schema_NestedBlock{
					{
						TypeName: "single",
						Nesting:  proto.Schema_NestedBlock_SINGLE,
						Block: &proto.Schema_Block{
							BlockTypes: []*proto.Schema_NestedBlock{
								{
									TypeName: "list",
									Nesting:  proto.Schema_NestedBlock_LIST,
									Block: &proto.Schema_Block{
										BlockTypes: []*proto.Schema_NestedBlock{
											{
												TypeName: "set",
												Nesting:  proto.Schema_NestedBlock_SET,
												Block:    &proto.Schema_Block{},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			&configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"single": {
						Nesting: configschema.NestingSingle,
						Block: configschema.Block{
							BlockTypes: map[string]*configschema.NestedBlock{
								"list": {
									Nesting: configschema.NestingList,
									Block: configschema.Block{
										BlockTypes: map[string]*configschema.NestedBlock{
											"set": {
												Nesting: configschema.NestingSet,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			converted := ConfigSchemaToProto(tc.Block)
			if !cmp.Equal(converted, tc.Want, typeComparer, equateEmpty, ignoreUnexported) {
				t.Fatal(cmp.Diff(converted, tc.Want, typeComparer, equateEmpty, ignoreUnexported))
			}
		})
	}
}

func TestProtoToResourceIdentitySchema(t *testing.T) {
	tests := map[string]struct {
		Schema *proto.ResourceIdentitySchema
		Want   *providers.ResourceIdentitySchema
	}{
		"nil schema should return nil": {
			Schema: nil,
			Want:   nil,
		},
		"simple schema should convert with no errors": {
			Schema: &proto.ResourceIdentitySchema{
				Version: 1,
				IdentityAttributes: []*proto.ResourceIdentitySchema_IdentityAttribute{
					{
						Name:        "simple",
						Type:        []byte(`"string"`),
						Description: "A simple string attribute",
					},
				},
			},
			Want: &providers.ResourceIdentitySchema{
				Version: 1,
				Body: &configschema.Object{
					Attributes: map[string]*configschema.Attribute{
						"simple": {
							Type:            cty.String,
							Description:     "A simple string attribute",
							DescriptionKind: configschema.StringPlain,
						},
					},
					Nesting: configschema.NestingSingle,
				},
			},
		},
		"schema with required attribute shoud convert with no errors": {
			Schema: &proto.ResourceIdentitySchema{
				Version: 1,
				IdentityAttributes: []*proto.ResourceIdentitySchema_IdentityAttribute{
					{
						Name:              "required",
						Type:              []byte(`"string"`),
						Description:       "A required string attribute",
						RequiredForImport: true,
					},
				},
			},
			Want: &providers.ResourceIdentitySchema{
				Version: 1,
				Body: &configschema.Object{
					Attributes: map[string]*configschema.Attribute{
						"required": {
							Type:            cty.String,
							Description:     "A required string attribute",
							DescriptionKind: configschema.StringPlain,
							Required:        true,
						},
					},
					Nesting: configschema.NestingSingle,
				},
			},
		},
		"schema with optional attribute shoud convert with no errors": {
			Schema: &proto.ResourceIdentitySchema{
				Version: 1,
				IdentityAttributes: []*proto.ResourceIdentitySchema_IdentityAttribute{
					{
						Name:              "optional",
						Type:              []byte(`"string"`),
						Description:       "An optional string attribute",
						OptionalForImport: true,
					},
				},
			},
			Want: &providers.ResourceIdentitySchema{
				Version: 1,
				Body: &configschema.Object{
					Attributes: map[string]*configschema.Attribute{
						"optional": {
							Type:            cty.String,
							Description:     "An optional string attribute",
							DescriptionKind: configschema.StringPlain,
							Optional:        true,
						},
					},
					Nesting: configschema.NestingSingle,
				},
			},
		},
		"schema with an attribute that has a description should convert with no errors": {
			Schema: &proto.ResourceIdentitySchema{
				Version: 1,
				IdentityAttributes: []*proto.ResourceIdentitySchema_IdentityAttribute{
					{
						Name:        "described",
						Type:        []byte(`"string"`),
						Description: "A described string attribute",
					},
				},
			},
			Want: &providers.ResourceIdentitySchema{
				Version: 1,
				Body: &configschema.Object{
					Attributes: map[string]*configschema.Attribute{
						"described": {
							Type:            cty.String,
							Description:     "A described string attribute",
							DescriptionKind: configschema.StringPlain,
						},
					},
					Nesting: configschema.NestingSingle,
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			converted := ProtoToResourceIdentitySchema(tc.Schema)
			if !cmp.Equal(converted, tc.Want, typeComparer, equateEmpty, ignoreUnexported) {
				t.Fatal(cmp.Diff(converted, tc.Want, typeComparer, equateEmpty, ignoreUnexported))
			}
		})
	}
}

func TestResourceIdentitySchemaToProto(t *testing.T) {
	tests := map[string]struct {
		Want   *proto.ResourceIdentitySchema
		Schema *providers.ResourceIdentitySchema
	}{
		"nil schema should return nil": {
			Schema: nil,
			Want:   nil,
		},
		"simple schema should convert with no errors": {
			Schema: &providers.ResourceIdentitySchema{
				Version: 1,
				Body: &configschema.Object{
					Attributes: map[string]*configschema.Attribute{
						"simple": {
							Type:            cty.String,
							Description:     "A simple string attribute",
							DescriptionKind: configschema.StringPlain,
						},
					},
					Nesting: configschema.NestingSingle,
				},
			},
			Want: &proto.ResourceIdentitySchema{
				Version: 1,
				IdentityAttributes: []*proto.ResourceIdentitySchema_IdentityAttribute{
					{
						Name:        "simple",
						Type:        []byte(`"string"`),
						Description: "A simple string attribute",
					},
				},
			},
		},
		"schema with required attribute shoud convert with no errors": {
			Schema: &providers.ResourceIdentitySchema{
				Version: 1,
				Body: &configschema.Object{
					Attributes: map[string]*configschema.Attribute{
						"required": {
							Type:            cty.String,
							Description:     "A required string attribute",
							DescriptionKind: configschema.StringPlain,
							Required:        true,
						},
					},
					Nesting: configschema.NestingSingle,
				},
			},
			Want: &proto.ResourceIdentitySchema{
				Version: 1,
				IdentityAttributes: []*proto.ResourceIdentitySchema_IdentityAttribute{
					{
						Name:              "required",
						Type:              []byte(`"string"`),
						Description:       "A required string attribute",
						RequiredForImport: true,
					},
				},
			},
		},
		"schema with optional attribute shoud convert with no errors": {
			Schema: &providers.ResourceIdentitySchema{
				Version: 1,
				Body: &configschema.Object{
					Attributes: map[string]*configschema.Attribute{
						"optional": {
							Type:            cty.String,
							Description:     "An optional string attribute",
							DescriptionKind: configschema.StringPlain,
							Optional:        true,
						},
					},
					Nesting: configschema.NestingSingle,
				},
			},
			Want: &proto.ResourceIdentitySchema{
				Version: 1,
				IdentityAttributes: []*proto.ResourceIdentitySchema_IdentityAttribute{
					{
						Name:              "optional",
						Type:              []byte(`"string"`),
						Description:       "An optional string attribute",
						OptionalForImport: true,
					},
				},
			},
		},
		"schema with an attribute that has a description should convert with no errors": {
			Schema: &providers.ResourceIdentitySchema{
				Version: 1,
				Body: &configschema.Object{
					Attributes: map[string]*configschema.Attribute{
						"described": {
							Type:            cty.String,
							Description:     "A described string attribute",
							DescriptionKind: configschema.StringPlain,
						},
					},
					Nesting: configschema.NestingSingle,
				},
			},
			Want: &proto.ResourceIdentitySchema{
				Version: 1,
				IdentityAttributes: []*proto.ResourceIdentitySchema_IdentityAttribute{
					{
						Name:        "described",
						Type:        []byte(`"string"`),
						Description: "A described string attribute",
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			converted := ResourceIdentitySchemaToProto(tc.Schema)
			if !cmp.Equal(converted, tc.Want, protocmp.Transform(), typeComparer, equateEmpty, ignoreUnexported) {
				t.Fatal(cmp.Diff(converted, tc.Want, protocmp.Transform(), typeComparer, equateEmpty, ignoreUnexported))
			}
		})
	}
}
