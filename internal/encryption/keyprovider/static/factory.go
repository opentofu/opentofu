package static

import (
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

func New() keyprovider.Factory {
	return &factory{}
}

type factory struct {
}

func (f factory) ID() keyprovider.ID {
	return "static"
}

func (f factory) ConfigStruct() keyprovider.Config {
	return &Config{}
}
