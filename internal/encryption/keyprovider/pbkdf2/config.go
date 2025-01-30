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

// HashFunction is a provider of a hash.Hash.
type HashFunction func() hash.Hash

// HashFunctionName describes a hash function to use for PBKDF2 hash generation. While you could theoretically supply
// your own from outside the package, please don't do that. Include your hash function in this package. (Thanks Go for
// the lack of visibility constraints.)
type HashFunctionName string

// Validate checks if the specified hash function name is valid.
func (h HashFunctionName) Validate() error {
	if h == "" {
		return &keyprovider.ErrInvalidConfiguration{Message: "please specify a hash function"}
	}
	if _, ok := hashFunctions[h]; !ok {
		return &keyprovider.ErrInvalidConfiguration{Message: fmt.Sprintf("invalid hash function name: %s", h)}
	}
	return nil
}

// Function returns the underlying hash function for the name.
func (h HashFunctionName) Function() HashFunction {
	return hashFunctions[h]
}

type Config struct {
	// Set by the descriptor.
	randomSource io.Reader

	// Passprase is a single passphrase to use for encryption. This is mutually exclusive with Passphrases.
	Passphrase string `hcl:"passphrase,optional"`
	// Chain are two separate passphrases supplied from a chained provider. This is mutually exclusive with
	// Passphrase.
	Chain        *keyprovider.Output `hcl:"chain,optional"`
	KeyLength    int                 `hcl:"key_length,optional"`
	Iterations   int                 `hcl:"iterations,optional"`
	HashFunction HashFunctionName    `hcl:"hash_function,optional"`
	SaltLength   int                 `hcl:"salt_length,optional"`
}

// WithPassphrase adds the passphrase and returns the same config for chaining.
func (c *Config) WithPassphrase(passphrase string) *Config {
	c.Passphrase = passphrase
	return c
}

// WithChain adds a separate encryption/decryption key chained from an upstream keyprovider.
func (c *Config) WithChain(chain *keyprovider.Output) *Config {
	c.Chain = chain
	return c
}

// WithKeyLength sets the key length and returns the same config for chaining
func (c *Config) WithKeyLength(length int) *Config {
	c.KeyLength = length
	return c
}

// WithIterations sets the iterations and returns the same config for chaining
func (c *Config) WithIterations(iterations int) *Config {
	c.Iterations = iterations
	return c
}

// WithSaltLength sets the salt length and returns the same config for chaining
func (c *Config) WithSaltLength(length int) *Config {
	c.SaltLength = length
	return c
}

// WithHashFunction sets the hash function and returns the same config for chaining
func (c *Config) WithHashFunction(hashFunction HashFunctionName) *Config {
	c.HashFunction = hashFunction
	return c
}

func (c *Config) Build() (keyprovider.KeyProvider, keyprovider.KeyMeta, error) {
	if c.randomSource == nil {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{
			Message: "missing randomness source (please don't initialize the Config struct directly, use the descriptor)",
		}
	}

	if c.Passphrase == "" && c.Chain == nil {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{
			Message: "no passphrase provided and no chained provider defined",
		}
	}
	if c.Passphrase != "" && c.Chain != nil {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{
			Message: "passphrase and chain are mutually exclusive",
		}
	}
	if c.Chain != nil {
		if c.Chain.EncryptionKey == nil {
			return nil, nil, &keyprovider.ErrInvalidConfiguration{
				Message: "no encryption key provided from upstream key provider",
			}
		}
		if len(c.Chain.EncryptionKey) < MinimumPassphraseLength {
			return nil, nil, &keyprovider.ErrInvalidConfiguration{
				Message: fmt.Sprintf("upstream key provider supplied an encryption key that is too short (minimum %d characters)", MinimumPassphraseLength),
			}
		}
		if c.Chain.DecryptionKey != nil {
			if len(c.Chain.DecryptionKey) < MinimumPassphraseLength {
				return nil, nil, &keyprovider.ErrInvalidConfiguration{
					Message: fmt.Sprintf("upstream key provider supplied an decryption key that is too short (minimum %d characters)", MinimumPassphraseLength),
				}
			}
		}
	}
	if c.Passphrase != "" && len(c.Passphrase) < MinimumPassphraseLength {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{
			Message: fmt.Sprintf("passphrase is too short (minimum %d characters)", MinimumPassphraseLength),
		}
	}

	if c.KeyLength <= 0 {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{
			Message: "the key length must be larger than zero",
		}
	}

	if c.Iterations <= 0 {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{
			Message: "the number of iterations must be larger than zero",
		}
	}
	if c.Iterations < MinimumIterations {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{
			Message: fmt.Sprintf("the number of iterations is dangerously low (<%d), refusing to generate key", MinimumIterations),
		}
	}

	if c.SaltLength <= 0 {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{
			Message: "the salt length must be larger than zero",
		}
	}

	if err := c.HashFunction.Validate(); err != nil {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{
			Cause: err,
		}
	}

	return &pbkdf2KeyProvider{*c}, new(Metadata), nil
}
