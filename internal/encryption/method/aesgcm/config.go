// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package aesgcm

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"

	"github.com/opentofu/opentofu/internal/collections"

	"github.com/opentofu/opentofu/internal/encryption/method"
)

// validKeyLengths holds the valid key lengths supported by this method.
var validKeyLengths = collections.NewSet[int](16, 24, 32)

// Config is the configuration for the AES-GCM method.
type Config struct {
	// Key is the encryption key for the AES-GCM encryption. It has to be 16, 24, or 32 bytes long for AES-128, 192, or
	// 256, respectively.
	Keys keyprovider.Output

	// AAD is the Additional Authenticated Data that is authenticated, but not encrypted. In the Go implementation, this
	// data serves as a canary value against replay attacks. The AAD value on decryption must match this setting,
	// otherwise the decryption will fail. (Note: this is Go-specific and differs from the NIST SP 800-38D description
	// of the AAD.)
	AAD []byte
}

// Build checks the validity of the configuration and returns a ready-to-use AES-GCM implementation.
func (c *Config) Build() (method.Method, error) {
	encryptionKey := c.Keys.EncryptionKey
	decryptionKey := c.Keys.DecryptionKey

	if !validKeyLengths.Has(len(encryptionKey)) {
		return nil, &method.ErrInvalidConfiguration{
			Cause: fmt.Errorf(
				"AES-GCM requires the key length to be one of: %s, received %d bytes in the encryption key",
				validKeyLengths.String(),
				len(encryptionKey),
			),
		}
	}

	if len(decryptionKey) > 0 {
		if !validKeyLengths.Has(len(decryptionKey)) {
			return nil, &method.ErrInvalidConfiguration{
				Cause: fmt.Errorf(
					"AES-GCM requires the key length to be one of: %s, received %d bytes in the decryption key",
					validKeyLengths.String(),
					len(decryptionKey),
				),
			}
		}
	}

	return &aesgcm{
		encryptionKey,
		decryptionKey,
		c.AAD,
	}, nil
}
