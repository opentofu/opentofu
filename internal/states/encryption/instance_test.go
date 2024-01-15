package encryption

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
)

func TestInstanceEnforcesDotInKey(t *testing.T) {
	defer expectPanic(t, "call to encryption.Instance with a key that does not contain '.'. This is a bug.")()
	_, _ = Instance("no_dot")
}

func envConfig(configKey string, logicallyValid bool) string {
	if logicallyValid {
		return fmt.Sprintf(`{"%s":{"key_provider":{"config":{"passphrase":"somephrase"}}}}`, configKey)
	} else {
		return fmt.Sprintf(`{"%s":{}}`, configKey)
	}
}

type instanceTestCase struct {
	testcase              string
	key                   string
	encEnv                string
	decEnv                string
	expectInstanceError   error
	expectValidationError error
}

func instanceTestCases(configKey string, canExtendConfigKey bool) []instanceTestCase {
	key := func(base string, num int) string {
		if canExtendConfigKey {
			return fmt.Sprintf("%s[%d]", base, num)
		} else {
			return base
		}
	}

	return []instanceTestCase{
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
			encEnv:   envConfig("default", true),
			decEnv:   envConfig("default", true),
		},
		// validation error cases
		{
			testcase: "logically_invalid_enc",
			key:      key(configKey, 4),
			encEnv:   envConfig(key(configKey, 4), false),
			decEnv:   envConfig(key(configKey, 4), true),
			expectValidationError: errors.New(
				"error invalid encryption configuration after merge: " +
					"error in configuration for key provider passphrase: passphrase missing or empty",
			),
		},
		{
			testcase: "logically_invalid_dec",
			key:      key(configKey, 5),
			encEnv:   envConfig(key(configKey, 5), true),
			decEnv:   envConfig(key(configKey, 5), false),
			expectValidationError: errors.New(
				"error invalid decryption fallback configuration after merge: " +
					"error in configuration for key provider passphrase: passphrase missing or empty",
			),
		},
		// instance creation error cases (parse failure)
		{
			testcase: "syntactically_invalid_enc",
			key:      key(configKey, 6),
			encEnv:   `{`,
			decEnv:   envConfig(key(configKey, 6), true),
			expectInstanceError: fmt.Errorf(
				"error parsing encryption configuration from environment variable %s: "+
					"json parse error, wrong structure, or unknown fields - "+
					"details omitted for security reasons (may contain key related settings)",
				encryptionconfig.ConfigEnvName,
			),
		},
		{
			testcase: "syntactically_invalid_dec",
			key:      key(configKey, 7),
			encEnv:   envConfig(key(configKey, 7), true),
			decEnv:   `{not_a_json}}}}}}`,
			expectInstanceError: fmt.Errorf(
				"error parsing fallback decryption configuration from environment variable %s: "+
					"json parse error, wrong structure, or unknown fields - "+
					"details omitted for security reasons (may contain key related settings)",
				encryptionconfig.FallbackConfigEnvName,
			),
		},
	}
}

func TestInstance_NoCache(t *testing.T) {
	testCases := instanceTestCases("unit_testing.instance_no_cache", true)

	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			if cache != nil {
				t.Fatal("cache was enabled at start of test - probably some other test forgot to defer DisableCache()")
			}

			if tc.encEnv != "" {
				t.Setenv(encryptionconfig.ConfigEnvName, tc.encEnv)
			}
			if tc.decEnv != "" {
				t.Setenv(encryptionconfig.FallbackConfigEnvName, tc.decEnv)
			}

			instance, err := Instance(tc.key)
			expectErr(t, err, tc.expectInstanceError)
			if err == nil {
				if instance == nil {
					t.Fatal("instance was unexpectedly nil despite no error")
				}

				err := instance.MergeAndValidateConfigurations()
				expectErr(t, err, tc.expectValidationError)
			}
		})
	}
}

func TestInstance_Cache(t *testing.T) {
	testCases := instanceTestCases("unit_testing.instance_cache", true)

	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			EnableCaching()
			defer DisableCaching()

			if tc.encEnv != "" {
				t.Setenv(encryptionconfig.ConfigEnvName, tc.encEnv)
			}
			if tc.decEnv != "" {
				t.Setenv(encryptionconfig.FallbackConfigEnvName, tc.decEnv)
			}

			instance, err := Instance(tc.key)
			expectErr(t, err, tc.expectInstanceError)
			if err == nil {
				if instance == nil {
					t.Fatal("instance was unexpectedly nil despite no error")
				}

				err := instance.MergeAndValidateConfigurations()
				expectErr(t, err, tc.expectValidationError)
			}
		})
	}
}

