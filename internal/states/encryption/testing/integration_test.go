package testing

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/states/encryption"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionflow"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// This file contains integration tests that test the encryption subsystem from the outside.
//
// They emulate how the rest of tofu interacts with the encryption subsystem.
//
// Since encryption involves some randomness, the way to test is usually by performing a round trip:
// take a plaintext, encrypt it, then decrypt it again.

// These test cases cover the business requirements from the RFC, see
// https://github.com/opentofu/opentofu/issues/874

type integrationTestcase struct {
	// the name of the testcase
	name string

	// the human-readable description of the testcase
	description string

	// if this is set, the testcase will be skipped (e.g. not yet implemented feature)
	skipReason string

	// skip the test if this environment variable is unset (e.g. AWS KMS setup)
	skipUnlessEnvSet string

	// value to set TF_STATE_ENCRYPTION to, if not empty
	TF_STATE_ENCRYPTION string

	// value to set TF_STATE_DECRYPTION_FALLBACK to, if not empty
	TF_STATE_DECRYPTION_FALLBACK string

	// expected environment variable early parse result - encryption
	expectedEnvEncryptionParseError error

	// expected environment variable early parse result - decryption fallback
	expectedEnvDecryptionParseError error

	// expected environment variable early parse result when applying
	expectedEnvApplyError error

	// configuration that is injected from code in .tf files at tofu parse time
	//
	// The corresponding code is either in the terraform block, or in remote state data source blocks.
	//
	// This is from all the state_encryption sections.
	codeConfigStateEncryption map[string]encryptionconfig.Config

	// configuration that is injected from code in .tf files at tofu parse time
	//
	// The corresponding code is either in the terraform block, or in remote state data source blocks.
	//
	// This is from all the state_decryption_fallback sections.
	codeConfigStateDecryptionFallback map[string]encryptionconfig.Config

	// expected config validation result
	expectedEarlyValidationResults tfdiags.Diagnostics

	// the flow accessor to use for encryption/decryption of state or plan for this test case
	singletonAccessor func(singleton encryption.Encryption) (encryptionflow.Flow, error)

	// what to pass to the encryption (filename in testdata/)
	inputFilename string

	// if set, what to check the encryption output against (filename in testdata/), only useful if not encrypted
	expectedEncryptionOutputFilename string

	// if set, what to inject as encryption result before decrypting (filename in testdata/)
	//
	// useful to provoke certain decryption errors
	encryptionOutputOverrideFilename string

	// what to expect as decryption result (filename in testdata/)
	expectedOutputFilename string

	// expected errors of the encryption / decryption steps
	expectedEncryptionError error
	expectedDecryptionError error
}

func remoteStateAccessor(singleton encryption.Encryption) (encryptionflow.Flow, error) {
	flow, err := singleton.RemoteState()
	if flow != nil {
		return flow.(encryptionflow.Flow), err
	} else {
		return nil, err
	}
}

func stateFileAccessor(singleton encryption.Encryption) (encryptionflow.Flow, error) {
	flow, err := singleton.StateFile()
	if flow != nil {
		return flow.(encryptionflow.Flow), err
	} else {
		return nil, err
	}
}

func remoteStateDataSourceAccessor(key encryptionconfig.Key) func(encryption.Encryption) (encryptionflow.Flow, error) {
	return func(singleton encryption.Encryption) (encryptionflow.Flow, error) {
		flow, err := singleton.RemoteStateDatasource(key)
		if flow != nil {
			return flow.(encryptionflow.Flow), err
		} else {
			return nil, err
		}
	}
}

