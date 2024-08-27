// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pbkdf2

import (
	"fmt"

	"github.com/terramate-io/opentofulib/internal/encryption/keyprovider"
)

// Metadata describes the metadata to be stored alongside the encrypted form.
type Metadata struct {
	Salt         []byte           `json:"salt"`
	Iterations   int              `json:"iterations"`
	HashFunction HashFunctionName `json:"hash_function"`
	KeyLength    int              `json:"key_length"`
}

func (m Metadata) isPresent() bool {
	return len(m.Salt) != 0 && m.Iterations != 0 && m.HashFunction != "" && m.KeyLength != 0
}

func (m Metadata) validate() error {
	if m.Iterations < 0 {
		return &keyprovider.ErrInvalidMetadata{
			Message: fmt.Sprintf("invalid number of iterations (%d)", m.Iterations),
		}
	}
	if m.KeyLength < 0 {
		return &keyprovider.ErrInvalidMetadata{
			Message: fmt.Sprintf("invalid key length (%d)", m.KeyLength),
		}
	}
	if m.HashFunction != "" {
		if err := m.HashFunction.Validate(); err != nil {
			return &keyprovider.ErrInvalidMetadata{
				Message: "invalid hash function name",
				Cause:   err,
			}
		}
	}
	return nil
}
