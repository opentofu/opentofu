package azure_kms

import (
	"fmt"
	"testing"
	// "github.com/opentofu/opentofu/internal/encryption/keyprovider"
	// "github.com/opentofu/opentofu/internal/gohcl"
	// "github.com/opentofu/opentofu/internal/httpclient"
	// "github.com/opentofu/opentofu/version"
)

func TestValidate(t *testing.T) {
	testCases := []struct {
		name     string
		input    Config
		expected error
	}{
		{
			name: "missing VaultName",
			input: Config{
				KeyName:      "my-key",
				KeyAlgorithm: "AES-256",
				KeySize:      256,
			},
			expected: fmt.Errorf("No vault_name provided"),
		},
		{
			name: "missing key_name",
			input: Config{
				VaultName:    "my-vault",
				KeyAlgorithm: "AES-256",
				KeySize:      256,
			},
			expected: fmt.Errorf("No key_name provided"),
		},
		{
			name: "missing key_algorithm",
			input: Config{
				VaultName: "my-vault",
				KeyName:   "my-key",
				KeySize:   256,
			},
			expected: fmt.Errorf("No key_algorithm provided"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.input.validate()

			if tc.expected != nil {
				if err.Error() != tc.expected.Error() {
					t.Fatalf("Expected %q, got %q", tc.expected.Error(), err.Error())
				}
			}
		})
	}
}
