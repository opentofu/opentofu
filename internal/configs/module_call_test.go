// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"os"
	"runtime"
	"testing"

	"github.com/go-test/deep"
	version "github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/zclconf/go-cty/cty"
)

func TestLoadModuleCall(t *testing.T) {
	src, err := os.ReadFile("testdata/invalid-files/module-calls.tf")
	if err != nil {
		t.Fatal(err)
	}

	parser := testParser(map[string]string{
		"module-calls.tf": string(src),
	})

	file, diags := parser.LoadConfigFile("module-calls.tf")
	assertExactDiagnostics(t, diags, []string{
		`module-calls.tf:19,3-20,11: Invalid combination of "count" and "for_each"; The "count" and "for_each" meta-arguments are mutually-exclusive. Only one should be used to be explicit about the number of module instances to be created.`,
		`module-calls.tf:34,5-36,8: Invalid combination of "count" and "enabled"; The "count" and "enabled" meta-arguments are mutually-exclusive. Only one should be used to be explicit about the number of module instances to be created.`,
	})

	gotModules := file.ModuleCalls
	wantModules := []*ModuleCall{
		{
			Name:          "foo",
			SourceAddr:    addrs.ModuleSourceLocal("./foo"),
			SourceAddrRaw: "./foo",
			SourceSet:     true,
			DeclRange: hcl.Range{
				Filename: "module-calls.tf",
				Start:    hcl.Pos{Line: 2, Column: 1, Byte: 1},
				End:      hcl.Pos{Line: 2, Column: 13, Byte: 13},
			},
		},
		{
			Name: "bar",
			SourceAddr: addrs.ModuleSourceRegistry{
				Package: addrs.ModuleRegistryPackage{
					Host:         addrs.DefaultModuleRegistryHost,
					Namespace:    "hashicorp",
					Name:         "bar",
					TargetSystem: "aws",
				},
			},
			SourceAddrRaw: "hashicorp/bar/aws",
			SourceSet:     true,
			DeclRange: hcl.Range{
				Filename: "module-calls.tf",
				Start:    hcl.Pos{Line: 7, Column: 1, Byte: 87},
				End:      hcl.Pos{Line: 7, Column: 13, Byte: 99},
			},
		},
		{
			Name: "baz",
			SourceAddr: addrs.ModuleSourceRemote{
				Package: addrs.ModulePackage("git::https://example.com/"),
			},
			SourceAddrRaw: "git::https://example.com/",
			SourceSet:     true,
			DependsOn: []hcl.Traversal{
				{
					hcl.TraverseRoot{
						Name: "module",
						SrcRange: hcl.Range{
							Filename: "module-calls.tf",
							Start:    hcl.Pos{Line: 23, Column: 5, Byte: 298},
							End:      hcl.Pos{Line: 23, Column: 11, Byte: 304},
						},
					},
					hcl.TraverseAttr{
						Name: "bar",
						SrcRange: hcl.Range{
							Filename: "module-calls.tf",
							Start:    hcl.Pos{Line: 23, Column: 11, Byte: 304},
							End:      hcl.Pos{Line: 23, Column: 15, Byte: 308},
						},
					},
				},
			},
			Providers: []PassedProviderConfig{
				{
					InChild: &ProviderConfigRef{
						Name: "aws",
						NameRange: hcl.Range{
							Filename: "module-calls.tf",
							Start:    hcl.Pos{Line: 27, Column: 5, Byte: 335},
							End:      hcl.Pos{Line: 27, Column: 8, Byte: 338},
						},
					},
					InParent: &ProviderConfigRef{
						Name: "aws",
						NameRange: hcl.Range{
							Filename: "module-calls.tf",
							Start:    hcl.Pos{Line: 27, Column: 11, Byte: 341},
							End:      hcl.Pos{Line: 27, Column: 14, Byte: 344},
						},
						Alias: "foo",
						AliasRange: &hcl.Range{
							Filename: "module-calls.tf",
							Start:    hcl.Pos{Line: 27, Column: 14, Byte: 344},
							End:      hcl.Pos{Line: 27, Column: 18, Byte: 348},
						},
					},
				},
			},
			DeclRange: hcl.Range{
				Filename: "module-calls.tf",
				Start:    hcl.Pos{Line: 14, Column: 1, Byte: 167},
				End:      hcl.Pos{Line: 14, Column: 13, Byte: 179},
			},
		},
		{
			Name:          "enabled_test",
			SourceAddr:    addrs.ModuleSourceLocal("./foo"),
			SourceAddrRaw: "./foo",
			Enabled: &hclsyntax.LiteralValueExpr{
				Val: cty.BoolVal(true),
				SrcRange: hcl.Range{
					Filename: "module-calls.tf",
					Start:    hcl.Pos{Line: 34, Column: 15, Byte: 427},
					End:      hcl.Pos{Line: 34, Column: 19, Byte: 431},
				},
			},
			SourceSet: true,
			DeclRange: hcl.Range{
				Filename: "module-calls.tf",
				Start:    hcl.Pos{Line: 31, Column: 1, Byte: 356},
				End:      hcl.Pos{Line: 31, Column: 22, Byte: 377},
			},
		},
	}

	// We'll hide all of the bodies/exprs since we're treating them as opaque
	// here anyway... the point of this test is to ensure we handle everything
	// else properly.
	for _, m := range gotModules {
		// This is a structural issue which existed before static evaluation, but has been made worse by it
		// See https://github.com/opentofu/opentofu/issues/1467 for more details
		eval := NewStaticEvaluator(nil, RootModuleCallForTesting())
		diags := m.decodeStaticFields(t.Context(), eval)
		if diags.HasErrors() {
			t.Fatal(diags.Error())
		}
		m.Source = nil

		m.Config = nil
		m.Count = nil
		m.ForEach = nil
	}

	for _, problem := range deep.Equal(gotModules, wantModules) {
		t.Error(problem)
	}
}

