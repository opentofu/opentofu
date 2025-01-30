// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package lang

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/instances"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

func TestScopeEvalContext(t *testing.T) {
	data := &dataForTests{
		CountAttrs: map[string]cty.Value{
			"index": cty.NumberIntVal(0),
		},
		ForEachAttrs: map[string]cty.Value{
			"key":   cty.StringVal("a"),
			"value": cty.NumberIntVal(1),
		},
		Resources: map[string]cty.Value{
			"null_resource.foo": cty.ObjectVal(map[string]cty.Value{
				"attr": cty.StringVal("bar"),
			}),
			"data.null_data_source.foo": cty.ObjectVal(map[string]cty.Value{
				"attr": cty.StringVal("bar"),
			}),
			"null_resource.multi": cty.TupleVal([]cty.Value{
				cty.ObjectVal(map[string]cty.Value{
					"attr": cty.StringVal("multi0"),
				}),
				cty.ObjectVal(map[string]cty.Value{
					"attr": cty.StringVal("multi1"),
				}),
			}),
			"null_resource.each": cty.ObjectVal(map[string]cty.Value{
				"each0": cty.ObjectVal(map[string]cty.Value{
					"attr": cty.StringVal("each0"),
				}),
				"each1": cty.ObjectVal(map[string]cty.Value{
					"attr": cty.StringVal("each1"),
				}),
			}),
			"null_resource.multi[1]": cty.ObjectVal(map[string]cty.Value{
				"attr": cty.StringVal("multi1"),
			}),
		},
		LocalValues: map[string]cty.Value{
			"foo": cty.StringVal("bar"),
		},
		Modules: map[string]cty.Value{
			"module.foo": cty.ObjectVal(map[string]cty.Value{
				"output0": cty.StringVal("bar0"),
				"output1": cty.StringVal("bar1"),
			}),
		},
		PathAttrs: map[string]cty.Value{
			"module": cty.StringVal("foo/bar"),
		},
		TerraformAttrs: map[string]cty.Value{
			"workspace": cty.StringVal("default"),
		},
		InputVariables: map[string]cty.Value{
			"baz": cty.StringVal("boop"),
		},
	}

	tests := []struct {
		Expr string
		Want map[string]cty.Value
	}{
		{
			`12`,
			map[string]cty.Value{},
		},
		{
			`count.index`,
			map[string]cty.Value{
				"count": cty.ObjectVal(map[string]cty.Value{
					"index": cty.NumberIntVal(0),
				}),
			},
		},
		{
			`each.key`,
			map[string]cty.Value{
				"each": cty.ObjectVal(map[string]cty.Value{
					"key": cty.StringVal("a"),
				}),
			},
		},
		{
			`each.value`,
			map[string]cty.Value{
				"each": cty.ObjectVal(map[string]cty.Value{
					"value": cty.NumberIntVal(1),
				}),
			},
		},
		{
			`local.foo`,
			map[string]cty.Value{
				"local": cty.ObjectVal(map[string]cty.Value{
					"foo": cty.StringVal("bar"),
				}),
			},
		},
		{
			`null_resource.foo`,
			map[string]cty.Value{
				"null_resource": cty.ObjectVal(map[string]cty.Value{
					"foo": cty.ObjectVal(map[string]cty.Value{
						"attr": cty.StringVal("bar"),
					}),
				}),
				"resource": cty.ObjectVal(map[string]cty.Value{
					"null_resource": cty.ObjectVal(map[string]cty.Value{
						"foo": cty.ObjectVal(map[string]cty.Value{
							"attr": cty.StringVal("bar"),
						}),
					}),
				}),
			},
		},
		{
			`null_resource.foo.attr`,
			map[string]cty.Value{
				"null_resource": cty.ObjectVal(map[string]cty.Value{
					"foo": cty.ObjectVal(map[string]cty.Value{
						"attr": cty.StringVal("bar"),
					}),
				}),
				"resource": cty.ObjectVal(map[string]cty.Value{
					"null_resource": cty.ObjectVal(map[string]cty.Value{
						"foo": cty.ObjectVal(map[string]cty.Value{
							"attr": cty.StringVal("bar"),
						}),
					}),
				}),
			},
		},
		{
			`null_resource.multi`,
			map[string]cty.Value{
				"null_resource": cty.ObjectVal(map[string]cty.Value{
					"multi": cty.TupleVal([]cty.Value{
						cty.ObjectVal(map[string]cty.Value{
							"attr": cty.StringVal("multi0"),
						}),
						cty.ObjectVal(map[string]cty.Value{
							"attr": cty.StringVal("multi1"),
						}),
					}),
				}),
				"resource": cty.ObjectVal(map[string]cty.Value{
					"null_resource": cty.ObjectVal(map[string]cty.Value{
						"multi": cty.TupleVal([]cty.Value{
							cty.ObjectVal(map[string]cty.Value{
								"attr": cty.StringVal("multi0"),
							}),
							cty.ObjectVal(map[string]cty.Value{
								"attr": cty.StringVal("multi1"),
							}),
						}),
					}),
				}),
			},
		},
		{
			// at this level, all instance references return the entire resource
			`null_resource.multi[1]`,
			map[string]cty.Value{
				"null_resource": cty.ObjectVal(map[string]cty.Value{
					"multi": cty.TupleVal([]cty.Value{
						cty.ObjectVal(map[string]cty.Value{
							"attr": cty.StringVal("multi0"),
						}),
						cty.ObjectVal(map[string]cty.Value{
							"attr": cty.StringVal("multi1"),
						}),
					}),
				}),
				"resource": cty.ObjectVal(map[string]cty.Value{
					"null_resource": cty.ObjectVal(map[string]cty.Value{
						"multi": cty.TupleVal([]cty.Value{
							cty.ObjectVal(map[string]cty.Value{
								"attr": cty.StringVal("multi0"),
							}),
							cty.ObjectVal(map[string]cty.Value{
								"attr": cty.StringVal("multi1"),
							}),
						}),
					}),
				}),
			},
		},
		{
			// at this level, all instance references return the entire resource
			`null_resource.each["each1"]`,
			map[string]cty.Value{
				"null_resource": cty.ObjectVal(map[string]cty.Value{
					"each": cty.ObjectVal(map[string]cty.Value{
						"each0": cty.ObjectVal(map[string]cty.Value{
							"attr": cty.StringVal("each0"),
						}),
						"each1": cty.ObjectVal(map[string]cty.Value{
							"attr": cty.StringVal("each1"),
						}),
					}),
				}),
				"resource": cty.ObjectVal(map[string]cty.Value{
					"null_resource": cty.ObjectVal(map[string]cty.Value{
						"each": cty.ObjectVal(map[string]cty.Value{
							"each0": cty.ObjectVal(map[string]cty.Value{
								"attr": cty.StringVal("each0"),
							}),
							"each1": cty.ObjectVal(map[string]cty.Value{
								"attr": cty.StringVal("each1"),
							}),
						}),
					}),
				}),
			},
		},
		{
			// at this level, all instance references return the entire resource
			`null_resource.each["each1"].attr`,
			map[string]cty.Value{
				"null_resource": cty.ObjectVal(map[string]cty.Value{
					"each": cty.ObjectVal(map[string]cty.Value{
						"each0": cty.ObjectVal(map[string]cty.Value{
							"attr": cty.StringVal("each0"),
						}),
						"each1": cty.ObjectVal(map[string]cty.Value{
							"attr": cty.StringVal("each1"),
						}),
					}),
				}),
				"resource": cty.ObjectVal(map[string]cty.Value{
					"null_resource": cty.ObjectVal(map[string]cty.Value{
						"each": cty.ObjectVal(map[string]cty.Value{
							"each0": cty.ObjectVal(map[string]cty.Value{
								"attr": cty.StringVal("each0"),
							}),
							"each1": cty.ObjectVal(map[string]cty.Value{
								"attr": cty.StringVal("each1"),
							}),
						}),
					}),
				}),
			},
		},
		{
			`foo(null_resource.multi, null_resource.multi[1])`,
			map[string]cty.Value{
				"null_resource": cty.ObjectVal(map[string]cty.Value{
					"multi": cty.TupleVal([]cty.Value{
						cty.ObjectVal(map[string]cty.Value{
							"attr": cty.StringVal("multi0"),
						}),
						cty.ObjectVal(map[string]cty.Value{
							"attr": cty.StringVal("multi1"),
						}),
					}),
				}),
				"resource": cty.ObjectVal(map[string]cty.Value{
					"null_resource": cty.ObjectVal(map[string]cty.Value{
						"multi": cty.TupleVal([]cty.Value{
							cty.ObjectVal(map[string]cty.Value{
								"attr": cty.StringVal("multi0"),
							}),
							cty.ObjectVal(map[string]cty.Value{
								"attr": cty.StringVal("multi1"),
							}),
						}),
					}),
				}),
			},
		},
		{
			`data.null_data_source.foo`,
			map[string]cty.Value{
				"data": cty.ObjectVal(map[string]cty.Value{
					"null_data_source": cty.ObjectVal(map[string]cty.Value{
						"foo": cty.ObjectVal(map[string]cty.Value{
							"attr": cty.StringVal("bar"),
						}),
					}),
				}),
			},
		},
		{
			`module.foo`,
			map[string]cty.Value{
				"module": cty.ObjectVal(map[string]cty.Value{
					"foo": cty.ObjectVal(map[string]cty.Value{
						"output0": cty.StringVal("bar0"),
						"output1": cty.StringVal("bar1"),
					}),
				}),
			},
		},
		// any module reference returns the entire module
		{
			`module.foo.output1`,
			map[string]cty.Value{
				"module": cty.ObjectVal(map[string]cty.Value{
					"foo": cty.ObjectVal(map[string]cty.Value{
						"output0": cty.StringVal("bar0"),
						"output1": cty.StringVal("bar1"),
					}),
				}),
			},
		},
		{
			`path.module`,
			map[string]cty.Value{
				"path": cty.ObjectVal(map[string]cty.Value{
					"module": cty.StringVal("foo/bar"),
				}),
			},
		},
		{
			`self.baz`,
			map[string]cty.Value{
				"self": cty.ObjectVal(map[string]cty.Value{
					"attr": cty.StringVal("multi1"),
				}),
			},
		},
		{
			`terraform.workspace`,
			map[string]cty.Value{
				"terraform": cty.ObjectVal(map[string]cty.Value{
					"workspace": cty.StringVal("default"),
				}),
				"tofu": cty.ObjectVal(map[string]cty.Value{
					"workspace": cty.StringVal("default"),
				}),
			},
		},
		{
			`tofu.workspace`,
			map[string]cty.Value{
				"terraform": cty.ObjectVal(map[string]cty.Value{
					"workspace": cty.StringVal("default"),
				}),
				"tofu": cty.ObjectVal(map[string]cty.Value{
					"workspace": cty.StringVal("default"),
				}),
			},
		},
		{
			`var.baz`,
			map[string]cty.Value{
				"var": cty.ObjectVal(map[string]cty.Value{
					"baz": cty.StringVal("boop"),
				}),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Expr, func(t *testing.T) {
			expr, parseDiags := hclsyntax.ParseExpression([]byte(test.Expr), "", hcl.Pos{Line: 1, Column: 1})
			if len(parseDiags) != 0 {
				t.Errorf("unexpected diagnostics during parse")
				for _, diag := range parseDiags {
					t.Errorf("- %s", diag)
				}
				return
			}

			refs, refsDiags := ReferencesInExpr(addrs.ParseRef, expr)
			if refsDiags.HasErrors() {
				t.Fatal(refsDiags.Err())
			}

			scope := &Scope{
				Data:     data,
				ParseRef: addrs.ParseRef,

				// "self" will just be an arbitrary one of the several resource
				// instances we have in our test dataset.
				SelfAddr: addrs.ResourceInstance{
					Resource: addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "null_resource",
						Name: "multi",
					},
					Key: addrs.IntKey(1),
				},
			}
			ctx, ctxDiags := scope.EvalContext(refs)
			if ctxDiags.HasErrors() {
				t.Fatal(ctxDiags.Err())
			}

			// For easier test assertions we'll just remove any top-level
			// empty objects from our variables map.
			for k, v := range ctx.Variables {
				if v.RawEquals(cty.EmptyObjectVal) {
					delete(ctx.Variables, k)
				}
			}

			gotVal := cty.ObjectVal(ctx.Variables)
			wantVal := cty.ObjectVal(test.Want)

			if !gotVal.RawEquals(wantVal) {
				// We'll JSON-ize our values here just so it's easier to
				// read them in the assertion output.
				gotJSON := formattedJSONValue(gotVal)
				wantJSON := formattedJSONValue(wantVal)

				t.Errorf(
					"wrong result\nexpr: %s\ngot:  %s\nwant: %s",
					test.Expr, gotJSON, wantJSON,
				)
			}
		})
	}
}

