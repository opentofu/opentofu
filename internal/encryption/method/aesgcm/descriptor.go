package aesgcm

import "github.com/opentofu/opentofu/internal/encryption/method"

// New creates a new descriptor for the AES-GCM encryption method, which requires a 32-byte key.
func New() method.Descriptor {
	return &descriptor{}
}

type descriptor struct {
}

func (f *descriptor) ID() method.ID {
	return "aes_gcm"
}

func (f *descriptor) ConfigStruct() method.Config {
	return &Config{}
}
