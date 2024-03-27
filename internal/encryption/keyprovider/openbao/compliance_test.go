package openbao

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	openbao "github.com/openbao/openbao/api"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/compliancetest"
)

func getBaoKeyName() string {
	// Acceptance tests are disabled, running with mock.
	if os.Getenv("TF_ACC") == "" {
		return ""
	}
	return os.Getenv("TF_BAO_KEY_NAME")
}

const defaultTestKeyName = "test-key"

func TestKeyProvider(t *testing.T) {
	testKeyName := getBaoKeyName()

	if testKeyName == "" {
		testKeyName = defaultTestKeyName

		mock := prepareClientMockForKeyProviderTest(t, testKeyName)

		injectMock(mock)

		// Token has to be present either in HCL configuration or in env vars.
		// Exposing it as an env var makes it easier to run tests without mocks.
		t.Setenv("BAO_TOKEN", "s.dummytoken")
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
				"invalid-data-key-bit-size": {
					HCL: fmt.Sprintf(`key_provider "openbao" "foo" {
							key_name = "%s"
							data_key_bit_size = 257
						}`, testKeyName),
					ValidHCL:   true,
					ValidBuild: false,
				},
				"no-key-name": {
					HCL: `key_provider "openbao" "foo" {
							data_key_bit_size = 128
						}`,
					ValidHCL:   false,
					ValidBuild: false,
				},
				"unknown-property": {
					HCL: fmt.Sprintf(`key_provider "openbao" "foo" {
							key_name = "%s"
							data_key_bit_size = 128
							unknown_property = "foo"
						}`, testKeyName),
					ValidHCL:   false,
					ValidBuild: false,
				},
			},
			ConfigStructTestCases: map[string]compliancetest.ConfigStructTestCase[*Config, *keyProvider]{
				"success": {
					Config: &Config{
						KeyName:        testKeyName,
						DataKeyBitSize: 128,
					},
					ValidBuild: true,
					Validate: func(p *keyProvider) error {
						if p.keyName != testKeyName {
							return fmt.Errorf("key names don't match: %v and %v", p.keyName, testKeyName)
						}
						if p.dataKeyBitSize != 128 {
							return fmt.Errorf("invalid data key bit size: %v", p.dataKeyBitSize)
						}
						return nil
					},
				},
				"success-default-data-key-bit-size": {
					Config: &Config{
						KeyName: testKeyName,
					},
					ValidBuild: true,
					Validate: func(p *keyProvider) error {
						if p.keyName != testKeyName {
							return fmt.Errorf("key names don't match: %v and %v", p.keyName, testKeyName)
						}
						if p.dataKeyBitSize != 256 {
							return fmt.Errorf("invalid default data key bit size: %v", p.dataKeyBitSize)
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
// but in order to cover as much as we can, it has to have some logic in here.

func prepareClientMockForKeyProviderTest(t *testing.T, testKeyName string) mockClientFunc {
	generateDataKeyPath := fmt.Sprintf("/transit/datakey/plaintext/%s", testKeyName)
	decryptPath := fmt.Sprintf("/transit/decrypt/%s", testKeyName)

	return func(ctx context.Context, path string, data []byte) (*openbao.Secret, error) {
		reqBody := make(map[string]interface{})

		if err := json.Unmarshal(data, &reqBody); err != nil {
			t.Fatalf("Invalid JSON data supplied to mock: %v", err)
		}

		switch path {
		case generateDataKeyPath:
			bits, ok := reqBody["bits"].(float64)
			if !ok {
				t.Fatalf("Invalid bits in data suplied to mock: not an int")
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
			ciphertext, ok := reqBody["ciphertext"].(string)
			if !ok {
				t.Fatalf("Invalid ciphertext in data suuplied to mock: not an string")
			}

			plaintext := []byte(ciphertext[len(testKeyName):])

			s := &openbao.Secret{
				Data: map[string]interface{}{
					"plaintext": base64.StdEncoding.EncodeToString(plaintext),
				},
			}

			return s, nil

		default:
			t.Fatalf("Invalid path suuplied to mock: %s", path)
		}

		// unreachable code
		return nil, nil
	}
}
