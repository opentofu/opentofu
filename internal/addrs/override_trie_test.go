// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0
package addrs

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// mustAbsResourceRange parses the given string into an AbsResourceInstance.
// If the given string is not parsable, it panics.
func mustAbsResourceRange(str string) AbsResourceInstance {
	out, diags := parseAbsResourceRangeStr(str)
	if diags.HasErrors() {
		panic(diags)
	}
	return out
}

type override struct {
	Address *AbsResourceInstance
	Value   string
}

func TestOverrideTrie(t *testing.T) {
	tests := []struct {
		TestName    string
		Default     string
		Overrides   []override
		Query       *AbsResourceInstance
		ErrorSubstr string
		Want        *string
	}{
		{
			TestName: "basic input",
			Overrides: []override{
				{
					Address: new(mustAbsResourceRange(`module.vps["us-central1"].tofu_network.spiderweb`)),
					Value:   "usa",
				},
			},
			Query: new(mustAbsResourceRange(`module.vps["us-central1"].tofu_network.spiderweb`)),
			Want:  new("usa"),
		},
		{
			TestName: "wildcard override",
			Overrides: []override{
				{
					Address: new(mustAbsResourceRange(`module.vps[*].tofu_network.spiderweb`)),
					Value:   "global",
				},
			},
			Query: new(mustAbsResourceRange(`module.vps["us-central1"].tofu_network.spiderweb`)),
			Want:  new("global"),
		},
		{
			TestName: "use default",
			Overrides: []override{
				{
					Address: new(mustAbsResourceRange(`module.vps["apac"].tofu_network.spiderweb`)),
					Value:   "australia",
				},
			},
			Query: new(mustAbsResourceRange(`module.vps["us-central1"].tofu_network.spiderweb`)),
		},
		{
			TestName: "error on wildcard",
			Overrides: []override{
				{
					Address: new(mustAbsResourceRange(`module.vps["apac"].tofu_network.spiderweb`)),
					Value:   "australia",
				},
			},
			Query:       new(mustAbsResourceRange(`module.vps[*].tofu_network.spiderweb`)),
			ErrorSubstr: "Wildcard key not expected",
		},
		{
			TestName: "wildcard fallback",
			Overrides: []override{
				{
					Address: new(mustAbsResourceRange(`module.vps[*].module.subnet[*].tofu_network.spiderweb["a"]`)),
					Value:   "australia",
				},
				{
					Address: new(mustAbsResourceRange(`module.vps["a"].module.subnet["a"].tofu_network.spiderweb["b"]`)),
					Value:   "burkina faso",
				},
			},
			Query: new(mustAbsResourceRange(`module.vps["a"].module.subnet["a"].tofu_network.spiderweb["a"]`)),
			Want:  new("australia"),
		},
	}
	for _, test := range tests {
		t.Run(test.TestName, func(t *testing.T) {
			trie := NewOverrideTrie[string]()
			for _, override := range test.Overrides {
				trie.Set(override.Address, override.Value, nil)
			}

			got, diags := trie.Get(test.Query)
			var gotErr string
			if diags.HasErrors() {
				gotErr = diags.Err().Error()
			}

			// if ErrorSubstr is empty, this checks that there were no errors
			if !strings.Contains(gotErr, test.ErrorSubstr) {
				// this is incorrect:
				if test.ErrorSubstr == "" {
					// we either got an error when we didn't expect...
					t.Errorf("unexpected error encountered: %s", gotErr)
				} else {
					// or got no error when we *were* expecting one
					t.Errorf("expected error containing \"%s\", but no error was returned", test.ErrorSubstr)
				}
			}
			if diff := cmp.Diff(test.Want, got); diff != "" {
				t.Errorf("unexpected returned value (-want,+got):\n%s", diff)
			}
		})
	}
}
