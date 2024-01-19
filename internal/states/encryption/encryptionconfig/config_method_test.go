package encryptionconfig

import (
	"errors"
	"testing"
)

func TestEncryptionMethodConfigValidate(t *testing.T) {
	testCases := []struct {
		testcase    string
		config      MethodConfig
		expectedErr error
	}{
		{
			testcase: "correct",
			config: MethodConfig{
				Name: "full",
			},
			expectedErr: nil,
		},
		{
			testcase: "unknown_name",
			config: MethodConfig{
				Name: "unknown",
			},
			expectedErr: errors.New("error in configuration for encryption method unknown: no registered encryption method with this name"),
		},
		{
			testcase: "incorrect",
			config: MethodConfig{
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
		})
	}
}
