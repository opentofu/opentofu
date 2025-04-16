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
	tempFile.WriteString(m.fileContent)
	if err != nil {
		tt.Fatal(err)
	}
	if err := tempFile.Close(); err != nil {
		tt.Fatal(err)
	}
}
