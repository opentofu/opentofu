// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package response

// ModuleProvider represents a single provider for modules.
type ModuleProvider struct {
	Name        string `json:"name"`
	Downloads   int    `json:"downloads"`
	ModuleCount int    `json:"module_count"`
}

// ModuleProviderList is the response structure for a pageable list of ModuleProviders.
type ModuleProviderList struct {
	Meta      PaginationMeta    `json:"meta"`
	Providers []*ModuleProvider `json:"providers"`
}