// TestScopeEvalContextWithParent tests if the resulting EvalCtx has correct parent.
func TestScopeEvalContextWithParent(t *testing.T) {
	t.Run("with-parent", func(t *testing.T) {
		barStr, barFunc := cty.StringVal("bar"), function.New(&function.Spec{
			Impl: func(_ []cty.Value, _ cty.Type) (cty.Value, error) {
				return cty.NilVal, nil
			},
		})

		scope, parent := &Scope{}, &hcl.EvalContext{
			Variables: map[string]cty.Value{
				"foo": barStr,
			},
			Functions: map[string]function.Function{
				"foo": barFunc,
			},
		}

		child, diags := scope.EvalContextWithParent(parent, nil)
		if len(diags) != 0 {
			t.Errorf("Unexpected diagnostics:")
			for _, diag := range diags {
				t.Errorf("- %s", diag)
			}
			return
		}

		if child.Parent() == nil {
			t.Fatalf("Child EvalCtx has no parent")
		}

		if child.Parent() != parent {
			t.Fatalf("Child EvalCtx has different parent:\n GOT:%v\nWANT:%v", child.Parent(), parent)
		}

		if ln := len(child.Parent().Variables); ln != 1 {
			t.Fatalf("EvalContextWithParent modified parent's variables: incorrect length: %d", ln)
		}

		if v := child.Parent().Variables["foo"]; !v.RawEquals(barStr) {
			t.Fatalf("EvalContextWithParent modified parent's variables:\n GOT:%v\nWANT:%v", v, barStr)
		}

		if ln := len(child.Parent().Functions); ln != 1 {
			t.Fatalf("EvalContextWithParent modified parent's functions: incorrect length: %d", ln)
		}

		if v := child.Parent().Functions["foo"]; !reflect.DeepEqual(v, barFunc) {
			t.Fatalf("EvalContextWithParent modified parent's functions:\n GOT:%v\nWANT:%v", v, barFunc)
		}
	})

	t.Run("zero-parent", func(t *testing.T) {
		scope := &Scope{}

		root, diags := scope.EvalContextWithParent(nil, nil)
		if len(diags) != 0 {
			t.Errorf("Unexpected diagnostics:")
			for _, diag := range diags {
				t.Errorf("- %s", diag)
			}
			return
		}

		if root.Parent() != nil {
			t.Fatalf("Resulting EvalCtx has unexpected parent: %v", root.Parent())
		}
	})
}

