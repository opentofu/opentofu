// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package mcpconfigs

import (
	"github.com/hashicorp/hcl/v2"
)

type MCPToolRequest struct {
	Name    WithRange[string]
	ForEach hcl.Expression

	ServerRef hcl.Expression
	ToolName  hcl.Expression
	Arguments hcl.Expression

	DeclRange hcl.Range
}

var _ MCPRequest = (*MCPToolRequest)(nil)

// mcpRequestSigil implements MCPRequest.
func (m *MCPToolRequest) mcpRequestSigil() {}
