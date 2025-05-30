// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package mcpconfigs

import (
	"github.com/hashicorp/hcl/v2"
)

// MCPMapping describes how to implement a provider operation via one or more
// MCP requests, with the results then projected into the corresponding resource
// type's result attributes.
type MCPMapping struct {
	Calls          map[string]MCPRequest
	Locals         map[string]hcl.Expression
	PlannedResults map[string]hcl.Expression
	FinalResults   map[string]hcl.Expression

	DeclRange hcl.Range
}

// MCPRequest is the interface that [MCPResourceRequest] and [MCPToolRequest]
// have in common. There are no other implementations of this interface.
type MCPRequest interface {
	mcpRequestSigil()
}