func TestScopeExpandEvalBlock(t *testing.T) {
	nestedObjTy := cty.Object(map[string]cty.Type{
		"boop": cty.String,
	})
	schema := &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"foo":         {Type: cty.String, Optional: true},
			"list_of_obj": {Type: cty.List(nestedObjTy), Optional: true},
		},
		BlockTypes: map[string]*configschema.NestedBlock{
			"bar": {
				Nesting: configschema.NestingMap,
				Block: configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"baz": {Type: cty.String, Optional: true},
					},
				},
			},
		},
	}
	data := &dataForTests{
		LocalValues: map[string]cty.Value{
			"greeting": cty.StringVal("howdy"),
			"list": cty.ListVal([]cty.Value{
				cty.StringVal("elem0"),
				cty.StringVal("elem1"),
			}),
			"map": cty.MapVal(map[string]cty.Value{
				"key1": cty.StringVal("val1"),
				"key2": cty.StringVal("val2"),
			}),
		},
	}

	tests := map[string]struct {
		Config string
		Want   cty.Value
	}{
		"empty": {
			`
			`,
			cty.ObjectVal(map[string]cty.Value{
				"foo":         cty.NullVal(cty.String),
				"list_of_obj": cty.NullVal(cty.List(nestedObjTy)),
				"bar": cty.MapValEmpty(cty.Object(map[string]cty.Type{
					"baz": cty.String,
				})),
			}),
		},
		"literal attribute": {
			`
			foo = "hello"
			`,
			cty.ObjectVal(map[string]cty.Value{
				"foo":         cty.StringVal("hello"),
				"list_of_obj": cty.NullVal(cty.List(nestedObjTy)),
				"bar": cty.MapValEmpty(cty.Object(map[string]cty.Type{
					"baz": cty.String,
				})),
			}),
		},
		"variable attribute": {
			`
			foo = local.greeting
			`,
			cty.ObjectVal(map[string]cty.Value{
				"foo":         cty.StringVal("howdy"),
				"list_of_obj": cty.NullVal(cty.List(nestedObjTy)),
				"bar": cty.MapValEmpty(cty.Object(map[string]cty.Type{
					"baz": cty.String,
				})),
			}),
		},
		"one static block": {
			`
			bar "static" {}
			`,
			cty.ObjectVal(map[string]cty.Value{
				"foo":         cty.NullVal(cty.String),
				"list_of_obj": cty.NullVal(cty.List(nestedObjTy)),
				"bar": cty.MapVal(map[string]cty.Value{
					"static": cty.ObjectVal(map[string]cty.Value{
						"baz": cty.NullVal(cty.String),
					}),
				}),
			}),
		},
		"two static blocks": {
			`
			bar "static0" {
				baz = 0
			}
			bar "static1" {
				baz = 1
			}
			`,
			cty.ObjectVal(map[string]cty.Value{
				"foo":         cty.NullVal(cty.String),
				"list_of_obj": cty.NullVal(cty.List(nestedObjTy)),
				"bar": cty.MapVal(map[string]cty.Value{
					"static0": cty.ObjectVal(map[string]cty.Value{
						"baz": cty.StringVal("0"),
					}),
					"static1": cty.ObjectVal(map[string]cty.Value{
						"baz": cty.StringVal("1"),
					}),
				}),
			}),
		},
		"dynamic blocks from list": {
			`
			dynamic "bar" {
				for_each = local.list
				labels = [bar.value]
				content {
					baz = bar.key
				}
			}
			`,
			cty.ObjectVal(map[string]cty.Value{
				"foo":         cty.NullVal(cty.String),
				"list_of_obj": cty.NullVal(cty.List(nestedObjTy)),
				"bar": cty.MapVal(map[string]cty.Value{
					"elem0": cty.ObjectVal(map[string]cty.Value{
						"baz": cty.StringVal("0"),
					}),
					"elem1": cty.ObjectVal(map[string]cty.Value{
						"baz": cty.StringVal("1"),
					}),
				}),
			}),
		},
		"dynamic blocks from map": {
			`
			dynamic "bar" {
				for_each = local.map
				labels = [bar.key]
				content {
					baz = bar.value
				}
			}
			`,
			cty.ObjectVal(map[string]cty.Value{
				"foo":         cty.NullVal(cty.String),
				"list_of_obj": cty.NullVal(cty.List(nestedObjTy)),
				"bar": cty.MapVal(map[string]cty.Value{
					"key1": cty.ObjectVal(map[string]cty.Value{
						"baz": cty.StringVal("val1"),
					}),
					"key2": cty.ObjectVal(map[string]cty.Value{
						"baz": cty.StringVal("val2"),
					}),
				}),
			}),
		},
		"list-of-object attribute": {
			`
			list_of_obj = [
				{
					boop = local.greeting
				},
			]
			`,
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.NullVal(cty.String),
				"list_of_obj": cty.ListVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"boop": cty.StringVal("howdy"),
					}),
				}),
				"bar": cty.MapValEmpty(cty.Object(map[string]cty.Type{
					"baz": cty.String,
				})),
			}),
		},
		"list-of-object attribute as blocks": {
			`
			list_of_obj {
				boop = local.greeting
			}
			`,
			cty.ObjectVal(map[string]cty.Value{
				"foo": cty.NullVal(cty.String),
				"list_of_obj": cty.ListVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"boop": cty.StringVal("howdy"),
					}),
				}),
				"bar": cty.MapValEmpty(cty.Object(map[string]cty.Type{
					"baz": cty.String,
				})),
			}),
		},
		"lots of things at once": {
			`
			foo = "whoop"
			bar "static0" {
				baz = "s0"
			}
			dynamic "bar" {
				for_each = local.list
				labels = [bar.value]
				content {
					baz = bar.key
				}
			}
			bar "static1" {
				baz = "s1"
			}
			dynamic "bar" {
				for_each = local.map
				labels = [bar.key]
				content {
					baz = bar.value
				}
			}
			bar "static2" {
				baz = "s2"
			}
			`,
			cty.ObjectVal(map[string]cty.Value{
				"foo":         cty.StringVal("whoop"),
				"list_of_obj": cty.NullVal(cty.List(nestedObjTy)),
				"bar": cty.MapVal(map[string]cty.Value{
					"key1": cty.ObjectVal(map[string]cty.Value{
						"baz": cty.StringVal("val1"),
					}),
					"key2": cty.ObjectVal(map[string]cty.Value{
						"baz": cty.StringVal("val2"),
					}),
					"elem0": cty.ObjectVal(map[string]cty.Value{
						"baz": cty.StringVal("0"),
					}),
					"elem1": cty.ObjectVal(map[string]cty.Value{
						"baz": cty.StringVal("1"),
					}),
					"static0": cty.ObjectVal(map[string]cty.Value{
						"baz": cty.StringVal("s0"),
					}),
					"static1": cty.ObjectVal(map[string]cty.Value{
						"baz": cty.StringVal("s1"),
					}),
					"static2": cty.ObjectVal(map[string]cty.Value{
						"baz": cty.StringVal("s2"),
					}),
				}),
			}),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			file, parseDiags := hclsyntax.ParseConfig([]byte(test.Config), "", hcl.Pos{Line: 1, Column: 1})
			if len(parseDiags) != 0 {
				t.Errorf("unexpected diagnostics during parse")
				for _, diag := range parseDiags {
					t.Errorf("- %s", diag)
				}
				return
			}

			body := file.Body
			scope := &Scope{
				Data:     data,
				ParseRef: addrs.ParseRef,
			}

			body, expandDiags := scope.ExpandBlock(body, schema)
			if expandDiags.HasErrors() {
				t.Fatal(expandDiags.Err())
			}

			got, valDiags := scope.EvalBlock(body, schema)
			if valDiags.HasErrors() {
				t.Fatal(valDiags.Err())
			}

			if !got.RawEquals(test.Want) {
				// We'll JSON-ize our values here just so it's easier to
				// read them in the assertion output.
				gotJSON := formattedJSONValue(got)
				wantJSON := formattedJSONValue(test.Want)

				t.Errorf(
					"wrong result\nconfig: %s\ngot:   %s\nwant:  %s",
					test.Config, gotJSON, wantJSON,
				)
			}

		})
	}
}

