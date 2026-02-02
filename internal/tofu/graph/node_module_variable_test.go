// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package graph

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty-debug/ctydebug"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestNodeModuleVariablePath(t *testing.T) {
	n := &nodeModuleVariable{
		Addr: addrs.RootModuleInstance.InputVariable("foo"),
		Config: &configs.Variable{
			Name:           "foo",
			Type:           cty.String,
			ConstraintType: cty.String,
		},
	}

	want := addrs.RootModuleInstance
	got := n.Path()
	if got.String() != want.String() {
		t.Fatalf("wrong module address %s; want %s", got, want)
	}
}

func TestNodeModuleVariableReferenceableName(t *testing.T) {
	n := &nodeExpandModuleVariable{
		Addr: addrs.InputVariable{Name: "foo"},
		Config: &configs.Variable{
			Name:           "foo",
			Type:           cty.String,
			ConstraintType: cty.String,
		},
	}

	{
		expected := []addrs.Referenceable{
			addrs.InputVariable{Name: "foo"},
		}
		actual := n.ReferenceableAddrs()
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("%#v != %#v", actual, expected)
		}
	}

	{
		gotSelfPath, gotReferencePath := n.ReferenceOutside()
		wantSelfPath := addrs.RootModuleInstance
		wantReferencePath := addrs.RootModuleInstance
		if got, want := gotSelfPath.String(), wantSelfPath.String(); got != want {
			t.Errorf("wrong self path\ngot:  %s\nwant: %s", got, want)
		}
		if got, want := gotReferencePath.String(), wantReferencePath.String(); got != want {
			t.Errorf("wrong reference path\ngot:  %s\nwant: %s", got, want)
		}
	}

}

func TestNodeModuleVariableReference(t *testing.T) {
	n := &nodeExpandModuleVariable{
		Addr:   addrs.InputVariable{Name: "foo"},
		Module: addrs.RootModule.Child("bar"),
		Config: &configs.Variable{
			Name:           "foo",
			Type:           cty.String,
			ConstraintType: cty.String,
		},
		Expr: &hclsyntax.ScopeTraversalExpr{
			Traversal: hcl.Traversal{
				hcl.TraverseRoot{Name: "var"},
				hcl.TraverseAttr{Name: "foo"},
			},
		},
	}

	want := []*addrs.Reference{
		{
			Subject: addrs.InputVariable{Name: "foo"},
		},
	}
	got := n.References()
	if diff := cmp.Diff(want, got, addrs.CmpOptionsForTesting); diff != "" {
		t.Error("wrong references\n" + diff)
	}
}

func TestNodeModuleVariableReference_grandchild(t *testing.T) {
	n := &nodeExpandModuleVariable{
		Addr:   addrs.InputVariable{Name: "foo"},
		Module: addrs.RootModule.Child("bar"),
		Config: &configs.Variable{
			Name:           "foo",
			Type:           cty.String,
			ConstraintType: cty.String,
		},
		Expr: &hclsyntax.ScopeTraversalExpr{
			Traversal: hcl.Traversal{
				hcl.TraverseRoot{Name: "var"},
				hcl.TraverseAttr{Name: "foo"},
			},
		},
	}

	want := []*addrs.Reference{
		{
			Subject: addrs.InputVariable{Name: "foo"},
		},
	}
	got := n.References()
	if diff := cmp.Diff(want, got, addrs.CmpOptionsForTesting); diff != "" {
		t.Error("wrong references\n" + diff)
	}
}

func TestNodeModuleVariable_warningDiags(t *testing.T) {
	t.Run("unused object attribute", func(t *testing.T) {
		n := &nodeModuleVariable{
			Addr: addrs.InputVariable{Name: "foo"}.Absolute(addrs.RootModuleInstance),
			Config: &configs.Variable{
				Name: "foo",
				ConstraintType: cty.Object(map[string]cty.Type{
					"foo": cty.String,
					"bar": cty.Object(map[string]cty.Type{"nested": cty.EmptyObject}),
				}),
			},
			Expr: &hclsyntax.ObjectConsExpr{
				SrcRange: hcl.Range{Filename: "context1.tofu"},
				Items: []hclsyntax.ObjectConsItem{
					{
						KeyExpr: &hclsyntax.LiteralValueExpr{
							Val: cty.StringVal("baz"),
							SrcRange: hcl.Range{
								Filename: "test1.tofu",
							},
						},
						ValueExpr: &hclsyntax.LiteralValueExpr{
							Val: cty.StringVal("..."),
						},
					},
					{
						KeyExpr: &hclsyntax.LiteralValueExpr{
							Val: cty.StringVal("bar"),
							SrcRange: hcl.Range{
								Filename: "test.tofu",
							},
						},
						ValueExpr: &hclsyntax.ObjectConsExpr{
							SrcRange: hcl.Range{Filename: "context2.tofu"},
							Items: []hclsyntax.ObjectConsItem{
								{
									KeyExpr: &hclsyntax.LiteralValueExpr{
										Val: cty.StringVal("beep"),
										SrcRange: hcl.Range{
											Filename: "test2.tofu",
										},
									},
									ValueExpr: &hclsyntax.LiteralValueExpr{
										Val: cty.StringVal("..."),
									},
								},
							},
						},
					},
				},
			},
			ModuleInstance: addrs.RootModuleInstance,
		}
		// We use the "ForRPC" representation of the diagnostics just because
		// it's more friendly for comparison and we care only about the
		// user-facing information in the diagnostics, not their concrete types.
		gotDiags := n.warningDiags().ForRPC()
		var wantDiags tfdiags.Diagnostics
		wantDiags = wantDiags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagWarning,
			Summary:  "Object attribute is ignored",
			Detail:   `The object type for input variable "foo" does not include an attribute named "baz", so this definition is unused. Did you mean to set attribute "bar" instead?`,
			Subject: &hcl.Range{
				Filename: "test1.tofu", // from synthetic source range in constructed expression above
			},
			Context: &hcl.Range{
				Filename: "context1.tofu", // from synthetic source range in constructed expression above
			},
		})
		wantDiags = wantDiags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagWarning,
			Summary:  "Object attribute is ignored",
			Detail:   `The object type for input variable "foo" nested value .bar does not include an attribute named "beep", so this definition is unused.`,
			Subject: &hcl.Range{
				Filename: "test2.tofu", // from synthetic source range in constructed expression above
			},
			Context: &hcl.Range{
				Filename: "context2.tofu", // from synthetic source range in constructed expression above
			},
		})
		wantDiags = wantDiags.ForRPC()
		if diff := cmp.Diff(wantDiags, gotDiags, ctydebug.CmpOptions); diff != "" {
			t.Error("wrong diagnostics\n" + diff)
		}
	})
}
