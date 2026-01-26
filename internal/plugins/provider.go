// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plugins

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/providers"
)

type ProviderFactories map[addrs.Provider]providers.Factory

func (p ProviderFactories) HasProvider(addr addrs.Provider) bool {
	_, ok := p[addr]
	return ok
}

func (p ProviderFactories) NewInstance(addr addrs.Provider) (providers.Interface, error) {
	f, ok := p[addr]
	if !ok {
		return nil, fmt.Errorf("unavailable provider %q", addr)
	}

	return f()
}
