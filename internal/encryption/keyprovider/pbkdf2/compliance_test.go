// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pbkdf2

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/compliancetest"
)

func TestCompliance(t *testing.T) {
	validConfig := &Config{
		randomSource: rand.Reader,
		Passphrase:   "Hello world! 123",
		KeyLength:    DefaultKeyLength,
		Iterations:   DefaultIterations,
		HashFunction: SHA256HashFunctionName,
		SaltLength:   DefaultSaltLength,
	}
	compliancetest.ComplianceTest(
		t,
		compliancetest.TestConfiguration[*descriptor, *Config, *Metadata, *pbkdf2KeyProvider]{
			Descriptor: New().(*descriptor),
			HCLParseTestCases: map[string]compliancetest.HCLParseTestCase[*Config, *pbkdf2KeyProvider]{
				"invalid": {
					HCL: `key_provider "pbkdf2" "foo" {
    chain = {
        encryption_key = "Hello world! 123"
    }
}`,
					ValidHCL: false,
				},
				"empty": {
					HCL: `key_provider "pbkdf2" "foo" {
}`,
					ValidHCL:   true,
					ValidBuild: false,
					Validate:   nil,
				},
				"basic": {
					HCL: `key_provider "pbkdf2" "foo" {
    passphrase = "Hello world! 123"
}`,
					ValidHCL:   true,
					ValidBuild: true,
					Validate: func(config *Config, keyProvider *pbkdf2KeyProvider) error {
						if config.Passphrase != "Hello world! 123" {
							return fmt.Errorf("invalid passphrase after HCL parsing")
						}
						if keyProvider.Passphrase != "Hello world! 123" {
							return fmt.Errorf("invalid passphrase in key provideer")
						}
						return nil
					},
				},
				"both-passphrase-and-chain": {
					HCL: `key_provider "pbkdf2" "foo" {
    passphrase = "Hello world! 123"
    chain = {
        encryption_key = [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16]
    }
}`,
					ValidHCL:   true,
					ValidBuild: false,
				},
				"chain": {
					HCL: `key_provider "pbkdf2" "foo" {
    chain = {
        encryption_key = [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16]
    }
}`,
					ValidHCL:   true,
					ValidBuild: true,
					Validate: func(config *Config, keyProvider *pbkdf2KeyProvider) error {
						if config.Chain == nil {
							return fmt.Errorf("no chain after parsing")
						}
						if len(config.Chain.EncryptionKey) != 16 {
							return fmt.Errorf("tncorrect encryption key length")
						}
						if !bytes.Equal(config.Chain.EncryptionKey, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}) {
							return fmt.Errorf("tncorrect encryption key")
						}
						return nil
					},
				},
				"extended": {
					HCL: fmt.Sprintf(`key_provider "pbkdf2" "foo" {
    passphrase = "Hello world! 123"
    key_length = %d
    iterations = %d
    salt_length = %d
    hash_function = "%s"
}`, DefaultKeyLength+1, DefaultIterations+1, DefaultSaltLength+1, SHA256HashFunctionName),
					ValidHCL:   true,
					ValidBuild: true,
					Validate: func(config *Config, keyProvider *pbkdf2KeyProvider) error {
						if config.KeyLength != DefaultKeyLength+1 {
							return fmt.Errorf("incorrect key length after HCL parsing: %d", config.KeyLength)
						}
						if config.Iterations != DefaultIterations+1 {
							return fmt.Errorf("incorrect iterations after HCL parsing: %d", config.Iterations)
						}
						if config.SaltLength != DefaultSaltLength+1 {
							return fmt.Errorf("incorrect salt length after HCL parsing: %d", config.SaltLength)
						}
						if config.HashFunction != SHA256HashFunctionName {
							return fmt.Errorf("incorrect hash function after HCL parsing: %s", config.HashFunction)
						}
						return nil
					},
				},
				"short-passphrase": {
					HCL: `key_provider "pbkdf2" "foo" {
    passphrase = "Hello world! 12"
}`,
					ValidHCL:   true,
					ValidBuild: false,
				},
				"too-small-iterations": {
					HCL: fmt.Sprintf(`key_provider "pbkdf2" "foo" {
    passphrase = "Hello world! 123"
    iterations = %d
}`, MinimumIterations-1),
					ValidHCL:   true,
					ValidBuild: false,
				},
				"invalid-hash-function": {
					HCL: `key_provider "pbkdf2" "foo" {
    passphrase = "Hello world! 123"
    hash_function = "non_existent"
}`,
					ValidHCL:   true,
					ValidBuild: false,
				},
			},
			ConfigStructTestCases: map[string]compliancetest.ConfigStructTestCase[*Config, *pbkdf2KeyProvider]{},
			MetadataStructTestCases: map[string]compliancetest.MetadataStructTestCase[*Config, *Metadata]{
				"not-present-salt": {
					ValidConfig: validConfig,
					Meta: &Metadata{
						Salt:         nil,
						Iterations:   DefaultIterations,
						HashFunction: SHA256HashFunctionName,
						KeyLength:    32,
					},
					IsPresent: false,
				},
				"not-present-iterations": {
					ValidConfig: validConfig,
					Meta: &Metadata{
						Salt:         []byte("Hello world!"),
						Iterations:   0,
						HashFunction: SHA256HashFunctionName,
						KeyLength:    32,
					},
					IsPresent: false,
				},
				"not-present-hash-func": {
					ValidConfig: validConfig,
					Meta: &Metadata{
						Salt:         []byte("Hello world!"),
						Iterations:   DefaultIterations,
						HashFunction: "",
						KeyLength:    32,
					},
					IsPresent: false,
				},
				"not-present-key-length": {
					ValidConfig: validConfig,
					Meta: &Metadata{
						Salt:         []byte("Hello world!"),
						Iterations:   DefaultIterations,
						HashFunction: SHA256HashFunctionName,
						KeyLength:    0,
					},
					IsPresent: false,
				},
				"present-valid": {
					ValidConfig: validConfig,
					Meta: &Metadata{
						Salt:         []byte("Hello world!"),
						Iterations:   DefaultIterations,
						HashFunction: SHA256HashFunctionName,
						KeyLength:    32,
					},
					IsPresent: true,
					IsValid:   true,
				},
				"present-valid-too-few-iterations": {
					ValidConfig: validConfig,
					Meta: &Metadata{
						Salt:         []byte("Hello world!"),
						Iterations:   MinimumIterations - 1,
						HashFunction: SHA256HashFunctionName,
						KeyLength:    32,
					},
					IsPresent: true,
					IsValid:   true,
				},
				"invalid-iterations": {
					ValidConfig: validConfig,
					Meta: &Metadata{
						Salt:         []byte("Hello world!"),
						Iterations:   -1,
						HashFunction: SHA256HashFunctionName,
						KeyLength:    32,
					},
					IsPresent: true,
					IsValid:   false,
				},
				"invalid-salt-length": {
					ValidConfig: validConfig,
					Meta: &Metadata{
						Salt:         []byte("Hello world!"),
						Iterations:   DefaultIterations,
						HashFunction: SHA256HashFunctionName,
						KeyLength:    -1,
					},
					IsPresent: true,
					IsValid:   false,
				},
			},
			ProvideTestCase: compliancetest.ProvideTestCase[*Config, *Metadata]{
				ValidConfig: &Config{
					randomSource: &testRandomSource{t: t},
					Passphrase:   "Hello world! 123",
					KeyLength:    DefaultKeyLength,
					Iterations:   DefaultIterations,
					HashFunction: DefaultHashFunctionName,
					SaltLength:   DefaultSaltLength,
				},
				ExpectedOutput: &keyprovider.Output{
					EncryptionKey: []byte{87, 192, 98, 53, 186, 42, 63, 139, 58, 118, 223, 169, 46, 84, 139, 29, 130, 59, 247, 106, 82, 61, 235, 144, 97, 131, 60, 229, 195, 109, 81, 111},
					DecryptionKey: []byte{87, 192, 98, 53, 186, 42, 63, 139, 58, 118, 223, 169, 46, 84, 139, 29, 130, 59, 247, 106, 82, 61, 235, 144, 97, 131, 60, 229, 195, 109, 81, 111},
				},
				ValidateKeys: nil,
				ValidateMetadata: func(meta *Metadata) error {
					if !meta.isPresent() {
						return fmt.Errorf("output metadata is not present")
					}
					if err := meta.validate(); err != nil {
						return err
					}
					if meta.KeyLength != DefaultKeyLength {
						return fmt.Errorf("incorrect output metadata key length: %d", meta.KeyLength)
					}
					if meta.Iterations != DefaultIterations {
						return fmt.Errorf("incorrect output metadata iterations: %d", meta.Iterations)
					}
					if len(meta.Salt) != DefaultSaltLength {
						return fmt.Errorf("incorrect output salt length: %d", len(meta.Salt))
					}
					if meta.HashFunction != DefaultHashFunctionName {
						return fmt.Errorf("incorrect output hash function name: %s", meta.HashFunction)
					}
					return nil
				},
			},
		},
	)
}
