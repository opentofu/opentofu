// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsonconfig

import (
	"fmt"
	"slices"
	"testing"
)

var findSourceProviderConfigTests = map[string]struct {
	StartKey    string
	FullName    string
	ProviderMap map[string]providerConfig
	Want        []string
}{
	"zero": {
		StartKey:    "null",
		FullName:    "hashicorp/null",
		ProviderMap: map[string]providerConfig{},
		Want:        nil,
	},
	"no-parent": {
		StartKey: "null",
		FullName: "hashicorp/null",
		ProviderMap: map[string]providerConfig{
			"null": {
				Name:          "null",
				FullName:      "hashicorp/null",
				ModuleAddress: "",
			},
		},
		Want: []string{"null"},
	},
	"unknown-start": {
		StartKey: "null2",
		FullName: "hashicorp/null",
		ProviderMap: map[string]providerConfig{
			"null": {
				Name:          "null",
				FullName:      "hashicorp/null",
				ModuleAddress: "",
			},
		},
		Want: nil,
	},
	"unknown-full-name": {
		StartKey: "null",
		FullName: "hashicorp2/null",
		ProviderMap: map[string]providerConfig{
			"null": {
				Name:          "null",
				FullName:      "hashicorp/null",
				ModuleAddress: "",
			},
		},
		Want: nil,
	},
	"simple": {
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
				parentKeys:    []string{"null"},
			},
		},
		Want: []string{"null"},
	},
	"diff-full-name": {
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
				parentKeys:    []string{"null"},
			},
		},
		Want: []string{"module.a:null"},
	},
	"sub-sub-module": {
		StartKey: "module.a.module.b:null",
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
				parentKeys:    []string{"null"},
			},
			"module.a.module.b:null": {
				Name:          "module.b:null",
				FullName:      "hashicorp/null",
				ModuleAddress: "module.a.module.b",
				parentKeys:    []string{"module.a:null"},
			},
		},
		Want: []string{"null"},
	},
	"simple-multiple-providers": {
		StartKey: "module.a.module.b:null",
		FullName: "hashicorp/null",
		ProviderMap: map[string]providerConfig{
			"null": {
				Name:          "null",
				FullName:      "hashicorp/null",
				ModuleAddress: "",
			},
			"null.a": {
				Name:          "null.a",
				FullName:      "hashicorp/null",
				ModuleAddress: "",
			},
			"module.a:null": {
				Name:          "module.a:null",
				FullName:      "hashicorp/null",
				ModuleAddress: "module.a",
				parentKeys:    []string{"null", "null.a"},
			},
			"module.a.module.b:null": {
				Name:          "module.b:null",
				FullName:      "hashicorp/null",
				ModuleAddress: "module.a.module.b",
				parentKeys:    []string{"module.a:null"},
			},
		},
		Want: []string{"null", "null.a"},
	},
	"multiple-providers": {
		StartKey: "module.a.module.b:null",
		FullName: "hashicorp/null",
		ProviderMap: map[string]providerConfig{
			"null": {
				Name:          "null",
				FullName:      "hashicorp/null",
				ModuleAddress: "",
			},
			"null.a": {
				Name:          "null.a",
				FullName:      "hashicorp/null",
				ModuleAddress: "",
			},
			"null.b": {
				Name:          "null.a",
				FullName:      "hashicorp/null",
				ModuleAddress: "",
			},
			"module.a:null": {
				Name:          "module.a:null",
				FullName:      "hashicorp/null",
				ModuleAddress: "module.a",
				parentKeys:    []string{"null", "null.a"},
			},
			"module.a:null.a": {
				Name:          "module.a:null.a",
				FullName:      "hashicorp/null",
				ModuleAddress: "module.a",
				parentKeys:    []string{"null", "null.b"},
			},
			"module.a.module.b:null": {
				Name:          "module.b:null",
				FullName:      "hashicorp/null",
				ModuleAddress: "module.a.module.b",
				parentKeys:    []string{"module.a:null", "module.a:null.a"},
			},
		},
		// FindSourceProviderConfig doesn't do normalizing so
		// it is fine to have 'duplicated' source providers.
		Want: []string{"null", "null.a", "null", "null.b"},
	},
}

func TestFindSourceProviderConfig(t *testing.T) {
	t.Parallel()

	for name, test := range findSourceProviderConfigTests {
		test := test

		t.Run(name, func(t *testing.T) {
			if name == "multiple-providers" {
				fmt.Println("hello")
			}

			got := findSourceProviderKeys([]string{test.StartKey}, test.FullName, test.ProviderMap)
			if slices.Compare(got, test.Want) != 0 {
				t.Errorf("wrong result:\nGot: %#v\nWant: %#v\n", got, test.Want)
			}
		})
	}
}

var normalizeProviderConfigKeysTestProviders = map[string]providerConfig{
	"null":   {},
	"null.a": {},
	"module.a:null": {
		parentKeys: []string{"null"},
	},
	"module.a:null.a": {
		parentKeys: []string{"null.a"},
	},
	"module.a:null.b": {
		parentKeys: []string{"null"},
	},
}

var normalizeProviderConfigKeysTests = map[string]struct {
	inputMod  module
	resultMod module
}{
	"no-changes": {
		inputMod: module{
			Resources: []resource{{ProviderConfigKeys: []string{"null"}}},
		},
		resultMod: module{
			Resources: []resource{{ProviderConfigKeys: []string{"null"}}},
		},
	},
	"single-parent": {
		inputMod: module{
			Resources: []resource{{ProviderConfigKeys: []string{"null"}}},
			ModuleCalls: map[string]moduleCall{
				"a": {
					Module: module{
						Resources: []resource{{ProviderConfigKeys: []string{"module.a:null"}}},
					},
				},
			},
		},
		resultMod: module{
			Resources: []resource{{ProviderConfigKeys: []string{"null"}}},
			ModuleCalls: map[string]moduleCall{
				"a": {
					Module: module{
						Resources: []resource{{ProviderConfigKeys: []string{"null"}}},
					},
				},
			},
		},
	},
	"multiple-parents": {
		inputMod: module{
			Resources: []resource{
				{ProviderConfigKeys: []string{"null"}},
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
				{ProviderConfigKeys: []string{"null"}},
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
				{ProviderConfigKeys: []string{"null"}},
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
				{ProviderConfigKeys: []string{"null"}},
			},
			ModuleCalls: map[string]moduleCall{
				"a": {
					Module: module{
						Resources: []resource{{
							ProviderConfigKeys: []string{"null"},
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

		if len(want.ProviderConfigKeys) != len(got.ProviderConfigKeys) {
			t.Fatalf("Resources have different number of ProviderConfigKeys: %v and %v", want.ProviderConfigKeys, got.ProviderConfigKeys)
		}

		slices.Sort(want.ProviderConfigKeys)
		slices.Sort(got.ProviderConfigKeys)

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
