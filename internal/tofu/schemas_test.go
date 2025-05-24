// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
)

func simpleTestSchemas() *Schemas {
	provider := simpleMockProvider()
	provisioner := simpleMockProvisioner()

	return &Schemas{
		Providers: map[addrs.Provider]providers.ProviderSchema{
			addrs.NewDefaultProvider("test"): provider.GetProviderSchema(context.TODO()),
		},
		Provisioners: map[string]*configschema.Block{
			"test": provisioner.GetSchemaResponse.Provisioner,
		},
	}
}

// schemaOnlyProvidersForTesting is a testing helper that constructs a
// plugin library that contains a set of providers that only know how to
// return schema, and will exhibit undefined behavior if used for any other
// purpose.
//
// The intended use for this is in testing components that use schemas to
// drive other behavior, such as reference analysis during graph construction,
// but that don't actually need to interact with providers otherwise.
//
// This function automatically preloads the schemas from the providers into
// the cache of the returned object, and so this function is appropriate for
// unit testing of individual components that typically expect someone else
// to have already loaded the schemas, but should not be used for context
// tests where schemas are supposed to be loaded on entry to the Context
// methods, or any other situation where the correctness of the code under
// test includes that it explicitly arranges for schemas to be loaded.
func schemaOnlyProvidersForTesting(schemas map[addrs.Provider]providers.ProviderSchema, t *testing.T) *contextPlugins {
	factories := make(map[addrs.Provider]providers.Factory, len(schemas))

	for providerAddr, schema := range schemas {
		schema := schema

		provider := &MockProvider{
			GetProviderSchemaResponse: &schema,
		}

		factories[providerAddr] = func() (providers.Interface, error) {
			return provider, nil
		}
	}

	ret := newContextPlugins(factories, nil)
	ret.preloadAllProviderSchemasForUnitTest(t)
	return ret
}
