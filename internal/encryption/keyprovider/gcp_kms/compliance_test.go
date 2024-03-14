package gcp_kms

import (
	"fmt"
	"os"
	"testing"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider/compliancetest"
)

// skipCheck checks if the test should be skipped or not based on environment variables
func skipCheckGetKey(t *testing.T) string {
	// check if TF_ACC and TF_KMS_TEST are unset
	// if so, skip the test
	if os.Getenv("TF_ACC") == "" && os.Getenv("TF_KMS_TEST") == "" {
		t.Log("Skipping test because TF_ACC or TF_KMS_TEST is not set")
		t.Skip()
	}
	key := os.Getenv("TF_GCP_KMS_KEY")
	if key == "" {
		t.Log("Skipping test because TF_AWS_GCP_KEY is not set")
		t.Skip()
	}
	return key
}

func TestKeyProvider(t *testing.T) {
	testKeyId := skipCheckGetKey(t)

	compliancetest.ComplianceTest(
		t,
		compliancetest.TestConfiguration[*descriptor, *Config, *keyMeta, *keyProvider]{
			Descriptor: New().(*descriptor),
			HCLParseTestCases: map[string]compliancetest.HCLParseTestCase[*Config, *keyProvider]{
				"success": {
					HCL: fmt.Sprintf(`key_provider "gcp_kms" "foo" {
							kms_encryption_key = "%s"
							key_size = 32
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
							key_size = -1
							}`, testKeyId),
					ValidHCL:   true,
					ValidBuild: false,
				},
				"empty-key-id": {
					HCL: `key_provider "gcp_kms" "foo" {
							kms_encryption_key = ""
							key_size = 32
							}`,
					ValidHCL:   true,
					ValidBuild: false,
				},
				"large-key-size": {
					HCL: `key_provider "gcp_kms" "foo" {
							kms_encryption_key = "alias/temp"
							key_size = 999999999999
							}`,
					ValidHCL:   true,
					ValidBuild: false,
				},
				"unknown-property": {
					HCL: fmt.Sprintf(`key_provider "gcp_kms" "foo" {
							kms_encryption_key = "%s"	
							key_size = 32	
							unknown_property = "foo"
				}`, testKeyId),
					ValidHCL:   false,
					ValidBuild: false,
				},
				"with-access-token": {
					HCL: `key_provider "gcp_kms" "foo" {
							kms_encryption_key = "alias/temp"
							key_size = 32
							access_token = "my-access-token"
							}`,
					ValidHCL:   true,
					ValidBuild: true,
				},
				"bad-credentials": {
					HCL: `key_provider "gcp_kms" "foo" {
							kms_encryption_key = "alias/temp"
							key_size = 32
							credentials = "AS{DU*@#8UQDD*a"
							}`,
					ValidHCL:   true,
					ValidBuild: false,
				},
				"impersonation": {
					HCL: `key_provider "gcp_kms" "foo" {
							kms_encryption_key = "alias/temp"
							key_size = 32
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
						KeySize:    32,
					},
					ValidBuild: true,
					Validate:   nil,
				},
				"empty": {
					Config: &Config{
						KMSKeyName: "",
						KeySize:    0,
					},
					ValidBuild: false,
					Validate:   nil,
				},
			},
			MetadataStructTestCases: map[string]compliancetest.MetadataStructTestCase[*Config, *keyMeta]{
				"empty": {
					ValidConfig: &Config{
						KMSKeyName: testKeyId,
						KeySize:    32,
					},
					Meta:      &keyMeta{},
					IsPresent: false,
					IsValid:   false,
				},
			},
			ProvideTestCase: compliancetest.ProvideTestCase[*Config, *keyMeta]{
				ValidConfig: &Config{
					KMSKeyName: testKeyId,
					KeySize:    32,
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
