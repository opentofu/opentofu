// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"github.com/hashicorp/hcl/v2"
)

type StateStorage struct {
	Type      string
	TypeRange hcl.Range
	Name      string
	NameRange hcl.Range

	Provider hcl.Expression

	DeclRange hcl.Range
}

func DecodeStateStorageBlock(block *hcl.Block) (*StateStorage, hcl.Diagnostics) {
	panic("not yet implemented")
}
