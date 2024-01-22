package encryptionconfig

import (
	"errors"
	"reflect"
	"testing"
)

func TestConfigValidate(t *testing.T) {
	testCases := []struct {
		testcase    string
		config      Config
		expectedErr error
	}{
		{
			testcase: "correct",
			config: Config{
				KeyProvider: KeyProviderConfig{
					Name:   "passphrase",
					Config: map[string]string{"passphrase": "quick brown fox"},
				},
				Method: MethodConfig{
					Name: "full",
				},
			},
			expectedErr: nil,
		},
		{
			testcase: "key_provider_wrong",
			config: Config{
				KeyProvider: KeyProviderConfig{
					Name: "unknown",
				},
				Method: MethodConfig{
					Name: "full",
				},
			},
			expectedErr: errors.New("error in configuration for key provider unknown (no registered key provider with this name)"),
		},
		{
			testcase: "method_wrong",
			config: Config{
				KeyProvider: KeyProviderConfig{
					Name:   "passphrase",
					Config: map[string]string{"passphrase": "quick brown fox"},
				},
				Method: MethodConfig{
					Name: "unknown",
				},
			},
			expectedErr: errors.New("error in configuration for encryption method unknown: no registered encryption method with this name"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			err := tc.config.Validate()
			expectErr(t, err, tc.expectedErr)
		})
	}
}

func TestConfigMap_Merge(t *testing.T) {
	// used as input and expected result
	completeConfig := Config{
		KeyProvider: KeyProviderConfig{
			Name: "passphrase",
			Config: map[string]string{
				"passphrase": "quick brown fox",
			},
		},
		Method: MethodConfig{
			Name: "full",
		},
	}

	// used as inputs
	passphraseConfig := Config{
		KeyProvider: KeyProviderConfig{
			Config: map[string]string{
				"passphrase": "quick brown fox",
			},
		},
	}
	incompleteConfig := Config{
		Enforced: true,
	}

	// used as expected results
	mergedIncompleteConfig := Config{
		KeyProvider: KeyProviderConfig{
			Name: "passphrase",
		},
		Method: MethodConfig{
			Name: "full",
		},
		Enforced: true,
	}

	mergedEnforcingConfig := completeConfig
	mergedEnforcingConfig.Enforced = true

	testCases := []struct {
		testcase       string
		key            Key
		config         ConfigMap
		expectedResult *Config
		expectedErr    error
	}{
		{
			testcase:       "no_configs_key_uses_defaults",
			key:            KeyBackend,
			config:         nil,
			expectedResult: nil,
			expectedErr:    nil,
		},
		{
			testcase:       "no_configs_key_ignores_defaults",
			key:            KeyStateFile,
			config:         nil,
			expectedResult: nil,
			expectedErr:    nil,
		},
		// defaults
		{
			testcase: "valid_default_config_key_uses_defaults",
			key:      Key("terraform_remote_state.foo[hello]"),
			config: ConfigMap{
				Meta{SourceEnv, KeyDefaultRemote}: completeConfig,
			},
			expectedResult: &completeConfig,
			expectedErr:    nil,
		},
		{
			testcase: "valid_default_config_key_ignores_defaults",
			key:      KeyPlanFile,
			config: ConfigMap{
				Meta{SourceEnv, KeyDefaultRemote}: completeConfig,
			},
			expectedResult: nil, // default is ignored for non-remote
			expectedErr:    nil,
		},
		// validation errors
		{
			testcase: "incomplete_default_config",
			key:      KeyBackend,
			config: ConfigMap{
				Meta{SourceEnv, KeyDefaultRemote}: incompleteConfig,
			},
			expectedResult: &mergedIncompleteConfig,
			expectedErr:    errors.New("invalid configuration after merge (error in configuration for key provider passphrase (passphrase missing or empty))"),
		},
		{
			testcase: "incomplete_code_config",
			key:      KeyBackend,
			config: ConfigMap{
				Meta{SourceHCL, KeyBackend}: incompleteConfig,
			},
			expectedResult: &mergedIncompleteConfig,
			expectedErr:    errors.New("invalid configuration after merge (error in configuration for key provider passphrase (passphrase missing or empty))"),
		},
		// merges in default values for key provider and method
		{
			testcase: "successful_merge_with_injected_default_names",
			key:      KeyPlanFile,
			config: ConfigMap{
				Meta{SourceHCL, KeyPlanFile}: incompleteConfig,
				Meta{SourceEnv, KeyPlanFile}: passphraseConfig,
			},
			expectedResult: &mergedEnforcingConfig,
			expectedErr:    nil,
		},
		// ignores other keys
		{
			testcase: "other_keys_ignored_key_ignores_defaults",
			key:      KeyStateFile,
			config: ConfigMap{
				Meta{SourceEnv, KeyDefaultRemote}: incompleteConfig,
				Meta{SourceHCL, KeyPlanFile}:      incompleteConfig,
				Meta{SourceEnv, KeyPlanFile}:      passphraseConfig,
				Meta{SourceHCL, KeyBackend}:       incompleteConfig,
				Meta{SourceEnv, KeyBackend}:       passphraseConfig,
			},
			expectedResult: nil,
			expectedErr:    nil,
		},
		{
			testcase: "other_keys_ignored_key_uses_defaults",
			key:      KeyBackend,
			config: ConfigMap{
				Meta{SourceEnv, KeyDefaultRemote}: incompleteConfig,
				Meta{SourceHCL, KeyPlanFile}:      completeConfig,
				Meta{SourceEnv, KeyPlanFile}:      completeConfig,
				Meta{SourceHCL, KeyStateFile}:     completeConfig,
				Meta{SourceEnv, KeyStateFile}:     completeConfig,
			},
			expectedResult: &mergedIncompleteConfig,
			expectedErr:    errors.New("invalid configuration after merge (error in configuration for key provider passphrase (passphrase missing or empty))"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			actual, err := tc.config.Merge(tc.key)
			expectErr(t, err, tc.expectedErr)
			if actual == nil || tc.expectedResult == nil {
				if actual != nil {
					t.Errorf("merged configuration is unexpectedly non-nil")
				}
				if tc.expectedResult != nil {
					t.Errorf("merged configuration is unexpectedly nil")
				}
			} else {
				if !reflect.DeepEqual(*actual, *tc.expectedResult) {
					t.Errorf("merged configuration wrong:\n%v\nexpected:\n%v", *actual, *tc.expectedResult)
				}
			}
		})
	}
}

