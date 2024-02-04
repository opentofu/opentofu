package oras

import (
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

type Backend struct{}

func New() backend.Backend {
	return &Backend{}
}

func (b *Backend) ConfigSchema() *configschema.Block {
	return &configschema.Block{
		Attributes: map[string]*configschema.Attribute{},
	}
}

func (b *Backend) PrepareConfig(obj cty.Value) (cty.Value, tfdiags.Diagnostics) {
	panic("unimplemented")
}

func (b *Backend) Configure(obj cty.Value) tfdiags.Diagnostics { panic("unimplemented") }
