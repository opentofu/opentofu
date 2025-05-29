// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package providers

import (
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
)

type Schemas interface {
	ProviderSchemas() map[addrs.Provider]ProviderSchema
	HasProvider(addr addrs.Provider) bool
	ProviderSchema(addr addrs.Provider) (ProviderSchema, error)
	ProviderConfigSchema(providerAddr addrs.Provider) (*configschema.Block, error)
	ResourceTypeSchema(providerAddr addrs.Provider, resourceMode addrs.ResourceMode, resourceType string) (*configschema.Block, uint64, error)
}
