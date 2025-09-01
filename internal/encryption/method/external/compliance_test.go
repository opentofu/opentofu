// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package external

import (
	"fmt"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/method/compliancetest"
	"github.com/opentofu/opentofu/internal/encryption/method/external/testmethod"
)

func TestComplianceBinary(t *testing.T) {
	runTest(t, testmethod.Go(t))
}

func TestCompliancePython(t *testing.T) {
	runTest(t, testmethod.Python(t))
}

func runTest(t *testing.T, cmd []string) {
	cmd = slices.Clip(cmd) // Make sure that the following appends are forced to allocate capacity
	encryptCommand := append(cmd, "--encrypt")
	decryptCommand := append(cmd, "--decrypt")

	compliancetest.ComplianceTest(t, compliancetest.TestConfiguration[*descriptor, *Config, *command]{
		Descriptor: New().(*descriptor), //nolint:errcheck //This is safe.
		HCLParseTestCases: map[string]compliancetest.HCLParseTestCase[*descriptor, *Config, *command]{
			"empty": {
				HCL:        `method "external" "foo" {}`,
				ValidHCL:   false,
				ValidBuild: false,
				Validate:   nil,
			},
			"empty-command": {
				HCL: `method "external" "foo" {
	encrypt_command = []
	decrypt_command = []
}`,
				ValidHCL: true,
			},
			"command": {
				HCL: fmt.Sprintf(`method "external" "foo" {
	encrypt_command = ["%s"]
	decrypt_command = ["%s"]
}`, strings.Join(encryptCommand, `","`), strings.Join(decryptCommand, `","`)),
				ValidHCL:   true,
				ValidBuild: true,
				Validate: func(config *Config, method *command) error {
					// We need to normalize in order to match with the decoded config
					if runtime.GOOS == "windows" {
						for i, v := range encryptCommand {
							encryptCommand[i] = strings.ReplaceAll(v, `\\`, "\\")
						}
						for i, v := range decryptCommand {
							decryptCommand[i] = strings.ReplaceAll(v, `\\`, "\\")
						}
					}
					if diff := cmp.Diff(config.EncryptCommand, encryptCommand); diff != "" {
						return fmt.Errorf("incorrect encrypt command after HCL parsing: %s", diff)
					}
					if diff := cmp.Diff(config.DecryptCommand, decryptCommand); diff != "" {
						return fmt.Errorf("incorrect decrypt command after HCL parsing: %s", diff)
					}
					return nil
				},
			},
		},
		ConfigStructTestCases: map[string]compliancetest.ConfigStructTestCase[*Config, *command]{
			"empty": {
				Config:     &Config{},
				ValidBuild: false,
				Validate:   nil,
			},
		},
		EncryptDecryptTestCase: compliancetest.EncryptDecryptTestCase[*Config, *command]{
			ValidEncryptOnlyConfig: &Config{
				Keys: &keyprovider.Output{
					EncryptionKey: []byte{20},
					DecryptionKey: nil,
				},
				EncryptCommand: encryptCommand,
				DecryptCommand: decryptCommand,
			},
			ValidFullConfig: &Config{
				Keys: &keyprovider.Output{
					EncryptionKey: []byte{20},
					DecryptionKey: []byte{20},
				},
				EncryptCommand: encryptCommand,
				DecryptCommand: decryptCommand,
			},
			DecryptCannotBeVerified: true,
		},
	})
}
