// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package mcpconfigs

import (
	"github.com/hashicorp/hcl/v2"
)

type DataResourceType struct {
	Name   WithRange[string]
	Schema *Schema

	ReadMapping *MCPMapping

	DeclRange hcl.Range
}

var dataResourceTypeSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "schema"},
		{Type: "read"},
	},
}
