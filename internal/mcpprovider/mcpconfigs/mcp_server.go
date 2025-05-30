// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package mcpconfigs

import (
	"fmt"

	"github.com/apparentlymart/go-versions/versions/constraints"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty/gocty"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

type MCPServer struct {
	// DeclName is the name used to refer to this server slot in the
	// provider configuration block, when specifying how to launch and/or
	// contact this server.
	DeclName WithRange[string]

	// ServerName specifies the exact name that must appear in the
	// serverInfo.name field in the server's initialization response.
	ServerName WithRange[string]
	// AllowedVersions specifies version numbers that are acceptable for the
	// server to return in the serverInfo.version field in its initialization
	// response.
	AllowedVersions WithRange[constraints.IntersectionSpec]

	DeclRange hcl.Range
}

func decodeMCPServer(block *hcl.Block) (*MCPServer, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	ret := &MCPServer{
		DeclName:  withRange(block.Labels[0], block.LabelRanges[0]),
		DeclRange: block.DefRange,
	}

	content, hclDiags := block.Body.Content(mcpServerSchema)
	diags = diags.Append(hclDiags)

	if attr := content.Attributes["name"]; attr != nil {
		nameVal, hclDiags := attr.Expr.Value(nil)
		diags = diags.Append(hclDiags)
		if !hclDiags.HasErrors() {
			var name string
			err := gocty.FromCtyValue(nameVal, &name)
			if err != nil {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid MCP server name",
					Detail:   fmt.Sprintf("Unsupported value for the \"name\" argument: %s.", tfdiags.FormatError(err)),
				})
			}
			ret.ServerName = withRange(name, attr.Expr.Range())
		}
	}

	if attr := content.Attributes["versions"]; attr != nil {
		versionsVal, hclDiags := attr.Expr.Value(nil)
		diags = diags.Append(hclDiags)
		if !hclDiags.HasErrors() {
			var versionsStr string
			err := gocty.FromCtyValue(versionsVal, &versionsStr)
			if err != nil {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid MCP server version constraints",
					Detail:   fmt.Sprintf("Unsupported value for the \"versions\" argument: %s.", tfdiags.FormatError(err)),
				})
			} else {
				allowed, err := constraints.ParseRubyStyleMulti(versionsStr)
				if err != nil {
					diags = diags.Append(&hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Invalid MCP server version constraints",
						Detail:   fmt.Sprintf("Unsupported value for the \"versions\" argument: %s.", tfdiags.FormatError(err)),
					})
				}
				ret.AllowedVersions = withRange(allowed, attr.Expr.Range())
			}
		}
	}

	return ret, diags
}

var mcpServerSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "name", Required: true},
		{Name: "versions"},
	},
}
