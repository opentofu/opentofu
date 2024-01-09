package encryptionflow

import "testing"

const invalidConfigurationSource ConfigurationSource = "invalid"

func TestConfigurationSourceEnum(t *testing.T) {
	testCases := []struct {
		value                ConfigurationSource
		expectValid          bool
		expectForExternalUse bool
	}{
		{
			value:                invalidConfigurationSource,
			expectValid:          false,
			expectForExternalUse: false,
		},
		{
			value:                ConfigurationSourceEnvDefault,
			expectValid:          true,
			expectForExternalUse: false,
		},
		{
			value:                ConfigurationSourceCode,
			expectValid:          true,
			expectForExternalUse: true,
		},
		{
			value:                ConfigurationSourceEnv,
			expectValid:          true,
			expectForExternalUse: false,
		},
	}

	for _, tc := range testCases {
		t.Run(string(tc.value), func(t *testing.T) {
			if tc.expectValid != tc.value.IsValid() {
				t.Error("unexpected result for IsValid")
			}
			if tc.expectForExternalUse != tc.value.IsForExternalUse() {
				t.Error("unexpected result for IsForExternalUse")
			}
		})
	}
}