func TestKey(t *testing.T) {
	testCases := []struct {
		testcase                 string
		key                      Key
		expectUsesRemoteDefaults bool
		expectIsRemoteDataSource bool
		expectValidationErr      error
	}{
		{
			testcase:                 "valid_default_remote",
			key:                      KeyDefaultRemote,
			expectUsesRemoteDefaults: false,
			expectIsRemoteDataSource: false,
			expectValidationErr:      nil,
		},
		{
			testcase:                 "valid_backend",
			key:                      KeyBackend,
			expectUsesRemoteDefaults: true,
			expectIsRemoteDataSource: false,
			expectValidationErr:      nil,
		},
		{
			testcase:                 "valid_statefile",
			key:                      KeyStateFile,
			expectUsesRemoteDefaults: false,
			expectIsRemoteDataSource: false,
			expectValidationErr:      nil,
		},
		{
			testcase:                 "valid_planfile",
			key:                      KeyPlanFile,
			expectUsesRemoteDefaults: false,
			expectIsRemoteDataSource: false,
			expectValidationErr:      nil,
		},
		{
			testcase:                 "valid_remote_state",
			key:                      Key("terraform_remote_state.foo[42]"),
			expectUsesRemoteDefaults: true,
			expectIsRemoteDataSource: true,
			expectValidationErr:      nil,
		},
		{
			testcase:                 "empty_invalid",
			key:                      Key(""),
			expectUsesRemoteDefaults: true,
			expectIsRemoteDataSource: false,
			expectValidationErr:      errors.New("invalid encryption configuration key:  (must be one of default_remote, backend, planfile, statefile or contain a dot to specify a remote state data source)"),
		},
		{
			testcase:                 "no_dot_invalid",
			key:                      Key("no-dot-contained"),
			expectUsesRemoteDefaults: true,
			expectIsRemoteDataSource: false,
			expectValidationErr:      errors.New("invalid encryption configuration key: no-dot-contained (must be one of default_remote, backend, planfile, statefile or contain a dot to specify a remote state data source)"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			if tc.expectUsesRemoteDefaults != tc.key.UsesRemoteDefaults() {
				t.Errorf("UsesRemoteDefaults returned unexpected value")
			}
			if tc.expectIsRemoteDataSource != tc.key.IsRemoteDataSource() {
				t.Error("IsRemoteDataSource returned unexpected value")
			}
			err := tc.key.Validate()
			expectErr(t, err, tc.expectValidationErr)
		})
	}
}

