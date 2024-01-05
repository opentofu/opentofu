package encryptionconfig

import (
	"errors"
	"fmt"
	"os"
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
				Method: EncryptionMethodConfig{
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
				Method: EncryptionMethodConfig{
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
				Method: EncryptionMethodConfig{
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
	empty, err := EncryptionConfigurationsFromEnv()
	if err != nil {
		t.Fatalf("unexpected error for empty env: %s", err)
	}
	if empty != nil {
		t.Fatalf("unexpected result for empty env: %s", err)
	}
}

func TestFallbackConfigurationsFromEnv(t *testing.T) {
	empty, err := FallbackConfigurationsFromEnv()
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
		nameInvalid bool
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
			nameInvalid: true,
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

			err = tc.config.NameValid()
			if tc.nameInvalid {
				expectErr(t, err, tc.expectedErr)
			} else {
				expectErr(t, err, nil)
			}
		})
	}
}

func TestEncryptionMethodConfigValidate(t *testing.T) {
	testCases := []struct {
		testcase    string
		config      EncryptionMethodConfig
		nameInvalid bool
		expectedErr error
	}{
		{
			testcase: "correct",
			config: EncryptionMethodConfig{
				Name: "full",
			},
			expectedErr: nil,
		},
		{
			testcase: "unknown_name",
			config: EncryptionMethodConfig{
				Name: "unknown",
			},
			nameInvalid: true,
			expectedErr: errors.New("error in configuration for encryption method unknown: no registered encryption method with this name"),
		},
		{
			testcase: "incorrect",
			config: EncryptionMethodConfig{
				Name:   "full",
				Config: map[string]string{"unexpected": "quick brown fox"},
			},
			expectedErr: errors.New("error in configuration for encryption method full: unexpected fields, this method needs no configuration"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			err := tc.config.Validate()
			expectErr(t, err, tc.expectedErr)

			err = tc.config.NameValid()
			if tc.nameInvalid {
				expectErr(t, err, tc.expectedErr)
			} else {
				expectErr(t, err, nil)
			}
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
	"required": true
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
			expectedErr: errors.New("json parse error, wrong structure, or unknown fields - details omitted for security reasons (may contain key related settings)"),
		},
		{
			testcase:    "unknown_fields",
			input:       invalidJsonConfigUnexpectedKeys,
			outputSize:  0,
			expectedErr: errors.New("json parse error, wrong structure, or unknown fields - details omitted for security reasons (may contain key related settings)"),
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
			expectedErr: errors.New("error parsing what from environment variable STATE_ENCRYPTION_TESTCASE_TestParseEnvJsonStructure_invalid: json parse error, wrong structure, or unknown fields - details omitted for security reasons (may contain key related settings)"),
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
			os.Setenv(envName, tc.input)
			defer os.Unsetenv(envName)

			actual, err := parseEnvJsonStructure(envName, "what")
			expectErr(t, err, tc.expectedErr)
			if len(actual) != tc.outputSize {
				t.Errorf("wrong output size %d instead of %d", len(actual), tc.outputSize)
			}
		})
	}
}

func expectErr(t *testing.T, actual error, expected error) {
	if actual != nil {
		if actual.Error() != expected.Error() {
			t.Errorf("received unexpected error '%s' instead of '%s'", actual.Error(), expected.Error())
		}
	} else {
		if expected != nil {
			t.Errorf("unexpected success instead of expected error '%s'", expected.Error())
		}
	}
}
