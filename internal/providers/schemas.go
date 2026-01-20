// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package providers

import (
	"fmt"
	"sync"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
)

// ProviderSchema is an overall container for all of the schemas for all
// configurable objects defined within a particular provider. All storage of
// provider schemas should use this type.
type ProviderSchema = GetProviderSchemaResponse

// SchemaForResourceType attempts to find a schema for the given mode and type.
// Returns nil if no such schema is available.
func (ss ProviderSchema) SchemaForResourceType(mode addrs.ResourceMode, typeName string) (schema *configschema.Block, version uint64) {
	switch mode {
	case addrs.ManagedResourceMode:
		res := ss.ResourceTypes[typeName]
		return res.Block, uint64(res.Version)
	case addrs.DataResourceMode:
		// Data resources don't have schema versions right now, since state is discarded for each refresh
		return ss.DataSources[typeName].Block, 0
	case addrs.EphemeralResourceMode:
		// Ephemeral resources don't have schema versions right now, since state is discarded for each refresh
		return ss.EphemeralResources[typeName].Block, 0
	default:
		// Shouldn't happen, because the above cases are comprehensive.
		return nil, 0
	}
}

// SchemaForResourceAddr attempts to find a schema for the mode and type from
// the given resource address. Returns nil if no such schema is available.
func (ss ProviderSchema) SchemaForResourceAddr(addr addrs.Resource) (schema *configschema.Block, version uint64) {
	return ss.SchemaForResourceType(addr.Mode, addr.Type)
}

func (resp ProviderSchema) Validate(addr addrs.Provider) error {
	if resp.Diagnostics.HasErrors() {
		return fmt.Errorf("failed to retrieve schema from provider %q: %w", addr, resp.Diagnostics.Err())
	}

	if resp.Provider.Version < 0 {
		// We're not using the version numbers here yet, but we'll check
		// for validity anyway in case we start using them in future.
		return fmt.Errorf("provider %s has invalid negative schema version for its configuration blocks,which is a bug in the provider", addr)
	}

	for t, r := range resp.ResourceTypes {
		if err := r.Block.InternalValidate(); err != nil {
			return fmt.Errorf("provider %s has invalid schema for managed resource type %q, which is a bug in the provider: %w", addr, t, err)
		}
		if r.Version < 0 {
			return fmt.Errorf("provider %s has invalid negative schema version for managed resource type %q, which is a bug in the provider", addr, t)
		}
	}

	for t, d := range resp.DataSources {
		if err := d.Block.InternalValidate(); err != nil {
			return fmt.Errorf("provider %s has invalid schema for data resource type %q, which is a bug in the provider: %w", addr, t, err)
		}
		if d.Version < 0 {
			// We're not using the version numbers here yet, but we'll check
			// for validity anyway in case we start using them in future.
			return fmt.Errorf("provider %s has invalid negative schema version for data resource type %q, which is a bug in the provider", addr, t)
		}
	}

	for t, d := range resp.EphemeralResources {
		if err := d.Block.InternalValidate(); err != nil {
			return fmt.Errorf("provider %s has invalid schema for ephemeral resource type %q, which is a bug in the provider: %w", addr, t, err)
		}
		if d.Version < 0 {
			// We're not using the version numbers here yet, but we'll check
			// for validity anyway in case we start using them in future.
			return fmt.Errorf("provider %s has invalid negative schema version for ephemeral resource type %q, which is a bug in the provider", addr, t)
		}
	}

	return nil
}

type SchemaCache func(func() ProviderSchema) ProviderSchema

func NewSchemaCache() SchemaCache {
	var once sync.Once
	var schema ProviderSchema

	return func(getSchema func() ProviderSchema) ProviderSchema {
		once.Do(func() {
			schema = getSchema()
		})
		return schema
	}
}
