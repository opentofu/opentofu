// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
)

// ProviderInstanceConfig provides the information needed to either instantiate
// and configure a provider instance for the first time or to find a
// previously-configured object for the same provider instance.
type ProviderInstanceConfig struct {
	// Addr is the address for the provider instance, unique across the whole
	// configuration.
	//
	// This is a good key to use for a table of previously-configured provider
	// objects.
	Addr addrs.AbsProviderInstanceCorrect

	// ConfigVal is the configuration value to send to the provider when
	// configuring it. The relationship between Addr and ConfigVal is
	// guaranteed to be consistent for all ProviderInstanceConfig objects
	// produced through a particular [ConfigInstance], and so it's safe
	// to reuse a previously-configured provider (and thus ignore ConfigVal)
	// when the address matches.
	ConfigVal cty.Value
}
