package addrs

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// parseAbsResourceRangeStr is copied from ParseAbsResourceInstanceStr (more or less),
// but with functions edited to refer to ranges instead.
// TODO should this be outside of the test file?
func parseAbsResourceRangeStr(str string) (AbsResourceInstance, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	expr, parseDiags := hclsyntax.ParseExpression([]byte(str), "", hcl.InitialPos)
	diags = diags.Append(parseDiags)
	if parseDiags.HasErrors() {
		return AbsResourceInstance{}, diags
	}

	traversal, parseDiags := hcl.AbsTraversalPatternForExpr(expr)
	diags = diags.Append(parseDiags)
	if parseDiags.HasErrors() {
		return AbsResourceInstance{}, diags
	}

	addr, addrDiags := ParseAbsResourceRange(traversal)
	diags = diags.Append(addrDiags)
	return addr, diags
}

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
		WantDefault bool
		Want        string
	}{
		{
			TestName: "basic input",
			Default:  "somewhere",
			Overrides: []override{
				{
					Address: new(getAbsResourceRangeOrPanic(`module.vps["us-central1"].tofu_network.spiderweb`)),
					Values:  "usa",
				},
			},
			Query: new(getAbsResourceRangeOrPanic(`module.vps["us-central1"].tofu_network.spiderweb`)),
			Want:  "usa",
		},
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
