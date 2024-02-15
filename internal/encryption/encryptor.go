package encryption

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/encryption/method"
	"github.com/opentofu/opentofu/internal/encryption/registry"
	"github.com/zclconf/go-cty/cty"
)

type encryptor struct {
	cfg *Config
	reg registry.Registry

	// Used to evaluate hcl expressions
	ctx *hcl.EvalContext

	metadata map[string][]byte

	// Used to build EvalContext (and related mappings)
	keyValues    map[string]map[string]cty.Value
	methodValues map[string]map[string]cty.Value
	methods      map[string]method.Method
}

func newEncryptor(e *encryption, meta map[string][]byte) (*encryptor, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	enc := &encryptor{
		cfg: e.cfg,
		reg: e.reg,

		ctx: &hcl.EvalContext{
			Variables: map[string]cty.Value{},
		},

		metadata: meta,
	}

	diags = append(diags, enc.setupKeyProviders()...)
	if diags.HasErrors() {
		return nil, diags
	}
	diags = append(diags, enc.setupMethods()...)
	if diags.HasErrors() {
		return nil, diags
	}

	return enc, diags
}
