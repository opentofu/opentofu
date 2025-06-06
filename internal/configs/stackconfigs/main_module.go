package stackconfigs

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"

	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type MainModule struct {
	Source       hcl.Expression
	Inputs       hcl.Expression
	StateStorage *configs.StateStorage

	DeclRange hcl.Range
}

func decodeMainModuleBlock(block *hcl.Block) (*MainModule, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	content, hclDiags := block.Body.Content(mainModuleSchema)
	diags = diags.Append(hclDiags)
	if hclDiags.HasErrors() {
		return nil, diags
	}

	ret := &MainModule{
		DeclRange: block.DefRange,
	}

	if attr := content.Attributes["source"]; attr != nil {
		ret.Source = attr.Expr
	}
	if attr := content.Attributes["inputs"]; attr != nil {
		ret.Inputs = attr.Expr
	}
	for _, block := range content.Blocks {
		switch block.Type {
		case "state_storage":
			if ret.StateStorage != nil {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate state_storage block",
					Detail:   fmt.Sprintf("The main module's state storage was already configured at %s.", ret.StateStorage.DeclRange),
					Subject:  block.DefRange.Ptr(),
				})
				continue
			}
			ss, hclDiags := configs.DecodeStateStorageBlock(block)
			diags = diags.Append(hclDiags)
			ret.StateStorage = ss
		default:
			// Should not get here because the cases above should cover
			// everything listed in mainModuleSchema.
			panic(fmt.Sprintf("unhandled block type %q", block.Type))
		}
	}

	return ret, diags
}

var mainModuleSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "source"},
		{Name: "inputs"},
	},
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "state_storage", LabelNames: []string{"type", "name"}},
	},
}
