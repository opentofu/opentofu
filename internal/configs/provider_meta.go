// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import "github.com/hashicorp/hcl/v2"

// ProviderMeta represents a "provider_meta" block inside a "terraform" block
// in a module or file.
type ProviderMeta struct {
	Provider string
	Config   hcl.Body

	ProviderRange hcl.Range
	DeclRange     hcl.Range
}
