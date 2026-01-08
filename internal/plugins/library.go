// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plugins

import (
	"sync"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Library represents a suite of provider and provisioner plugins. It does not expose
// much functionality itself, instead serving as a starting point for the more complex
// managers.
type Library interface {
	NewProviderManager() ProviderManager
	NewProvisionerManager() ProvisionerManager

	HasProvider(addr addrs.Provider) bool
	HasProvisioner(typ string) bool
}

func NewLibrary(providerFactories ProviderFactories, provisionerFactories ProvisionerFactories) Library {
	return &library{
		providerFactories: providerFactories,
		providerSchemas:   map[addrs.Provider]*providerSchemaEntry{},

		provisionerFactories: provisionerFactories,
		provisionerSchemas:   map[string]*provisionerSchemaEntry{},
	}
}

// library is the default Library implementation, with included fields to facilitate
// schema caching among managers.
type library struct {
	providerSchemasLock sync.Mutex
	providerSchemas     map[addrs.Provider]*providerSchemaEntry
	providerFactories   ProviderFactories

	provisionerSchemasLock sync.Mutex
	provisionerSchemas     map[string]*provisionerSchemaEntry
	provisionerFactories   ProvisionerFactories
}

type providerSchemaEntry struct {
	sync.Mutex
	populated bool

	schema providers.ProviderSchema
	diags  tfdiags.Diagnostics
}

type provisionerSchemaEntry struct {
	sync.Mutex
	populated bool

	schema *configschema.Block
	err    error
}

func (l *library) HasProvider(addr addrs.Provider) bool {
	return l.providerFactories.HasProvider(addr)
}
func (l *library) HasProvisioner(typ string) bool {
	return l.provisionerFactories.HasProvisioner(typ)
}
