// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package externalcommand

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"runtime"
	"testing"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/compliancetest"
)

func TestCompliance(t *testing.T) {
	testProviderBinaryName := "testprovider-binary"
	if runtime.GOOS == "windows" {
		testProviderBinaryName += ".exe"
	}
	_, err := os.Stat(testProviderBinaryName)
	if err != nil {
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatalf("Failed to compile test provider binary (%v)", err)
		}
		cmd := exec.Command("go", "build", "-o", "../"+testProviderBinaryName)
		cmd.Dir = path.Join(cwd, "testprovider")
		// TODO move this to a proper test logger
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to compile test provider binary (%v)", err)
		}
	}

	validConfig := &Config{
		Command: []string{"./" + testProviderBinaryName},
	}
	compliancetest.ComplianceTest(
		t,
		compliancetest.TestConfiguration[*descriptor, *Config, *Metadata, *keyProvider]{
			Descriptor: New().(*descriptor), //nolint:errcheck //No clue why errcheck fires here.
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
    command = ["` + testProviderBinaryName + `"]
}`,
					ValidHCL:   true,
					ValidBuild: true,
					Validate: func(config *Config, keyProvider *keyProvider) error {
						if len(config.Command) != 1 {
							return fmt.Errorf("invalid command after parsing")
						}
						if config.Command[0] != testProviderBinaryName {
							return fmt.Errorf("invalid command after parsing")
						}
						return nil
					},
				},
				"empty-binary": {
					HCL: `key_provider "externalcommand" "foo" {
    command = []
}`,
					ValidHCL:   true,
					ValidBuild: false,
				},
			},
			ConfigStructTestCases: map[string]compliancetest.ConfigStructTestCase[*Config, *keyProvider]{},
			MetadataStructTestCases: map[string]compliancetest.MetadataStructTestCase[*Config, *Metadata]{
				"not-present-externaldata": {
					ValidConfig: validConfig,
					Meta:        nil,
					IsPresent:   false,
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
				ValidConfig: validConfig,
				ExpectedOutput: &keyprovider.Output{
					EncryptionKey: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
					DecryptionKey: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
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
