// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package mcpconfigs

import (
	"github.com/hashicorp/hcl/v2"
)

type MCPResourceRequest struct {
	Name    WithRange[string]
	ForEach hcl.Expression

	ResourceURI hcl.Expression

	DeclRange hcl.Range
}

var _ MCPRequest = (*MCPResourceRequest)(nil)

// mcpRequestSigil implements MCPRequest.
func (m *MCPResourceRequest) mcpRequestSigil() {}
