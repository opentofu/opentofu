// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package openbao

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"testing"

	openbao "github.com/openbao/openbao/api/v2"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/compliancetest"
)

func getBaoKeyName() string {
	// Acceptance tests are disabled, running with mock.
	if os.Getenv("TF_ACC") == "" {
		return ""
	}
	return os.Getenv("TF_ACC_BAO_KEY_NAME")
}

const defaultTestKeyName = "test-key"

func TestKeyProvider(t *testing.T) {
	testKeyName := getBaoKeyName()

	if testKeyName == "" {
		testKeyName = defaultTestKeyName

		mock := prepareClientMockForKeyProviderTest(t, testKeyName)

		injectMock(mock)

		t.Cleanup(func() {
			injectDefaultClient()
		})
	}

	compliancetest.ComplianceTest(
		t,
		compliancetest.TestConfiguration[*descriptor, *Config, *keyMeta, *keyProvider]{
			Descriptor: New().(*descriptor),
			HCLParseTestCases: map[string]compliancetest.HCLParseTestCase[*Config, *keyProvider]{
				"success": {
					HCL: fmt.Sprintf(`key_provider "openbao" "foo" {
							key_name = "%s"
						}`, testKeyName),
					ValidHCL:   true,
					ValidBuild: true,
				},
				"success-full-creds": {
					HCL: fmt.Sprintf(`key_provider "openbao" "foo" {
							token = "s.dummytoken"
							address = "http://127.0.0.1:8201"
							key_name = "%s"
						}`, testKeyName),
					ValidHCL:   true,
					ValidBuild: true,
				},
				"empty": {
					HCL:        `key_provider "openbao" "foo" {}`,
					ValidHCL:   false,
					ValidBuild: false,
				},
				"empty-key-name": {
					HCL: `key_provider "openbao" "foo" {
							key_name = ""
						}`,
					ValidHCL:   true,
					ValidBuild: false,
				},
				"invalid-key-length": {
					HCL: fmt.Sprintf(`key_provider "openbao" "foo" {
							key_name = "%s"
							key_length = 17
						}`, testKeyName),
					ValidHCL:   true,
					ValidBuild: false,
				},
				"no-key-name": {
					HCL: `key_provider "openbao" "foo" {
							key_length = 16
						}`,
					ValidHCL:   false,
					ValidBuild: false,
				},
				"unknown-property": {
					HCL: fmt.Sprintf(`key_provider "openbao" "foo" {
							key_name = "%s"
							key_length = 16
							unknown_property = "foo"
						}`, testKeyName),
					ValidHCL:   false,
					ValidBuild: false,
				},
				"transit-path": {
					HCL: fmt.Sprintf(`key_provider "openbao" "foo" {
							key_name = "%s"
							key_length = 16
							transit_engine_path = "foo"
						}`, testKeyName),
					ValidHCL:   true,
					ValidBuild: true,
				},
			},
			ConfigStructTestCases: map[string]compliancetest.ConfigStructTestCase[*Config, *keyProvider]{
				"success": {
					Config: &Config{
						KeyName:           testKeyName,
						KeyLength:         16,
						TransitEnginePath: "/pki",
					},
					ValidBuild: true,
					Validate: func(p *keyProvider) error {
						if p.keyName != testKeyName {
							return fmt.Errorf("key names don't match: %v and %v", p.keyName, testKeyName)
						}
						if p.keyLength != 16 {
							return fmt.Errorf("invalid key length: %v", p.keyLength)
						}
						if p.svc.transitPath != "/pki" {
							return fmt.Errorf("invalid transit path: %v", p.svc.transitPath)
						}
						return nil
					},
				},
				"success-default-values": {
					Config: &Config{
						KeyName: testKeyName,
					},
					ValidBuild: true,
					Validate: func(p *keyProvider) error {
						if p.keyName != testKeyName {
							return fmt.Errorf("key names don't match: %v and %v", p.keyName, testKeyName)
						}
						if p.keyLength != 32 {
							return fmt.Errorf("invalid default key length: %v", p.keyLength)
						}
						if p.svc.transitPath != "/transit" {
							return fmt.Errorf("invalid default transit path: %v; expected: '/transit'", p.svc.transitPath)
						}
						return nil
					},
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
						KeyName: testKeyName,
					},
					Meta:      &keyMeta{},
					IsPresent: false,
					IsValid:   false,
				},
			},
			ProvideTestCase: compliancetest.ProvideTestCase[*Config, *keyMeta]{
				ValidConfig: &Config{
					KeyName: testKeyName,
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
		},
	)
}

// Mocking is a bit complicated due to how openbao/api package is structured,
// but in order to test cover as much as we can, it has to have some logic in here.

func prepareClientMockForKeyProviderTest(t *testing.T, testKeyName string) mockClientFunc {
	escapedTestKeyName := url.PathEscape(testKeyName)

	// Mock uses default transit engine path: "/transit".
	generateDataKeyPath := fmt.Sprintf("/transit/datakey/plaintext/%s", escapedTestKeyName)
	decryptPath := fmt.Sprintf("/transit/decrypt/%s", escapedTestKeyName)

	return func(ctx context.Context, path string, data map[string]interface{}) (*openbao.Secret, error) {
		switch path {
		case generateDataKeyPath:
			bits, ok := data["bits"].(int)
			if !ok {
				t.Fatalf("Invalid bits in data supplied to mock: not a number")
			}

			plaintext := make([]byte, int(bits)/8)
			if _, err := rand.Read(plaintext); err != nil {
				panic(fmt.Errorf("generating random data key in mock: %w", err))
			}

			s := &openbao.Secret{
				Data: map[string]interface{}{
					"plaintext":  base64.StdEncoding.EncodeToString(plaintext),
					"ciphertext": string(append([]byte(testKeyName), plaintext...)),
				},
			}

			return s, nil

		case decryptPath:
			ciphertext, ok := data["ciphertext"].(string)
			if !ok {
				t.Fatalf("Invalid ciphertext in data supplied to mock: not an string")
			}

			plaintext := []byte(ciphertext[len(testKeyName):])

			s := &openbao.Secret{
				Data: map[string]interface{}{
					"plaintext": base64.StdEncoding.EncodeToString(plaintext),
				},
			}

			return s, nil

		default:
			t.Fatalf("Invalid path supplied to mock: %s", path)
		}

		// unreachable code
		return nil, nil
	}
}