func formattedJSONValue(val cty.Value) string {
	val = cty.UnknownAsNull(val) // since JSON can't represent unknowns
	j, err := ctyjson.Marshal(val, val.Type())
	if err != nil {
		panic(err)
	}
	var buf bytes.Buffer
	json.Indent(&buf, j, "", "  ")
	return buf.String()
}

func TestScopeEvalSelfBlock(t *testing.T) {
	data := &dataForTests{
		PathAttrs: map[string]cty.Value{
			"module": cty.StringVal("foo/bar"),
			"cwd":    cty.StringVal("/home/foo/bar"),
			"root":   cty.StringVal("/home/foo"),
		},
		TerraformAttrs: map[string]cty.Value{
			"workspace": cty.StringVal("default"),
		},
	}
	schema := &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"attr": {
				Type: cty.String,
			},
			"num": {
				Type: cty.Number,
			},
		},
	}

	tests := []struct {
		Config  string
		Self    cty.Value
		KeyData instances.RepetitionData
		Want    map[string]cty.Value
	}{
		{
			Config: `attr = self.foo`,
			Self: cty.ObjectVal(map[string]cty.Value{
				"foo": cty.StringVal("bar"),
			}),
			KeyData: instances.RepetitionData{
				CountIndex: cty.NumberIntVal(0),
			},
			Want: map[string]cty.Value{
				"attr": cty.StringVal("bar"),
				"num":  cty.NullVal(cty.Number),
			},
		},
		{
			Config: `num = count.index`,
			KeyData: instances.RepetitionData{
				CountIndex: cty.NumberIntVal(0),
			},
			Want: map[string]cty.Value{
				"attr": cty.NullVal(cty.String),
				"num":  cty.NumberIntVal(0),
			},
		},
		{
			Config: `attr = each.key`,
			KeyData: instances.RepetitionData{
				EachKey: cty.StringVal("a"),
			},
			Want: map[string]cty.Value{
				"attr": cty.StringVal("a"),
				"num":  cty.NullVal(cty.Number),
			},
		},
		{
			Config: `attr = path.cwd`,
			Want: map[string]cty.Value{
				"attr": cty.StringVal("/home/foo/bar"),
				"num":  cty.NullVal(cty.Number),
			},
		},
		{
			Config: `attr = path.module`,
			Want: map[string]cty.Value{
				"attr": cty.StringVal("foo/bar"),
				"num":  cty.NullVal(cty.Number),
			},
		},
		{
			Config: `attr = path.root`,
			Want: map[string]cty.Value{
				"attr": cty.StringVal("/home/foo"),
				"num":  cty.NullVal(cty.Number),
			},
		},
		{
			Config: `attr = terraform.workspace`,
			Want: map[string]cty.Value{
				"attr": cty.StringVal("default"),
				"num":  cty.NullVal(cty.Number),
			},
		},
		{
			Config: `attr = tofu.workspace`,
			Want: map[string]cty.Value{
				"attr": cty.StringVal("default"),
				"num":  cty.NullVal(cty.Number),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Config, func(t *testing.T) {
			file, parseDiags := hclsyntax.ParseConfig([]byte(test.Config), "", hcl.Pos{Line: 1, Column: 1})
			if len(parseDiags) != 0 {
				t.Errorf("unexpected diagnostics during parse")
				for _, diag := range parseDiags {
					t.Errorf("- %s", diag)
				}
				return
			}

			body := file.Body

			scope := &Scope{
				Data:     data,
				ParseRef: addrs.ParseRef,
			}

			gotVal, ctxDiags := scope.EvalSelfBlock(body, test.Self, schema, test.KeyData)
			if ctxDiags.HasErrors() {
				t.Fatal(ctxDiags.Err())
			}

			wantVal := cty.ObjectVal(test.Want)

			if !gotVal.RawEquals(wantVal) {
				t.Errorf(
					"wrong result\nexpr: %s\ngot:  %#v\nwant: %#v",
					test.Config, gotVal, wantVal,
				)
			}
		})
	}
}