func TestStateEncryptionRoundtrip(t *testing.T) {
	testCases := []integrationTestcase{
		// --- Summary ---
		// "Encryption is off-by-default."
		{
			name:                             "off_by_default",
			description:                      "encryption is off by default if no configuration is provided",
			singletonAccessor:                remoteStateAccessor,
			inputFilename:                    "terraform.tfstate.original",
			expectedEncryptionOutputFilename: "terraform.tfstate.original",
			expectedOutputFilename:           "terraform.tfstate.original",
		},
		// --- User-facing description: Getting started ---
		// "you could simply set an environment variable [...] suddenly, your remote state looks like this"
		{
			name:                   "full_encryption_explicit_config_env",
			description:            "encryption configured using only environment variable TF_STATE_ENCRYPTION with all fields specified explicitly",
			TF_STATE_ENCRYPTION:    `{"backend":{"method":{"name":"full"},"key_provider":{"name":"passphrase","config":{"passphrase":"foobarbaz"}}}}`,
			singletonAccessor:      remoteStateAccessor,
			inputFilename:          "terraform.tfstate.original",
			expectedOutputFilename: "terraform.tfstate.original",
		},
		// "Actually, most of the settings shown in the environment variable have sensible defaults, so this also works"
		{
			name:                   "full_encryption_config_defaults",
			description:            "encryption configured using only environment variable TF_STATE_ENCRYPTION with defaults omitted",
			TF_STATE_ENCRYPTION:    `{"backend":{"key_provider":{"config":{"passphrase":"foobarbaz"}}}}`,
			singletonAccessor:      remoteStateAccessor,
			inputFilename:          "terraform.tfstate.original",
			expectedOutputFilename: "terraform.tfstate.original",
		},
		// "You can also specify the 32-byte key directly instead of providing a passphrase"
		{
			name:                   "full_encryption_key_provider_direct",
			description:            "encryption configured using only environment variable TF_STATE_ENCRYPTION with key provider 'direct'",
			TF_STATE_ENCRYPTION:    `{"backend":{"key_provider":{"name":"direct","config":{"key":"a0a1a2a3a4a5a6a7a8a9b0b1b2b3b4b5b6b7b8b9c0c1c2c3c4c5c6c7c8c9d0d1"}}}}`,
			singletonAccessor:      remoteStateAccessor,
			inputFilename:          "terraform.tfstate.original",
			expectedOutputFilename: "terraform.tfstate.original",
		},
		// "Better yet, the key can also come from AWS KMS"
		{
			name:        "key_provider_aws",
			description: "obtaining key from AWS KMS",
			skipReason:  "key provider AWS not yet implemented",
		},
		// "Or [...] from an Azure Key Vault"
		{
			name:        "key_provider_azure_key_vault",
			description: "obtaining key from Azure Key Vault",
			skipReason:  "key provider Azure Key Vault not yet implemented",
		},
		// "or GCP Key Mgmt"
		{
			name:        "key_provider_gcp_key_mgmt",
			description: "obtaining key from GCP Key Mgmt",
			skipReason:  "key provider GCP Key Mgmt not yet implemented",
		},
		// "or Vault"
		{
			name:        "key_provider_vault",
			description: "obtaining key from Vault",
			skipReason:  "key provider Vault not yet implemented",
		},
		// "Instead of full state encryption, you can have just the sensitive values encrypted in the state"
		{
			name:        "sensitive_encryption_method",
			description: "encrypting just the sensitive fields",
			skipReason:  "encryption method sensitive not yet implemented (also see open questions in RFC)",
		},
		// --- User-facing description: Once Your State Is Encrypted ---
		// "If you want to rotate state encryption keys ..."
		{
			name:                             "key_rotation_same_method_decryption_part",
			description:                      "decryption can use a fallback key to allow key rotation",
			TF_STATE_ENCRYPTION:              `{"backend":{"key_provider":{"config":{"passphrase":"build a better passphrase"}}}}`,
			TF_STATE_DECRYPTION_FALLBACK:     `{"backend":{"key_provider":{"config":{"passphrase":"foobarbaz"}}}}`,
			singletonAccessor:                remoteStateAccessor,
			encryptionOutputOverrideFilename: "terraform.tfstate.full.foobarbaz",
			expectedOutputFilename:           "terraform.tfstate.original",
		},
		// "or even switch state encryption methods"
		{
			name:                             "key_rotation_different_method_decryption_part",
			description:                      "decryption can use a fallback key to allow key rotation",
			TF_STATE_ENCRYPTION:              `{"backend":{"method":{"name":"partial"},"key_provider":{"config":{"passphrase":"could be the same"}}}}`,
			TF_STATE_DECRYPTION_FALLBACK:     `{"backend":{"key_provider":{"config":{"passphrase":"foobarbaz"}}}}`,
			singletonAccessor:                remoteStateAccessor,
			encryptionOutputOverrideFilename: "terraform.tfstate.full.foobarbaz",
			expectedOutputFilename:           "terraform.tfstate.original",
		},
		// "Unencrypted state is recognized and automatically bypasses the decryption step. That's what happens during initial encryption"
		{
			name:                             "initial_encryption_local_statefile",
			description:                      "decryption is bypassed if state is not actually encrypted",
			TF_STATE_ENCRYPTION:              `{"statefile":{"key_provider":{"config":{"passphrase":"foobarbaz"}}}}`,
			TF_STATE_DECRYPTION_FALLBACK:     `{"statefile":{"key_provider":{"config":{"passphrase":"even older foobarbaz"}}}}`,
			singletonAccessor:                stateFileAccessor,
			encryptionOutputOverrideFilename: "terraform.tfstate.original",
			expectedOutputFilename:           "terraform.tfstate.original",
		},
		// "If you set TF_STATE_DECRYPTION_FALLBACK but not TF_STATE_ENCRYPTION, the next apply will decrypt your state"
		{
			name:                             "decrypt_state_encryption_step_only",
			description:                      "not setting an encryption method decrypts state",
			TF_STATE_ENCRYPTION:              "",
			TF_STATE_DECRYPTION_FALLBACK:     `{"statefile":{"key_provider":{"config":{"passphrase":"foobarbaz"}}}}`,
			singletonAccessor:                stateFileAccessor,
			inputFilename:                    "terraform.tfstate.original",
			expectedEncryptionOutputFilename: "terraform.tfstate.original", // not encrypted
			expectedOutputFilename:           "terraform.tfstate.original",
		},
		// --- User-facing description: Advanced Configuration ---
		// "you can also add equivalent configuration to the remote state configuration in your code"
		{
			name:        "terraform_block_remote_state_config",
			description: "full configuration from terraform block",
			codeConfigStateEncryption: map[string]encryptionconfig.Config{
				"terraform_block_with_remote_state": {
					KeyProvider: encryptionconfig.KeyProviderConfig{
						Name: "passphrase",
						Config: map[string]string{
							"passphrase": "foobarbaz", // not a realistic example, shouldn't hardcode the key in terraform block
						},
					},
					Method:   encryptionconfig.MethodConfig{},
					Enforced: true,
				},
			},
			singletonAccessor:      remoteStateAccessor,
			inputFilename:          "terraform.tfstate.original",
			expectedOutputFilename: "terraform.tfstate.original",
		},
		{
			name:        "terraform_block_remote_state_config_fallback",
			description: "full configuration from terraform block",
			codeConfigStateDecryptionFallback: map[string]encryptionconfig.Config{
				"terraform_block_with_remote_state": {
					KeyProvider: encryptionconfig.KeyProviderConfig{
						Name: "passphrase",
						Config: map[string]string{
							"passphrase": "foobarbaz", // not a realistic example, shouldn't hardcode the key in terraform block
						},
					},
					Method: encryptionconfig.MethodConfig{},
				},
			},
			singletonAccessor:                remoteStateAccessor,
			encryptionOutputOverrideFilename: "terraform.tfstate.full.foobarbaz",
			expectedOutputFilename:           "terraform.tfstate.original",
		},
		{
			name:        "terraform_block_no_remote_state_config",
			description: "full configuration from terraform block",
			codeConfigStateEncryption: map[string]encryptionconfig.Config{
				"terraform_block_with_local_state": {
					KeyProvider: encryptionconfig.KeyProviderConfig{
						Name: "passphrase",
						Config: map[string]string{
							"passphrase": "foobarbaz", // not a realistic example, shouldn't hardcode the key in terraform block
						},
					},
					Method:   encryptionconfig.MethodConfig{},
					Enforced: true,
				},
			},
			singletonAccessor:      stateFileAccessor, // no backend block -> local state
			inputFilename:          "terraform.tfstate.original",
			expectedOutputFilename: "terraform.tfstate.original",
		},
		{
			name:        "terraform_block_no_remote_state_config_fallback",
			description: "full configuration from terraform block",
			codeConfigStateDecryptionFallback: map[string]encryptionconfig.Config{
				"terraform_block_with_local_state": {
					KeyProvider: encryptionconfig.KeyProviderConfig{
						Name: "passphrase",
						Config: map[string]string{
							"passphrase": "foobarbaz", // not a realistic example, shouldn't hardcode the key in terraform block
						},
					},
					Method: encryptionconfig.MethodConfig{},
				},
			},
			singletonAccessor:                stateFileAccessor, // no backend block -> local state
			encryptionOutputOverrideFilename: "terraform.tfstate.full.foobarbaz",
			expectedOutputFilename:           "terraform.tfstate.original",
		},
		// "configuration is merged between [...] in that order"
		{
			name:                "config_merge_env_terraform_block",
			description:         "configuration is merged between terraform block and environment",
			TF_STATE_ENCRYPTION: `{"statefile":{"key_provider":{"config":{"key":"a0a1a2a3a4a5a6a7a8a9b0b1b2b3b4b5b6b7b8b9c0c1c2c3c4c5c6c7c8c9d0d1"}}}}`,
			codeConfigStateEncryption: map[string]encryptionconfig.Config{
				"terraform_block_with_local_state": {
					KeyProvider: encryptionconfig.KeyProviderConfig{
						Name: "direct",
					},
				},
			},
			singletonAccessor:      stateFileAccessor, // no backend block -> local state
			inputFilename:          "terraform.tfstate.original",
			expectedOutputFilename: "terraform.tfstate.original",
		},
		{
			name:        "config_merge_env_terraform_block_forgot_to_set_env",
			description: "configuration is merged between terraform block and environment: forgot to set environment variable",
			codeConfigStateEncryption: map[string]encryptionconfig.Config{
				"terraform_block_with_local_state": {
					KeyProvider: encryptionconfig.KeyProviderConfig{
						Name: "direct",
					},
					Method:   encryptionconfig.MethodConfig{},
					Enforced: true,
				},
			},
			singletonAccessor: stateFileAccessor, // no backend block -> local state
			expectedEarlyValidationResults: tfdiags.Diagnostics{
				tfdiags.Sourceless(tfdiags.Error,
					"Invalid state encryption configuration for configuration key statefile",
					"failed to merge encryption configuration (invalid configuration after merge (error in configuration for key provider direct (field 'key' missing or empty)))",
				),
			},
			inputFilename:          "terraform.tfstate.original",
			expectedOutputFilename: "terraform.tfstate.original",
		},
		// --- User-facing description: Mixing encryption keys and the terraform_remote_state data source ---
		// "That is why the terraform_remote_state data source will be expanded to also allow its own encryption configuration"
		{
			name:        "config_for_remote_data_source_in_code",
			description: "separate configuration for a terraform_remote_state data source in the code that declares the data source",
			codeConfigStateEncryption: map[string]encryptionconfig.Config{
				"terraform_remote_state.foo": {
					KeyProvider: encryptionconfig.KeyProviderConfig{
						Name: "passphrase",
						Config: map[string]string{
							"passphrase": "this is a bad idea", // not a very realistic example, should not hardcode passphrase
						},
					},
					Method: encryptionconfig.MethodConfig{
						Name: "partial",
					},
				},
			},
			singletonAccessor:      remoteStateDataSourceAccessor("terraform_remote_state.foo"),
			inputFilename:          "terraform.tfstate.original",
			expectedOutputFilename: "terraform.tfstate.sorted", // partial encryption method sorts state json fields in current implementation
		},
		{
			name:        "config_for_remote_data_source_in_code_decryption_fallback",
			description: "separate configuration for a terraform_remote_state data source in the code that declares the data source: decryption fallback for key rotation",
			codeConfigStateEncryption: map[string]encryptionconfig.Config{
				"terraform_remote_state.foo[3]": {
					KeyProvider: encryptionconfig.KeyProviderConfig{
						Name: "passphrase",
						Config: map[string]string{
							"passphrase": "this is a bad idea", // not a very realistic example, should not hardcode passphrases
						},
					},
					Method: encryptionconfig.MethodConfig{
						Name: "full",
					},
				},
			},
			codeConfigStateDecryptionFallback: map[string]encryptionconfig.Config{
				"terraform_remote_state.foo[3]": {
					KeyProvider: encryptionconfig.KeyProviderConfig{
						Name: "passphrase",
						Config: map[string]string{
							"passphrase": "foobarbaz", // this one will work
						},
					},
					Method: encryptionconfig.MethodConfig{
						Name: "full",
					},
				},
			},
			singletonAccessor:                remoteStateDataSourceAccessor("terraform_remote_state.foo[3]"),
			encryptionOutputOverrideFilename: "terraform.tfstate.full.foobarbaz",
			expectedOutputFilename:           "terraform.tfstate.original",
		},
		// "Again, this configuration can also be specified via environment variable"
		{
			name:                   "config_for_remote_data_source_in_env",
			description:            "separate configuration for a terraform_remote_state data source via environment variable",
			TF_STATE_ENCRYPTION:    `{"terraform_remote_state.foo":{"method":{"name":"full"},"key_provider":{"name":"passphrase","config":{"passphrase":"foobarbaz"}}}}`,
			singletonAccessor:      remoteStateDataSourceAccessor("terraform_remote_state.foo"),
			inputFilename:          "terraform.tfstate.original",
			expectedOutputFilename: "terraform.tfstate.original",
		},

		//{
		//	name:                              "fill_this",
		//	description:                       "encryption is off by default if no configuration is provided",
		//	skipReason:                        "set to skip",
		//	TF_STATE_ENCRYPTION:               "",
		//	TF_STATE_DECRYPTION_FALLBACK:      "",
		//	expectedEnvParseError:             nil,
		//	codeConfigStateEncryption:         nil,
		//	codeConfigStateDecryptionFallback: nil,
		//	expectedEarlyValidationResults:    nil,
		//	singletonAccessor:                 remoteStateAccessor,
		//	inputFilename:                     "terraform.tfstate.original",
		//	expectedEncryptionOutputFilename:  "terraform.tfstate.original",
		//	expectedOutputFilename:            "terraform.tfstate.original",
		//	expectedEncryptionError:           nil,
		//	expectedDecryptionError:           nil,
		//},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			roundTripIntegrationTestcase(t, tc)
		})
	}
}

