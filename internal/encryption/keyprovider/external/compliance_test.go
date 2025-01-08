// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package external

import (
	"fmt"
	"testing"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/compliancetest"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/external/testprovider"
)

func TestComplianceBinary(t *testing.T) {
	runTest(t, testprovider.Go(t))
}

func TestCompliancePython(t *testing.T) {
	runTest(t, testprovider.Python(t))
}

func TestCompliancePOSIXShell(t *testing.T) {
	runTest(t, testprovider.POSIXShell(t))
}

func runTest(t *testing.T, command []string) {
	validConfig := &Config{
		Command: command,
	}
	compliancetest.ComplianceTest(
		t,
		compliancetest.TestConfiguration[*descriptor, *Config, *MetadataV1, *keyProvider]{
			Descriptor: New().(*descriptor), //nolint:errcheck //No clue why errcheck fires here.
			HCLParseTestCases: map[string]compliancetest.HCLParseTestCase[*Config, *keyProvider]{
				"empty": {
					HCL: `key_provider "external" "foo" {
}`,
					ValidHCL:   false,
					ValidBuild: false,
					Validate:   nil,
				},
				"basic": {
					HCL: `key_provider "external" "foo" {
    command = ["test-provider"]
}`,
					ValidHCL:   true,
					ValidBuild: true,
					Validate: func(config *Config, keyProvider *keyProvider) error {
						if len(config.Command) != 1 {
							return fmt.Errorf("invalid command after parsing")
						}
						if config.Command[0] != "test-provider" {
							return fmt.Errorf("invalid command after parsing")
						}
						return nil
					},
				},
				"empty-binary": {
					HCL: `key_provider "external" "foo" {
    command = []
}`,
					ValidHCL:   true,
					ValidBuild: false,
				},
			},
			ConfigStructTestCases: map[string]compliancetest.ConfigStructTestCase[*Config, *keyProvider]{},
			MetadataStructTestCases: map[string]compliancetest.MetadataStructTestCase[*Config, *MetadataV1]{
				"not-present-externaldata": {
					ValidConfig: validConfig,
					Meta:        nil,
					IsPresent:   false,
				},
				"present-valid": {
					ValidConfig: validConfig,
					Meta: &MetadataV1{
						ExternalData: map[string]any{},
					},
					IsPresent: true,
					IsValid:   true,
				},
			},
			ProvideTestCase: compliancetest.ProvideTestCase[*Config, *MetadataV1]{
				ValidConfig: validConfig,
				ExpectedOutput: &keyprovider.Output{
					EncryptionKey: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
					DecryptionKey: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
				},
				ValidateKeys: nil,
				ValidateMetadata: func(meta *MetadataV1) error {
					if meta.ExternalData == nil {
						return fmt.Errorf("output metadata is not present")
					}
					return nil
				},
			},
		},
	)
}
