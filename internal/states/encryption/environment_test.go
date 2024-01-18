package encryption

import (
	"errors"
	"fmt"
	"testing"

	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionflow"
)

func TestParseEnvironmentVariables(t *testing.T) {
	configKey := "unit_testing.parse_environment_variables"

	testCases := []struct {
		testcase    string
		encEnv      string
		decEnv      string
		expectError error
	}{
		{
			testcase: "no_configuration",
		},
		// parse failures
		{
			testcase: "syntactically_invalid_enc",
			encEnv:   `{`,
			decEnv:   envConfig(configKey, true),
			expectError: fmt.Errorf(
				"error parsing encryption configuration from environment variable %s: "+
					"json parse error, wrong structure, or unknown fields - "+
					"details omitted for security reasons (may contain key related settings)",
				encryptionconfig.ConfigEnvName,
			),
		},
		{
			testcase: "syntactically_invalid_dec",
			encEnv:   envConfig(configKey, true),
			decEnv:   `{not_a_json}}}}}}`,
			expectError: fmt.Errorf(
				"error parsing fallback decryption configuration from environment variable %s: "+
					"json parse error, wrong structure, or unknown fields - "+
					"details omitted for security reasons (may contain key related settings)",
				encryptionconfig.FallbackConfigEnvName,
			),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			if tc.encEnv != "" {
				t.Setenv(encryptionconfig.ConfigEnvName, tc.encEnv)
			}
			if tc.decEnv != "" {
				t.Setenv(encryptionconfig.FallbackConfigEnvName, tc.decEnv)
			}

			err := ParseEnvironmentVariables()
			expectErr(t, err, tc.expectError)
		})
	}
}

type tstAlwaysFailingFlowBuilder struct{}

var alwaysFailError = errors.New("always fails")

func (t *tstAlwaysFailingFlowBuilder) EncryptionConfiguration(_ encryptionflow.ConfigurationSource, _ encryptionconfig.Config) error {
	return alwaysFailError
}

func (t *tstAlwaysFailingFlowBuilder) DecryptionFallbackConfiguration(_ encryptionflow.ConfigurationSource, _ encryptionconfig.Config) error {
	return alwaysFailError
}

func (t *tstAlwaysFailingFlowBuilder) Build() (encryptionflow.Flow, error) {
	return nil, alwaysFailError
}

func TestApplyEncryptionConfigIfExists_ApplyError(t *testing.T) {
	configKey := "unit_testing.apply_encryption_config_if_exists"
	t.Setenv(encryptionconfig.ConfigEnvName, envConfig(configKey, true))

	failFlow := &tstAlwaysFailingFlowBuilder{}
	err := applyEncryptionConfigIfExists(failFlow, encryptionflow.ConfigurationSourceEnv, configKey)
	expectErr(t, err, alwaysFailError)
}

func TestApplyDecryptionFallbackConfigIfExists_ApplyError(t *testing.T) {
	configKey := "unit_testing.apply_decryption_fallback_config_if_exists"
	t.Setenv(encryptionconfig.FallbackConfigEnvName, envConfig(configKey, true))

	failFlow := &tstAlwaysFailingFlowBuilder{}
	err := applyDecryptionFallbackConfigIfExists(failFlow, encryptionflow.ConfigurationSourceEnv, configKey)
	expectErr(t, err, alwaysFailError)
}
