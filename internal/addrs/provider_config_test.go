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
		Want     AbsProviderInstance
		WantDiag string
	}{
		{
			`provider["registry.opentofu.org/hashicorp/aws"]`,
			AbsProviderInstance{
				Module: RootModuleInstance,
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
			AbsProviderInstance{
				Module: RootModuleInstance,
				Provider: Provider{
					Type:      "aws",
					Namespace: "hashicorp",
					Hostname:  "registry.opentofu.org",
				},
				Key: StringKey("foo"),
			},
			``,
		},
		{
			`module.baz.provider["registry.opentofu.org/hashicorp/aws"]`,
			AbsProviderInstance{
				Module: RootModuleInstance.Child("baz", NoKey),
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
			AbsProviderInstance{
				Module: RootModuleInstance.Child("baz", NoKey),
				Provider: Provider{
					Type:      "aws",
					Namespace: "hashicorp",
					Hostname:  "registry.opentofu.org",
				},
				Key: StringKey("foo"),
			},
			``,
		},
		{
			`module.baz["foo"].provider["registry.opentofu.org/hashicorp/aws"]`,
			AbsProviderInstance{},
			`Provider address cannot contain module indexes`,
		},
		{
			`module.baz[1].provider["registry.opentofu.org/hashicorp/aws"]`,
			AbsProviderInstance{},
			`Provider address cannot contain module indexes`,
		},
		{
			`module.baz[1].module.bar.provider["registry.opentofu.org/hashicorp/aws"]`,
			AbsProviderInstance{},
			`Provider address cannot contain module indexes`,
		},
		{
			`aws`,
			AbsProviderInstance{},
			`Provider address must begin with "provider.", followed by a provider type name.`,
		},
		{
			`aws.foo`,
			AbsProviderInstance{},
			`Provider address must begin with "provider.", followed by a provider type name.`,
		},
		{
			`provider`,
			AbsProviderInstance{},
			`Provider address must begin with "provider.", followed by a provider type name.`,
		},
		{
			`provider.aws.foo.bar`,
			AbsProviderInstance{},
			`Extraneous operators after provider configuration alias.`,
		},
		{
			`provider["aws"]["foo"]`,
			AbsProviderInstance{},
			`Provider type name must be followed by a configuration alias name.`,
		},
		{
			`module.foo`,
			AbsProviderInstance{},
			`Provider address must begin with "provider.", followed by a provider type name.`,
		},
		{
			`provider[0]`,
			AbsProviderInstance{},
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

			got, diags := ParseAbsProviderInstance(traversal)

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
		Config AbsProviderInstance
		Want   string
	}{
		{
			AbsProviderInstance{
				Module:   RootModuleInstance,
				Provider: NewLegacyProvider("foo"),
			},
			`provider["registry.opentofu.org/-/foo"]`,
		},
		{
			AbsProviderInstance{
				Module:   RootModuleInstance.Child("child_module", NoKey),
				Provider: NewDefaultProvider("foo"),
			},
			`module.child_module.provider["registry.opentofu.org/hashicorp/foo"]`,
		},
		{
			AbsProviderInstance{
				Module:   RootModuleInstance,
				Key:      StringKey("bar"),
				Provider: NewDefaultProvider("foo"),
			},
			`provider["registry.opentofu.org/hashicorp/foo"].bar`,
		},
		{
			AbsProviderInstance{
				Module:   RootModuleInstance.Child("child_module", NoKey),
				Key:      StringKey("bar"),
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

func TestParseLegacyAbsProviderInstanceStr(t *testing.T) {
	tests := []struct {
		Input string
		Want  AbsProviderInstance
	}{
		{
			`provider.foo`,
			AbsProviderInstance{
				Module:   RootModuleInstance,
				Provider: NewLegacyProvider("foo"),
			},
		},
		{
			`module.child_module.provider.foo`,
			AbsProviderInstance{
				Module:   RootModuleInstance.Child("child_module", NoKey),
				Provider: NewLegacyProvider("foo"),
			},
		},
		{
			`provider.terraform`,
			AbsProviderInstance{
				Module:   RootModuleInstance,
				Provider: NewBuiltInProvider("terraform"),
			},
		},
	}

	for _, test := range tests {
		got, _ := ParseLegacyAbsProviderInstanceStr(test.Input)
		if !reflect.DeepEqual(got, test.Want) {
			t.Errorf("wrong result. Got %s, want %s\n", got, test.Want)
		}
	}
}
