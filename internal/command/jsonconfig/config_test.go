// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsonconfig

import (
	"testing"
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

var normalizeProviderConfigKeysTestProviders = map[string]providerConfig{
	"null":   {},
	"null.a": {},
	"module.a:null": {
		parentKey: "null",
	},
	"module.a:null.a": {
		parentKey: "null.a",
	},
	"module.a:null.b": {
		parentKey: "null",
	},
}

var normalizeProviderConfigKeysTests = map[string]struct {
	inputMod  module
	resultMod module
}{
	"no-changes": {
		inputMod: module{
			Resources: []resource{{ProviderConfigKey: "null"}},
		},
		resultMod: module{
			Resources: []resource{{ProviderConfigKey: "null"}},
		},
	},
	"single-parent": {
		inputMod: module{
			Resources: []resource{{ProviderConfigKey: "null"}},
			ModuleCalls: map[string]moduleCall{
				"a": {
					Module: module{
						Resources: []resource{{ProviderConfigKey: "module.a:null"}},
					},
				},
			},
		},
		resultMod: module{
			Resources: []resource{{ProviderConfigKey: "null"}},
			ModuleCalls: map[string]moduleCall{
				"a": {
					Module: module{
						Resources: []resource{{ProviderConfigKey: "null"}},
					},
				},
			},
		},
	},
	"multiple-parents": {
		inputMod: module{
			Resources: []resource{
				{ProviderConfigKey: "null"},
			},
			ModuleCalls: map[string]moduleCall{
				"a": {
					Module: module{
						Resources: []resource{{
							ProviderConfigKeys: []string{"module.a:null", "module.a:null.a"},
						}},
					},
				},
			},
		},
		resultMod: module{
			Resources: []resource{
				{ProviderConfigKey: "null"},
			},
			ModuleCalls: map[string]moduleCall{
				"a": {
					Module: module{
						Resources: []resource{{
							ProviderConfigKeys: []string{"null", "null.a"},
						}},
					},
				},
			},
		},
	},
	"multiple-duplicated-parents": {
		inputMod: module{
			Resources: []resource{
				{ProviderConfigKey: "null"},
			},
			ModuleCalls: map[string]moduleCall{
				"a": {
					Module: module{
						Resources: []resource{{
							ProviderConfigKeys: []string{"module.a:null.b", "module.a:null"},
						}},
					},
				},
			},
		},
		resultMod: module{
			Resources: []resource{
				{ProviderConfigKey: "null"},
			},
			ModuleCalls: map[string]moduleCall{
				"a": {
					Module: module{
						Resources: []resource{{
							ProviderConfigKey: "null",
						}},
					},
				},
			},
		},
	},
}

func TestNormalizeProviderConfigKeys(t *testing.T) {
	t.Parallel()

	for name, test := range normalizeProviderConfigKeysTests {
		test := test

		t.Run(name, func(t *testing.T) {
			normalizeModuleProviderKeys(&test.inputMod, normalizeProviderConfigKeysTestProviders)

			compareResourceProviders(t, test.resultMod, test.inputMod)
		})
	}
}

func compareResourceProviders(t *testing.T, want, got module) {
	if len(want.Resources) != len(got.Resources) {
		t.Fatalf("Modules have different number of resources: %v, %v", want.Resources, got.Resources)
	}

	for i := range want.Resources {
		want, got := want.Resources[i], got.Resources[i]

		if want.ProviderConfigKey != got.ProviderConfigKey {
			t.Fatalf("ProviderConfigKey fields do not match: '%v' and '%v'", want.ProviderConfigKey, got.ProviderConfigKey)
		}

		if len(want.ProviderConfigKeys) != len(got.ProviderConfigKeys) {
			t.Fatalf("Resources have different number of ProviderConfigKeys: %v and %v", want.ProviderConfigKeys, got.ProviderConfigKeys)
		}

		for i := range want.ProviderConfigKeys {
			want, got := want.ProviderConfigKeys[i], got.ProviderConfigKeys[i]
			if want != got {
				t.Fatalf("Resources have different ProviderConfigKeys on %d index: %v, %v", i, want, got)
			}
		}
	}

	if len(want.ModuleCalls) != len(got.ModuleCalls) {
		t.Fatalf("Modules have different number of module calls: %v, %v", want.ModuleCalls, got.ModuleCalls)
	}

	for k := range want.ModuleCalls {
		if _, ok := got.ModuleCalls[k]; !ok {
			t.Fatalf("Module call not found: %v", k)
		}

		compareResourceProviders(t, want.ModuleCalls[k].Module, got.ModuleCalls[k].Module)
	}
}
