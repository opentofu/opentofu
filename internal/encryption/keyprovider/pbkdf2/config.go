// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pbkdf2

import (
	"fmt"
	"hash"
	"io"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

// HashFunctionName describes a hash function to use for PBKDF2 hash generation. the hash function influences
type HashFunctionName string

func (h HashFunctionName) Validate() error {
	if h == "" {
		return &keyprovider.ErrInvalidConfiguration{Message: "please specify a hash function"}
	}
	if _, ok := hashFunctions[h]; !ok {
		return &keyprovider.ErrInvalidConfiguration{Message: fmt.Sprintf("invalid hash function name: %s", h)}
	}
	return nil
}

type hashFunction struct {
	functionProvider func() hash.Hash
}

type Config struct {
	randomSource io.Reader

	Passphrase   string           `hcl:"passphrase"`
	KeyLength    int              `hcl:"key_length,optional"`
	Iterations   int              `hcl:"iterations,optional"`
	HashFunction HashFunctionName `hcl:"hash_function,optional"`
	SaltLength   int              `hcl:"salt_length,optional"`
}

func (c Config) Build() (keyprovider.KeyProvider, keyprovider.KeyMeta, error) {
	// TODO: validate passphrase length.
	// TODO: validate iterations
	// TODO: validate salt length

	if err := c.HashFunction.Validate(); err != nil {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{
			Cause: err,
		}
	}

	encryptHashFunction := hashFunctions[c.HashFunction]

	return &pbkdf2KeyProvider{
		randomSource:         c.randomSource,
		passphrase:           c.Passphrase,
		keyLength:            c.KeyLength,
		iterations:           c.Iterations,
		hashFunctionName:     c.HashFunction,
		hashFunctionProvider: encryptHashFunction.functionProvider,
		saltLength:           c.SaltLength,
	}, Metadata{}, nil
}
