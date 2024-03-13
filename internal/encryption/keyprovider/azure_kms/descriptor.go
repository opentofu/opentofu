package azure_kms

import (
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

func New() keyprovider.Descriptor {
	return &descriptor{}
}

type descriptor struct {
}

func (f descriptor) ID() keyprovider.ID {
	return "azure_kms"
}

func (f descriptor) ConfigStruct() keyprovider.Config {
	return &Config{}
}
