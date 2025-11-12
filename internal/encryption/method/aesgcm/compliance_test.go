// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package aesgcm

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/method/compliancetest"
)

func TestCompliance(t *testing.T) {
	compliancetest.ComplianceTest(t, compliancetest.TestConfiguration[*descriptor, *Config, *aesgcm]{
		Descriptor: New().(*descriptor),
		HCLParseTestCases: map[string]compliancetest.HCLParseTestCase[*descriptor, *Config, *aesgcm]{
			"empty": {
				HCL:        `method "aes_gcm" "foo" {}`,
				ValidHCL:   false,
				ValidBuild: false,
				Validate:   nil,
			},
			"empty_keys": {
				HCL: `method "aes_gcm" "foo" {
						keys = {
							encryption_key = []
							decryption_key = []
						}
					}`,
				ValidHCL:   true,
				ValidBuild: false,
				Validate:   nil,
			},
			"short-keys": {
				HCL: `method "aes_gcm" "foo" {
						keys = {
							encryption_key = [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15]
							decryption_key = [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15]
						}
					}`,
				ValidHCL:   true,
				ValidBuild: false,
				Validate:   nil,
			},
			"short-decryption-key": {
				HCL: `method "aes_gcm" "foo" {
						keys = {
							encryption_key = [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16]
							decryption_key = [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15]
						}
					}`,
				ValidHCL:   true,
				ValidBuild: false,
				Validate:   nil,
			},
			"short-encryption-key": {
				HCL: `method "aes_gcm" "foo" {
						keys = {
							encryption_key = [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15]
							decryption_key = [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16]
						}
					}`,
				ValidHCL:   true,
				ValidBuild: false,
				Validate:   nil,
			},
			"only-decryption-key": {
				HCL: `method "aes_gcm" "foo" {
						keys = {
							encryption_key = []
							decryption_key = [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16]
						}
					}`,
				ValidHCL:   true,
				ValidBuild: false,
			},
			"only-encryption-key": {
				HCL: `method "aes_gcm" "foo" {
						keys = {
							encryption_key = [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16]
							decryption_key = []
						}
					}`,
				ValidHCL:   true,
				ValidBuild: true,
				Validate: func(config *Config, method *aesgcm) error {
					if len(config.Keys.DecryptionKey) > 0 {
						return fmt.Errorf("decryption key found in config despite no decryption key being provided")
					}
					if len(method.decryptionKey) > 0 {
						return fmt.Errorf("decryption key found in method despite no decryption key being provided")
					}
					if !bytes.Equal(config.Keys.EncryptionKey, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}) {
						return fmt.Errorf("incorrect encryption key found after HCL parsing in config")
					}
					if !bytes.Equal(method.encryptionKey, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}) {
						return fmt.Errorf("incorrect encryption key found after HCL parsing in config")
					}
					return nil
				},
			},
			"encryption-decryption-key": {
				HCL: `method "aes_gcm" "foo" {
						keys = {
							encryption_key = [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16]
							decryption_key = [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16]
						}
					}`,
				ValidHCL:   true,
				ValidBuild: true,
				Validate: func(config *Config, method *aesgcm) error {
					if !bytes.Equal(config.Keys.DecryptionKey, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}) {
						return fmt.Errorf("incorrect decryption key found after HCL parsing in config")
					}
					if !bytes.Equal(method.decryptionKey, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}) {
						return fmt.Errorf("incorrect decryption key found after HCL parsing in config")
					}

					if !bytes.Equal(config.Keys.EncryptionKey, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}) {
						return fmt.Errorf("incorrect encryption key found after HCL parsing in config")
					}
					if !bytes.Equal(method.encryptionKey, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}) {
						return fmt.Errorf("incorrect encryption key found after HCL parsing in config")
					}
					return nil
				},
			},
			"no-aad": {
				HCL: `method "aes_gcm" "foo" {
						keys = {
							encryption_key = [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16]
							decryption_key = [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16]
						}
					}`,
				ValidHCL:   true,
				ValidBuild: true,
				Validate: func(config *Config, method *aesgcm) error {
					if len(config.AAD) != 0 {
						return fmt.Errorf("invalid AAD in config after HCL parsing")
					}
					if len(method.aad) != 0 {
						return fmt.Errorf("invalid AAD in method after Build()")
					}
					return nil
				},
			},
			"aad": {
				HCL: `method "aes_gcm" "foo" {
						keys = {
							encryption_key = [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16]
							decryption_key = [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16]
						}
						aad = [1,2,3,4]
					}`,
				ValidHCL:   true,
				ValidBuild: true,
				Validate: func(config *Config, method *aesgcm) error {
					if !bytes.Equal(config.AAD, []byte{1, 2, 3, 4}) {
						return fmt.Errorf("invalid AAD in config after HCL parsing")
					}
					if !bytes.Equal(method.aad, []byte{1, 2, 3, 4}) {
						return fmt.Errorf("invalid AAD in method after Build()")
					}
					return nil
				},
			},
			"encryption-key-len-fail-on-build": {
				HCL: `method "aes_gcm" "foo" {
						keys = {
							encryption_key = []
							decryption_key = [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16]
						}
					}`,
				ValidHCL:   true,
				ValidBuild: false,
				Validate:   nil,
			},
		},
		ConfigStructTestCases: map[string]compliancetest.ConfigStructTestCase[*Config, *aesgcm]{
			"empty": {
				Config: &Config{
					Keys: keyprovider.Output{},
					AAD:  nil,
				},
				ValidBuild: false,
				Validate:   nil,
			},
		},
		EncryptDecryptTestCase: compliancetest.EncryptDecryptTestCase[*Config, *aesgcm]{
			ValidEncryptOnlyConfig: &Config{
				Keys: keyprovider.Output{
					EncryptionKey: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
					DecryptionKey: nil,
				},
			},
			ValidFullConfig: &Config{
				Keys: keyprovider.Output{
					EncryptionKey: []byte{17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
					DecryptionKey: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
				},
			},
		},
	})
}
