package encryption

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
)

func PassphraseKeyProvider() (DefinitionSchema, KeyProvider) {
	schema := DefinitionSchema{
		BodySchema: &hcl.BodySchema{
			Attributes: []hcl.AttributeSchema{
				{Name: "passphrase", Required: true},
			},
		},
	}

	return schema, func(content *hcl.BodyContent, deps map[string]KeyData) (KeyData, hcl.Diagnostics) {
		var passphrase string

		diags := gohcl.DecodeExpression(content.Attributes["passphrase"].Expr, nil, &passphrase)
		if diags.HasErrors() {
			return nil, diags
		}

		if len(passphrase) == 0 {
			panic("TODO diags: empty passphrase")
		}

		return []byte(passphrase), nil
	}
}
