// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pbkdf2_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/encryption/config"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/pbkdf2"
	"github.com/zclconf/go-cty/cty"
)

func TestHashFunctionName_Validate(t *testing.T) {
	tc := map[string]struct {
		hashFunctionName pbkdf2.HashFunctionName
		valid            bool
	}{
		"empty": {
			hashFunctionName: "",
			valid:            false,
		},
		"sha256": {
			hashFunctionName: pbkdf2.SHA256HashFunctionName,
			valid:            true,
		},
		"sha0": {
			hashFunctionName: "sha0",
			valid:            false,
		},
	}

	for name, testCase := range tc {
		t.Run(name, func(t *testing.T) {
			err := testCase.hashFunctionName.Validate()
			if testCase.valid && err != nil {
				t.Fatalf("unexpected error: %v", err)
			} else if !testCase.valid && err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func generateFixedStringHelper(length int) string {
	result := ""
	for i := 0; i < length; i++ {
		result += "a"
	}
	return result
}

func TestConfig_Build(t *testing.T) {
	knownGood := func() *pbkdf2.Config {
		return pbkdf2.New().TypedConfig().WithPassphrase(generateFixedStringHelper(pbkdf2.MinimumPassphraseLength))
	}
	tc := map[string]struct {
		config *pbkdf2.Config
		valid  bool
	}{
		"empty": {
			config: &pbkdf2.Config{},
			valid:  false,
		},
		"default": {
			// Missing passphrase
			config: pbkdf2.New().ConfigStruct().(*pbkdf2.Config),
			valid:  false,
		},
		"default-short-passphrase": {
			config: pbkdf2.New().TypedConfig().WithPassphrase(generateFixedStringHelper(pbkdf2.MinimumPassphraseLength - 1)),
			valid:  false,
		},
		"default-good-passphrase": {
			config: knownGood(),
			valid:  true,
		},
		"invalid-key-length": {
			config: knownGood().WithKeyLength(0),
			valid:  false,
		},
		"invalid-iterations": {
			config: knownGood().WithIterations(0),
			valid:  false,
		},
		"low-iterations": {
			config: knownGood().WithIterations(pbkdf2.MinimumIterations - 1),
			valid:  false,
		},
		"invalid-salt-length": {
			config: knownGood().WithSaltLength(0),
			valid:  false,
		},
		"invalid-hash-function": {
			config: knownGood().WithHashFunction(""),
			valid:  false,
		},
	}
	for name, testCase := range tc {
		t.Run(name, func(t *testing.T) {
			_, _, err := testCase.config.Build()
			if testCase.valid && err != nil {
				t.Fatalf("unexpected error: %v", err)
			} else if !testCase.valid && err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestConfig_DecodeBody(t *testing.T) {
	tc := map[string]struct {
		setup          func(t *testing.T) (hcl.Body, *hcl.EvalContext)
		expectedConfig *pbkdf2.Config
	}{
		"nil body": {
			setup: func(*testing.T) (hcl.Body, *hcl.EvalContext) {
				return nil, nil
			},
			expectedConfig: &pbkdf2.Config{},
		},
		"body with passphrase": {
			setup: func(t *testing.T) (hcl.Body, *hcl.EvalContext) {
				parsedSourceConfig, diags := config.LoadConfigFromString("source", `
		key_provider "pbkdf2" "base1" {
			passphrase = "Hello world! 123"
		}`)
				if diags.HasErrors() {
					t.Fatalf("failed to create config: %s", diags.Error())
				}
				return parsedSourceConfig.KeyProviderConfigs[0].Body, &hcl.EvalContext{}
			},
			expectedConfig: &pbkdf2.Config{Passphrase: "Hello world! 123"},
		},
		"hcl body with chain reference": {
			setup: func(t *testing.T) (hcl.Body, *hcl.EvalContext) {
				parsedSourceConfig, diags := config.LoadConfigFromString("source", `
		key_provider "pbkdf2" "base1" {
			passphrase = "Hello world! 123"
		}
		key_provider "pbkdf2" "base2" {
			chain = key_provider.pbkdf2.base1
		}`)
				if diags.HasErrors() {
					t.Fatalf("failed to create config: %s", diags.Error())
				}
				evalCtx := &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"key_provider": cty.ObjectVal(map[string]cty.Value{
							"pbkdf2": cty.ObjectVal(map[string]cty.Value{
								"base1": cty.ObjectVal(map[string]cty.Value{
									"encryption_key": byteToCty([]byte("Hello world! 123")),
									"decryption_key": byteToCty([]byte("Hello world! 123")),
								}),
							}),
						}),
					},
				}
				return parsedSourceConfig.KeyProviderConfigs[1].Body, evalCtx
			},
			expectedConfig: &pbkdf2.Config{
				Chain: &keyprovider.Output{
					EncryptionKey: []byte("Hello world! 123"),
					DecryptionKey: []byte("Hello world! 123"),
				},
			},
		},
		"json body with chain reference as string": {
			setup: func(t *testing.T) (hcl.Body, *hcl.EvalContext) {
				parsedSourceConfig, diags := config.LoadConfigFromString("source", `{
		      "key_provider": {
		        "pbkdf2": {
		          "base1": {
					"passphrase": "Hello world! 123"
		          },
		          "base2": {
					"chain": "key_provider.pbkdf2.base1"
		          }
		        }
		      }
		    }`)
				if diags.HasErrors() {
					t.Fatalf("failed to create config: %s", diags.Error())
				}
				evalCtx := &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"key_provider": cty.ObjectVal(map[string]cty.Value{
							"pbkdf2": cty.ObjectVal(map[string]cty.Value{
								"base1": cty.ObjectVal(map[string]cty.Value{
									"encryption_key": byteToCty([]byte("Hello world! 123")),
									"decryption_key": byteToCty([]byte("Hello world! 123")),
								}),
							}),
						}),
					},
				}
				return parsedSourceConfig.KeyProviderConfigs[1].Body, evalCtx
			},
			expectedConfig: &pbkdf2.Config{
				Chain: &keyprovider.Output{
					EncryptionKey: []byte("Hello world! 123"),
					DecryptionKey: []byte("Hello world! 123"),
				},
			},
		},
		"json body with interpolated chain reference": {
			setup: func(t *testing.T) (hcl.Body, *hcl.EvalContext) {
				parsedSourceConfig, diags := config.LoadConfigFromString("source", `{
		      "key_provider": {
		        "pbkdf2": {
		          "base1": {
					"passphrase": "Hello world! 123"
		          },
		          "base2": {
					"chain": "${key_provider.pbkdf2.base1}"
		          }
		        }
		      }
		    }`)
				if diags.HasErrors() {
					t.Fatalf("failed to create config: %s", diags.Error())
				}
				evalCtx := &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"key_provider": cty.ObjectVal(map[string]cty.Value{
							"pbkdf2": cty.ObjectVal(map[string]cty.Value{
								"base1": cty.ObjectVal(map[string]cty.Value{
									"encryption_key": byteToCty([]byte("Hello world! 123")),
									"decryption_key": byteToCty([]byte("Hello world! 123")),
								}),
							}),
						}),
					},
				}
				return parsedSourceConfig.KeyProviderConfigs[1].Body, evalCtx
			},
			expectedConfig: &pbkdf2.Config{
				Chain: &keyprovider.Output{
					EncryptionKey: []byte("Hello world! 123"),
					DecryptionKey: []byte("Hello world! 123"),
				},
			},
		},
		"json with all properties defined": {
			setup: func(t *testing.T) (hcl.Body, *hcl.EvalContext) {
				parsedSourceConfig, diags := config.LoadConfigFromString("source", `{
		      "key_provider": {
		        "pbkdf2": {
		          "base1": {
					"passphrase": "Hello world! 123"
		          },
		          "base2": {
					"chain": "${key_provider.pbkdf2.base1}",
					"key_length": 256,
					"iterations": 3,
					"hash_function": "sha256",
					"salt_length": 3
		          }
		        }
		      }
		    }`)
				if diags.HasErrors() {
					t.Fatalf("failed to create config: %s", diags.Error())
				}
				evalCtx := &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"key_provider": cty.ObjectVal(map[string]cty.Value{
							"pbkdf2": cty.ObjectVal(map[string]cty.Value{
								"base1": cty.ObjectVal(map[string]cty.Value{
									"encryption_key": byteToCty([]byte("Hello world! 123")),
									"decryption_key": byteToCty([]byte("Hello world! 123")),
								}),
							}),
						}),
					},
				}
				return parsedSourceConfig.KeyProviderConfigs[1].Body, evalCtx
			},
			expectedConfig: &pbkdf2.Config{
				Chain: &keyprovider.Output{
					EncryptionKey: []byte("Hello world! 123"),
					DecryptionKey: []byte("Hello world! 123"),
				},
				KeyLength:    256,
				Iterations:   3,
				HashFunction: "sha256",
				SaltLength:   3,
			},
		},
		"hcl with all properties defined": {
			setup: func(t *testing.T) (hcl.Body, *hcl.EvalContext) {
				parsedSourceConfig, diags := config.LoadConfigFromString("source", `
key_provider "pbkdf2" "base1" {
	passphrase = "Hello world! 123"
}

key_provider "pbkdf2" "base2" {
	chain = key_provider.pbkdf2.base1
	key_length = 256
	iterations = 3
	hash_function = "sha256"
	salt_length = 3
}`)
				if diags.HasErrors() {
					t.Fatalf("failed to create config: %s", diags.Error())
				}
				evalCtx := &hcl.EvalContext{
					Variables: map[string]cty.Value{
						"key_provider": cty.ObjectVal(map[string]cty.Value{
							"pbkdf2": cty.ObjectVal(map[string]cty.Value{
								"base1": cty.ObjectVal(map[string]cty.Value{
									"encryption_key": byteToCty([]byte("Hello world! 123")),
									"decryption_key": byteToCty([]byte("Hello world! 123")),
								}),
							}),
						}),
					},
				}
				return parsedSourceConfig.KeyProviderConfigs[1].Body, evalCtx
			},
			expectedConfig: &pbkdf2.Config{
				Chain: &keyprovider.Output{
					EncryptionKey: []byte("Hello world! 123"),
					DecryptionKey: []byte("Hello world! 123"),
				},
				KeyLength:    256,
				Iterations:   3,
				HashFunction: "sha256",
				SaltLength:   3,
			},
		},
	}
	for name, testCase := range tc {
		t.Run(name, func(t *testing.T) {
			body, evalCtx := testCase.setup(t)
			cfg := &pbkdf2.Config{}
			diags := cfg.DecodeConfig(body, evalCtx)
			if diff := cmp.Diff(testCase.expectedConfig, cfg, cmpopts.IgnoreUnexported(pbkdf2.Config{})); diff != "" {
				t.Errorf("wrong config. diff (-want, +got):\n%s", diff)
			}
			if len(diags) != 0 {
				t.Errorf("unexpected diagnostics: %s", diags.Error())
			}
		})
	}
}

func byteToCty(data []byte) cty.Value {
	if len(data) == 0 {
		return cty.NullVal(cty.List(cty.Number))
	}
	ctyData := make([]cty.Value, len(data))
	for i, d := range data {
		ctyData[i] = cty.NumberIntVal(int64(d))
	}
	return cty.ListVal(ctyData)
}
