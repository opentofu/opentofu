// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pbkdf2

import (
	"fmt"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/compliancetest"
	"testing"
)

func TestCompliance(t *testing.T) {
	compliancetest.ComplianceTest(
		t,
		New(),
		map[string]compliancetest.TestCase{
			"passphrase": {
				`key_provider "pbkdf2" "foo" {
    passphrase = "Hello world! 123"
}`,
				true,
				func(config keyprovider.Config) error {
					typedConfig := config.(*Config)
					if parsedKey := typedConfig.Passphrase; parsedKey != "Hello world! 123" {
						return fmt.Errorf("incorrect key in parsed config: %s", parsedKey)
					}
					if typedConfig.KeyLength != DefaultKeyLength {
						return fmt.Errorf("incorrect key length in parsed config: %d", typedConfig.KeyLength)
					}
					if typedConfig.Iterations != DefaultIterations {
						return fmt.Errorf("incorrect iterations in parsed config: %d", typedConfig.Iterations)
					}
					if typedConfig.SaltLength != DefaultSaltLength {
						return fmt.Errorf("incorrect salt length in parsed config: %d", typedConfig.SaltLength)
					}
					if typedConfig.HashFunction != DefaultHashFunctionName {
						return fmt.Errorf("incorrect hash function name in parsed config: %s", typedConfig.HashFunction)
					}
					typedConfig.randomSource = &testRandomSource{t}
					return nil
				},
				false,
				true,
				nil,
				&keyprovider.Output{
					EncryptionKey: []byte{87, 192, 98, 53, 186, 42, 63, 139, 58, 118, 223, 169, 46, 84, 139, 29, 130, 59, 247, 106, 82, 61, 235, 144, 97, 131, 60, 229, 195, 109, 81, 111},
					DecryptionKey: []byte{87, 192, 98, 53, 186, 42, 63, 139, 58, 118, 223, 169, 46, 84, 139, 29, 130, 59, 247, 106, 82, 61, 235, 144, 97, 131, 60, 229, 195, 109, 81, 111},
				},
				func() any {
					return &Metadata{
						Iterations:   1,
						Salt:         []byte("Hello world!"),
						HashFunction: SHA256HashFunctionName,
						KeyLength:    -1,
					}
				},
				func(output keyprovider.Output, meta any) error {
					typedMeta := meta.(*Metadata)
					if !typedMeta.isPresent() {
						return fmt.Errorf("output metadata is not present")
					}
					if err := typedMeta.validate(); err != nil {
						return err
					}
					if typedMeta.KeyLength != DefaultKeyLength {
						return fmt.Errorf("incorrect output metadata key length: %d", typedMeta.KeyLength)
					}
					if typedMeta.Iterations != DefaultIterations {
						return fmt.Errorf("incorrect output metadata iterations: %d", typedMeta.Iterations)
					}
					if len(typedMeta.Salt) != DefaultSaltLength {
						return fmt.Errorf("incorrect output salt length: %d", len(typedMeta.Salt))
					}
					if typedMeta.HashFunction != DefaultHashFunctionName {
						return fmt.Errorf("incorrect output hash function name: %s", typedMeta.HashFunction)
					}
					return nil
				},
			},
		},
	)
}
