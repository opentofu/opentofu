// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package gcp_kms

import (
	"fmt"
	"os"
	"testing"

	"cloud.google.com/go/kms/apiv1/kmspb"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/compliancetest"
)

func getKey(t *testing.T) string {
	if os.Getenv("TF_ACC") == "" && os.Getenv("TF_KMS_TEST") == "" {
		return ""
	}
	return os.Getenv("TF_GCP_KMS_KEY")
}

func TestKeyProvider(t *testing.T) {
	testKeyId := getKey(t)

	if testKeyId == "" {
		testKeyId = "projects/local-vehicle-id/locations/global/keyRings/ringid/cryptoKeys/keyid"
		mock := &mockKMC{
			encrypt: func(req *kmspb.EncryptRequest) (*kmspb.EncryptResponse, error) {
				return &kmspb.EncryptResponse{
					Ciphertext: append([]byte(testKeyId), req.Plaintext...),
				}, nil
			},
			decrypt: func(req *kmspb.DecryptRequest) (*kmspb.DecryptResponse, error) {
				return &kmspb.DecryptResponse{
					Plaintext: req.Ciphertext[len(testKeyId):],
				}, nil
			},
		}

		injectMock(mock)

		// Used by impersonation tests
		t.Setenv("GOOGLE_CREDENTIALS", `{"type": "service_account"}`)

	}

	compliancetest.ComplianceTest(
		t,
		compliancetest.TestConfiguration[*descriptor, *Config, *keyMeta, *keyProvider]{
			Descriptor: New().(*descriptor),
			HCLParseTestCases: map[string]compliancetest.HCLParseTestCase[*Config, *keyProvider]{
				"success": {
					HCL: fmt.Sprintf(`key_provider "gcp_kms" "foo" {
							kms_encryption_key = "%s"
							key_length = 32
						}`, testKeyId),
					ValidHCL:   true,
					ValidBuild: true,
					Validate: func(config *Config, keyProvider *keyProvider) error {
						if config.KMSKeyName != testKeyId {
							return fmt.Errorf("incorrect key ID returned")
						}
						return nil
					},
				},
				"empty": {
					HCL:        `key_provider "gcp_kms" "foo" {}`,
					ValidHCL:   false,
					ValidBuild: false,
				},
				"invalid-key-size": {
					HCL: fmt.Sprintf(`key_provider "gcp_kms" "foo" {
							kms_encryption_key = "%s"
							key_length = -1
							}`, testKeyId),
					ValidHCL:   true,
					ValidBuild: false,
				},
				"empty-key-id": {
					HCL: `key_provider "gcp_kms" "foo" {
							kms_encryption_key = ""
							key_length = 32
							}`,
					ValidHCL:   true,
					ValidBuild: false,
				},
				"large-key-size": {
					HCL: `key_provider "gcp_kms" "foo" {
							kms_encryption_key = "alias/temp"
							key_length = 99999999
							}`,
					ValidHCL:   true,
					ValidBuild: false,
				},
				"unknown-property": {
					HCL: fmt.Sprintf(`key_provider "gcp_kms" "foo" {
							kms_encryption_key = "%s"	
							key_length = 32	
							unknown_property = "foo"
				}`, testKeyId),
					ValidHCL:   false,
					ValidBuild: false,
				},
				"with-access-token": {
					HCL: `key_provider "gcp_kms" "foo" {
							kms_encryption_key = "alias/temp"
							key_length = 32
							access_token = "my-access-token"
							}`,
					ValidHCL:   true,
					ValidBuild: true,
				},
				"bad-credentials": {
					HCL: `key_provider "gcp_kms" "foo" {
							kms_encryption_key = "alias/temp"
							key_length = 32
							credentials = "AS{DU*@#8UQDD*a"
							}`,
					ValidHCL:   true,
					ValidBuild: false,
				},
				"impersonation": {
					HCL: `key_provider "gcp_kms" "foo" {
							kms_encryption_key = "alias/temp"
							key_length = 32
							impersonate_service_account = "batman"
							}`,
					ValidHCL:   true,
					ValidBuild: true,
				},
			},
			ConfigStructTestCases: map[string]compliancetest.ConfigStructTestCase[*Config, *keyProvider]{
				"success": {
					Config: &Config{
						KMSKeyName: testKeyId,
						KeyLength:  32,
					},
					ValidBuild: true,
					Validate:   nil,
				},
				"empty": {
					Config: &Config{
						KMSKeyName: "",
						KeyLength:  0,
					},
					ValidBuild: false,
					Validate:   nil,
				},
			},
			MetadataStructTestCases: map[string]compliancetest.MetadataStructTestCase[*Config, *keyMeta]{
				"empty": {
					ValidConfig: &Config{
						KMSKeyName: testKeyId,
						KeyLength:  32,
					},
					Meta:      &keyMeta{},
					IsPresent: false,
					IsValid:   false,
				},
			},
			ProvideTestCase: compliancetest.ProvideTestCase[*Config, *keyMeta]{
				ValidConfig: &Config{
					KMSKeyName: testKeyId,
					KeyLength:  32,
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
					if len(meta.Ciphertext) == 0 {
						return fmt.Errorf("ciphertext is empty")
					}
					return nil
				},
			},
		})
}
