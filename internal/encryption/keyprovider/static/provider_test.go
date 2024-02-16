package static_test

import (
	"testing"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider/static"
)

func TestKeyProvider(t *testing.T) {
	type testCase struct {
		name          string
		key           string
		expectSuccess bool
		expectedData  string
		expectedMeta  string
	}

	testCases := []testCase{
		{
			name:          "Empty",
			expectSuccess: true,
			expectedData:  "",
			expectedMeta:  "magic",
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
			expectedData:  "Hello world!",
			expectedMeta:  "magic",
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
