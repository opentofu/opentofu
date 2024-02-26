// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pbkdf2

import (
	"crypto/rand"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"io"
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
func New() keyprovider.Descriptor {
	return &descriptor{
		randomSource: rand.Reader,
	}
}

type descriptor struct {
	randomSource io.Reader
}

func (f descriptor) ID() keyprovider.ID {
	return "pbkdf2"
}

func (f descriptor) ConfigStruct() keyprovider.Config {
	return &Config{
		randomSource: f.randomSource,
		Passphrase:   "",
		KeyLength:    DefaultKeyLength,
		Iterations:   DefaultIterations,
		HashFunction: DefaultHashFunctionName,
		SaltLength:   DefaultSaltLength,
	}
}
