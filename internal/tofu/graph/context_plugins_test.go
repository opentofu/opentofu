// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package graph

import (
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/provisioners"
	"github.com/opentofu/opentofu/internal/tofu/testhelpers"
)

// simpleMockPluginLibrary returns a plugin library pre-configured with
// one provider and one provisioner, both called "test".
//
// The provider is built with simpletesthelpers.MockProvider and the provisioner with
// simpletesthelpers.MockProvisioner, and all schemas used in both are as built by
// function simpleTestSchema.
//
// Each call to this function produces an entirely-separate set of objects,
// so the caller can feel free to modify the returned value to further
// customize the mocks contained within.
func simpleMockPluginLibrary() *contextPlugins {
	// We create these out here, rather than in the factory functions below,
	// because we want each call to the factory to return the _same_ instance,
	// so that test code can customize it before passing this component
	// factory into real code under test.
	provider := testhelpers.SimpleMockProvider()
	provisioner := testhelpers.SimpleMockProvisioner()
	ret := newContextPlugins(map[addrs.Provider]providers.Factory{
		addrs.NewDefaultProvider("test"): func() (providers.Interface, error) {
			return provider, nil
		},
	}, map[string]provisioners.Factory{
		"test": func() (provisioners.Interface, error) {
			return provisioner, nil
		},
	})
	return ret
}