func Test_enhanceFunctionDiags(t *testing.T) {
	tests := []struct {
		Name    string
		Config  string
		Summary string
		Detail  string
	}{
		{
			"Missing builtin function",
			"attr = missing_function(54)",
			"Call to unknown function",
			"There is no function named \"missing_function\".",
		},
		{
			"Missing core function",
			"attr = core::missing_function(54)",
			"Call to unknown function",
			"There is no builtin (core::) function named \"missing_function\".",
		},
		{
			"Invalid prefix",
			"attr = magic::missing_function(54)",
			"Unknown function namespace",
			"Function \"magic::missing_function\" does not exist within a valid namespace (provider,core)",
		},
		{
			"Too many namespaces",
			"attr = provider::foo::bar::extra::extra2::missing_function(54)",
			"Invalid function format",
			"invalid provider function \"provider::foo::bar::extra::extra2::missing_function\": expected provider::<name>::<function> or provider::<name>::<alias>::<function>",
		},
	}

	schema := &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"attr": {
				Type: cty.String,
			},
		},
	}
	spec := schema.DecoderSpec()

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			file, parseDiags := hclsyntax.ParseConfig([]byte(test.Config), "", hcl.Pos{Line: 1, Column: 1})
			if len(parseDiags) != 0 {
				t.Errorf("unexpected diagnostics during parse")
				for _, diag := range parseDiags {
					t.Errorf("- %s", diag)
				}
				return
			}

			body := file.Body

			scope := &Scope{}

			ctx, ctxDiags := scope.EvalContext(nil)
			if ctxDiags.HasErrors() {
				t.Fatalf("Unexpected ctxDiags, %#v", ctxDiags)
			}

			_, evalDiags := hcldec.Decode(body, spec, ctx)
			diags := enhanceFunctionDiags(evalDiags)
			if len(diags) != 1 {
				t.Fatalf("Expected 1 diag, got %d", len(diags))
			}
			diag := diags[0]
			if diag.Summary != test.Summary {
				t.Fatalf("Expected Summary %q, got %q", test.Summary, diag.Summary)
			}
			if diag.Detail != test.Detail {
				t.Fatalf("Expected Detail %q, got %q", test.Detail, diag.Detail)
			}

		})
	}
}
