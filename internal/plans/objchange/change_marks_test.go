// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package objchange

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty-debug/ctydebug"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
)

func TestMarkPendingChanges(t *testing.T) {
	instAddr := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "test",
		Name: "example",
	}.Absolute(addrs.RootModuleInstance).Instance(addrs.NoKey)

	tests := map[string]struct {
		Schema         *configschema.Block
		Prior, Planned cty.Value
		Want           cty.Value
	}{
		"empty unchanged": {
			&configschema.Block{},
			cty.EmptyObjectVal,
			cty.EmptyObjectVal,
			cty.EmptyObjectVal,
		},
		"null unchanged": {
			&configschema.Block{},
			cty.NullVal(cty.EmptyObject),
			cty.NullVal(cty.EmptyObject),
			cty.NullVal(cty.EmptyObject),
		},
		"now fully unknown": {
			&configschema.Block{},
			cty.NullVal(cty.EmptyObject),
			cty.UnknownVal(cty.EmptyObject),
			ValuePendingChange(cty.UnknownVal(cty.EmptyObject), instAddr),
		},

		// Shallow changes to primitive-typed attributes.
		"string unchanged": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type:     cty.String,
						Required: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.StringVal("bar"),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.StringVal("bar"),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.StringVal("bar"),
			}),
		},
		"string changed": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type:     cty.String,
						Required: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.StringVal("bar"),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.StringVal("baz"),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": ValuePendingChange(cty.StringVal("baz"), instAddr),
			}),
		},
		"string becomes null": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type:     cty.String,
						Optional: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.StringVal("bar"),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.NullVal(cty.String),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": ValuePendingChange(cty.NullVal(cty.String), instAddr),
			}),
		},
		"bool unchanged": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type:     cty.Bool,
						Required: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.False,
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.False,
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.False,
			}),
		},
		"bool changed": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type:     cty.Bool,
						Required: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.False,
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.True,
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": ValuePendingChange(cty.True, instAddr),
			}),
		},
		"bool becomes null": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type:     cty.Bool,
						Optional: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.False,
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.NullVal(cty.Bool),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": ValuePendingChange(cty.NullVal(cty.Bool), instAddr),
			}),
		},
		"number unchanged": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type:     cty.Number,
						Required: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.Zero,
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.Zero,
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.Zero,
			}),
		},
		"number changed": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type:     cty.Bool,
						Required: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.Zero,
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.NumberIntVal(1),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": ValuePendingChange(cty.NumberIntVal(1), instAddr),
			}),
		},
		"number becomes null": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type:     cty.Number,
						Optional: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.Zero,
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.NullVal(cty.Number),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": ValuePendingChange(cty.NullVal(cty.Number), instAddr),
			}),
		},

		// Attributes of object type, non-structural
		"attribute in object type changes": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type: cty.Object(map[string]cty.Type{
							"greeting": cty.String,
						}),
						Required: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ObjectVal(map[string]cty.Value{
					"greeting": cty.StringVal("hello"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ObjectVal(map[string]cty.Value{
					"greeting": cty.StringVal("howdy"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ObjectVal(map[string]cty.Value{
					"greeting": ValuePendingChange(cty.StringVal("howdy"), instAddr),
				}),
			}),
		},
		"attribute in object type becomes unknown": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type: cty.Object(map[string]cty.Type{
							"greeting": cty.String,
						}),
						Required: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ObjectVal(map[string]cty.Value{
					"greeting": cty.StringVal("hello"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ObjectVal(map[string]cty.Value{
					"greeting": cty.UnknownVal(cty.String),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ObjectVal(map[string]cty.Value{
					"greeting": ValuePendingChange(cty.UnknownVal(cty.String), instAddr),
				}),
			}),
		},
		"entire object attribute becomes unknown": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type: cty.Object(map[string]cty.Type{
							"greeting": cty.String,
						}),
						Required: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ObjectVal(map[string]cty.Value{
					"greeting": cty.StringVal("hello"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.UnknownVal(cty.Object(map[string]cty.Type{
					"greeting": cty.String,
				})),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": ValuePendingChange(
					cty.UnknownVal(cty.Object(map[string]cty.Type{
						"greeting": cty.String,
					})),
					instAddr,
				),
			}),
		},

		// Attributes of object type, structural
		"attribute in single-nested object attribute changes": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						NestedType: &configschema.Object{
							Nesting: configschema.NestingSingle,
							Attributes: map[string]*configschema.Attribute{
								"greeting": {
									Type:     cty.String,
									Optional: true,
								},
							},
						},
						Required: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ObjectVal(map[string]cty.Value{
					"greeting": cty.StringVal("hello"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ObjectVal(map[string]cty.Value{
					"greeting": cty.StringVal("howdy"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ObjectVal(map[string]cty.Value{
					"greeting": ValuePendingChange(cty.StringVal("howdy"), instAddr),
				}),
			}),
		},
		"attribute in group-nested object attribute changes": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						NestedType: &configschema.Object{
							Nesting: configschema.NestingGroup,
							Attributes: map[string]*configschema.Attribute{
								"greeting": {
									Type:     cty.String,
									Optional: true,
								},
							},
						},
						Required: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ObjectVal(map[string]cty.Value{
					"greeting": cty.StringVal("hello"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ObjectVal(map[string]cty.Value{
					"greeting": cty.StringVal("howdy"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ObjectVal(map[string]cty.Value{
					"greeting": ValuePendingChange(cty.StringVal("howdy"), instAddr),
				}),
			}),
		},
		"attribute in list-nested object attribute changes": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						NestedType: &configschema.Object{
							Nesting: configschema.NestingList,
							Attributes: map[string]*configschema.Attribute{
								"greeting": {
									Type:     cty.String,
									Optional: true,
								},
							},
						},
						Required: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ListVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"greeting": cty.StringVal("hello"),
					}),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ListVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"greeting": cty.StringVal("howdy"),
					}),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ListVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"greeting": ValuePendingChange(cty.StringVal("howdy"), instAddr),
					}),
				}),
			}),
		},
		"attribute in set-nested object attribute changes": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						NestedType: &configschema.Object{
							Nesting: configschema.NestingSet,
							Attributes: map[string]*configschema.Attribute{
								"greeting": {
									Type:     cty.String,
									Optional: true,
								},
							},
						},
						Required: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.SetVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"greeting": cty.StringVal("hello"),
					}),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.SetVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"greeting": cty.StringVal("howdy"),
					}),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				// cty sets don't support nested marks, so in this case the
				// marks all aggregate up on the set itself.
				"foo": ValuePendingChange(cty.SetVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"greeting": cty.StringVal("howdy"),
					}),
				}), instAddr),
			}),
		},

		// Attributes in nested blocks
		"attribute in single-nested block changes": {
			&configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"foo": {
						Nesting: configschema.NestingSingle,
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"greeting": {
									Type:     cty.String,
									Optional: true,
								},
							},
						},
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ObjectVal(map[string]cty.Value{
					"greeting": cty.StringVal("hello"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ObjectVal(map[string]cty.Value{
					"greeting": cty.StringVal("howdy"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ObjectVal(map[string]cty.Value{
					"greeting": ValuePendingChange(cty.StringVal("howdy"), instAddr),
				}),
			}),
		},
		"attribute in group-nested block changes": {
			&configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"foo": {
						Nesting: configschema.NestingGroup,
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"greeting": {
									Type:     cty.String,
									Optional: true,
								},
							},
						},
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ObjectVal(map[string]cty.Value{
					"greeting": cty.StringVal("hello"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ObjectVal(map[string]cty.Value{
					"greeting": cty.StringVal("howdy"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ObjectVal(map[string]cty.Value{
					"greeting": ValuePendingChange(cty.StringVal("howdy"), instAddr),
				}),
			}),
		},
		"attribute in list-nested block changes": {
			&configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"foo": {
						Nesting: configschema.NestingList,
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"greeting": {
									Type:     cty.String,
									Optional: true,
								},
							},
						},
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ListVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"greeting": cty.StringVal("hello"),
					}),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ListVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"greeting": cty.StringVal("howdy"),
					}),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ListVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"greeting": ValuePendingChange(cty.StringVal("howdy"), instAddr),
					}),
				}),
			}),
		},
		"attribute in set-nested block changes": {
			&configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"foo": {
						Nesting: configschema.NestingSet,
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"greeting": {
									Type:     cty.String,
									Optional: true,
								},
							},
						},
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.SetVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"greeting": cty.StringVal("hello"),
					}),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.SetVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"greeting": cty.StringVal("howdy"),
					}),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				// cty sets don't support nested marks, so in this case the
				// marks all aggregate up on the set itself.
				"foo": ValuePendingChange(cty.SetVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"greeting": cty.StringVal("howdy"),
					}),
				}), instAddr),
			}),
		},

		// Elements of lists
		"list attribute unchanged": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type:     cty.List(cty.String),
						Required: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ListVal([]cty.Value{
					cty.StringVal("Hello"),
					cty.StringVal("Howdy"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ListVal([]cty.Value{
					cty.StringVal("Hello"),
					cty.StringVal("Howdy"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ListVal([]cty.Value{
					cty.StringVal("Hello"),
					cty.StringVal("Howdy"),
				}),
			}),
		},
		"list attribute element changed in-place": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type:     cty.List(cty.String),
						Required: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ListVal([]cty.Value{
					cty.StringVal("Hello"),
					cty.StringVal("Howdy"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ListVal([]cty.Value{
					cty.StringVal("Hello"),
					cty.StringVal("Greetings"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ListVal([]cty.Value{
					cty.StringVal("Hello"),
					ValuePendingChange(cty.StringVal("Greetings"), instAddr),
				}),
			}),
		},
		"list attribute new element added": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type:     cty.List(cty.String),
						Required: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ListVal([]cty.Value{
					cty.StringVal("Hello"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ListVal([]cty.Value{
					cty.StringVal("Hello"),
					cty.StringVal("Howdy"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				// The entire list is marked because the length of the list
				// is considered to be a part of the list value itself rather
				// than any of its elements individually.
				"foo": ValuePendingChange(cty.ListVal([]cty.Value{
					cty.StringVal("Hello"),
					cty.StringVal("Howdy"),
				}), instAddr),
			}),
		},
		"list attribute element removed": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type:     cty.List(cty.String),
						Required: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ListVal([]cty.Value{
					cty.StringVal("Hello"),
					cty.StringVal("Howdy"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ListVal([]cty.Value{
					cty.StringVal("Hello"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				// The entire list is marked because the length of the list
				// is considered to be a part of the list value itself rather
				// than any of its elements individually.
				"foo": ValuePendingChange(cty.ListVal([]cty.Value{
					cty.StringVal("Hello"),
				}), instAddr),
			}),
		},

		// Elements of maps
		"map attribute unchanged": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type:     cty.Map(cty.String),
						Required: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.MapVal(map[string]cty.Value{
					"a": cty.StringVal("a value"),
					"b": cty.StringVal("b value"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.MapVal(map[string]cty.Value{
					"a": cty.StringVal("a value"),
					"b": cty.StringVal("b value"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.MapVal(map[string]cty.Value{
					"a": cty.StringVal("a value"),
					"b": cty.StringVal("b value"),
				}),
			}),
		},
		"map attribute changed in-place": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type:     cty.Map(cty.String),
						Required: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.MapVal(map[string]cty.Value{
					"a": cty.StringVal("a value"),
					"b": cty.StringVal("b value"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.MapVal(map[string]cty.Value{
					"a": cty.StringVal("new a value"),
					"b": cty.StringVal("b value"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.MapVal(map[string]cty.Value{
					"a": ValuePendingChange(cty.StringVal("new a value"), instAddr),
					"b": cty.StringVal("b value"),
				}),
			}),
		},
		"map attribute element added": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type:     cty.Map(cty.String),
						Required: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.MapVal(map[string]cty.Value{
					"a": cty.StringVal("a value"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.MapVal(map[string]cty.Value{
					"a": cty.StringVal("a value"),
					"b": cty.StringVal("b value"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				// The keys of a map are considered a property of the map itself,
				// so the entire map is marked in this case.
				"foo": ValuePendingChange(cty.MapVal(map[string]cty.Value{
					"a": cty.StringVal("a value"),
					"b": cty.StringVal("b value"),
				}), instAddr),
			}),
		},
		"map attribute element removed": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type:     cty.Map(cty.String),
						Required: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.MapVal(map[string]cty.Value{
					"a": cty.StringVal("a value"),
					"b": cty.StringVal("b value"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.MapVal(map[string]cty.Value{
					"b": cty.StringVal("b value"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				// The keys of a map are considered a property of the map itself,
				// so the entire map is marked in this case.
				"foo": ValuePendingChange(cty.MapVal(map[string]cty.Value{
					"b": cty.StringVal("b value"),
				}), instAddr),
			}),
		},

		// Elements of sets
		"set attribute unchanged": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type:     cty.Set(cty.String),
						Required: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.SetVal([]cty.Value{
					cty.StringVal("Hello"),
					cty.StringVal("Howdy"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.SetVal([]cty.Value{
					cty.StringVal("Hello"),
					cty.StringVal("Howdy"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.SetVal([]cty.Value{
					cty.StringVal("Hello"),
					cty.StringVal("Howdy"),
				}),
			}),
		},
		"set attribute replaced": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type:     cty.Set(cty.String),
						Required: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.SetVal([]cty.Value{
					cty.StringVal("Hello"),
					cty.StringVal("Howdy"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.SetVal([]cty.Value{
					cty.StringVal("Hello"),
					cty.StringVal("Greetings"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				// cty doesn't track marks within a set, so all marks for sets
				// aggregate on the set itself. In this case the mark is
				// representing that the elements in the set have changed,
				// and so functions like "setunion" should pass on the mark.
				"foo": ValuePendingChange(cty.SetVal([]cty.Value{
					cty.StringVal("Hello"),
					cty.StringVal("Greetings"),
				}), instAddr),
			}),
		},
		"set attribute removed": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type:     cty.Set(cty.String),
						Required: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.SetVal([]cty.Value{
					cty.StringVal("Hello"),
					cty.StringVal("Howdy"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.SetVal([]cty.Value{
					cty.StringVal("Hello"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				// cty doesn't track marks within a set, so all marks for sets
				// aggregate on the set itself. In this case the mark is
				// representing that the elements in the set have changed,
				// and so functions like "setunion" should pass on the mark.
				"foo": ValuePendingChange(cty.SetVal([]cty.Value{
					cty.StringVal("Hello"),
				}), instAddr),
			}),
		},
		"set attribute added": {
			&configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {
						Type:     cty.Set(cty.String),
						Required: true,
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.SetVal([]cty.Value{
					cty.StringVal("Hello"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.SetVal([]cty.Value{
					cty.StringVal("Hello"),
					cty.StringVal("Greetings"),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				// cty doesn't track marks within a set, so all marks for sets
				// aggregate on the set itself. In this case the mark is
				// representing that the elements in the set have changed,
				// and so functions like "setunion" should pass on the mark.
				"foo": ValuePendingChange(cty.SetVal([]cty.Value{
					cty.StringVal("Hello"),
					cty.StringVal("Greetings"),
				}), instAddr),
			}),
		},

		// Blocks of nesting mode "single"
		"single-nested block of a type unchanged": {
			&configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"foo": {
						Nesting: configschema.NestingSingle,
						Block:   configschema.Block{},
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.EmptyObjectVal,
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.EmptyObjectVal,
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.EmptyObjectVal,
			}),
		},
		"single-nested block of a type becomes null": {
			&configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"foo": {
						Nesting: configschema.NestingSingle,
						Block:   configschema.Block{},
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.EmptyObjectVal,
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.NullVal(cty.EmptyObject),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": ValuePendingChange(cty.NullVal(cty.EmptyObject), instAddr),
			}),
		},
		"single-nested block of a type becomes non-null": {
			&configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"foo": {
						Nesting: configschema.NestingSingle,
						Block:   configschema.Block{},
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.NullVal(cty.EmptyObject),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.EmptyObjectVal,
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": ValuePendingChange(cty.EmptyObjectVal, instAddr),
			}),
		},
		"single-nested block of a type becomes unknown": {
			&configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"foo": {
						Nesting: configschema.NestingSingle,
						Block:   configschema.Block{},
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.EmptyObjectVal,
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.UnknownVal(cty.EmptyObject),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": ValuePendingChange(cty.UnknownVal(cty.EmptyObject), instAddr),
			}),
		},

		// Blocks of nesting mode "group"
		"group-nested block of a type unchanged": {
			&configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"foo": {
						Nesting: configschema.NestingGroup,
						Block:   configschema.Block{},
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.EmptyObjectVal,
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.EmptyObjectVal,
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.EmptyObjectVal,
			}),
		},

		// Blocks of nesting mode "list"
		"list-nested block of a type unchanged (list)": {
			&configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"foo": {
						Nesting: configschema.NestingList,
						Block:   configschema.Block{},
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ListVal([]cty.Value{
					cty.EmptyObjectVal,
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ListVal([]cty.Value{
					cty.EmptyObjectVal,
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ListVal([]cty.Value{
					cty.EmptyObjectVal,
				}),
			}),
		},
		"list-nested block of a type unchanged (tuple)": {
			&configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"foo": {
						Nesting: configschema.NestingList,
						Block:   configschema.Block{},
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.TupleVal([]cty.Value{
					cty.EmptyObjectVal,
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.TupleVal([]cty.Value{
					cty.EmptyObjectVal,
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.TupleVal([]cty.Value{
					cty.EmptyObjectVal,
				}),
			}),
		},
		"list-nested block of a type item added": {
			&configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"foo": {
						Nesting: configschema.NestingList,
						Block:   configschema.Block{},
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ListVal([]cty.Value{
					cty.EmptyObjectVal,
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ListVal([]cty.Value{
					cty.EmptyObjectVal,
					cty.EmptyObjectVal,
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": ValuePendingChange(cty.ListVal([]cty.Value{
					cty.EmptyObjectVal,
					cty.EmptyObjectVal,
				}), instAddr),
			}),
		},
		"list-nested block of a type item removed": {
			&configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"foo": {
						Nesting: configschema.NestingList,
						Block:   configschema.Block{},
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ListVal([]cty.Value{
					cty.EmptyObjectVal,
					cty.EmptyObjectVal,
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.ListVal([]cty.Value{
					cty.EmptyObjectVal,
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": ValuePendingChange(cty.ListVal([]cty.Value{
					cty.EmptyObjectVal,
				}), instAddr),
			}),
		},

		// Blocks of nesting mode "set"
		"set-nested block of a type unchanged": {
			&configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"foo": {
						Nesting: configschema.NestingSet,
						Block: configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"foo": {
									Type:     cty.Bool,
									Required: true,
								},
							},
						},
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.SetVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"foo": cty.True,
					}),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.SetVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"foo": cty.True,
					}),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.SetVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"foo": cty.True,
					}),
				}),
			}),
		},
		"set-nested block of a type item added": {
			&configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"foo": {
						Nesting: configschema.NestingSet,
						Block: configschema.Block{
							// We need to include something in this case
							// because otherwise we could have only one
							// element because there's only one value
							// of the empty object type.
							Attributes: map[string]*configschema.Attribute{
								"foo": {
									Type:     cty.Bool,
									Required: true,
								},
							},
						},
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.SetVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"foo": cty.True,
					}),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.SetVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"foo": cty.True,
					}),
					cty.ObjectVal(map[string]cty.Value{
						"foo": cty.False,
					}),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": ValuePendingChange(cty.SetVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"foo": cty.True,
					}),
					cty.ObjectVal(map[string]cty.Value{
						"foo": cty.False,
					}),
				}), instAddr),
			}),
		},
		"set-nested block of a type item removed": {
			&configschema.Block{
				BlockTypes: map[string]*configschema.NestedBlock{
					"foo": {
						Nesting: configschema.NestingSet,
						Block: configschema.Block{
							// We need to include something in this case
							// because otherwise we could have only one
							// element because there's only one value
							// of the empty object type.
							Attributes: map[string]*configschema.Attribute{
								"foo": {
									Type:     cty.Bool,
									Required: true,
								},
							},
						},
					},
				},
			},
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.SetVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"foo": cty.True,
					}),
					cty.ObjectVal(map[string]cty.Value{
						"foo": cty.False,
					}),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.SetVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"foo": cty.True,
					}),
				}),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"foo": ValuePendingChange(cty.SetVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"foo": cty.True,
					}),
				}), instAddr),
			}),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := MarkPendingChanges(test.Schema, test.Prior, test.Planned, instAddr)
			if diff := cmp.Diff(test.Want, got, ctydebug.CmpOptions); diff != "" {
				t.Error("wrong result\n" + diff)
			}
		})
	}
}
