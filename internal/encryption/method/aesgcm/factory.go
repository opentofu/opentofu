package aesgcm

import "github.com/opentofu/opentofu/internal/encryption/method"

// New creates a new factory for the AES-GCM encryption method, which requires a 32-byte key.
func New() method.Factory {
	return &factory{}
}

type factory struct {
}

func (f *factory) ID() method.ID {
	return "aes_gcm"
}

func (f *factory) ConfigStruct() method.Config {
	return &Config{}
}
