package encryptionconfig

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestInjectDefaultNamesIfUnset(t *testing.T) {
	testCases := []struct {
		testcase   string
		config     *Config
		expectNil  bool
		expectedKP string
		expectedM  string
	}{
		{
			testcase:  "no_configuration",
			config:    nil,
			expectNil: true,
		},
		{
			testcase:   "all_defaults",
			config:     &Config{},
			expectNil:  false,
			expectedKP: "passphrase",
			expectedM:  "full",
		},
		{
			testcase: "key_provider_default",
			config: &Config{
				Method: MethodConfig{
					Name: "other",
				},
			},
			expectNil:  false,
			expectedKP: "passphrase",
			expectedM:  "other",
		},
		{
			testcase: "method_default",
			config: &Config{
				KeyProvider: KeyProviderConfig{
					Name: "direct",
					Config: map[string]string{
						"key": validKey1,
					},
				},
			},
			expectNil:  false,
			expectedKP: "direct",
			expectedM:  "full",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			InjectDefaultNamesIfNotSet(tc.config)
			if tc.expectNil != (tc.config == nil) {
				t.Error("unexpected nil state change")
			}
			if tc.config != nil {
				if tc.expectedKP != string(tc.config.KeyProvider.Name) {
					t.Errorf("unexpected key provider name '%s' instead of '%s'", tc.config.KeyProvider.Name, tc.expectedKP)
				}
				if tc.expectedM != string(tc.config.Method.Name) {
					t.Errorf("unexpected method name '%s' instead of '%s'", tc.config.Method.Name, tc.expectedM)
				}
			}
		})
	}
}

func deepMergeTestConfig(keyProviderName string, keyProviderKeyCount int, methodName string, methodKeyCount int, enforced bool) *Config {
	buildTestConfiguration := func(configKeyPrefix string, configKeyCount int) map[string]string {
		var result map[string]string
		if configKeyCount >= 0 {
			result = make(map[string]string)
			for i := 0; i < configKeyCount; i++ {
				result[fmt.Sprintf("%s.%d", configKeyPrefix, i+1)] = fmt.Sprintf("%d", i+1)
			}
		}
		return result
	}

	return &Config{
		KeyProvider: KeyProviderConfig{
			Name:   KeyProviderName(keyProviderName),
			Config: buildTestConfiguration(keyProviderName, keyProviderKeyCount),
		},
		Method: MethodConfig{
			Name:   MethodName(methodName),
			Config: buildTestConfiguration(methodName, methodKeyCount),
		},
		Enforced: enforced,
	}
}

// TODO missing a test case for empty maps and for showing maps with overlapping config values.
func TestMergeConfigs(t *testing.T) {
	testCases := []struct {
		testcase      string
		defaultConfig *Config
		mergeList     []*Config
		expected      *Config
	}{
		{
			testcase:      "nothing",
			defaultConfig: nil,
			mergeList:     nil,
			expected:      nil,
		},
		{
			testcase:      "many_nils",
			defaultConfig: nil,
			mergeList:     []*Config{nil, nil, nil, nil, nil},
			expected:      nil,
		},
		{
			testcase:      "use_default",
			defaultConfig: deepMergeTestConfig("kpdefault", 1, "mdefault", 2, false),
			mergeList:     []*Config{nil, nil},
			expected:      deepMergeTestConfig("kpdefault", 1, "mdefault", 2, false),
		},
		{
			testcase:      "use_non_nil1_ignore_default",
			defaultConfig: deepMergeTestConfig("kpdefault", 1, "mdefault", 2, false),
			mergeList:     []*Config{deepMergeTestConfig("kp1", 0, "m1", 1, true), nil, nil},
			expected:      deepMergeTestConfig("kp1", 0, "m1", 1, true),
		},
		{
			testcase:      "use_non_nil2",
			defaultConfig: nil,
			mergeList:     []*Config{nil, deepMergeTestConfig("kp2", 3, "m2", 2, false)},
			expected:      deepMergeTestConfig("kp2", 3, "m2", 2, false),
		},
		{
			testcase:      "merge_ignore_default",
			defaultConfig: deepMergeTestConfig("kpdefault", 1, "mdefault", 2, true),
			mergeList: []*Config{
				deepMergeTestConfig("kp1", 1, "m1", 0, false),
				deepMergeTestConfig("kp2", 0, "m2", 2, false),
				deepMergeTestConfig("kp3", 1, "m3", 1, false),
			},
			expected: &Config{
				KeyProvider: KeyProviderConfig{
					Name: "kp3",
					Config: map[string]string{
						"kp1.1": "1",
						"kp3.1": "1",
					},
				},
				Method: MethodConfig{
					Name: "m3",
					Config: map[string]string{
						"m2.1": "1",
						"m2.2": "2",
						"m3.1": "1",
					},
				},
				Enforced: false,
			},
		},
	}

	helpfulDeepCompare := func(t *testing.T, expected Config, actual Config) {
		if expected.Enforced != actual.Enforced {
			t.Errorf("expected enforced to be %T, received %T", expected.Enforced, actual.Enforced)
		}
		if expected.KeyProvider.Name != actual.KeyProvider.Name {
			t.Errorf("expected key provider name '%s', received '%s'", actual.KeyProvider.Name, expected.KeyProvider.Name)
		}
		if !reflect.DeepEqual(expected.KeyProvider.Config, actual.KeyProvider.Config) {
			expectedConfigJSON, _ := json.Marshal(expected.KeyProvider.Config)
			actualConfigJSON, _ := json.Marshal(actual.KeyProvider.Config)
			t.Errorf("expected key provider config:\n%s\nreceived:\n%s", expectedConfigJSON, actualConfigJSON)
		}
		if expected.Method.Name != actual.Method.Name {
			t.Errorf("expected method name: '%s', received: '%s'", actual.Method.Name, expected.Method.Name)
		}
		if !reflect.DeepEqual(expected.Method.Config, actual.Method.Config) {
			expectedConfigJSON, _ := json.Marshal(expected.Method.Config)
			actualConfigJSON, _ := json.Marshal(actual.Method.Config)
			t.Errorf("expected method config:\n%s\nreceived:\n%s", expectedConfigJSON, actualConfigJSON)
		}
		if !reflect.DeepEqual(expected, actual) {
			expectedJSON, _ := json.Marshal(expected)
			actualJSON, _ := json.Marshal(actual)
			t.Errorf(
				"⚠️ Missing test case for one or more fields, unexpected config difference. "+
					"Please add an explicit check.\nExpected:\n%s\nReceived:\n%s",
				expectedJSON,
				actualJSON,
			)
		}
	}

	for _, tc := range testCases {
		t.Run(tc.testcase, func(t *testing.T) {
			actual := MergeConfigs(tc.defaultConfig, tc.mergeList...)
			if tc.expected == nil || actual == nil {
				if tc.expected != actual {
					t.Error("unexpected nil state")
				}
			} else {
				helpfulDeepCompare(t, *tc.expected, *actual)
			}
		})
	}
}
