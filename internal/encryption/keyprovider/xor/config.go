// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package xor

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

// Config contains the configuration for this key provider supplied by the user. This struct must have hcl tags in order
// to function.
type Config struct {
	A keyprovider.Output `hcl:"a"`
	B keyprovider.Output `hcl:"b"`
}

// Build will create the usable key provider.
func (c Config) Build() (keyprovider.KeyProvider, keyprovider.KeyMeta, error) {
	if len(c.A.EncryptionKey) == 0 {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{
			Message: "Missing A encryption key",
		}
	}
	if len(c.B.EncryptionKey) == 0 {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{
			Message: "Missing B encryption key",
		}
	}
	if len(c.A.EncryptionKey) != len(c.B.EncryptionKey) {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{
			Message: fmt.Sprintf("The two provided encryption keys are not equal in length (%d vs %d bytes)", len(c.A.EncryptionKey), len(c.B.EncryptionKey)),
		}
	}
	if len(c.A.DecryptionKey) != len(c.B.DecryptionKey) {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{
			Message: fmt.Sprintf("The two provided decryption keys are not equal in length (%d vs %d bytes)", len(c.A.DecryptionKey), len(c.B.DecryptionKey)),
		}
	}

	encryptionKey := make([]byte, len(c.A.EncryptionKey))
	for i := range c.A.EncryptionKey {
		encryptionKey[i] = c.A.EncryptionKey[i] ^ c.B.EncryptionKey[i]
	}
	decryptionKey := make([]byte, len(c.A.DecryptionKey))
	for i := range c.A.DecryptionKey {
		decryptionKey[i] = c.A.DecryptionKey[i] ^ c.B.DecryptionKey[i]
	}
	return &xorKeyProvider{keyprovider.Output{
		EncryptionKey: encryptionKey,
		DecryptionKey: decryptionKey,
	}}, nil, nil
}
