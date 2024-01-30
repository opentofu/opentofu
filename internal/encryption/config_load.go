package encryption

import (
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/json"
)

func LoadConfigFromString(sourceName string, rawInput string) (*Config, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	var file *hcl.File

	if strings.TrimSpace(rawInput)[0] == '{' {
		file, diags = json.Parse([]byte(rawInput), sourceName)
	} else {
		file, diags = hclsyntax.ParseConfig([]byte(rawInput), sourceName, hcl.Pos{Byte: 0, Line: 1, Column: 1})
	}

	cfg, cfgDiags := DecodeConfig(file.Body, hcl.Range{Filename: sourceName})
	diags = append(diags, cfgDiags...)

	return cfg, diags
}
