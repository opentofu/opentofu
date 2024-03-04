// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import "github.com/hashicorp/hcl/v2"

type StateEncryption struct {
	Type   string
	Config hcl.Body

	TypeRange hcl.Range
	DeclRange hcl.Range
}
