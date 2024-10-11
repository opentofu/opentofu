// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"reflect"
	"testing"

	"github.com/go-test/deep"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

func TestParseAbsProviderConfig(t *testing.T) {
	tests := []struct {
		Input    string
		Want     ConfigProviderInstance
		WantDiag string
	}{
		{
			`provider["registry.opentofu.org/hashicorp/aws"]`,
			ConfigProviderInstance{
				Module: RootModule,
				Provider: Provider{
					Type:      "aws",
					Namespace: "hashicorp",
					Hostname:  "registry.opentofu.org",
				},
			},
			``,
		},
		{
			`provider["registry.opentofu.org/hashicorp/aws"].foo`,
			ConfigProviderInstance{
				Module: RootModule,
				Provider: Provider{
					Type:      "aws",
					Namespace: "hashicorp",
					Hostname:  "registry.opentofu.org",
				},
				Alias: "foo",
			},
			``,
		},
		{
			`module.baz.provider["registry.opentofu.org/hashicorp/aws"]`,
			ConfigProviderInstance{
				Module: Module{"baz"},
				Provider: Provider{
					Type:      "aws",
					Namespace: "hashicorp",
					Hostname:  "registry.opentofu.org",
				},
			},
			``,
		},
		{
			`module.baz.provider["registry.opentofu.org/hashicorp/aws"].foo`,
			ConfigProviderInstance{
				Module: Module{"baz"},
				Provider: Provider{
					Type:      "aws",
					Namespace: "hashicorp",
					Hostname:  "registry.opentofu.org",
				},
				Alias: "foo",
			},
			``,
		},
		{
			`module.baz["foo"].provider["registry.opentofu.org/hashicorp/aws"]`,
			ConfigProviderInstance{},
			`Provider address cannot contain module indexes`,
		},
		{
			`module.baz[1].provider["registry.opentofu.org/hashicorp/aws"]`,
			ConfigProviderInstance{},
			`Provider address cannot contain module indexes`,
		},
		{
			`module.baz[1].module.bar.provider["registry.opentofu.org/hashicorp/aws"]`,
			ConfigProviderInstance{},
			`Provider address cannot contain module indexes`,
		},
		{
			`aws`,
			ConfigProviderInstance{},
			`Provider address must begin with "provider.", followed by a provider type name.`,
		},
		{
			`aws.foo`,
			ConfigProviderInstance{},
			`Provider address must begin with "provider.", followed by a provider type name.`,
		},
		{
			`provider`,
			ConfigProviderInstance{},
			`Provider address must begin with "provider.", followed by a provider type name.`,
		},
		{
			`provider.aws.foo.bar`,
			ConfigProviderInstance{},
			`Extraneous operators after provider configuration alias.`,
		},
		{
			`provider["aws"]["foo"]`,
			ConfigProviderInstance{},
			`Provider type name must be followed by a configuration alias name.`,
		},
		{
			`module.foo`,
			ConfigProviderInstance{},
			`Provider address must begin with "provider.", followed by a provider type name.`,
		},
		{
			`provider[0]`,
			ConfigProviderInstance{},
			`The prefix "provider." must be followed by a provider type name.`,
		},
	}

	for _, test := range tests {
		t.Run(test.Input, func(t *testing.T) {
			traversal, parseDiags := hclsyntax.ParseTraversalAbs([]byte(test.Input), "", hcl.Pos{})
			if len(parseDiags) != 0 {
				t.Errorf("unexpected diagnostics during parse")
				for _, diag := range parseDiags {
					t.Logf("- %s", diag)
				}
				return
			}

			got, diags := ParseConfigProviderInstance(traversal)

			if test.WantDiag != "" {
				if len(diags) != 1 {
					t.Fatalf("got %d diagnostics; want 1", len(diags))
				}
				gotDetail := diags[0].Description().Detail
				if gotDetail != test.WantDiag {
					t.Fatalf("wrong diagnostic detail\ngot:  %s\nwant: %s", gotDetail, test.WantDiag)
				}
				return
			} else {
				if len(diags) != 0 {
					t.Fatalf("got %d diagnostics; want 0", len(diags))
				}
			}

			for _, problem := range deep.Equal(got, test.Want) {
				t.Error(problem)
			}
		})
	}
}

func TestAbsProviderConfigString(t *testing.T) {
	tests := []struct {
		Config ConfigProviderInstance
		Want   string
	}{
		{
			ConfigProviderInstance{
				Module:   RootModule,
				Provider: NewLegacyProvider("foo"),
			},
			`provider["registry.opentofu.org/-/foo"]`,
		},
		{
			ConfigProviderInstance{
				Module:   RootModule.Child("child_module"),
				Provider: NewDefaultProvider("foo"),
			},
			`module.child_module.provider["registry.opentofu.org/hashicorp/foo"]`,
		},
		{
			ConfigProviderInstance{
				Module:   RootModule,
				Alias:    "bar",
				Provider: NewDefaultProvider("foo"),
			},
			`provider["registry.opentofu.org/hashicorp/foo"].bar`,
		},
		{
			ConfigProviderInstance{
				Module:   RootModule.Child("child_module"),
				Alias:    "bar",
				Provider: NewDefaultProvider("foo"),
			},
			`module.child_module.provider["registry.opentofu.org/hashicorp/foo"].bar`,
		},
	}

	for _, test := range tests {
		got := test.Config.String()
		if got != test.Want {
			t.Errorf("wrong result. Got %s, want %s\n", got, test.Want)
		}
	}
}

func TestAbsProviderConfigLegacyString(t *testing.T) {
	tests := []struct {
		Config ConfigProviderInstance
		Want   string
	}{
		{
			ConfigProviderInstance{
				Module:   RootModule,
				Provider: NewLegacyProvider("foo"),
			},
			`provider.foo`,
		},
		{
			ConfigProviderInstance{
				Module:   RootModule.Child("child_module"),
				Provider: NewLegacyProvider("foo"),
			},
			`module.child_module.provider.foo`,
		},
		{
			ConfigProviderInstance{
				Module:   RootModule,
				Alias:    "bar",
				Provider: NewLegacyProvider("foo"),
			},
			`provider.foo.bar`,
		},
		{
			ConfigProviderInstance{
				Module:   RootModule.Child("child_module"),
				Alias:    "bar",
				Provider: NewLegacyProvider("foo"),
			},
			`module.child_module.provider.foo.bar`,
		},
	}

	for _, test := range tests {
		got := test.Config.LegacyString()
		if got != test.Want {
			t.Errorf("wrong result. Got %s, want %s\n", got, test.Want)
		}
	}
}

func TestParseLegacyAbsProviderConfigStr(t *testing.T) {
	tests := []struct {
		Config string
		Want   ConfigProviderInstance
	}{
		{
			`provider.foo`,
			ConfigProviderInstance{
				Module:   RootModule,
				Provider: NewLegacyProvider("foo"),
			},
		},
		{
			`module.child_module.provider.foo`,
			ConfigProviderInstance{
				Module:   RootModule.Child("child_module"),
				Provider: NewLegacyProvider("foo"),
			},
		},
		{
			`provider.terraform`,
			ConfigProviderInstance{
				Module:   RootModule,
				Provider: NewBuiltInProvider("terraform"),
			},
		},
	}

	for _, test := range tests {
		got, _ := ParseLegacyConfigProviderInstanceStr(test.Config)
		if !reflect.DeepEqual(got, test.Want) {
			t.Errorf("wrong result. Got %s, want %s\n", got, test.Want)
		}
	}
}