func TestConfigurationFromEnv(t *testing.T) {
	empty, err := ConfigurationFromEnv("")
	if err != nil {
		t.Fatalf("unexpected error for empty env: %s", err)
	}
	if empty != nil {
		t.Fatalf("unexpected result for empty env: %s", err)
	}
}

const validKey1 = "a0a1a2a3a4a5a6a7a8a9b0b1b2b3b4b5b6b7b8b9c0c1c2c3c4c5c6c7c8c9d0d1"

const tooShortKey = "a0a1a2a3a4a5a6a7a8a9b0b1b2b3b4b5b6b7b8b9c0c1c2c3c4c5c6c7c8c9"
const tooLongKey = "a0a1a2a3a4a5a6a7a8a9b0b1b2b3b4b5b6b7b8b9c0c1c2c3c4c5c6c7c8c9d0d1d2d3d4d5"
const invalidChars = "somethingsomethinga9b0b1b2b3b4b5b6b7b8b9c0c1c2c3c4c5c6c7c8c9d0d1"

func TestKeyProviderConfigValidate(t *testing.T) {
	testCases := []struct {
		testcase    string
		config      KeyProviderConfig
		expectedErr error
	}{
		{
			testcase: "correct",
			config: KeyProviderConfig{
				Name:   "passphrase",
				Config: map[string]string{"passphrase": "quick brown fox"},
			},
			expectedErr: nil,
		},
		{
			testcase: "unknown_name",
			config: KeyProviderConfig{
				Name: "unknown",
			},
			expectedErr: errors.New("error in configuration for key provider unknown (no registered key provider with this name)"),
		},
		// tests for "passphrase" validation
		{
			testcase: "passphrase_no_phrase",
			config: KeyProviderConfig{
				Name: "passphrase",
			},
			expectedErr: errors.New("error in configuration for key provider passphrase (passphrase missing or empty)"),
		},
		{
			testcase: "passphrase_additional",
			config: KeyProviderConfig{
				Name: "passphrase",
				Config: map[string]string{
					"passphrase": "12345",
					"additional": "nonsense",
				},
			},
			expectedErr: errors.New("error in configuration for key provider passphrase (unexpected additional configuration fields, only 'passphrase' is allowed for this key provider)"),
		},
		// tests for "direct" validation
		{
			testcase: "direct_no_key",
			config: KeyProviderConfig{
				Name: "direct",
			},
			expectedErr: errors.New("error in configuration for key provider direct (field 'key' missing or empty)"),
		},
		{
			testcase: "direct_empty_key",
			config: KeyProviderConfig{
				Name: "direct",
				Config: map[string]string{
					"key": "",
				},
			},
			expectedErr: errors.New("error in configuration for key provider direct (field 'key' missing or empty)"),
		},
		{
			testcase: "direct_invalid_key",
			config: KeyProviderConfig{
				Name: "direct",
				Config: map[string]string{
					"key": invalidChars,
				},
			},
			expectedErr: errors.New("error in configuration for key provider direct (field 'key' is not a hex string representing 32 bytes)"),
		},
		{
			testcase: "direct_key_short",
			config: KeyProviderConfig{
				Name: "direct",
				Config: map[string]string{
					"key": tooShortKey,
				},
			},
			expectedErr: errors.New("error in configuration for key provider direct (field 'key' is not a hex string representing 32 bytes)"),
		},
		{
			testcase: "direct_key_long",
			config: KeyProviderConfig{
				Name: "direct",
				Config: map[string]string{
					"key": tooLongKey,
				},
			},
			expectedErr: errors.New("error in configuration for key provider direct (field 'key' is not a hex string representing 32 bytes)"),
		},
		{
			testcase: "direct_additional",
			config: KeyProviderConfig{
				Name: "direct",
				Config: map[string]string{
					"key":        validKey1,
					"additional": "nonsense",
				},
			},
			expectedErr: errors.New("error in configuration for key provider direct (unexpected additional configuration fields, only 'key' is allowed for this key provider)"),
		},
		{
			testcase: "direct_success",
			config: KeyProviderConfig{
				Name: "direct",
				Config: map[string]string{
					"key": validKey1,
				},
			},
			expectedErr: nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			err := tc.config.Validate()
			expectErr(t, err, tc.expectedErr)
		})
	}
}

