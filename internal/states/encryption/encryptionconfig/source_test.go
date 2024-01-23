package encryptionconfig

import (
	"testing"
)

const invalidConfigurationSource Source = "invalid"

func TestConfigurationSourceEnum(t *testing.T) {
	testCases := []struct {
		value                Source
		expectValid          bool
		expectForExternalUse bool
	}{
		{
			value:       invalidConfigurationSource,
			expectValid: false,
		},
		{
			value:       SourceCode,
			expectValid: true,
		},
		{
			value:       SourceEnv,
			expectValid: true,
		},
	}

	for _, tc := range testCases {
		t.Run(string(tc.value), func(t *testing.T) {
			if tc.expectValid != tc.value.IsValid() {
				t.Error("unexpected result for IsValid")
			}
		})
	}
}
