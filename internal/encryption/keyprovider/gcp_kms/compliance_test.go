// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package gcp_kms

import (
	"encoding/base64"
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
				"with-aad": {
					HCL: fmt.Sprintf(`key_provider "gcp_kms" "foo" {
							kms_encryption_key = "%s"
							key_length = 32
							additional_authenticated_data = "bXktYWFkLXZhbA=="
							}`, testKeyId),
					ValidHCL:   true,
					ValidBuild: true,
				},
				"invalid-aad-base64": {
					HCL: fmt.Sprintf(`key_provider "gcp_kms" "foo" {
							kms_encryption_key = "%s"
							key_length = 32
							additional_authenticated_data = "not-valid-base64!@#"
							}`, testKeyId),
					ValidHCL:   true,
					ValidBuild: false,
				},
			},
			JSONParseTestCases: map[string]compliancetest.JSONParseTestCase[*Config, *keyProvider]{
				"success": {
					JSON: fmt.Sprintf(`{
	"key_provider": {
		"gcp_kms": {
			"foo": {
				"kms_encryption_key": "%s",
				"key_length": 32
			}
		}
	}
}`, testKeyId),
					ValidJSON:  true,
					ValidBuild: true,
					Validate: func(config *Config, keyProvider *keyProvider) error {
						if config.KMSKeyName != testKeyId {
							return fmt.Errorf("incorrect key ID returned")
						}
						return nil
					},
				},
				"empty": {
					JSON: `{
	"key_provider": {
		"gcp_kms": {
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
		"gcp_kms": {
			"foo": {
				"kms_encryption_key": "%s",
				"key_length": -1
			}
		}
	}
}`, testKeyId),
					ValidJSON:  true,
					ValidBuild: false,
				},
				"empty-key-id": {
					JSON: `{
	"key_provider": {
		"gcp_kms": {
			"foo": {
				"kms_encryption_key": "",
				"key_length": 32
			}
		}
	}
}`,
					ValidJSON:  true,
					ValidBuild: false,
				},
				"large-key-size": {
					JSON: `{
	"key_provider": {
		"gcp_kms": {
			"foo": {
				"kms_encryption_key": "alias/temp",
				"key_length": 99999999
			}
		}
	}
}`,
					ValidJSON:  true,
					ValidBuild: false,
				},
				"unknown-property": {
					JSON: fmt.Sprintf(`{
	"key_provider": {
		"gcp_kms": {
			"foo": {
				"kms_encryption_key": "%s",
				"key_length": 32,
				"unknown_property": "foo"
			}
		}
	}
}`, testKeyId),
					ValidJSON:  false,
					ValidBuild: false,
				},
				"with-access-token": {
					JSON: `{
	"key_provider": {
		"gcp_kms": {
			"foo": {
				"kms_encryption_key": "alias/temp",
				"key_length": 32,
				"access_token": "my-access-token"
			}
		}
	}
}`,
					ValidJSON:  true,
					ValidBuild: true,
				},
				"bad-credentials": {
					JSON: `{
	"key_provider": {
		"gcp_kms": {
			"foo": {
				"kms_encryption_key": "alias/temp",
				"key_length": 32,
				"credentials": "AS{DU*@#8UQDD*a"
			}
		}
	}
}`,
					ValidJSON:  true,
					ValidBuild: false,
				},
				"impersonation": {
					JSON: `{
	"key_provider": {
		"gcp_kms": {
			"foo": {
				"kms_encryption_key": "alias/temp",
				"key_length": 32,
				"impersonate_service_account": "batman"
			}
		}
	}
}`,
					ValidJSON:  true,
					ValidBuild: true,
				},
				"with-aad": {
					JSON: fmt.Sprintf(`{
	"key_provider": {
		"gcp_kms": {
			"foo": {
				"kms_encryption_key": "%s",
				"key_length": 32,
				"additional_authenticated_data": "bXktYWFkLXZhbA=="
			}
		}
	}
}`, testKeyId),
					ValidJSON:  true,
					ValidBuild: true,
				},
				"invalid-aad-base64": {
					JSON: fmt.Sprintf(`{
	"key_provider": {
		"gcp_kms": {
			"foo": {
				"kms_encryption_key": "%s",
				"key_length": 32,
				"additional_authenticated_data": "not-valid-base64!@#"
			}
		}
	}
}`, testKeyId),
					ValidJSON:  true,
					ValidBuild: false,
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
				"with-aad": {
					Config: &Config{
						KMSKeyName:                  testKeyId,
						KeyLength:                   32,
						AdditionalAuthenticatedData: "bXktYWFkLXZhbA==",
					},
					ValidBuild: true,
				},
				"invalid-aad-base64": {
					Config: &Config{
						KMSKeyName:                  testKeyId,
						KeyLength:                   32,
						AdditionalAuthenticatedData: "not-valid-base64!@#",
					},
					ValidBuild: false,
				},
				"aad-too-large": {
					Config: &Config{
						KMSKeyName:                  testKeyId,
						KeyLength:                   32,
						AdditionalAuthenticatedData: base64.StdEncoding.EncodeToString(make([]byte, 65*1024)),
					},
					ValidBuild: false,
				},
				"unpadded-aad-base64": {
					Config: &Config{
						KMSKeyName:                  testKeyId,
						KeyLength:                   32,
						AdditionalAuthenticatedData: "bXktYWFkLXZhbA",
					},
					ValidBuild: false,
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

func TestAADForwardedToRPC(t *testing.T) {
	testKeyId := "projects/local-vehicle-id/locations/global/keyRings/ringid/cryptoKeys/keyid"
	expectedAAD := []byte("my-aad-val")

	injectMock(&mockKMC{
		encrypt: func(req *kmspb.EncryptRequest) (*kmspb.EncryptResponse, error) {
			if string(req.AdditionalAuthenticatedData) != string(expectedAAD) {
				t.Errorf("encrypt: AAD = %q, want %q", req.AdditionalAuthenticatedData, expectedAAD)
			}
			return &kmspb.EncryptResponse{
				Ciphertext: append([]byte(testKeyId), req.Plaintext...),
			}, nil
		},
		decrypt: func(req *kmspb.DecryptRequest) (*kmspb.DecryptResponse, error) {
			if string(req.AdditionalAuthenticatedData) != string(expectedAAD) {
				t.Errorf("decrypt: AAD = %q, want %q", req.AdditionalAuthenticatedData, expectedAAD)
			}
			return &kmspb.DecryptResponse{
				Plaintext: req.Ciphertext[len(testKeyId):],
			}, nil
		},
	})

	provider, meta, err := (&Config{
		KMSKeyName:                  testKeyId,
		KeyLength:                   32,
		AdditionalAuthenticatedData: "bXktYWFkLXZhbA==",
	}).Build()
	if err != nil {
		t.Fatal(err)
	}

	_, meta, err = provider.Provide(meta)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = provider.Provide(meta)
	if err != nil {
		t.Fatal(err)
	}
}
