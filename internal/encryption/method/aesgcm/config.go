// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package aesgcm

import (
	"fmt"
	"github.com/opentofu/opentofu/internal/collections"

	"github.com/opentofu/opentofu/internal/encryption/method"
)

// The following settings follow the NIST SP 800-38D recommendation.
const (
	defaultTagSize   = 16
	defaultNonceSize = 12
	minimumNonceSize = 1
)

// validKeyLengths holds the valid key lengths supported by this method.
var validKeyLengths = collections.Set[int]{
	16: {},
	24: {},
	32: {},
}

// validTagSizes contains the valid tag sizes according to NIST SP 800-38D 5.2.1.2
var validTagSizes = collections.Set[int]{
	// These values are not supported by Go, see gcmMinimumTagSize in aes_gcm.go. They should also not be used for
	// general-purpose applications.
	// 4:  {},
	// 8:  {},
	12: {},
	13: {},
	14: {},
	15: {},
	16: {},
}

// Config is the configuration for the AES-GCM method.
type Config struct {
	// Key is the encryption key for the AES-GCM encryption. It has to be 16, 24, or 32 bytes long for AES-128, 192, or
	// 256, respectively.
	Key []byte `hcl:"key"`

	// AAD is the Additional Authenticated Data that is authenticated, but not encrypted. In the Go implementation, this
	// data serves as a canary value against replay attacks. The AAD value on decryption must match this setting,
	// otherwise the decryption will fail. (Note: this is Go-specific and differs from the NIST SP 800-38D description
	// of the AAD.)
	AAD []byte `hcl:"aad,optional"`

	// NonceSize describes the length of the nonce. The default (and minimum) value is 12 bytes as per the NIST
	// SP 800-38D recommendation. Do not change this value unless you know what you are doing. This setting is included
	// to give future users the ability to upgrade/downgrade in case new research into AES-GCM emerges and rollovers
	// need to be handled.
	NonceSize int `hcl:"nonce_size,optional"`

	// TagSize describes the length of the message authentication tag. The default and maximum value is 16 bytes, the
	// minimum is 12 bytes as per the NIST SP 800-38D recommendation. Do not change, and especially do not lower this,
	// unless you know what you are doing. This setting is included to give future users the ability to
	// upgrade/downgrade in case new research into AES-GCM emerges and rollovers need to be handled.
	TagSize int `hcl:"tag_size,optional"`
}

// Build checks the validity of the configuration and returns a ready-to-use AES-GCM implementation.
func (c Config) Build() (method.Method, error) {
	keyLength := len(c.Key)
	if !validKeyLengths.Has(keyLength) {
		return nil, &method.ErrInvalidConfiguration{
			Cause: fmt.Errorf(
				"AES-GCM requires the key length to be one of: %s, received %d bytes",
				validKeyLengths.String(),
				keyLength,
			),
		}
	}

	if !validTagSizes.Has(c.TagSize) {
		return nil, &method.ErrInvalidConfiguration{
			Cause: fmt.Errorf(
				"AES-GCM requires one of the following tag lengths: %s, but %d was given",
				validKeyLengths.String(),
				c.TagSize,
			),
		}
	}

	if c.NonceSize < minimumNonceSize {
		return nil, &method.ErrInvalidConfiguration{
			Cause: fmt.Errorf(
				"the minimum nonce size for AES-GCM is %d, but only %d bytes were configured",
				minimumNonceSize,
				c.NonceSize,
			),
		}
	}

	return &aesgcm{
		c.Key,
		c.AAD,
		c.NonceSize,
		c.TagSize,
	}, nil
}
