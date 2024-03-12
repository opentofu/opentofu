// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package static_test

import (
	"fmt"
	"testing"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider/compliancetest"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider/static"
)

func TestKeyProvider(t *testing.T) {
	compliancetest.ComplianceTest(
		t,
		static.New(),
		map[string]compliancetest.TestCase{
			"success": {
				`key_provider "static" "foo" {
    key = "48656c6c6f20776f726c6421"
}`,
				true,
				func(config keyprovider.Config) error {
					if parsedKey := config.(*static.Config).Key; parsedKey != "48656c6c6f20776f726c6421" {
						return fmt.Errorf("incorrect key in parsed config: %s", parsedKey)
					}
					return nil
				},
				false,
				true,
				nil,
				&keyprovider.Output{
					EncryptionKey: []byte("Hello world!"), // "48656c6c6f20776f726c6421" in hex is "Hello world!"
					DecryptionKey: []byte("Hello world!"),
				},
				func() any {
					return &static.Metadata{
						Magic: "Broken magic.",
					}
				},
				func(output keyprovider.Output, meta any) error {
					if magic := meta.(*static.Metadata).Magic; magic != "Hello world!" {
						return fmt.Errorf("incorrect output magic: %s", magic)
					}
					return nil
				},
			},
			"empty": {
				`key_provider "static" "foo" {
}`,
				false,
				nil,
				true,
				false,
				nil,
				nil,
				nil,
				nil,
			},
			"empty-internal": {
				`key_provider "static" "foo" {
    key = "48656c6c6f20776f726c6421"
}`,
				true,
				func(config keyprovider.Config) error {
					// Inject incorrect key for internal validation test
					config.(*static.Config).Key = ""
					return nil
				},
				false,
				false,
				nil,
				nil,
				nil,
				nil,
			},
			"invalid-hex": {
				`key_provider "static" "foo" {
    key = "G"
}`,
				true,
				func(config keyprovider.Config) error {
					if parsedKey := config.(*static.Config).Key; parsedKey != "G" {
						return fmt.Errorf("incorrect key in parsed config: %s", parsedKey)
					}
					return nil
				},
				false,
				false,
				nil,
				nil,
				nil,
				nil,
			},
		},
	)
}
