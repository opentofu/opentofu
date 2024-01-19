package encryptionconfig

import (
	"errors"
	"fmt"
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
			expectedErr: errors.New("error in configuration for key provider unknown: no registered key provider with this name"),
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

func TestEncryptionConfigurationsFromEnv(t *testing.T) {
	empty, err := ConfigurationFromEnv(ConfigEnvName)
	if err != nil {
		t.Fatalf("unexpected error for empty env: %s", err)
	}
	if empty != nil {
		t.Fatalf("unexpected result for empty env: %s", err)
	}
}

// TODO missing the happy path test cases here.
func TestFallbackConfigurationsFromEnv(t *testing.T) {
	empty, err := ConfigurationFromEnv(FallbackConfigEnvName)
	if err != nil {
		t.Fatalf("unexpected error for empty env fallback: %s", err)
	}
	if empty != nil {
		t.Fatalf("unexpected result for empty env fallback: %s", err)
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
			expectedErr: errors.New("error in configuration for key provider unknown: no registered key provider with this name"),
		},
		// tests for "passphrase" validation
		{
			testcase: "passphrase_no_phrase",
			config: KeyProviderConfig{
				Name: "passphrase",
			},
			expectedErr: errors.New("error in configuration for key provider passphrase: passphrase missing or empty"),
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
			expectedErr: errors.New("error in configuration for key provider passphrase: unexpected additional configuration fields, only 'passphrase' is allowed for this key provider"),
		},
		// tests for "direct" validation
		{
			testcase: "direct_no_key",
			config: KeyProviderConfig{
				Name: "direct",
			},
			expectedErr: errors.New("error in configuration for key provider direct: field 'key' missing or empty"),
		},
		{
			testcase: "direct_empty_key",
			config: KeyProviderConfig{
				Name: "direct",
				Config: map[string]string{
					"key": "",
				},
			},
			expectedErr: errors.New("error in configuration for key provider direct: field 'key' missing or empty"),
		},
		{
			testcase: "direct_invalid_key",
			config: KeyProviderConfig{
				Name: "direct",
				Config: map[string]string{
					"key": invalidChars,
				},
			},
			expectedErr: errors.New("error in configuration for key provider direct: field 'key' is not a hex string representing 32 bytes"),
		},
		{
			testcase: "direct_key_short",
			config: KeyProviderConfig{
				Name: "direct",
				Config: map[string]string{
					"key": tooShortKey,
				},
			},
			expectedErr: errors.New("error in configuration for key provider direct: field 'key' is not a hex string representing 32 bytes"),
		},
		{
			testcase: "direct_key_long",
			config: KeyProviderConfig{
				Name: "direct",
				Config: map[string]string{
					"key": tooLongKey,
				},
			},
			expectedErr: errors.New("error in configuration for key provider direct: field 'key' is not a hex string representing 32 bytes"),
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
			expectedErr: errors.New("error in configuration for key provider direct: unexpected additional configuration fields, only 'key' is allowed for this key provider"),
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
			expectedErr: errors.New("error parsing environment variable STATE_ENCRYPTION_TESTCASE_TestParseEnvJsonStructure_invalid (failed to parse encryption configuration, please check if your configuration is correct (not showing error because it may contain sensitive credentials))"),
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
			envName := fmt.Sprintf("STATE_ENCRYPTION_TESTCASE_TestParseEnvJsonStructure_%s", tc.testcase)
			t.Setenv(envName, tc.input)

			actual, err := ConfigurationFromEnv(envName)
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
			t.Errorf("received unexpected error '%s' instead of success", actual.Error())
		} else if actual.Error() != expected.Error() {
			t.Errorf("received unexpected error '%s' instead of '%s'", actual.Error(), expected.Error())
		}
	} else {
		if expected != nil {
			t.Errorf("unexpected success instead of expected error '%s'", expected.Error())
		}
	}
}
