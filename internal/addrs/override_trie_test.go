package addrs

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty/cty"
)

func TestOverrideTrie(t *testing.T) {
	tests := []struct {
		TestName  string
		Default   map[string]cty.Value
		Overrides []struct {
			Address *AbsResourceInstance
			Values  map[string]cty.Value
		}
		Query       *AbsResourceInstance
		WantDefault bool
		Want        map[string]cty.Value
	}{
		// TODO add some good tests!
	}
	for _, test := range tests {
		t.Run(test.TestName, func(t *testing.T) {
			trie := NewOverrideTrie(test.Default)
			for _, override := range test.Overrides {
				trie.Set(override.Address, override.Values)
			}

			got, isNotDefault := trie.Get(test.Query)
			if isNotDefault == test.WantDefault {
				// TODO bad! We want the default!
			}

			if diff := cmp.Diff(test.Want, got, CmpOptionsForTesting); diff != "" {
				t.Error("wrong result:\n" + diff)
			}
		})
	}
}
