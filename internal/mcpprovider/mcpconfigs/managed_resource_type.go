// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package mcpconfigs

import (
	"github.com/hashicorp/hcl/v2"
)

type ManagedResourceType struct {
	Name   WithRange[string]
	Schema *Schema

	CreateMapping *MCPMapping
	ReadMapping   *MCPMapping
	UpdateMapping *MCPMapping
	DeleteMapping *MCPMapping

	DeclRange hcl.Range
}

var managedResourceTypeSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "schema"},
		{Type: "create"},
		{Type: "read"},
		{Type: "update"},
		{Type: "delete"},
	},
}
