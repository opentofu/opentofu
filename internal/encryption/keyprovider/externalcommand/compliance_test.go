// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package externalcommand

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"runtime"
	"testing"
	"time"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/compliancetest"
)

func TestComplianceBinary(t *testing.T) {
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

	runTest(t, []string{"./" + testProviderBinaryName}, testProviderBinaryName)
}

func TestCompliancePython(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "python", "--version")
	if err := cmd.Run(); err != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "python3", "--version")
		if err := cmd.Run(); err != nil {
			t.Skipf("No working Python installation found, skipping test (%v)", err)
		}
		runTest(t, []string{"python3", "./testprovider/testprovider.py"}, "testprovider.py")
	} else {
		runTest(t, []string{"python", "./testprovider/testprovider.py"}, "testprovider.py")
	}
}

func TestCompliancePOSIXShell(t *testing.T) {
	_, err := os.Stat("/bin/sh")
	if err != nil {
		t.Skipf("No /bin/sh present, skipping test.")
	}
	runTest(t, []string{"/bin/sh", "./testprovider/testprovider.sh"}, "testprovider.sh")
}

func runTest(t *testing.T, command []string, testProviderBinaryName string) {
	validConfig := &Config{
		Command: command,
	}
	compliancetest.ComplianceTest(
		t,
		compliancetest.TestConfiguration[*descriptor, *Config, *MetadataV1, *keyProvider]{
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
