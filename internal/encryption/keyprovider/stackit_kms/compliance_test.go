// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package stackit_kms

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider/compliancetest"
	"github.com/stackitcloud/stackit-sdk-go/services/kms/v1api"
)

const (
	testProjectID = "11111111-1111-1111-1111-111111111111"
	testRegion    = "eu01"
	testKeyRingID = "22222222-2222-2222-2222-222222222222"
	testKeyID     = "33333333-3333-3333-3333-333333333333"
)

// newTestMock's round trip only succeeds if Decrypt gets Encrypt's exact version.
func newTestMock() *mockKMC {
	return &mockKMC{
		encrypt: func(version int64, plaintext []byte) ([]byte, error) {
			prefix := []byte(fmt.Sprintf("v%d:", version))
			return append(prefix, plaintext...), nil
		},
		decrypt: func(version int64, ciphertext []byte) ([]byte, error) {
			prefix := []byte(fmt.Sprintf("v%d:", version))
			if !bytes.HasPrefix(ciphertext, prefix) {
				return nil, fmt.Errorf("ciphertext was not wrapped with key version %d", version)
			}
			return ciphertext[len(prefix):], nil
		},
		listVersions: func() ([]v1api.Version, error) {
			return []v1api.Version{
				{Number: 1, State: v1api.VERSIONSTATE_ACTIVE},
				{Number: 2, State: v1api.VERSIONSTATE_ACTIVE, Disabled: true},
				{Number: 3, State: v1api.VERSIONSTATE_DESTROYED},
			}, nil
		},
	}
}

