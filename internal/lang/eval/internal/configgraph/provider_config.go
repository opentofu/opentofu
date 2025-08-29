// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"github.com/opentofu/opentofu/internal/addrs"
)

type ProviderConfig struct {
	// FIXME: The current form of AbsProviderConfig is weird and not quite
	// right, because the "Abs" prefix is supposed to represent something
	// belonging to an addrs.ModuleInstance while this models addrs.Module
	// instead. We'll probably need to introduce some temporary new types
	// alongside the existing ones for the sake of this experiment, and then
	// have the new ones replace the old ones if we decide to move forward
	// with something like this.
	Addr addrs.AbsProviderConfig

	// ProviderAddr is the address of the provider that this is a configuration
	// for. This object can produce zero or more instances of this provider.
	ProviderAddr addrs.Provider
}
