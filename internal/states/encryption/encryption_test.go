package encryption

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/opentofu/opentofu/internal/states/encryption/encryptionflow"
	"github.com/opentofu/opentofu/internal/tfdiags"

	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
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
		expectEncParseError error
		expectDecParseError error
		expectError         error
		expectEncState      encryptionconfig.ConfigMap
		expectDecState      encryptionconfig.ConfigMap
	}{
		{
			testcase:       "no_configuration",
			expectEncState: map[encryptionconfig.Meta]encryptionconfig.Config{},
			expectDecState: map[encryptionconfig.Meta]encryptionconfig.Config{},
		},
		// parse failures - so we can ensure user gets sensible error messages
		{
			testcase: "syntactically_invalid_enc",
			encEnv:   `{`,
			decEnv:   envConfig(configKey, true),
			expectEncParseError: errors.New(
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
		// invalid keys
		{
			testcase: "key_invalid_enc",
			encEnv:   envConfig("invalid", true),
			decEnv:   envConfig(configKey, true),
			expectError: errors.New(
				"failed to parse encryption configuration from environment (" +
					"invalid encryption configuration key: invalid " +
					"(must be one of default_remote, backend, planfile, statefile or " +
					"contain a dot to specify a remote state data source))",
			),
		},
		{
			testcase: "key_invalid_dec",
			encEnv:   envConfig(configKey, true),
			decEnv:   envConfig("invalid", true),
			expectError: errors.New(
				"failed to parse decryption fallback configuration from environment (" +
					"invalid encryption configuration key: invalid " +
					"(must be one of default_remote, backend, planfile, statefile or " +
					"contain a dot to specify a remote state data source))",
			),
		},
		// all valid - logical validity cannot be determined here, that's the job of Validate()
		{
			testcase: "valid",
			encEnv:   envConfig(configKey, false),
			decEnv:   envConfig(configKey, true),
			expectEncState: map[encryptionconfig.Meta]encryptionconfig.Config{
				encryptionconfig.Meta{encryptionconfig.SourceEnv, configKey}: {},
			},
			expectDecState: map[encryptionconfig.Meta]encryptionconfig.Config{
				encryptionconfig.Meta{encryptionconfig.SourceEnv, configKey}: {
					KeyProvider: encryptionconfig.KeyProviderConfig{
						Config: map[string]string{
							"passphrase": "somephrase",
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			t.Cleanup(ClearSingleton)
			singleton := GetSingleton()

			encryptionConfigsFromEnv, err := encryptionconfig.ConfigurationFromEnv(tc.encEnv)
			expectErr(t, err, tc.expectEncParseError)
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
			if err != nil {
				return
			}

			if !reflect.DeepEqual(tc.expectEncState, singleton.(*encryption).encryptionConfigs) {
				t.Error("unexpected encryption config state after ApplyEnvConfigurations()")
			}
			if !reflect.DeepEqual(tc.expectDecState, singleton.(*encryption).decryptionFallbackConfigs) {
				t.Error("unexpected decryption fallback config state after ApplyEnvConfigurations()")
			}
		})
	}
}

func tstCodeConfiguration(valid bool) encryptionconfig.Config {
	config := encryptionconfig.Config{
		KeyProvider: encryptionconfig.KeyProviderConfig{
			Config: map[string]string{},
		},
		Method: encryptionconfig.MethodConfig{},
	}
	if valid {
		config.KeyProvider.Config["passphrase"] = "a new passphrase"
	}
	return config
}

func TestEncryption_ApplyHCLEncryptionConfiguration(t *testing.T) {
	configKey := encryptionconfig.Key("unit_testing.apply_hcl_encryption_configuration")

	testCases := []struct {
		testcase    string
		key         encryptionconfig.Key
		config      encryptionconfig.Config
		expectError error
		expectState encryptionconfig.ConfigMap
	}{
		{
			testcase: "key_invalid",
			key:      encryptionconfig.Key("invalid"),
			config:   tstCodeConfiguration(true),
			expectError: errors.New(
				"failed to parse encryption configuration from HCL (" +
					"invalid encryption configuration key: invalid " +
					"(must be one of default_remote, backend, planfile, statefile or " +
					"contain a dot to specify a remote state data source))",
			),
		},
		{
			testcase:    "valid",
			key:         configKey,
			config:      tstCodeConfiguration(true),
			expectError: nil,
			expectState: map[encryptionconfig.Meta]encryptionconfig.Config{
				encryptionconfig.Meta{encryptionconfig.SourceHCL, configKey}: {
					KeyProvider: encryptionconfig.KeyProviderConfig{
						Config: map[string]string{
							"passphrase": "a new passphrase",
						},
					},
					Method: encryptionconfig.MethodConfig{},
				},
			},
		},
		// logical validity cannot be discovered here, that's the job of Validate()
		{
			testcase:    "valid_with_logical_errors",
			key:         configKey,
			config:      tstCodeConfiguration(false),
			expectError: nil,
			expectState: map[encryptionconfig.Meta]encryptionconfig.Config{
				encryptionconfig.Meta{encryptionconfig.SourceHCL, configKey}: {
					KeyProvider: encryptionconfig.KeyProviderConfig{
						Config: map[string]string{},
					},
					Method: encryptionconfig.MethodConfig{},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			t.Cleanup(ClearSingleton)
			singleton := GetSingleton()

			err := singleton.ApplyHCLEncryptionConfiguration(tc.key, tc.config)
			expectErr(t, err, tc.expectError)
			if err != nil {
				return
			}

			if !reflect.DeepEqual(tc.expectState, singleton.(*encryption).encryptionConfigs) {
				t.Error("unexpected encryption config state after ApplyHCLEncryptionConfiguration()")
			}
		})
	}
}

func TestEncryption_ApplyHCLDecryptionFallbackConfiguration(t *testing.T) {
	configKey := encryptionconfig.Key("unit_testing.apply_hcl_decryption_fallback_configuration")

	testCases := []struct {
		testcase    string
		key         encryptionconfig.Key
		config      encryptionconfig.Config
		expectError error
		expectState encryptionconfig.ConfigMap
	}{
		{
			testcase: "key_invalid",
			key:      encryptionconfig.Key("invalid"),
			config:   tstCodeConfiguration(true),
			expectError: errors.New(
				"failed to parse decryption fallback configuration from HCL (" +
					"invalid encryption configuration key: invalid " +
					"(must be one of default_remote, backend, planfile, statefile or " +
					"contain a dot to specify a remote state data source))",
			),
		},
		{
			testcase:    "valid",
			key:         configKey,
			config:      tstCodeConfiguration(true),
			expectError: nil,
			expectState: map[encryptionconfig.Meta]encryptionconfig.Config{
				encryptionconfig.Meta{encryptionconfig.SourceHCL, configKey}: {
					KeyProvider: encryptionconfig.KeyProviderConfig{
						Config: map[string]string{
							"passphrase": "a new passphrase",
						},
					},
					Method: encryptionconfig.MethodConfig{},
				},
			},
		},
		// logical validity cannot be discovered here, that's the job of Validate()
		{
			testcase:    "valid_with_logical_errors",
			key:         configKey,
			config:      tstCodeConfiguration(false),
			expectError: nil,
			expectState: map[encryptionconfig.Meta]encryptionconfig.Config{
				encryptionconfig.Meta{encryptionconfig.SourceHCL, configKey}: {
					KeyProvider: encryptionconfig.KeyProviderConfig{
						Config: map[string]string{},
					},
					Method: encryptionconfig.MethodConfig{},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			t.Cleanup(ClearSingleton)
			singleton := GetSingleton()

			err := singleton.ApplyHCLDecryptionFallbackConfiguration(tc.key, tc.config)
			expectErr(t, err, tc.expectError)
			if err != nil {
				return
			}

			if !reflect.DeepEqual(tc.expectState, singleton.(*encryption).decryptionFallbackConfigs) {
				t.Error("unexpected decryption config state after ApplyHCLDecryptionFallbackConfiguration()")
			}
		})
	}
}

func TestEncryption_Validate(t *testing.T) {
	configKey := encryptionconfig.Key("unit_testing.validate")

	testCases := []struct {
		testcase           string
		encEnv             string
		decEnv             string
		expectErrorKey     encryptionconfig.Key
		expectErrorMessage string
	}{
		{
			testcase: "no_configuration",
		},
		{
			testcase: "valid",
			encEnv:   envConfig(configKey, true),
			decEnv:   envConfig(configKey, true),
		},
		{
			testcase:       "invalid_enc_config",
			encEnv:         envConfig(configKey, false),
			decEnv:         envConfig(encryptionconfig.KeyBackend, true),
			expectErrorKey: configKey,
			expectErrorMessage: "failed to merge encryption configuration (invalid configuration after merge " +
				"(error in configuration for key provider passphrase (passphrase missing or empty)))",
		},
		{
			testcase:       "invalid_dec_config",
			encEnv:         envConfig(configKey, true),
			decEnv:         envConfig(encryptionconfig.KeyBackend, false),
			expectErrorKey: encryptionconfig.KeyBackend,
			expectErrorMessage: "failed to merge fallback configuration (invalid configuration after merge " +
				"(error in configuration for key provider passphrase (passphrase missing or empty)))",
		},
		// corner case - check that errors in the default configuration are detected because the
		// "backend" configuration is built
		{
			testcase:       "invalid_default",
			encEnv:         envConfig(encryptionconfig.KeyDefaultRemote, false),
			decEnv:         "",
			expectErrorKey: encryptionconfig.KeyBackend, // NOT default!
			expectErrorMessage: "failed to merge encryption configuration (invalid configuration after merge " +
				"(error in configuration for key provider passphrase (passphrase missing or empty)))",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			t.Cleanup(ClearSingleton)
			singleton := GetSingleton()

			encryptionConfigsFromEnv, err := encryptionconfig.ConfigurationFromEnv(tc.encEnv)
			expectErr(t, err, nil)
			if err != nil {
				return
			}

			decryptionFallbackConfiggFromEnv, err := encryptionconfig.ConfigurationFromEnv(tc.decEnv)
			expectErr(t, err, nil)
			if err != nil {
				return
			}

			err = singleton.ApplyEnvConfigurations(encryptionConfigsFromEnv, decryptionFallbackConfiggFromEnv)
			expectErr(t, err, nil)
			if err != nil {
				return
			}

			diags := singleton.Validate()
			for i := range diags {
				if i > 0 {
					t.Errorf("unexpected extra diag %s", diags[i].Description())
				} else {
					expectDiag(t, diags[i], tfdiags.Error,
						fmt.Sprintf("Invalid state encryption configuration for configuration key %s", tc.expectErrorKey),
						tc.expectErrorMessage,
					)
				}
			}
		})
	}
}

type buildMethodTestCase struct {
	testcase    string
	key         encryptionconfig.Key
	encEnv      string
	decEnv      string
	expectError error
}

func getEncryptionBuildMethodsTestCases(configKey string, canExtendConfigKey bool) []buildMethodTestCase {
	key := func(base string, num int) encryptionconfig.Key {
		if canExtendConfigKey {
			return encryptionconfig.Key(fmt.Sprintf("%s[%d]", base, num))
		} else {
			return encryptionconfig.Key(base)
		}
	}

	return []buildMethodTestCase{
		// success cases
		{
			testcase: "no_configuration",
			key:      key(configKey, 1),
		},
		{
			testcase: "full_configuration",
			key:      key(configKey, 2),
			encEnv:   envConfig(key(configKey, 2), true),
			decEnv:   envConfig(key(configKey, 2), true),
		},
		{
			testcase: "all_defaults",
			key:      key(configKey, 3),
			encEnv:   envConfig("default_remote", true),
			decEnv:   envConfig("default_remote", true),
		},
		// validation error cases (the flow builder methods also validate again)
		{
			testcase: "logically_invalid_enc",
			key:      key(configKey, 4),
			encEnv:   envConfig(key(configKey, 4), false),
			decEnv:   envConfig(key(configKey, 4), true),
			expectError: errors.New(
				"failed to merge encryption configuration " +
					"(invalid configuration after merge " +
					"(error in configuration for key provider passphrase (passphrase missing or empty)))",
			),
		},
		{
			testcase: "logically_invalid_dec",
			key:      key(configKey, 5),
			encEnv:   envConfig(key(configKey, 5), true),
			decEnv:   envConfig(key(configKey, 5), false),
			expectError: errors.New(
				"failed to merge fallback configuration " +
					"(invalid configuration after merge " +
					"(error in configuration for key provider passphrase (passphrase missing or empty)))",
			),
		},
	}
}

func TestEncryption_RemoteState(t *testing.T) {
	testCases := getEncryptionBuildMethodsTestCases("backend", false)

	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			runEncryptionBuilderTestcase(t, tc, func(singleton Encryption) (encryptionflow.Flow, error) {
				flow, err := singleton.RemoteState()
				if flow == nil {
					return nil, err
				}
				return flow.(encryptionflow.Flow), err
			})
		})
	}
}

func TestEncryption_StateFile(t *testing.T) {
	testCases := getEncryptionBuildMethodsTestCases("statefile", false)

	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			runEncryptionBuilderTestcase(t, tc, func(singleton Encryption) (encryptionflow.Flow, error) {
				flow, err := singleton.StateFile()
				if flow == nil {
					return nil, err
				}
				return flow.(encryptionflow.Flow), err
			})
		})
	}
}