const invalidJsonConfigSyntax = `{
  "unclosed": {
}`

const invalidJsonConfigUnexpectedKeys = `{
  "unexpected_keys": {
    "tofu": true,
    "fries": false
  }
}`

const validJsonConfig = `{
  "default": { 
    "key_provider": {
	  "name": "awskms",
	  "config": {
	    "region": "us-east-1",
	    "key_id": "abc"
	  }
    },
	"method": {
	  "name": "full"
    },
	"enforced": true
  }
}`

func TestParseJsonStructure(t *testing.T) {
	testCases := []struct {
		testcase    string
		input       string
		outputSize  int
		expectedErr error
	}{
		{
			testcase:    "invalid_syntax",
			input:       invalidJsonConfigSyntax,
			outputSize:  0,
			expectedErr: errors.New("failed to parse encryption configuration, please check if your configuration is correct (not showing error because it may contain sensitive credentials)"),
		},
		{
			testcase:    "unknown_fields",
			input:       invalidJsonConfigUnexpectedKeys,
			outputSize:  0,
			expectedErr: errors.New("failed to parse encryption configuration, please check if your configuration is correct (not showing error because it may contain sensitive credentials)"),
		},
		{
			testcase:    "valid",
			input:       validJsonConfig,
			outputSize:  1,
			expectedErr: nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			actual, err := parseJsonStructure(tc.input)
			expectErr(t, err, tc.expectedErr)
			if len(actual) != tc.outputSize {
				t.Errorf("wrong output size %d instead of %d", len(actual), tc.outputSize)
			}
		})
	}
}

func TestParseEnvJsonStructure(t *testing.T) {
	testCases := []struct {
		testcase    string
		input       string
		outputSize  int
		expectedErr error
	}{
		{
			testcase:    "invalid",
			input:       invalidJsonConfigUnexpectedKeys,
			outputSize:  0,
			expectedErr: errors.New("error parsing environment variable (failed to parse encryption configuration, please check if your configuration is correct (not showing error because it may contain sensitive credentials))"),
		},
		{
			testcase:    "valid",
			input:       validJsonConfig,
			outputSize:  1,
			expectedErr: nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			actual, err := ConfigurationFromEnv(tc.input)
			expectErr(t, err, tc.expectedErr)
			if len(actual) != tc.outputSize {
				t.Errorf("wrong output size %d instead of %d", len(actual), tc.outputSize)
			}
		})
	}
}

func expectErr(t *testing.T, actual error, expected error) {
	t.Helper()
	if actual != nil {
		if expected == nil {
			t.Errorf("received unexpected error:\n%s\nexpected success", actual.Error())
		} else if actual.Error() != expected.Error() {
			t.Errorf("received unexpected error\n%s\nexpected:\n%s", actual.Error(), expected.Error())
		}
	} else {
		if expected != nil {
			t.Errorf("unexpected success instead of expected error '%s'", expected.Error())
		}
	}
}
