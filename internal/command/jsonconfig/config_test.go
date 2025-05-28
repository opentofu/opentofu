// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsonconfig

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tofu"
	"github.com/zclconf/go-cty/cty"
)

func TestFindSourceProviderConfig(t *testing.T) {
	tests := []struct {
		StartKey    string
		FullName    string
		ProviderMap map[string]providerConfig
		Want        string
	}{
		{
			StartKey:    "null",
			FullName:    "hashicorp/null",
			ProviderMap: map[string]providerConfig{},
			Want:        "",
		},
		{
			StartKey: "null",
			FullName: "hashicorp/null",
			ProviderMap: map[string]providerConfig{
				"null": {
					Name:          "null",
					FullName:      "hashicorp/null",
					ModuleAddress: "",
				},
			},
			Want: "null",
		},
		{
			StartKey: "null2",
			FullName: "hashicorp/null",
			ProviderMap: map[string]providerConfig{
				"null": {
					Name:          "null",
					FullName:      "hashicorp/null",
					ModuleAddress: "",
				},
			},
			Want: "",
		},
		{
			StartKey: "null",
			FullName: "hashicorp2/null",
			ProviderMap: map[string]providerConfig{
				"null": {
					Name:          "null",
					FullName:      "hashicorp/null",
					ModuleAddress: "",
				},
			},
			Want: "",
		},
		{
			StartKey: "module.a:null",
			FullName: "hashicorp/null",
			ProviderMap: map[string]providerConfig{
				"null": {
					Name:          "null",
					FullName:      "hashicorp/null",
					ModuleAddress: "",
				},
				"module.a:null": {
					Name:          "module.a:null",
					FullName:      "hashicorp/null",
					ModuleAddress: "module.a",
					parentKey:     "null",
				},
			},
			Want: "null",
		},
		{
			StartKey: "module.a:null",
			FullName: "hashicorp2/null",
			ProviderMap: map[string]providerConfig{
				"null": {
					Name:          "null",
					FullName:      "hashicorp/null",
					ModuleAddress: "",
				},
				"module.a:null": {
					Name:          "module.a:null",
					FullName:      "hashicorp2/null",
					ModuleAddress: "module.a",
					parentKey:     "null",
				},
			},
			Want: "module.a:null",
		},
	}

	for _, test := range tests {
		got := findSourceProviderKey(test.StartKey, test.FullName, test.ProviderMap)
		if got != test.Want {
			t.Errorf("wrong result:\nGot: %#v\nWant: %#v\n", got, test.Want)
		}
	}
}

func TestMarshalModule(t *testing.T) {
	t.Run("validate variables marshalling with all the required fields", func(t *testing.T) {
		varCfg := &configs.Variable{
			Name:        "myvar",
			Description: "myvar description",
			Deprecated:  "myvar deprecated message",
		}
		modCfg := configs.Config{
			Module: &configs.Module{
				Variables: map[string]*configs.Variable{
					"myvar": varCfg,
				},
			},
		}
		modCfg.Root = &modCfg

		out, err := marshalModule(&modCfg, &tofu.Schemas{}, addrs.RootModule.String())
		if err != nil {
			t.Fatalf("unexpected error during marshalling module: %s", err)
		}

		expected := module{
			Outputs:     map[string]output{},
			ModuleCalls: map[string]moduleCall{},
			Variables: map[string]*variable{
				"myvar": {
					Description: varCfg.Description,
					Deprecated:  varCfg.Deprecated,
				},
			},
		}
		if diff := cmp.Diff(expected, out); diff != "" {
			t.Errorf("unexpected diff: \n%s", diff)
		}
	})
	t.Run("validate resources marshalling", func(t *testing.T) {
		providerAddr := addrs.NewProvider("host", "namespace", "type")
		modCfg := configs.Config{
			Module: &configs.Module{
				ManagedResources: map[string]*configs.Resource{
					"test_res": {
						Mode: addrs.ManagedResourceMode,
						Name: "test_res",
						Type: "test_type",
						Config: &hclsyntax.Body{
							Attributes: map[string]*hclsyntax.Attribute{},
						},
						Provider: providerAddr,
					},
				},
				DataResources: map[string]*configs.Resource{
					"test_data": {
						Mode: addrs.DataResourceMode,
						Name: "test_data",
						Type: "test_type",
						Config: &hclsyntax.Body{
							Attributes: map[string]*hclsyntax.Attribute{},
						},
						Provider: providerAddr,
					},
				},
				EphemeralResources: map[string]*configs.Resource{
					"test_ephemeral": {
						Mode: addrs.EphemeralResourceMode,
						Name: "test_ephemeral",
						Type: "test_type",
						Config: &hclsyntax.Body{
							Attributes: map[string]*hclsyntax.Attribute{},
						},
						Provider: providerAddr,
					},
				},
			},
		}
		modCfg.Root = &modCfg

		resSchema := map[string]providers.Schema{
			"test_type": {
				Version: 0,
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"foo": {
							Type:     cty.String,
							Optional: true,
						},
					},
				},
			},
		}
		schema := &tofu.Schemas{
			Providers: map[addrs.Provider]providers.ProviderSchema{
				providerAddr: {
					ResourceTypes:      resSchema,
					EphemeralResources: resSchema,
					DataSources:        resSchema,
				},
			},
		}
		out, err := marshalModule(&modCfg, schema, addrs.RootModule.String())
		if err != nil {
			t.Fatalf("unexpected error during marshalling module: %s", err)
		}

		expected := module{
			Outputs:     map[string]output{},
			ModuleCalls: map[string]moduleCall{},
			Resources: []resource{
				{
					Address:           "test_type.test_res",
					Mode:              "managed",
					Type:              "test_type",
					Name:              "test_res",
					ProviderConfigKey: "test",
					Provisioners:      nil,
					Expressions:       make(map[string]any),
				},
				{
					Address:           "data.test_type.test_data",
					Mode:              "data",
					Type:              "test_type",
					Name:              "test_data",
					ProviderConfigKey: "test",
					Provisioners:      nil,
					Expressions:       make(map[string]any),
				},
				{
					Address:           "ephemeral.test_type.test_ephemeral",
					Mode:              "ephemeral",
					Type:              "test_type",
					Name:              "test_ephemeral",
					ProviderConfigKey: "test",
					Provisioners:      nil,
					Expressions:       make(map[string]any),
				},
			},
		}
		if diff := cmp.Diff(expected, out); diff != "" {
			t.Errorf("unexpected diff: \n%s", diff)
		}
	})
}
