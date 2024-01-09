package encryptionconfig

import (
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
				Method: EncryptionMethodConfig{
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
			InjectDefaultNamesIfUnset(tc.config)
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

func tstConfig(kp string, kpcount int, m string, mcount int, required bool) *Config {
	tstMap := func(base string, count int) map[string]string {
		var result map[string]string
		if count >= 0 {
			result = make(map[string]string)
			for i := 0; i < count; i++ {
				result[fmt.Sprintf("%s.%d", base, i+1)] = fmt.Sprintf("%d", i+1)
			}
		}
		return result
	}

	return &Config{
		KeyProvider: KeyProviderConfig{
			Name:   KeyProviderName(kp),
			Config: tstMap(kp, kpcount),
		},
		Method: EncryptionMethodConfig{
			Name:   EncryptionMethodName(m),
			Config: tstMap(m, mcount),
		},
		Required: required,
	}
}

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
			defaultConfig: tstConfig("kpdefault", 1, "mdefault", 2, false),
			mergeList:     []*Config{nil, nil},
			expected:      tstConfig("kpdefault", 1, "mdefault", 2, false),
		},
		{
			testcase:      "use_non_nil1_ignore_default",
			defaultConfig: tstConfig("kpdefault", 1, "mdefault", 2, false),
			mergeList:     []*Config{tstConfig("kp1", 0, "m1", 1, true), nil, nil},
			expected:      tstConfig("kp1", 0, "m1", 1, true),
		},
		{
			testcase:      "use_non_nil2",
			defaultConfig: nil,
			mergeList:     []*Config{nil, tstConfig("kp2", 3, "m2", 2, false)},
			expected:      tstConfig("kp2", 3, "m2", 2, false),
		},
		{
			testcase:      "merge_ignore_default",
			defaultConfig: tstConfig("kpdefault", 1, "mdefault", 2, true),
			mergeList: []*Config{
				tstConfig("kp1", 1, "m1", 0, false),
				tstConfig("kp2", 0, "m2", 2, false),
				tstConfig("kp3", 1, "m3", 1, false),
			},
			expected: &Config{
				KeyProvider: KeyProviderConfig{
					Name: "kp3",
					Config: map[string]string{
						"kp1.1": "1",
						"kp3.1": "1",
					},
				},
				Method: EncryptionMethodConfig{
					Name: "m3",
					Config: map[string]string{
						"m2.1": "1",
						"m2.2": "2",
						"m3.1": "1",
					},
				},
				Required: false,
			},
		},
	}

	helpfulDeepCompare := func(t *testing.T, expected Config, actual Config) {
		if expected.Required != actual.Required {
			t.Error("unexpected value for required")
		}
		if expected.KeyProvider.Name != actual.KeyProvider.Name {
			t.Errorf("unexpected key provider name '%s' instead of '%s'", actual.KeyProvider.Name, expected.KeyProvider.Name)
		}
		if !reflect.DeepEqual(expected.KeyProvider.Config, actual.KeyProvider.Config) {
			t.Error("key provider parameters differ")
		}
		if expected.Method.Name != actual.Method.Name {
			t.Errorf("unexpected method name '%s' instead of '%s'", actual.Method.Name, expected.Method.Name)
		}
		if !reflect.DeepEqual(expected.Method.Config, actual.Method.Config) {
			t.Error("method parameters differ")
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
