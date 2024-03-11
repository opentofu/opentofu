// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package static_test

import (
	"bytes"
	"testing"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider/static"
)

func TestKeyProvider(t *testing.T) {
	// TODO: Rework to check the expected errors and not just expectSuccess
	type testCase struct {
		name          string
		key           string
		expectSuccess bool
		expectedData  keyprovider.Output
	}

	testCases := []testCase{
		{
			name:          "Empty",
			expectSuccess: true,
			expectedData:  keyprovider.Output{},
		},
		{
			name:          "InvalidInput",
			key:           "G",
			expectSuccess: false,
		},
		{
			name:          "Success",
			key:           "48656c6c6f20776f726c6421",
			expectSuccess: true,
			expectedData:  keyprovider.Output{EncryptionKey: []byte("Hello world!"), DecryptionKey: []byte("Hello world!")}, // "48656c6c6f20776f726c6421" in hex is "Hello world!"
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			descriptor := static.New()
			c := descriptor.ConfigStruct().(*static.Config)

			// Set key if provided
			if tc.key != "" {
				c.Key = tc.key
			}

			keyProvider, keyMeta, buildErr := c.Build()
			if tc.expectSuccess {
				if buildErr != nil {
					t.Fatalf("unexpected error: %v", buildErr)
				}

				output, _, err := keyProvider.Provide(keyMeta)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !bytes.Equal(output.EncryptionKey, tc.expectedData.EncryptionKey) {
					t.Fatalf("unexpected encryption key in output: got %v, want %v", output.EncryptionKey, tc.expectedData.EncryptionKey)
				}
				if !bytes.Equal(output.DecryptionKey, tc.expectedData.DecryptionKey) {
					t.Fatalf("unexpected decryption key in output: got %v, want %v", output.DecryptionKey, tc.expectedData.EncryptionKey)
				}
			} else {
				if buildErr == nil {
					t.Fatalf("expected an error but got none")
				}
			}
		})
	}
}