// roundTripIntegrationTestcase is a good starting point if you wish to understand
// how the rest of tofu interacts with the encryption package from the outside.
func roundTripIntegrationTestcase(t *testing.T, tc integrationTestcase) {
	if tc.skipReason != "" {
		t.Skip(tc.skipReason)
		return
	}
	if tc.skipUnlessEnvSet != "" {
		if os.Getenv(tc.skipUnlessEnvSet) == "" {
			t.Skipf("skipping because environment variable %s is not set or empty", tc.skipUnlessEnvSet)
			return
		}
	}

	// during tofu startup (in the meta command):

	singleton := encryption.GetSingleton()
	defer encryption.ClearSingleton()

	// would normally use os.GetEnv
	encryptionConfigsFromEnv, err := encryptionconfig.ConfigurationFromEnv(tc.TF_STATE_ENCRYPTION)
	expectErr(t, err, tc.expectedEnvEncryptionParseError)
	if err != nil {
		// tofu aborts if envs fail to parse, end the test case
		return
	}

	// would normally use os.GetEnv
	decryptionFallbackConfiggFromEnv, err := encryptionconfig.ConfigurationFromEnv(tc.TF_STATE_DECRYPTION_FALLBACK)
	expectErr(t, err, tc.expectedEnvDecryptionParseError)
	if err != nil {
		// tofu aborts if envs fail to parse, end the test case
		return
	}

	err = singleton.ApplyEnvConfigurations(encryptionConfigsFromEnv, decryptionFallbackConfiggFromEnv)
	expectErr(t, err, tc.expectedEnvApplyError)
	if err != nil {
		// tofu aborts if envs fail to parse, end the test case
		return
	}

	// while parsing terraform {...} block and/or remote state data sources:
	for k, v := range tc.codeConfigStateEncryption {
		switch k {
		case "terraform_block_with_local_state":
			err = singleton.ApplyHCLEncryptionConfiguration(encryptionconfig.KeyStateFile, v)
			if err != nil {
				t.Fatalf("unexpected error applying code config: %s", err.Error())
			}
		case "terraform_block_with_remote_state":
			err = singleton.ApplyHCLEncryptionConfiguration(encryptionconfig.KeyBackend, v)
			if err != nil {
				t.Fatalf("unexpected error applying code config: %s", err.Error())
			}
		default:
			// some remote state data source, k is something like "terraform_remote_state.foo"
			err = singleton.ApplyHCLEncryptionConfiguration(encryptionconfig.Key(k), v)
			if err != nil {
				t.Fatalf("unexpected error applying code config: %s", err.Error())
			}
		}
	}
	for k, v := range tc.codeConfigStateDecryptionFallback {
		switch k {
		case "terraform_block_with_local_state":
			err = singleton.ApplyHCLDecryptionFallbackConfiguration(encryptionconfig.KeyStateFile, v)
			if err != nil {
				t.Fatalf("unexpected error applying code config: %s", err.Error())
			}
		case "terraform_block_with_remote_state":
			err = singleton.ApplyHCLDecryptionFallbackConfiguration(encryptionconfig.KeyBackend, v)
			if err != nil {
				t.Fatalf("unexpected error applying code config: %s", err.Error())
			}
		default:
			// some remote state data source, k is something like "terraform_remote_state.foo"
			err = singleton.ApplyHCLDecryptionFallbackConfiguration(encryptionconfig.Key(k), v)
			if err != nil {
				t.Fatalf("unexpected error applying code config: %s", err.Error())
			}
		}
	}

	// at the end of tofu configuration / parse time:
	diags := singleton.Validate()
	if len(diags) != len(tc.expectedEarlyValidationResults) {
		t.Fatalf("unexpected validation errors")
	}
	for i := range diags {
		if diags[i].Description() != tc.expectedEarlyValidationResults[i].Description() {
			t.Errorf("unexpected validation\n'%s'\ninstead of\n'%s'", diags[i].Description(), tc.expectedEarlyValidationResults[i].Description())
		}
		if diags[i].Severity() == tfdiags.Error {
			// test case ends here, because tofu would abort the run
			return
		}
	}

	// now we are during tofu resource/state/plan processing:
	instance, err := tc.singletonAccessor(singleton)
	if err != nil {
		t.Fatalf("unexpected singleton accessor error: %s", err.Error())
	}

	if tc.encryptionOutputOverrideFilename != "" {
		encrypted, err := os.ReadFile(fmt.Sprintf("testdata/%s", tc.encryptionOutputOverrideFilename))
		if err != nil {
			t.Fatalf(err.Error())
		}

		// this testcase tests some decryption error, so go straight to decryption
		decrypted, err := instance.DecryptState(encrypted)
		expectErr(t, err, tc.expectedDecryptionError)
		if err == nil {
			expectedOutput, err := os.ReadFile(fmt.Sprintf("testdata/%s", tc.expectedOutputFilename))
			if err != nil {
				t.Fatalf(err.Error())
			}

			if string(decrypted) != string(expectedOutput) {
				t.Error("decrypted result failed to equal expected result")
			}
		}
	} else {
		// full roundtrip testcase
		input, err := os.ReadFile(fmt.Sprintf("testdata/%s", tc.inputFilename))
		if err != nil {
			t.Fatalf(err.Error())
		}

		encrypted, err := instance.EncryptState(input)
		expectErr(t, err, tc.expectedEncryptionError)
		if err == nil {
			if tc.expectedEncryptionOutputFilename != "" {
				expectedEncryptionOutput, err := os.ReadFile(fmt.Sprintf("testdata/%s", tc.expectedEncryptionOutputFilename))
				if err != nil {
					t.Fatalf(err.Error())
				}

				if string(expectedEncryptionOutput) != string(encrypted) {
					t.Error("intermediate encryption result failed to equal expected value")
				}
			}

			decrypted, err := instance.DecryptState(encrypted)
			expectErr(t, err, tc.expectedDecryptionError)
			if err == nil {
				expectedOutput, err := os.ReadFile(fmt.Sprintf("testdata/%s", tc.expectedOutputFilename))
				if err != nil {
					t.Fatalf(err.Error())
				}

				if string(decrypted) != string(expectedOutput) {
					t.Error("decrypted result failed to equal expected result")
				}
			}
		}
	}
}

func expectErr(t *testing.T, actual error, expected error) {
	if actual != nil {
		if expected == nil {
			t.Errorf("received unexpected error '%s' instead of success", actual.Error())
		} else if strings.HasSuffix(expected.Error(), "*") {
			expectStr := strings.TrimSuffix(expected.Error(), "*")
			if !strings.HasPrefix(actual.Error(), expectStr) {
				t.Errorf("received unexpected error '%s' that does not start with '%s'", actual.Error(), expectStr)
			}
		} else if actual.Error() != expected.Error() {
			t.Errorf("received unexpected error '%s' instead of '%s'", actual.Error(), expected.Error())
		}
	} else {
		if expected != nil {
			t.Errorf("unexpected success instead of expected error '%s'", expected.Error())
		}
	}
}
