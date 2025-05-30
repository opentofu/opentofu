// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package mcpconfigs

import (
	"fmt"
	"os"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

type Config struct {
	MCPServers map[string]*MCPServer

	DataResourceTypes    map[string]*DataResourceType
	ManagedResourceTypes map[string]*ManagedResourceType
}

func LoadConfig(body hcl.Body) (*Config, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	content, hclDiags := body.Content(rootSchema)
	diags = diags.Append(hclDiags)

	ret := &Config{
		MCPServers:           make(map[string]*MCPServer),
		DataResourceTypes:    make(map[string]*DataResourceType),
		ManagedResourceTypes: make(map[string]*ManagedResourceType),
	}
	for _, block := range content.Blocks {
		switch block.Type {
		case "mcp_server":
			identifier := block.Labels[0]
			if existing, exists := ret.MCPServers[identifier]; exists {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate mcp_server block",
					Detail:   fmt.Sprintf("An mcp_server %q block was already declared at %s.", identifier, existing.DeclRange),
				})
				continue
			}
			server, moreDiags := decodeMCPServer(block)
			diags = diags.Append(moreDiags)
			ret.MCPServers[server.DeclName.Value] = server
		case "data_resource_type":
			name := block.Labels[0]
			if existing, exists := ret.DataResourceTypes[name]; exists {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate data_resource_type block",
					Detail:   fmt.Sprintf("An data_resource_type %q block was already declared at %s.", name, existing.DeclRange),
				})
				continue
			}
			// TODO: decodeDataResourceType
		case "managed_resource_type":
			name := block.Labels[0]
			if existing, exists := ret.ManagedResourceTypes[name]; exists {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate managed_resource_type block",
					Detail:   fmt.Sprintf("An managed_resource_type %q block was already declared at %s.", name, existing.DeclRange),
				})
				continue
			}
			// TODO: decodeManagedResourceType
		default:
			// Should not get here bcause the cases above should cover every
			// block type declared in rootSchema.
			panic(fmt.Sprintf("unsupported block type %q", block.Type))
		}
	}

	return ret, diags
}

func LoadConfigSource(src []byte, filename string, startPos hcl.Pos) (*Config, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	file, hclDiags := hclsyntax.ParseConfig(src, filename, startPos)
	diags = diags.Append(hclDiags)
	if hclDiags.HasErrors() {
		return nil, diags
	}

	ret, moreDiags := LoadConfig(file.Body)
	diags = diags.Append(moreDiags)
	return ret, diags
}

func LoadConfigFile(filename string) (*Config, tfdiags.Diagnostics) {
	src, err := os.ReadFile(filename)
	if err != nil {
		var diags tfdiags.Diagnostics
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"MCP provider configuration unreadable",
			fmt.Sprintf("Failed to read an MCP provider configuration: %s.", err),
		))
		return nil, diags
	}

	return LoadConfigSource(src, filename, hcl.InitialPos)
}

var rootSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{
			Type:       "mcp_server",
			LabelNames: []string{"identifier"},
		},
		{
			Type:       "managed_resource_type",
			LabelNames: []string{"name"},
		},
		{
			Type:       "data_resource_type",
			LabelNames: []string{"name"},
		},
	},
}
