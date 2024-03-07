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

type hashFunction func() hash.Hash

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

func (h HashFunctionName) Function() hashFunction {
	return hashFunctions[h]
}

type Config struct {
	// Set by the descriptor
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

	return &pbkdf2KeyProvider{c}, new(Metadata), nil
}
