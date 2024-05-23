package hcl2shim

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	hclJSON "github.com/hashicorp/hcl/v2/json"
	"github.com/zclconf/go-cty/cty"
)

func TestConvertJSONExpressionToHCL(t *testing.T) {
	tests := []struct {
		Input string
		Want  hcl.Traversal
	}{
		{
			Input: "hello",
			Want: hcl.Traversal{
				hcl.TraverseRoot{Name: "hello"}}},
		{
			Input: "resource.test_resource[0]",
			Want: hcl.Traversal{
				hcl.TraverseRoot{Name: "boop"},
				hcl.TraverseAttr{Name: "test_resource"},
				hcl.TraverseIndex{Key: cty.NumberIntVal(0)},
			},
		},
	}

	for _, test := range tests {
		JSONExpr, diags := hclJSON.ParseExpression([]byte(`"`+test.Input+`"`), "")
		if diags.HasErrors() {
			t.Errorf("got %d diagnostics; want 0", len(diags))
			for _, d := range diags {
				t.Logf("  - %s", d.Error())
			}
		}

		want, diags := hclsyntax.ParseExpression([]byte(test.Input), "", hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			t.Errorf("got %d diagnostics; want 0", len(diags))
			for _, d := range diags {
				t.Logf("  - %s", d.Error())
			}
		}

		t.Run(test.Input, func(t *testing.T) {
			resultExpr, diags := convertJSONExpressionToHCL(JSONExpr)
			if diags.HasErrors() {
				t.Errorf("got %d diagnostics; want 0", len(diags))
				for _, d := range diags {
					t.Logf("  - %s", d.Error())
				}
			}

			if resultExpr == want {
				t.Errorf("got %s, but want %s", resultExpr, want)
			}
		})
	}
}
