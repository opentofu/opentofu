// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"testing"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/provisioners"
)

// simpleMockPluginLibrary returns a plugin library pre-configured with
// one provider and one provisioner, both called "test".
//
// The provider is built with simpleMockProvider and the provisioner with
// simpleMockProvisioner, and all schemas used in both are as built by
// function simpleTestSchema.
//
// Each call to this function produces an entirely-separate set of objects,
// so the caller can feel free to modify the returned value to further
// customize the mocks contained within.
func simpleMockPluginLibrary(t *testing.T) *contextPlugins {
	// We create these out here, rather than in the factory functions below,
	// because we want each call to the factory to return the _same_ instance,
	// so that test code can customize it before passing this component
	// factory into real code under test.
	provider := simpleMockProvider()
	provisioner := simpleMockProvisioner()
	ret := &contextPlugins{
		providerFactories: map[addrs.Provider]providers.Factory{
			addrs.NewDefaultProvider("test"): func() (providers.Interface, error) {
				return provider, nil
			},
		},
		provisionerFactories: map[string]provisioners.Factory{
			"test": func() (provisioners.Interface, error) {
				return provisioner, nil
			},
		},
	}
	ret.preloadAllProviderSchemasForUnitTest(t)
	return ret
}

// simpleTestSchema returns a block schema that contains a few optional
// attributes for use in tests.
//
// The returned schema contains the following optional attributes:
//
//   - test_string, of type string
//   - test_number, of type number
//   - test_bool, of type bool
//   - test_list, of type list(string)
//   - test_map, of type map(string)
//
// Each call to this function produces an entirely new schema instance, so
// callers can feel free to modify it once returned.
func simpleTestSchema() *configschema.Block {
	return &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"test_string": {
				Type:     cty.String,
				Optional: true,
			},
			"test_number": {
				Type:     cty.Number,
				Optional: true,
			},
			"test_bool": {
				Type:     cty.Bool,
				Optional: true,
			},
			"test_list": {
				Type:     cty.List(cty.String),
				Optional: true,
			},
			"test_map": {
				Type:     cty.Map(cty.String),
				Optional: true,
			},
		},
	}
}

// preloadAllProviderSchemasForUnitTest is a unit-testing-only helper method
// that simulates the effect of calling [contextPlugins.LoadProviderSchemas]
// with a configuration that makes use of all of the providers that are
// available in this [contextPlugins] object.
//
// This is only for use in unit tests for components that typically expect
// that some other part of the system will have preloaded the schemas they
// need. It should not be used in context tests because the exported entrypoints
// of [Context] are supposed to arrange themselves for schemas to be loaded.
func (cp *contextPlugins) preloadAllProviderSchemasForUnitTest(t *testing.T) {
	cp.cache.preloadAllProviderSchemasForUnitTest(t, cp.providerFactories)
}
