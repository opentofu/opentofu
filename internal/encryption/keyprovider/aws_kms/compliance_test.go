// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package aws_kms

import (
	"fmt"
	"testing"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider/compliancetest"
)

func TestKeyProvider(t *testing.T) {

	// TODO: stop skipping the test once we have the infrastructure set up for testing with an existing key in our AWS account
	//t.Skip()

	skipCheck(t)

	compliancetest.ComplianceTest(
		t,
		compliancetest.TestConfiguration[*descriptor, *Config, *keyMeta, *keyProvider]{
			Descriptor: New().(*descriptor),
			HCLParseTestCases: map[string]compliancetest.HCLParseTestCase[*Config, *keyProvider]{
				"success": {
					HCL: `key_provider "aws_kms" "foo" {
							kms_key_id = "alias/opentofu-test-key"
							key_spec = "AES_256"
						}`,
					ValidHCL:   true,
					ValidBuild: true,
					Validate: func(config *Config, keyProvider *keyProvider) error {
						if config.KMSKeyID != "alias/opentofu-test-key" {
							return fmt.Errorf("incorrect key ID returned")
						}
						return nil
					},
				},
				"empty": {
					HCL:        `key_provider "aws_kms" "foo" {}`,
					ValidHCL:   false,
					ValidBuild: false,
				},
				"invalid-key-spec": {
					HCL: `key_provider "aws_kms" "foo" {
							kms_key_id = "alias/opentofu-test-key"
							key_spec = "BROKEN STUFF"
							}`,
					ValidHCL:   true,
					ValidBuild: false,
				},
				"empty-key-id": {
					HCL: `key_provider "aws_kms" "foo" {
							kms_key_id = ""
							key_spec = "AES_256"
							}`,
					ValidHCL:   true,
					ValidBuild: false,
				},
				"empty-key-spec": {
					HCL: `key_provider "aws_kms" "foo" {
							kms_key_id = "alias/temp"
							key_spec = ""
							}`,
					ValidHCL:   true,
					ValidBuild: false,
				},
				"unknown-property": {
					HCL: `key_provider "aws_kms" "foo" {
							kms_key_id = "alias/opentofu-test-key"	
							key_spec = "AES_256"	
							unknown_property = "foo"
				}`,
					ValidHCL:   false,
					ValidBuild: false,
				},
			},
			ConfigStructTestCases: map[string]compliancetest.ConfigStructTestCase[*Config, *keyProvider]{
				"success": {
					Config: &Config{
						KMSKeyID: "alias/opentofu-test-key",
						KeySpec:  "AES_256",
					},
					ValidBuild: true,
					Validate:   nil,
				},
				"empty": {
					Config: &Config{
						KMSKeyID: "",
						KeySpec:  "",
					},
					ValidBuild: false,
					Validate:   nil,
				},
			},
			MetadataStructTestCases: map[string]compliancetest.MetadataStructTestCase[*Config, *keyMeta]{
				"empty": {
					ValidConfig: &Config{
						KMSKeyID: "alias/opentofu-test-key",
						KeySpec:  "AES_256",
					},
					Meta:      &keyMeta{},
					IsPresent: false,
					IsValid:   false,
				},
				// TODO: Add a test case for an existing ciphertextblob to check if we can decrypt it
			},
			ProvideTestCase: compliancetest.ProvideTestCase[*Config, *keyMeta]{
				ValidConfig: &Config{
					KMSKeyID: "alias/opentofu-test-key",
					KeySpec:  "AES_256",
				},
				ValidateKeys: func(dec []byte, enc []byte) error {
					if len(dec) == 0 {
						return fmt.Errorf("decryption key is empty")
					}
					if len(enc) == 0 {
						return fmt.Errorf("encryption key is empty")
					}
					return nil
				},
				ValidateMetadata: func(meta *keyMeta) error {
					if meta.CiphertextBlob == nil || len(meta.CiphertextBlob) == 0 {
						return fmt.Errorf("ciphertext blob is nil")
					}
					return nil
				},
			},
		})
}
