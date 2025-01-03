// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package externalcommand

// Metadata describes the metadata structure of the external provider.
type Metadata struct {
	ExternalData map[string]any `hcl:"external_data" json:"external_data"`
}
