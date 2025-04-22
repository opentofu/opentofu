// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0
package arguments

import (
	"os"
	"testing"

	"github.com/hashicorp/hcl/v2"
)

type mockFile struct {
	filePath    string
	fileContent string
	diags       hcl.Diagnostics
}

func (m *mockFile) tempFileWriter(tt *testing.T) {
	tt.Helper()
	tempFile, err := os.CreateTemp(tt.TempDir(), "opentofu-test-files")
	if err != nil {
		tt.Fatal(err)
	}
	m.filePath = tempFile.Name()
	if _, err := tempFile.WriteString(m.fileContent); err != nil {
		tt.Fatal(err)
	}
	if err := tempFile.Close(); err != nil {
		tt.Fatal(err)
	}
}
