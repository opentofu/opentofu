// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	version "github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcltest"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/parser"
	"github.com/zclconf/go-cty/cty"
)

var (
	ignoreUnexported = cmpopts.IgnoreUnexported(version.Constraint{})
	comparer         = cmp.Comparer(func(x, y RequiredProvider) bool {
		if x.Name != y.Name {
			return false
		}
		if x.Type != y.Type {
			return false
		}
		if x.Source != y.Source {
			return false
		}
		if x.Requirement.Required.String() != y.Requirement.Required.String() {
			return false
		}
		if x.DeclRange != y.DeclRange {
			return false
		}
		return true
	})
	blockRange = hcl.Range{
		Filename: "mock.tf",
		Start:    hcl.Pos{Line: 3, Column: 12, Byte: 27},
		End:      hcl.Pos{Line: 3, Column: 19, Byte: 34},
	}
	mockRange = hcl.Range{
		Filename: "MockExprLiteral",
	}
)

func TestDecodeRequiredProvidersBlock(t *testing.T) {
	tests := map[string]struct {
		Block *parser.RequiredProviders
		Want  *RequiredProviders
		Error string
	}{
		"legacy": {
			Block: &parser.RequiredProviders{
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"default": {
							Name: "default",
							Expr: hcltest.MockExprLiteral(cty.StringVal("1.0.0")),
						},
					},
				}),
				DefRange: blockRange,
			},
			Want: &RequiredProviders{
				RequiredProviders: map[string]*RequiredProvider{
					"default": {
						Name:        "default",
						Type:        addrs.NewDefaultProvider("default"),
						Requirement: testVC("1.0.0"),
						DeclRange:   mockRange,
					},
				},
				DeclRange: blockRange,
			},
		},
		"provider source": {
			Block: &parser.RequiredProviders{
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"my-test": {
							Name: "my-test",
							Expr: hcltest.MockExprLiteral(cty.ObjectVal(map[string]cty.Value{
								"source":  cty.StringVal("mycloud/test"),
								"version": cty.StringVal("2.0.0"),
							})),
						},
					},
				}),
				DefRange: blockRange,
			},
			Want: &RequiredProviders{
				RequiredProviders: map[string]*RequiredProvider{
					"my-test": {
						Name:        "my-test",
						Source:      "mycloud/test",
						Type:        addrs.NewProvider(addrs.DefaultProviderRegistryHost, "mycloud", "test"),
						Requirement: testVC("2.0.0"),
						DeclRange:   mockRange,
					},
				},
				DeclRange: blockRange,
			},
		},
		"mixed": {
			Block: &parser.RequiredProviders{
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"legacy": {
							Name: "legacy",
							Expr: hcltest.MockExprLiteral(cty.StringVal("1.0.0")),
						},
						"my-test": {
							Name: "my-test",
							Expr: hcltest.MockExprLiteral(cty.ObjectVal(map[string]cty.Value{
								"source":  cty.StringVal("mycloud/test"),
								"version": cty.StringVal("2.0.0"),
							})),
						},
					},
				}),
				DefRange: blockRange,
			},
			Want: &RequiredProviders{
				RequiredProviders: map[string]*RequiredProvider{
					"legacy": {
						Name:        "legacy",
						Type:        addrs.NewDefaultProvider("legacy"),
						Requirement: testVC("1.0.0"),
						DeclRange:   mockRange,
					},
					"my-test": {
						Name:        "my-test",
						Source:      "mycloud/test",
						Type:        addrs.NewProvider(addrs.DefaultProviderRegistryHost, "mycloud", "test"),
						Requirement: testVC("2.0.0"),
						DeclRange:   mockRange,
					},
				},
				DeclRange: blockRange,
			},
		},
		"version-only block": {
			Block: &parser.RequiredProviders{
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"test": {
							Name: "test",
							Expr: hcltest.MockExprLiteral(cty.ObjectVal(map[string]cty.Value{
								"version": cty.StringVal("~>2.0.0"),
							})),
						},
					},
				}),
				DefRange: blockRange,
			},
			Want: &RequiredProviders{
				RequiredProviders: map[string]*RequiredProvider{
					"test": {
						Name:        "test",
						Type:        addrs.NewDefaultProvider("test"),
						Requirement: testVC("~>2.0.0"),
						DeclRange:   mockRange,
					},
				},
				DeclRange: blockRange,
			},
		},
		"invalid source": {
			Block: &parser.RequiredProviders{
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"my-test": {
							Name: "my-test",
							Expr: hcltest.MockExprLiteral(cty.ObjectVal(map[string]cty.Value{
								"source":  cty.StringVal("some/invalid/provider/source/test"),
								"version": cty.StringVal("~>2.0.0"),
							})),
						},
					},
				}),
				DefRange: blockRange,
			},
			Want: &RequiredProviders{
				RequiredProviders: map[string]*RequiredProvider{},
				DeclRange:         blockRange,
			},
			Error: "Invalid provider source string",
		},
		"invalid localname": {
			Block: &parser.RequiredProviders{
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"my_test": {
							Name: "my_test",
							Expr: hcltest.MockExprLiteral(cty.ObjectVal(map[string]cty.Value{
								"version": cty.StringVal("~>2.0.0"),
							})),
						},
					},
				}),
				DefRange: blockRange,
			},
			Want: &RequiredProviders{
				RequiredProviders: map[string]*RequiredProvider{},
				DeclRange:         blockRange,
			},
			Error: "Invalid provider local name",
		},
		"invalid localname caps": {
			Block: &parser.RequiredProviders{
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"MYTEST": {
							Name: "MYTEST",
							Expr: hcltest.MockExprLiteral(cty.ObjectVal(map[string]cty.Value{
								"version": cty.StringVal("~>2.0.0"),
							})),
						},
					},
				}),
				DefRange: blockRange,
			},
			Want: &RequiredProviders{
				RequiredProviders: map[string]*RequiredProvider{},
				DeclRange:         blockRange,
			},
			Error: "Invalid provider local name",
		},
		"version constraint error": {
			Block: &parser.RequiredProviders{
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"my-test": {
							Name: "my-test",
							Expr: hcltest.MockExprLiteral(cty.ObjectVal(map[string]cty.Value{
								"source":  cty.StringVal("mycloud/test"),
								"version": cty.StringVal("invalid"),
							})),
						},
					},
				}),
				DefRange: blockRange,
			},
			Want: &RequiredProviders{
				RequiredProviders: map[string]*RequiredProvider{},
				DeclRange:         blockRange,
			},
			Error: "Invalid version constraint",
		},
		"invalid required_providers attribute value": {
			Block: &parser.RequiredProviders{
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"test": {
							Name: "test",
							Expr: hcltest.MockExprLiteral(cty.ListVal([]cty.Value{cty.StringVal("2.0.0")})),
						},
					},
				}),
				DefRange: blockRange,
			},
			Want: &RequiredProviders{
				RequiredProviders: map[string]*RequiredProvider{},
				DeclRange:         blockRange,
			},
			Error: "Invalid required_providers object",
		},
		"invalid source attribute type": {
			Block: &parser.RequiredProviders{
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"my-test": {
							Name: "my-test",
							Expr: hcltest.MockExprLiteral(cty.ObjectVal(map[string]cty.Value{
								"source": cty.DynamicVal,
							})),
						},
					},
				}),
				DefRange: blockRange,
			},
			Want: &RequiredProviders{
				RequiredProviders: map[string]*RequiredProvider{},
				DeclRange:         blockRange,
			},
			Error: "Invalid source",
		},
		"additional attributes": {
			Block: &parser.RequiredProviders{
				Body: hcltest.MockBody(&hcl.BodyContent{
					Attributes: hcl.Attributes{
						"my-test": {
							Name: "my-test",
							Expr: hcltest.MockExprLiteral(cty.ObjectVal(map[string]cty.Value{
								"source":  cty.StringVal("mycloud/test"),
								"version": cty.StringVal("2.0.0"),
								"invalid": cty.BoolVal(true),
							})),
						},
					},
				}),
				DefRange: blockRange,
			},
			Want: &RequiredProviders{
				RequiredProviders: map[string]*RequiredProvider{},
				DeclRange:         blockRange,
			},
			Error: "Invalid required_providers object",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, diags := decodeRequiredProvidersBlock(test.Block)
			if diags.HasErrors() {
				if test.Error == "" {
					t.Fatalf("unexpected error: %v", diags)
				}
				if gotErr := diags[0].Summary; gotErr != test.Error {
					t.Errorf("wrong error, got %q, want %q", gotErr, test.Error)
				}
			} else if test.Error != "" {
				t.Fatalf("expected error")
			}

			if !cmp.Equal(got, test.Want, ignoreUnexported, comparer) {
				t.Fatalf("wrong result:\n %s", cmp.Diff(got, test.Want, ignoreUnexported, comparer))
			}
		})
	}
}

func testVC(ver string) VersionConstraint {
	constraint, _ := version.NewConstraint(ver)
	return VersionConstraint{
		Required:  constraint,
		DeclRange: hcl.Range{},
	}
}
