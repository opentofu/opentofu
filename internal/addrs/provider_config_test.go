// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

func TestParseAbsProviderConfigInstances(t *testing.T) {
	tests := []struct {
		Input    string
		Want     AbsProviderConfig
		WantKey  InstanceKey
		WantDiag string
	}{
		{
			`provider["registry.opentofu.org/hashicorp/aws"]`,
			AbsProviderConfig{
				Module: RootModule,
				Provider: Provider{
					Type:      "aws",
					Namespace: "hashicorp",
					Hostname:  "registry.opentofu.org",
				},
			},
			NoKey,
			``,
		},
		{
			`provider["registry.opentofu.org/hashicorp/aws"].foo`,
			AbsProviderConfig{
				Module: RootModule,
				Provider: Provider{
					Type:      "aws",
					Namespace: "hashicorp",
					Hostname:  "registry.opentofu.org",
				},
				Alias: "foo",
			},
			NoKey,
			``,
		},
		{
			`module.baz.provider["registry.opentofu.org/hashicorp/aws"]`,
			AbsProviderConfig{
				Module: Module{"baz"},
				Provider: Provider{
					Type:      "aws",
					Namespace: "hashicorp",
					Hostname:  "registry.opentofu.org",
				},
			},
			NoKey,
			``,
		},
		{
			`module.baz.provider["registry.opentofu.org/hashicorp/aws"].foo`,
			AbsProviderConfig{
				Module: Module{"baz"},
				Provider: Provider{
					Type:      "aws",
					Namespace: "hashicorp",
					Hostname:  "registry.opentofu.org",
				},
				Alias: "foo",
			},
			NoKey,
			``,
		},
		{
			`module.baz.provider["registry.opentofu.org/hashicorp/aws"].foo["keystr"]`,
			AbsProviderConfig{
				Module: Module{"baz"},
				Provider: Provider{
					Type:      "aws",
					Namespace: "hashicorp",
					Hostname:  "registry.opentofu.org",
				},
				Alias: "foo",
			},
			StringKey("keystr"),
			``,
		},
		{
			`module.baz["foo"].provider["registry.opentofu.org/hashicorp/aws"]`,
			AbsProviderConfig{},
			NoKey,
			`A provider configuration must not appear in a module instance that uses count or for_each.`,
		},
		{
			`module.baz[1].provider["registry.opentofu.org/hashicorp/aws"]`,
			AbsProviderConfig{},
			NoKey,
			`A provider configuration must not appear in a module instance that uses count or for_each.`,
		},
		{
			`module.baz[1].module.bar.provider["registry.opentofu.org/hashicorp/aws"]`,
			AbsProviderConfig{},
			NoKey,
			`A provider configuration must not appear in a module instance that uses count or for_each.`,
		},
		{
			`aws`,
			AbsProviderConfig{},
			NoKey,
			`Provider address must begin with "provider.", followed by a provider type name.`,
		},
		{
			`aws.foo`,
			AbsProviderConfig{},
			NoKey,
			`Provider address must begin with "provider.", followed by a provider type name.`,
		},
		{
			`provider`,
			AbsProviderConfig{},
			NoKey,
			`Provider address must begin with "provider.", followed by a provider type name.`,
		},
		{
			`provider.aws.foo["bar"].baz`,
			AbsProviderConfig{},
			NoKey,
			`Extraneous operators after provider configuration reference.`,
		},
		{
			`provider["aws"]["foo"]`,
			AbsProviderConfig{},
			NoKey,
			`Provider type name must be followed by a configuration alias name.`,
		},
		{
			`module.foo`,
			AbsProviderConfig{},
			NoKey,
			`Provider address must begin with "provider.", followed by a provider type name.`,
		},
		{
			`provider[0]`,
			AbsProviderConfig{},
			NoKey,
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

			got, key, diags := ParseAbsProviderConfigInstance(traversal)

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

			if diff := cmp.Diff(test.Want, got, CmpOptionsForTesting); diff != "" {
				t.Error("wrong result:\n" + diff)
			}

			if test.WantKey != key {
				t.Errorf("Wanted key %s, got key %s", test.WantKey, key)
			}
		})
	}
}

func TestAbsProviderConfigString(t *testing.T) {
	tests := []struct {
		Config AbsProviderConfig
		Want   string
	}{
		{
			AbsProviderConfig{
				Module:   RootModule,
				Provider: NewLegacyProvider("foo"),
			},
			`provider["registry.opentofu.org/-/foo"]`,
		},
		{
			AbsProviderConfig{
				Module:   RootModule.Child("child_module"),
				Provider: NewDefaultProvider("foo"),
			},
			`module.child_module.provider["registry.opentofu.org/hashicorp/foo"]`,
		},
		{
			AbsProviderConfig{
				Module:   RootModule,
				Alias:    "bar",
				Provider: NewDefaultProvider("foo"),
			},
			`provider["registry.opentofu.org/hashicorp/foo"].bar`,
		},
		{
			AbsProviderConfig{
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
		Config AbsProviderConfig
		Want   string
	}{
		{
			AbsProviderConfig{
				Module:   RootModule,
				Provider: NewLegacyProvider("foo"),
			},
			`provider.foo`,
		},
		{
			AbsProviderConfig{
				Module:   RootModule.Child("child_module"),
				Provider: NewLegacyProvider("foo"),
			},
			`module.child_module.provider.foo`,
		},
		{
			AbsProviderConfig{
				Module:   RootModule,
				Alias:    "bar",
				Provider: NewLegacyProvider("foo"),
			},
			`provider.foo.bar`,
		},
		{
			AbsProviderConfig{
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
		Want   AbsProviderConfig
	}{
		{
			`provider.foo`,
			AbsProviderConfig{
				Module:   RootModule,
				Provider: NewLegacyProvider("foo"),
			},
		},
		{
			`module.child_module.provider.foo`,
			AbsProviderConfig{
				Module:   RootModule.Child("child_module"),
				Provider: NewLegacyProvider("foo"),
			},
		},
		{
			`provider.terraform`,
			AbsProviderConfig{
				Module:   RootModule,
				Provider: NewBuiltInProvider("terraform"),
			},
		},
	}

	for _, test := range tests {
		got, _ := ParseLegacyAbsProviderConfigStr(test.Config)
		if !reflect.DeepEqual(got, test.Want) {
			t.Errorf("wrong result. Got %s, want %s\n", got, test.Want)
		}
	}
}
