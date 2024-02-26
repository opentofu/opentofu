// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package aesgcm

import "github.com/opentofu/opentofu/internal/encryption/method"

// Descriptor integrates the method.Descriptor and provides a TypedConfig for easier configuration.
type Descriptor interface {
	method.Descriptor

	// TypedConfig returns a config typed for this method.
	TypedConfig() *Config
}

// New creates a new descriptor for the AES-GCM encryption method, which requires a 32-byte key.
func New() Descriptor {
	return &descriptor{}
}

type descriptor struct {
}

func (f *descriptor) TypedConfig() *Config {
	return &Config{
		Key:       nil,
		AAD:       nil,
		NonceSize: defaultNonceSize,
		TagSize:   defaultTagSize,
	}
}

func (f *descriptor) ID() method.ID {
	return "aes_gcm"
}

func (f *descriptor) ConfigStruct() method.Config {
	return f.TypedConfig()
}
