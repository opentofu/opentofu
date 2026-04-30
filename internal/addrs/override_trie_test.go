// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0
package addrs

import (
	"testing"
)

// shorthand function to obtain known good strings.
// Please please please do not use this outside of tests!!!
func getAbsResourceRangeOrPanic(str string) AbsResourceInstance {
	out, diags := parseAbsResourceRangeStr(str)
	if diags.HasErrors() {
		panic(diags)
	}
	return out
}

type override struct {
	Address *AbsResourceInstance
	Values  string
}

func TestOverrideTrie(t *testing.T) {
	tests := []struct {
		TestName    string
		Default     string
		Overrides   []override
		Query       *AbsResourceInstance
		WantMissing bool
		WantError   bool
		Want        *string
	}{
		{
			TestName: "basic input",
			Overrides: []override{
				{
					Address: new(getAbsResourceRangeOrPanic(`module.vps["us-central1"].tofu_network.spiderweb`)),
					Values:  "usa",
				},
			},
			Query: new(getAbsResourceRangeOrPanic(`module.vps["us-central1"].tofu_network.spiderweb`)),
			Want:  new("usa"),
		},
		{
			TestName: "wildcard override",
			Overrides: []override{
				{
					Address: new(getAbsResourceRangeOrPanic(`module.vps[*].tofu_network.spiderweb`)),
					Values:  "global",
				},
			},
			Query: new(getAbsResourceRangeOrPanic(`module.vps["us-central1"].tofu_network.spiderweb`)),
			Want:  new("global"),
		},
		{
			TestName: "use default",
			Overrides: []override{
				{
					Address: new(getAbsResourceRangeOrPanic(`module.vps["apac"].tofu_network.spiderweb`)),
					Values:  "australia",
				},
			},
			Query:       new(getAbsResourceRangeOrPanic(`module.vps["us-central1"].tofu_network.spiderweb`)),
			WantMissing: true,
			Want:        new("somewhere"),
		},
		{
			TestName: "error on wildcard",
			Overrides: []override{
				{
					Address: new(getAbsResourceRangeOrPanic(`module.vps["apac"].tofu_network.spiderweb`)),
					Values:  "australia",
				},
			},
			Query:     new(getAbsResourceRangeOrPanic(`module.vps[*].tofu_network.spiderweb`)),
			WantError: true,
		},
		{
			TestName: "wildcard fallback",
			Overrides: []override{
				{
					Address: new(getAbsResourceRangeOrPanic(`module.vps[*].module.subnet[*].tofu_network.spiderweb["a"]`)),
					Values:  "australia",
				},
				{
					Address: new(getAbsResourceRangeOrPanic(`module.vps["a"].module.subnet["a"].tofu_network.spiderweb["b"]`)),
					Values:  "burkina faso",
				},
			},
			Query: new(getAbsResourceRangeOrPanic(`module.vps["a"].module.subnet["a"].tofu_network.spiderweb["a"]`)),
			Want:  new("australia"),
		},
	}
	for _, test := range tests {
		t.Run(test.TestName, func(t *testing.T) {
			trie := NewOverrideTrie[string]()
			for _, override := range test.Overrides {
				trie.Set(override.Address, override.Values, nil)
			}

			got, diags := trie.Get(test.Query)
			if diags.HasErrors() {
				if !test.WantError {
					// unexpectedly encountered an error
					t.Errorf("got an error from trie override retrieval: %s", diags.Err().Error())
				}
				return
			} else {
				if test.WantError {
					t.Fatal("expected an error, but did not get one")
				}
			}

			if test.WantMissing && got != nil {
				t.Error("expected to get nothing, but found something")
			}
			if !test.WantMissing && got == nil {
				t.Error("expected something, but didn't find anything")
			} else if !test.WantMissing && *test.Want != *got {
				t.Errorf("wrong result: expected %s, got %s\n", *test.Want, *got)
			}
		})
	}
}
