package aesgcm

import (
	"fmt"
	"github.com/opentofu/opentofu/internal/encryption/method"
)

type Config struct {
	Key []byte `hcl:"cipher"`
}

func (c Config) Build() (method.Method, error) {
	if len(c.Key) != 32 {
		return nil, fmt.Errorf("AES-GCM requires a 32-byte key")
	}
	return &aesgcm{
		c.Key,
	}, nil
}
