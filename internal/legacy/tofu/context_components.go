// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/provisioners"
)

// contextComponentFactory is the interface that Context uses
// to initialize various components such as providers and provisioners.
// This factory gets more information than the raw maps using to initialize
// a Context. This information is used for debugging.
type contextComponentFactory interface {
	// ResourceProvider creates a new ResourceProvider with the given type.
	ResourceProvider(typ addrs.Provider) (providers.Interface, error)
	ResourceProviders() []string

	// ResourceProvisioner creates a new ResourceProvisioner with the given
	// type.
	ResourceProvisioner(typ string) (provisioners.Interface, error)
	ResourceProvisioners() []string
}

// basicComponentFactory just calls a factory from a map directly.
type basicComponentFactory struct {
	providers    map[addrs.Provider]providers.Factory
	provisioners map[string]ProvisionerFactory
}

func (c *basicComponentFactory) ResourceProviders() []string {
	var result []string
	for k := range c.providers {
		result = append(result, k.String())
	}
	return result
}

func (c *basicComponentFactory) ResourceProvisioners() []string {
	var result []string
	for k := range c.provisioners {
		result = append(result, k)
	}

	return result
}

func (c *basicComponentFactory) ResourceProvider(typ addrs.Provider) (providers.Interface, error) {
	panic("legacy")
}

func (c *basicComponentFactory) ResourceProvisioner(typ string) (provisioners.Interface, error) {
	panic("legacy")
}