func TestModuleSourceAddrEntersNewPackage(t *testing.T) {
	absolutePath := "/absolute/path"
	if runtime.GOOS == "windows" {
		absolutePath = "C:\\absolute\\path"
	}

	tests := []struct {
		Addr string
		Want bool
	}{
		{
			"./",
			false,
		},
		{
			"../bork",
			false,
		},
		{
			absolutePath,
			true,
		},
		{
			"github.com/example/foo",
			true,
		},
		{
			"hashicorp/subnets/cidr", // registry module
			true,
		},
		{
			"registry.opentofu.org/hashicorp/subnets/cidr", // registry module
			true,
		},
	}

	for _, test := range tests {
		t.Run(test.Addr, func(t *testing.T) {
			addr, err := addrs.ParseModuleSource(test.Addr)
			if err != nil {
				t.Fatalf("parsing failed for %q: %s", test.Addr, err)
			}

			got := moduleSourceAddrEntersNewPackage(addr)
			if got != test.Want {
				t.Errorf("wrong result for %q\ngot:  %#v\nwant:  %#v", addr, got, test.Want)
			}
		})
	}
}

func TestModuleCallWithVersion(t *testing.T) {
	src, err := os.ReadFile("testdata/valid-files/modules-with-version.tf")
	if err != nil {
		t.Fatal(err)
	}

	parser := testParser(map[string]string{
		"modules-with-version.tf": string(src),
	})

	file, diags := parser.LoadConfigFile("modules-with-version.tf")
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Error())
	}

	// Create a module from the loaded file
	mod, diags := NewModule([]*File{file}, nil, RootModuleCallForTesting(), "testdata", SelectiveLoadAll)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors creating module: %s", diags.Error())
	}

	gotModules := file.ModuleCalls
	wantModules := []*ModuleCall{
		{
			Name:          "foo",
			SourceAddr:    addrs.ModuleSourceLocal("./foo"),
			SourceAddrRaw: "./foo",
			SourceSet:     true,
			DeclRange: hcl.Range{
				Filename: "modules-with-version.tf",
				Start:    hcl.Pos{Line: 4, Column: 1, Byte: 35},
				End:      hcl.Pos{Line: 4, Column: 13, Byte: 47},
			},
		},
		{
			Name: "foo_remote",
			SourceAddr: addrs.ModuleSourceRegistry{
				Package: addrs.ModuleRegistryPackage{
					Host:         addrs.DefaultModuleRegistryHost,
					Namespace:    "hashicorp",
					Name:         "foo",
					TargetSystem: "bar",
				},
			},
			SourceAddrRaw: "hashicorp/foo/bar",
			SourceSet:     true,
			VersionAttr: &hcl.Attribute{
				Name: "version",
				Expr: &hclsyntax.ScopeTraversalExpr{
					SrcRange: hcl.Range{
						Filename: "modules-with-version.tf",
						Start:    hcl.Pos{Line: 14, Column: 13, Byte: 214},
						End:      hcl.Pos{Line: 14, Column: 37, Byte: 238},
					},
					Traversal: hcl.Traversal{
						hcl.TraverseRoot{
							Name: "local",
							SrcRange: hcl.Range{
								Filename: "modules-with-version.tf",
								Start:    hcl.Pos{Line: 14, Column: 13, Byte: 214},
								End:      hcl.Pos{Line: 14, Column: 18, Byte: 219},
							},
						},
						hcl.TraverseAttr{
							Name: "module_version_set",
							SrcRange: hcl.Range{
								Filename: "modules-with-version.tf",
								Start:    hcl.Pos{Line: 14, Column: 18, Byte: 219},
								End:      hcl.Pos{Line: 14, Column: 37, Byte: 238},
							},
						},
					},
				},
				Range: hcl.Range{
					Filename: "modules-with-version.tf",
					Start:    hcl.Pos{Line: 14, Column: 3, Byte: 204},
					End:      hcl.Pos{Line: 14, Column: 37, Byte: 238},
				},
				NameRange: hcl.Range{
					Filename: "modules-with-version.tf",
					Start:    hcl.Pos{Line: 14, Column: 3, Byte: 204},
					End:      hcl.Pos{Line: 14, Column: 10, Byte: 211},
				},
			},
			Version: VersionConstraint{
				Required: version.MustConstraints(version.NewConstraint("1.0.0")),
				DeclRange: hcl.Range{
					Filename: "modules-with-version.tf",
					Start:    hcl.Pos{Line: 14, Column: 3, Byte: 204},
					End:      hcl.Pos{Line: 14, Column: 37, Byte: 238},
				},
			},
			DeclRange: hcl.Range{
				Filename: "modules-with-version.tf",
				Start:    hcl.Pos{Line: 12, Column: 1, Byte: 148},
				End:      hcl.Pos{Line: 12, Column: 20, Byte: 167},
			},
		},
	}

	for _, m := range gotModules {
		// Create a static evaluator with the module context
		eval := NewStaticEvaluator(mod, RootModuleCallForTesting())
		diags := m.decodeStaticFields(t.Context(), eval)
		if diags.HasErrors() {
			t.Fatal(diags.Error())
		}
		m.Source = nil
		m.Config = nil
		m.Count = nil
		m.ForEach = nil
	}

	for _, problem := range deep.Equal(gotModules, wantModules) {
		t.Error(problem)
	}
}