func TestKeyProvider(t *testing.T) {
	injectMock(newTestMock())

	compliancetest.ComplianceTest(
		t,
		compliancetest.TestConfiguration[*descriptor, *Config, *keyMeta, *keyProvider]{
			Descriptor: New().(*descriptor),
			HCLParseTestCases: map[string]compliancetest.HCLParseTestCase[*Config, *keyProvider]{
				"success": {
					HCL: fmt.Sprintf(`key_provider "stackit_kms" "foo" {
							project_id  = "%s"
							region      = "%s"
							key_ring_id = "%s"
							key_id      = "%s"
							key_version = 1
							key_length  = 32
						}`, testProjectID, testRegion, testKeyRingID, testKeyID),
					ValidHCL:   true,
					ValidBuild: true,
					Validate: func(config *Config, keyProvider *keyProvider) error {
						if config.ProjectID != testProjectID {
							return fmt.Errorf("incorrect project ID returned")
						}
						if keyProvider.version != 1 {
							return fmt.Errorf("incorrect key version resolved: %d", keyProvider.version)
						}
						return nil
					},
				},
				"success-auto-version": {
					HCL: fmt.Sprintf(`key_provider "stackit_kms" "foo" {
							project_id  = "%s"
							region      = "%s"
							key_ring_id = "%s"
							key_id      = "%s"
							key_length  = 32
						}`, testProjectID, testRegion, testKeyRingID, testKeyID),
					ValidHCL:   true,
					ValidBuild: true,
					Validate: func(config *Config, keyProvider *keyProvider) error {
						if keyProvider.version != 1 {
							return fmt.Errorf("expected latest active version 1 to be resolved, got %d", keyProvider.version)
						}
						return nil
					},
				},
				"empty": {
					HCL:        `key_provider "stackit_kms" "foo" {}`,
					ValidHCL:   false,
					ValidBuild: false,
				},
				"empty-project-id": {
					HCL: fmt.Sprintf(`key_provider "stackit_kms" "foo" {
							project_id  = ""
							region      = "%s"
							key_ring_id = "%s"
							key_id      = "%s"
							key_length  = 32
						}`, testRegion, testKeyRingID, testKeyID),
					ValidHCL:   true,
					ValidBuild: false,
				},
				"empty-region": {
					HCL: fmt.Sprintf(`key_provider "stackit_kms" "foo" {
							project_id  = "%s"
							region      = ""
							key_ring_id = "%s"
							key_id      = "%s"
							key_length  = 32
						}`, testProjectID, testKeyRingID, testKeyID),
					ValidHCL:   true,
					ValidBuild: false,
				},
				"empty-key-ring-id": {
					HCL: fmt.Sprintf(`key_provider "stackit_kms" "foo" {
							project_id  = "%s"
							region      = "%s"
							key_ring_id = ""
							key_id      = "%s"
							key_length  = 32
						}`, testProjectID, testRegion, testKeyID),
					ValidHCL:   true,
					ValidBuild: false,
				},
				"empty-key-id": {
					HCL: fmt.Sprintf(`key_provider "stackit_kms" "foo" {
							project_id  = "%s"
							region      = "%s"
							key_ring_id = "%s"
							key_id      = ""
							key_length  = 32
						}`, testProjectID, testRegion, testKeyRingID),
					ValidHCL:   true,
					ValidBuild: false,
				},
				"invalid-key-size": {
					HCL: fmt.Sprintf(`key_provider "stackit_kms" "foo" {
							project_id  = "%s"
							region      = "%s"
							key_ring_id = "%s"
							key_id      = "%s"
							key_length  = -1
						}`, testProjectID, testRegion, testKeyRingID, testKeyID),
					ValidHCL:   true,
					ValidBuild: false,
				},
				"large-key-size": {
					HCL: fmt.Sprintf(`key_provider "stackit_kms" "foo" {
							project_id  = "%s"
							region      = "%s"
							key_ring_id = "%s"
							key_id      = "%s"
							key_length  = 99999999
						}`, testProjectID, testRegion, testKeyRingID, testKeyID),
					ValidHCL:   true,
					ValidBuild: false,
				},
				"unknown-property": {
					HCL: fmt.Sprintf(`key_provider "stackit_kms" "foo" {
							project_id        = "%s"
							region            = "%s"
							key_ring_id       = "%s"
							key_id            = "%s"
							key_length        = 32
							unknown_property  = "foo"
						}`, testProjectID, testRegion, testKeyRingID, testKeyID),
					ValidHCL:   false,
					ValidBuild: false,
				},
			},
			JSONParseTestCases: map[string]compliancetest.JSONParseTestCase[*Config, *keyProvider]{
				"success": {
					JSON: fmt.Sprintf(`{
	"key_provider": {
		"stackit_kms": {
			"foo": {
				"project_id": "%s",
				"region": "%s",
				"key_ring_id": "%s",
				"key_id": "%s",
				"key_version": 1,
				"key_length": 32
			}
		}
	}
}`, testProjectID, testRegion, testKeyRingID, testKeyID),
					ValidJSON:  true,
					ValidBuild: true,
					Validate: func(config *Config, keyProvider *keyProvider) error {
						if config.ProjectID != testProjectID {
							return fmt.Errorf("incorrect project ID returned")
						}
						return nil
					},
				},
				"empty": {
					JSON: `{
	"key_provider": {
		"stackit_kms": {
			"foo": {
			}
		}
	}
}`,
					ValidJSON:  false,
					ValidBuild: false,
				},
				"invalid-key-size": {
					JSON: fmt.Sprintf(`{
	"key_provider": {
		"stackit_kms": {
			"foo": {
				"project_id": "%s",
				"region": "%s",
				"key_ring_id": "%s",
				"key_id": "%s",
				"key_length": -1
			}
		}
	}
}`, testProjectID, testRegion, testKeyRingID, testKeyID),
					ValidJSON:  true,
					ValidBuild: false,
				},
				"empty-project-id": {
					JSON: fmt.Sprintf(`{
	"key_provider": {
		"stackit_kms": {
			"foo": {
				"project_id": "",
				"region": "%s",
				"key_ring_id": "%s",
				"key_id": "%s",
				"key_length": 32
			}
		}
	}
}`, testRegion, testKeyRingID, testKeyID),
					ValidJSON:  true,
					ValidBuild: false,
				},
				"unknown-property": {
					JSON: fmt.Sprintf(`{
	"key_provider": {
		"stackit_kms": {
			"foo": {
				"project_id": "%s",
				"region": "%s",
				"key_ring_id": "%s",
				"key_id": "%s",
				"key_length": 32,
				"unknown_property": "foo"
			}
		}
	}
}`, testProjectID, testRegion, testKeyRingID, testKeyID),
					ValidJSON:  false,
					ValidBuild: false,
				},
			},
			ConfigStructTestCases: map[string]compliancetest.ConfigStructTestCase[*Config, *keyProvider]{
				"success": {
					Config: &Config{
						ProjectID:  testProjectID,
						Region:     testRegion,
						KeyRingID:  testKeyRingID,
						KeyID:      testKeyID,
						KeyVersion: 1,
						KeyLength:  32,
					},
					ValidBuild: true,
					Validate:   nil,
				},
				"empty": {
					Config:     &Config{},
					ValidBuild: false,
					Validate:   nil,
				},
			},
			MetadataStructTestCases: map[string]compliancetest.MetadataStructTestCase[*Config, *keyMeta]{
				"empty": {
					ValidConfig: &Config{
						ProjectID:  testProjectID,
						Region:     testRegion,
						KeyRingID:  testKeyRingID,
						KeyID:      testKeyID,
						KeyVersion: 1,
						KeyLength:  32,
					},
					Meta:      &keyMeta{},
					IsPresent: false,
					IsValid:   false,
				},
			},
			ProvideTestCase: compliancetest.ProvideTestCase[*Config, *keyMeta]{
				ValidConfig: &Config{
					ProjectID:  testProjectID,
					Region:     testRegion,
					KeyRingID:  testKeyRingID,
					KeyID:      testKeyID,
					KeyVersion: 1,
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
					if meta.Version != 1 {
						return fmt.Errorf("incorrect key version stored in metadata: %d", meta.Version)
					}
					return nil
				},
			},
		})
}

