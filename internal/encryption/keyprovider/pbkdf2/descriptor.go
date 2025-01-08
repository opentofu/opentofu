// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pbkdf2

import (
	"crypto/rand"
	"io"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

const (
	// DefaultSaltLength specifies the default salt length in bytes.
	DefaultSaltLength int = 32
	// DefaultIterations contains the default iterations to use. The number is set to the current recommendations
	// outlined here:
	// https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html#pbkdf2
	DefaultIterations int = 600000
	// DefaultKeyLength is the default output length. We set it to the key length required by AES-GCM 256
	DefaultKeyLength int = 32
)

// New creates a new PBKDF2 key provider descriptor.
func New() Descriptor {
	return &descriptor{
		randomSource: rand.Reader,
	}
}

// Descriptor provides TypedConfig on top of keyprovider.Descriptor.
type Descriptor interface {
	keyprovider.Descriptor

	TypedConfig() *Config
}

type descriptor struct {
	randomSource io.Reader
}

func (f descriptor) ID() keyprovider.ID {
	return "pbkdf2"
}

func (f descriptor) TypedConfig() *Config {
	return &Config{
		randomSource: f.randomSource,
		Passphrase:   "",
		Chain:        nil,
		KeyLength:    DefaultKeyLength,
		Iterations:   DefaultIterations,
		HashFunction: DefaultHashFunctionName,
		SaltLength:   DefaultSaltLength,
	}
}

func (f descriptor) ConfigStruct() keyprovider.Config {
	return f.TypedConfig()
}
