package arguments

import "github.com/hashicorp/hcl/v2"

type testFile struct {
	filePath    string
	fileContent string
	diags       hcl.Diagnostics
}
