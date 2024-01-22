package encryption

import (
	"errors"
	"fmt"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
	"testing"
)

func envConfig(configKey encryptionconfig.Key, logicallyValid bool) string {
	if logicallyValid {
		return fmt.Sprintf(`{"%s":{"key_provider":{"config":{"passphrase":"somephrase"}}}}`, configKey)
	} else {
		return fmt.Sprintf(`{"%s":{}}`, configKey)
	}
}

func TestEncryption_ApplyEnvConfigurations(t *testing.T) {
	configKey := encryptionconfig.Key("unit_testing.apply_env_configurations")

	testCases := []struct {
		testcase            string
		encEnv              string
		decEnv              string
		expectEnvParseError error
		expectDecParseError error
		expectError         error
	}{
		{
			testcase: "no_configuration",
		},
		// parse failures - so we can ensure user gets sensible error messages
		{
			testcase: "syntactically_invalid_enc",
			encEnv:   `{`,
			decEnv:   envConfig(configKey, true),
			expectEnvParseError: errors.New(
				"error parsing environment variable (" +
					"failed to parse encryption configuration, please check if your configuration is correct " +
					"(not showing error because it may contain sensitive credentials))",
			),
		},
		{
			testcase: "syntactically_invalid_dec",
			encEnv:   envConfig(configKey, true),
			decEnv:   `{not_a_json}}}}}}`,
			expectDecParseError: errors.New(
				"error parsing environment variable (" +
					"failed to parse encryption configuration, please check if your configuration is correct " +
					"(not showing error because it may contain sensitive credentials))",
			),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			singleton := GetSingleton()
			defer ClearSingleton()

			encryptionConfigsFromEnv, err := encryptionconfig.ConfigurationFromEnv(tc.encEnv)
			expectErr(t, err, tc.expectEnvParseError)
			if err != nil {
				return
			}

			decryptionFallbackConfiggFromEnv, err := encryptionconfig.ConfigurationFromEnv(tc.decEnv)
			expectErr(t, err, tc.expectDecParseError)
			if err != nil {
				return
			}

			err = singleton.ApplyEnvConfigurations(encryptionConfigsFromEnv, decryptionFallbackConfiggFromEnv)
			expectErr(t, err, tc.expectError)
		})
	}
}

func expectErr(t *testing.T, actual error, expected error) {
	t.Helper()
	if actual != nil {
		if expected == nil {
			t.Errorf("received unexpected error:\n%s\nexpected: success", actual.Error())
		} else if actual.Error() != expected.Error() {
			t.Errorf("received unexpected error:\n%s\nexpected:\n%s", actual.Error(), expected.Error())
		}
	} else {
		if expected != nil {
			t.Errorf("unexpected success instead of expected error:\n%s", expected.Error())
		}
	}
}