// Regression test: decrypt must use inMeta's version, not the current config's.
func TestDecryptUsesStoredVersionNotConfiguredVersion(t *testing.T) {
	injectMock(newTestMock())

	oldProvider, oldMeta, err := (&Config{
		ProjectID:  testProjectID,
		Region:     testRegion,
		KeyRingID:  testKeyRingID,
		KeyID:      testKeyID,
		KeyVersion: 1,
		KeyLength:  32,
	}).Build()
	if err != nil {
		t.Fatal(err)
	}
	_, oldMeta, err = oldProvider.Provide(oldMeta)
	if err != nil {
		t.Fatal(err)
	}

	newProvider, _, err := (&Config{
		ProjectID:  testProjectID,
		Region:     testRegion,
		KeyRingID:  testKeyRingID,
		KeyID:      testKeyID,
		KeyVersion: 2,
		KeyLength:  32,
	}).Build()
	if err != nil {
		t.Fatal(err)
	}

	output, _, err := newProvider.Provide(oldMeta)
	if err != nil {
		t.Fatalf("decrypting state wrapped with a since-rotated key version failed: %v", err)
	}
	if len(output.DecryptionKey) == 0 {
		t.Fatal("expected a decryption key")
	}
}

func TestNoActiveVersionFound(t *testing.T) {
	injectMock(&mockKMC{
		listVersions: func() ([]v1api.Version, error) {
			return []v1api.Version{
				{Number: 1, State: v1api.VERSIONSTATE_DESTROYED},
				{Number: 2, State: v1api.VERSIONSTATE_ACTIVE, Disabled: true},
			}, nil
		},
	})

	_, _, err := (&Config{
		ProjectID: testProjectID,
		Region:    testRegion,
		KeyRingID: testKeyRingID,
		KeyID:     testKeyID,
		KeyLength: 32,
	}).Build()
	if err == nil {
		t.Fatal("expected an error when no active key version is available, got nil")
	}
}

// TestKeyProviderLive round-trips against a real key; see README.md to enable it.
func TestKeyProviderLive(t *testing.T) {
	if os.Getenv("TF_ACC") == "" && os.Getenv("TF_KMS_TEST") == "" {
		t.Skip("set TF_ACC=1 or TF_KMS_TEST=1 to run this test against real STACKIT KMS")
	}
	projectID := os.Getenv("TF_STACKIT_KMS_PROJECT_ID")
	if projectID == "" {
		t.Skip("set TF_STACKIT_KMS_PROJECT_ID, TF_STACKIT_KMS_REGION, TF_STACKIT_KMS_KEY_RING_ID and TF_STACKIT_KMS_KEY_ID to run this test against real STACKIT KMS")
	}

	provider, meta, err := (&Config{
		ProjectID: projectID,
		Region:    os.Getenv("TF_STACKIT_KMS_REGION"),
		KeyRingID: os.Getenv("TF_STACKIT_KMS_KEY_RING_ID"),
		KeyID:     os.Getenv("TF_STACKIT_KMS_KEY_ID"),
		KeyLength: 32,
	}).Build()
	if err != nil {
		t.Fatalf("Build() against real STACKIT KMS failed: %v", err)
	}

	output, meta, err := provider.Provide(meta)
	if err != nil {
		t.Fatalf("first Provide() (encrypt) failed: %v", err)
	}
	if len(output.EncryptionKey) != 32 {
		t.Fatalf("expected a 32-byte encryption key, got %d bytes", len(output.EncryptionKey))
	}

	output2, _, err := provider.Provide(meta)
	if err != nil {
		t.Fatalf("second Provide() (decrypt) failed: %v", err)
	}
	if !bytes.Equal(output2.DecryptionKey, output.EncryptionKey) {
		t.Fatal("decrypted key does not match the originally encrypted key")
	}
}
