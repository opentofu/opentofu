// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package external

import (
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/method"
)

// Config is the configuration for the AES-GCM method.
type Config struct {
	Keys           *keyprovider.Output `hcl:"keys,optional" json:"keys,omitempty" yaml:"keys"`
	EncryptCommand []string            `hcl:"encrypt_command" json:"encrypt_command" yaml:"encrypt_command"`
	DecryptCommand []string            `hcl:"decrypt_command" json:"decrypt_command" yaml:"decrypt_command"`
}

// Build checks the validity of the configuration and returns a ready-to-use AES-GCM implementation.
func (c *Config) Build() (method.Method, error) {
	if len(c.EncryptCommand) < 1 {
		return nil, &method.ErrInvalidConfiguration{
			Cause: &method.ErrCryptoFailure{
				Message: "the encrypt_command option is required",
			},
		}
	}
	if len(c.EncryptCommand[0]) == 0 {
		return nil, &method.ErrInvalidConfiguration{
			Cause: &method.ErrCryptoFailure{
				Message: "the first entry of encrypt_command must not be empty",
			},
		}
	}
	if len(c.DecryptCommand) < 1 {
		return nil, &method.ErrInvalidConfiguration{
			Cause: &method.ErrCryptoFailure{
				Message: "the decrypt_command option is required",
			},
		}
	}
	if len(c.DecryptCommand[0]) == 0 {
		return nil, &method.ErrInvalidConfiguration{
			Cause: &method.ErrCryptoFailure{
				Message: "the first entry of decrypt_command must not be empty",
			},
		}
	}
	return &command{
		keys:           c.Keys,
		encryptCommand: c.EncryptCommand,
		decryptCommand: c.DecryptCommand,
	}, nil
}
