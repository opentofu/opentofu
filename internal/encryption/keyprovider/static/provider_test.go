// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package static_test

import (
	"testing"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider/static"
)

func TestKeyProvider(t *testing.T) {
	// TODO: Rework to check the expected errors and not just expectSuccess
	type testCase struct {
		name          string
		key           string
		expectSuccess bool
		expectedData  string // The key as a string taken from the hex value of the key
		expectedMeta  string
	}

	testCases := []testCase{
		{
			name:          "Empty",
			expectSuccess: true,
			expectedData:  "",
			expectedMeta:  "magic", // We currently always output the metadata "magic"
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
			expectedData:  "Hello world!", // "48656c6c6f20776f726c6421" in hex is "Hello world!"
			expectedMeta:  "magic",        // We currently always output the metadata "magic"
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

			keyProvider, buildErr := c.Build()
			if tc.expectSuccess {
				if buildErr != nil {
					t.Fatalf("unexpected error: %v", buildErr)
				}

				data, newMetadata, err := keyProvider.Provide(nil)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if string(data) != tc.expectedData {
					t.Fatalf("unexpected key output: got %v, want %v", data, tc.expectedData)
				}
				if string(newMetadata) != tc.expectedMeta {
					t.Fatalf("unexpected metadata: got %v, want %v", newMetadata, tc.expectedMeta)
				}
			} else {
				if buildErr == nil {
					t.Fatalf("expected an error but got none")
				}
			}
		})
	}
}
