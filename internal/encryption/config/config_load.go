// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package config

import (
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/json"
)

// LoadConfigFromString loads a configuration from a string. The sourceName is used to identify the source of the
// configuration in error messages.
// However! this method should only be used in tests, as we should be using gohcl to parse the configuration.
// TODO: Discuss if we can remove this now.
func LoadConfigFromString(sourceName string, rawInput string) (*Config, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	var file *hcl.File

	if strings.TrimSpace(rawInput)[0] == '{' {
		file, diags = json.Parse([]byte(rawInput), sourceName)
	} else {
		file, diags = hclsyntax.ParseConfig([]byte(rawInput), sourceName, hcl.Pos{Byte: 0, Line: 1, Column: 1})
	}

	cfg, cfgDiags := decodeConfig(file.Body, hcl.Range{Filename: sourceName})
	diags = append(diags, cfgDiags...)

	return cfg, diags
}
