// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package azure_vault

import (
	"fmt"
	"os"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/compliancetest"
)

func getKeyAndURI(_ *testing.T) (string, string) {
	if os.Getenv("TF_ACC") == "" || os.Getenv("TF_AZV_TEST") == "" {
		return "", ""
	}
	return os.Getenv("TF_AZ_KEY"), os.Getenv("TF_AZ_URI")
}

func TestKeyProvider(t *testing.T) {
	testKeyName, testVaultUri := getKeyAndURI(t)

	if testKeyName == "" {
		testKeyName = "test-key-name"
		testVaultUri = "https://myvaultname.vault.azure.net/"
		mock := &mockKMC{
			encrypt: func(req azkeys.KeyOperationParameters) (azkeys.EncryptResponse, error) {
				return azkeys.EncryptResponse{
					KeyOperationResult: azkeys.KeyOperationResult{
						Result: append([]byte(testKeyName), req.Value...),
					},
				}, nil
			},
			decrypt: func(req azkeys.KeyOperationParameters) (azkeys.DecryptResponse, error) {
				return azkeys.DecryptResponse{
					KeyOperationResult: azkeys.KeyOperationResult{
						Result: req.Value[len(testKeyName):],
					},
				}, nil
			},
		}

		injectMock(mock)
	}

	compliancetest.ComplianceTest(
		t,
		compliancetest.TestConfiguration[*descriptor, *Config, *keyMeta, *keyProvider]{
			Descriptor: New().(*descriptor),
			HCLParseTestCases: map[string]compliancetest.HCLParseTestCase[*Config, *keyProvider]{
				"success": {
					HCL: fmt.Sprintf(`key_provider "azure_vault" "foo" {
							vault_key_name = "%s"
							vault_uri = "%s"
							key_length = 32
						}`, testKeyName, testVaultUri),
					ValidHCL:   true,
					ValidBuild: true,
					Validate: func(config *Config, keyProvider *keyProvider) error {
						if config.VaultKeyName != testKeyName {
							return fmt.Errorf("incorrect key ID returned")
						}
						if config.Vault != testVaultUri {
							return fmt.Errorf("incorrect vault URI returned")
						}
						return nil
					},
				},
				"empty": {
					HCL:        `key_provider "azure_vault" "foo" {}`,
					ValidHCL:   false,
					ValidBuild: false,
				},
				"invalid-key-size": {
					HCL: fmt.Sprintf(`key_provider "azure_vault" "foo" {
							vault_uri = "%s"
							vault_key_name = "%s"
							key_length = -1
						}`, testKeyName, testVaultUri),
					ValidHCL:   true,
					ValidBuild: false,
				},
				"empty-key-uri": {
					HCL: `key_provider "azure_vault" "foo" {
							vault_key_name = ""
							vault_uri = ""
							key_length = 32
						}`,
					ValidHCL:   true,
					ValidBuild: false,
				},
				"large-key-size": {
					HCL: fmt.Sprintf(`key_provider "azure_vault" "foo" {
							vault_uri = "%s"
							vault_key_name = "%s"
							key_length = 99999999
						}`, testKeyName, testVaultUri),
					ValidHCL:   true,
					ValidBuild: false,
				},
				"unknown-property": {
					HCL: fmt.Sprintf(`key_provider "azure_vault" "foo" {
							vault_uri = "%s"
							vault_key_name = "%s"
							key_length = 32
							unknown_property = "foo"
						}`, testKeyName, testVaultUri),
					ValidHCL:   false,
					ValidBuild: false,
				},
			},
			ConfigStructTestCases: map[string]compliancetest.ConfigStructTestCase[*Config, *keyProvider]{
				"success": {
					Config: &Config{
						VaultKeyName: testKeyName,
						Vault:        testVaultUri,
						KeyLength:    32,
					},
					ValidBuild: true,
					Validate:   nil,
				},
				"empty": {
					Config: &Config{
						VaultKeyName: "",
						Vault:        "",
						KeyLength:    0,
					},
					ValidBuild: false,
					Validate:   nil,
				},
			},
			MetadataStructTestCases: map[string]compliancetest.MetadataStructTestCase[*Config, *keyMeta]{
				"empty": {
					ValidConfig: &Config{
						VaultKeyName: testKeyName,
						Vault:        testVaultUri,
						KeyLength:    32,
					},
					Meta:      &keyMeta{},
					IsPresent: false,
					IsValid:   false,
				},
			},
			ProvideTestCase: compliancetest.ProvideTestCase[*Config, *keyMeta]{
				ValidConfig: &Config{
					VaultKeyName: testKeyName,
					Vault:        testVaultUri,
					KeyLength:    32,
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
