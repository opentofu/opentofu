// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package externalcommand

import (
	"fmt"
	"testing"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/compliancetest"
)

func TestCompliance(t *testing.T) {
	validConfig := &Config{
		Command: []string{"testprovider"},
	}
	compliancetest.ComplianceTest(
		t,
		compliancetest.TestConfiguration[*descriptor, *Config, *Metadata, *keyProvider]{
			Descriptor: New().(*descriptor),
			HCLParseTestCases: map[string]compliancetest.HCLParseTestCase[*Config, *keyProvider]{
				"empty": {
					HCL: `key_provider "externalcommand" "foo" {
}`,
					ValidHCL:   false,
					ValidBuild: false,
					Validate:   nil,
				},
				"basic": {
					HCL: `key_provider "externalcommand" "foo" {
    command = ["keyprovider"]
}`,
					ValidHCL:   true,
					ValidBuild: true,
					Validate: func(config *Config, keyProvider *keyProvider) error {
						if len(config.Command) != 1 {
							return fmt.Errorf("invalid command after parsing")
						}
						if config.Command[0] != "keyprovider" {
							return fmt.Errorf("invalid command after parsing")
						}
						return nil
					},
				},
			},
			ConfigStructTestCases: map[string]compliancetest.ConfigStructTestCase[*Config, *keyProvider]{},
			MetadataStructTestCases: map[string]compliancetest.MetadataStructTestCase[*Config, *Metadata]{
				"not-present-externaldata": {
					ValidConfig: validConfig,
					Meta: &Metadata{
						ExternalData: nil,
					},
					IsPresent: false,
				},
				"present-valid": {
					ValidConfig: validConfig,
					Meta: &Metadata{
						ExternalData: map[string]any{},
					},
					IsPresent: true,
					IsValid:   true,
				},
			},
			ProvideTestCase: compliancetest.ProvideTestCase[*Config, *Metadata]{
				ValidConfig: &Config{
					Command: []string{"testcommand"},
				},
				ExpectedOutput: &keyprovider.Output{
					EncryptionKey: []byte{},
					DecryptionKey: []byte{},
				},
				ValidateKeys: nil,
				ValidateMetadata: func(meta *Metadata) error {
					if meta.ExternalData == nil {
						return fmt.Errorf("output metadata is not present")
					}
					return nil
				},
			},
		},
	)
}
