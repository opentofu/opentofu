// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plugins

import (
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/provisioners"
)

type Library interface {
	NewProviderInstance(addrs.Provider) (providers.Interface, error)
	NewProvisionerInstance(string) (provisioners.Interface, error)

	HasProvider(addr addrs.Provider) bool
	HasProvisioner(typ string) bool
}

func NewLibrary(providerFactories ProviderFactories, provisionerFactories ProvisionerFactories) Library {
	return &library{
		providerFactories:    providerFactories,
		provisionerFactories: provisionerFactories,
	}
}

type library struct {
	providerFactories    ProviderFactories
	provisionerFactories ProvisionerFactories
	// This is where we will eventually cache provider schemas
}

func (l *library) HasProvider(addr addrs.Provider) bool {
	return l.providerFactories.HasProvider(addr)
}
func (l library) NewProviderInstance(addr addrs.Provider) (providers.Interface, error) {
	return l.providerFactories.NewInstance(addr)
}
func (l *library) HasProvisioner(typ string) bool {
	return l.provisionerFactories.HasProvisioner(typ)
}
func (l library) NewProvisionerInstance(typ string) (provisioners.Interface, error) {
	return l.provisionerFactories.NewInstance(typ)
}
