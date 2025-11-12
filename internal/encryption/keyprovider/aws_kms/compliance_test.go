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
	testKeyId := getKey(t)

	if testKeyId == "" {
		testKeyId = "alias/my-mock-key"
		injectDefaultMock()

		t.Setenv("AWS_REGION", "us-east-1")
		t.Setenv("AWS_ACCESS_KEY_ID", "accesskey")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "secretkey")
	}

	compliancetest.ComplianceTest(
		t,
		compliancetest.TestConfiguration[*descriptor, *Config, *keyMeta, *keyProvider]{
			Descriptor: New().(*descriptor),
			HCLParseTestCases: map[string]compliancetest.HCLParseTestCase[*Config, *keyProvider]{
				"success": {
					HCL: fmt.Sprintf(`key_provider "aws_kms" "foo" {
							kms_key_id = "%s"
							key_spec = "AES_256"
							skip_credentials_validation = true // required for mocking
						}`, testKeyId),
					ValidHCL:   true,
					ValidBuild: true,
					Validate: func(config *Config, keyProvider *keyProvider) error {
						if config.KMSKeyID != testKeyId {
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
					HCL: fmt.Sprintf(`key_provider "aws_kms" "foo" {
							kms_key_id = "%s"
							key_spec = "BROKEN STUFF"
							}`, testKeyId),
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
					HCL: fmt.Sprintf(`key_provider "aws_kms" "foo" {
							kms_key_id = "%s"	
							key_spec = "AES_256"	
							unknown_property = "foo"
				}`, testKeyId),
					ValidHCL:   false,
					ValidBuild: false,
				},
			},
			ConfigStructTestCases: map[string]compliancetest.ConfigStructTestCase[*Config, *keyProvider]{
				"success": {
					Config: &Config{
						KMSKeyID: testKeyId,
						KeySpec:  "AES_256",

						SkipCredsValidation: true, // Required for mocking
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
						KMSKeyID: testKeyId,
						KeySpec:  "AES_256",

						SkipCredsValidation: true, // Required for mocking
					},
					Meta:      &keyMeta{},
					IsPresent: false,
					IsValid:   false,
				},
			},
			ProvideTestCase: compliancetest.ProvideTestCase[*Config, *keyMeta]{
				ValidConfig: &Config{
					KMSKeyID:            testKeyId,
					KeySpec:             "AES_256",
					SkipCredsValidation: true, // Required for mocking
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
					if len(meta.CiphertextBlob) == 0 {
						return fmt.Errorf("ciphertext blob is empty")
					}
					return nil
				},
			},
		})
}