func TestRemoteStateInstance_NoCache(t *testing.T) {
	testCases := instanceTestCases("backend", false)

	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			if tc.encEnv != "" {
				t.Setenv(encryptionconfig.ConfigEnvName, tc.encEnv)
			}
			if tc.decEnv != "" {
				t.Setenv(encryptionconfig.FallbackConfigEnvName, tc.decEnv)
			}

			instance, err := RemoteStateInstance()
			expectErr(t, err, tc.expectInstanceError)
			if err == nil {
				if instance == nil {
					t.Fatal("instance was unexpectedly nil despite no error")
				}
			}
		})
	}
}

func TestRemoteStateInstance_Cache(t *testing.T) {
	testCases := instanceTestCases("backend", false)

	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			EnableCaching()
			defer DisableCaching()

			if tc.encEnv != "" {
				t.Setenv(encryptionconfig.ConfigEnvName, tc.encEnv)
			}
			if tc.decEnv != "" {
				t.Setenv(encryptionconfig.FallbackConfigEnvName, tc.decEnv)
			}

			instance, err := RemoteStateInstance()
			expectErr(t, err, tc.expectInstanceError)
			if err == nil {
				if instance == nil {
					t.Fatal("instance was unexpectedly nil despite no error")
				}
			}
		})
	}
}

func TestStatefileInstance_NoCache(t *testing.T) {
	testCases := instanceTestCases("statefile", false)

	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			if tc.encEnv != "" {
				t.Setenv(encryptionconfig.ConfigEnvName, tc.encEnv)
			}
			if tc.decEnv != "" {
				t.Setenv(encryptionconfig.FallbackConfigEnvName, tc.decEnv)
			}

			instance, err := StatefileInstance()
			expectErr(t, err, tc.expectInstanceError)
			if err == nil {
				if instance == nil {
					t.Fatal("instance was unexpectedly nil despite no error")
				}
			}
		})
	}
}

func TestStatefileInstance_Cache(t *testing.T) {
	testCases := instanceTestCases("statefile", false)

	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			EnableCaching()
			defer DisableCaching()

			if tc.encEnv != "" {
				t.Setenv(encryptionconfig.ConfigEnvName, tc.encEnv)
			}
			if tc.decEnv != "" {
				t.Setenv(encryptionconfig.FallbackConfigEnvName, tc.decEnv)
			}

			instance, err := StatefileInstance()
			expectErr(t, err, tc.expectInstanceError)
			if err == nil {
				if instance == nil {
					t.Fatal("instance was unexpectedly nil despite no error")
				}
			}
		})
	}
}

func TestPlanfileInstance_NoCache(t *testing.T) {
	testCases := instanceTestCases("planfile", false)

	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			if tc.encEnv != "" {
				t.Setenv(encryptionconfig.ConfigEnvName, tc.encEnv)
			}
			if tc.decEnv != "" {
				t.Setenv(encryptionconfig.FallbackConfigEnvName, tc.decEnv)
			}

			instance, err := PlanfileInstance()
			expectErr(t, err, tc.expectInstanceError)
			if err == nil {
				if instance == nil {
					t.Fatal("instance was unexpectedly nil despite no error")
				}
			}
		})
	}
}

func TestPlanfileInstance_Cache(t *testing.T) {
	testCases := instanceTestCases("planfile", false)

	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			EnableCaching()
			defer DisableCaching()

			if tc.encEnv != "" {
				t.Setenv(encryptionconfig.ConfigEnvName, tc.encEnv)
			}
			if tc.decEnv != "" {
				t.Setenv(encryptionconfig.FallbackConfigEnvName, tc.decEnv)
			}

			instance, err := PlanfileInstance()
			expectErr(t, err, tc.expectInstanceError)
			if err == nil {
				if instance == nil {
					t.Fatal("instance was unexpectedly nil despite no error")
				}
			}
		})
	}
}

func expectPanic(t *testing.T, snippet string) func() {
	return func() {
		r := recover()
		if r == nil {
			t.Errorf("expected a panic")
		} else {
			actual := fmt.Sprintf("%v", r)
			if !strings.Contains(actual, snippet) {
				t.Errorf("panic message did not contain '%s'", snippet)
			}
		}
	}
}

func expectErr(t *testing.T, actual error, expected error) {
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
