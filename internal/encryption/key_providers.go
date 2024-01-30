package encryption

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
)

type PassphraseKeyProvider struct{}

func (p PassphraseKeyProvider) Schema() DefinitionSchema {
	return DefinitionSchema{
		BodySchema: &hcl.BodySchema{
			Attributes: []hcl.AttributeSchema{
				{Name: "passphrase", Required: true},
			},
		},
	}
}

func (p PassphraseKeyProvider) Configure(content *hcl.BodyContent, deps map[string]KeyProvider) (KeyProvider, hcl.Diagnostics) {
	var passphrase string

	diags := gohcl.DecodeExpression(content.Attributes["passphrase"].Expr, nil, &passphrase)
	if diags.HasErrors() {
		return nil, diags
	}

	if len(passphrase) == 0 {
		panic("TODO diags: empty passphrase")
	}

	return func() ([]byte, error) {
		return []byte(passphrase), nil
	}, diags
}
