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
// This method serves as an example for how someone using this library might want to load a configuration.
// if they were not using gohcl directly.
func LoadConfigFromString(sourceName string, rawInput string) (*EncryptionConfig, hcl.Diagnostics) {
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