func TestEncryption_PlanFile(t *testing.T) {
	testCases := getEncryptionBuildMethodsTestCases("planfile", false)

	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			runEncryptionBuilderTestcase(t, tc, func(singleton Encryption) (encryptionflow.Flow, error) {
				flow, err := singleton.PlanFile()
				if flow == nil {
					return nil, err
				}
				return flow.(encryptionflow.Flow), err
			})
		})
	}

}

func TestEncryption_RemoteStateDatasource(t *testing.T) {
	testCases := getEncryptionBuildMethodsTestCases("unit_testing.remote_state_data_source", true)

	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			runEncryptionBuilderTestcase(t, tc, func(singleton Encryption) (encryptionflow.Flow, error) {
				flow, err := singleton.RemoteStateDatasource(tc.key)
				if flow == nil {
					return nil, err
				}
				return flow.(encryptionflow.Flow), err
			})
		})
	}

}

func TestEncryption_RemoteStateDatasource_NotRemote(t *testing.T) {
	tc := buildMethodTestCase{
		key:         "no-dot-in-key",
		expectError: errors.New("the specified configuration key is not a valid remote data source key (this is likely a bug, did you want to call RemoteState(), StateFile(), or PlanFile()?)"),
	}
	runEncryptionBuilderTestcase(t, tc, func(singleton Encryption) (encryptionflow.Flow, error) {
		_, err := singleton.RemoteStateDatasource(tc.key)
		return nil, err
	})
}

