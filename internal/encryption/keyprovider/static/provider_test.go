// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package static

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/terramate-io/opentofulib/internal/encryption/keyprovider/compliancetest"

	"github.com/terramate-io/opentofulib/internal/encryption/keyprovider"
)

func TestKeyProvider(t *testing.T) {
	compliancetest.ComplianceTest(
		t,
		compliancetest.TestConfiguration[*descriptor, *Config, *Metadata, *staticKeyProvider]{
			Descriptor: New().(*descriptor),
			HCLParseTestCases: map[string]compliancetest.HCLParseTestCase[*Config, *staticKeyProvider]{
				"success": {
					HCL: `key_provider "static" "foo" {
    key = "48656c6c6f20776f726c6421"
}`,
					ValidHCL:   true,
					ValidBuild: true,
					Validate: func(config *Config, keyProvider *staticKeyProvider) error {
						if config.Key != "48656c6c6f20776f726c6421" {
							return fmt.Errorf("incorrect key returned")
						}
						if !bytes.Equal(keyProvider.key, []byte("Hello world!")) {
							return fmt.Errorf("key provider contains invalid key")
						}
						return nil
					},
				},
				"empty": {
					HCL:        `key_provider "static" "foo" {}`,
					ValidHCL:   false,
					ValidBuild: false,
				},
				"bad-hex": {
					HCL: `key_provider "static" "foo" {
	key = "G"
}`,
					ValidHCL:   true,
					ValidBuild: false,
				},
				"bad-argument": {
					HCL: `key_provider "static" "foo" {
	keys = "48656c6c6f20776f726c6421" # Note the incorrect key name
}`,
					ValidHCL:   false,
					ValidBuild: false,
				},
			},
			ConfigStructTestCases: map[string]compliancetest.ConfigStructTestCase[*Config, *staticKeyProvider]{
				"empty": {
					Config: &Config{
						Key: "",
					},
					ValidBuild: false,
					Validate:   nil,
				},
			},
			MetadataStructTestCases: map[string]compliancetest.MetadataStructTestCase[*Config, *Metadata]{
				"empty": {
					ValidConfig: &Config{
						Key: "48656c6c6f20776f726c6421",
					},
					Meta:      &Metadata{},
					IsPresent: false,
					IsValid:   false,
				},
				"invalid": {
					ValidConfig: &Config{
						Key: "48656c6c6f20776f726c6421",
					},
					Meta: &Metadata{
						Magic: "Invalid",
					},
					IsPresent: true,
					IsValid:   false,
				},
				"valid": {
					ValidConfig: &Config{
						Key: "48656c6c6f20776f726c6421",
					},
					Meta: &Metadata{
						Magic: "Hello world!",
					},
					IsPresent: true,
					IsValid:   true,
				},
			},
			ProvideTestCase: compliancetest.ProvideTestCase[*Config, *Metadata]{
				ValidConfig: &Config{
					Key: "48656c6c6f20776f726c6421",
				},
				ExpectedOutput: &keyprovider.Output{
					EncryptionKey: []byte("Hello world!"), // "48656c6c6f20776f726c6421" in hex is "Hello world!"
					DecryptionKey: []byte("Hello world!"),
				},
				ValidateKeys: nil,
				ValidateMetadata: func(meta *Metadata) error {
					if meta.Magic != "Hello world!" {
						return fmt.Errorf("incorrect output magic: %s", meta.Magic)
					}
					return nil
				},
			},
		},
	)
}