func runEncryptionBuilderTestcase(t *testing.T, tc buildMethodTestCase, functionUnderTest func(Encryption) (encryptionflow.Flow, error)) {
	t.Cleanup(ClearSingleton)
	singleton := GetSingleton()

	enc, err := encryptionconfig.ConfigurationFromEnv(tc.encEnv)
	if err != nil {
		t.Fatalf("error in testcase definition, the encryption config failed to parse: %s", err.Error())
	}
	dec, err := encryptionconfig.ConfigurationFromEnv(tc.decEnv)
	if err != nil {
		t.Fatalf("error in testcase definition, the decryption fallback config failed to parse: %s", err.Error())
	}
	err = singleton.ApplyEnvConfigurations(enc, dec)
	if err != nil {
		t.Fatalf("error in testcase definition, the env configs failed to apply: %s", err.Error())
	}

	flow, err := functionUnderTest(singleton)
	expectErr(t, err, tc.expectError)
	if err == nil {
		if flow == nil {
			t.Fatal("flow was unexpectedly nil despite no error")
		}
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

func expectDiag(t *testing.T, actual tfdiags.Diagnostic, expectSeverity tfdiags.Severity, expectSummary string, expectDetail string) {
	t.Helper()
	if actual == nil {
		t.Error("unexpected nil diag")
	} else {
		if expectSeverity != actual.Severity() {
			t.Error("unexpected severity")
		}
		if expectSummary != actual.Description().Summary || expectDetail != actual.Description().Detail {
			t.Errorf("unexpected:\n%s\n%s\nexpected:\n%s\n%s",
				actual.Description().Summary, actual.Description().Detail,
				expectSummary, expectDetail)
		}
	}
}
