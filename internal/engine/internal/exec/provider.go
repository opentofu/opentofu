// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package exec

import (
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/providers"
)

// ProviderInstanceConfig represents a value that can be used to "configure"
// a provider instance, making it ready to support the methods of
// [providers.Configured].
type ProviderInstanceConfig struct {
	// InstanceAddr is the address of the provider instance this configuration
	// was prepared for.
	InstanceAddr addrs.AbsProviderInstanceCorrect

	// ConfigVal is the value that should be sent to the provider instance's
	// "configure" function.
	ConfigVal cty.Value
}

// ProviderClient wraps a [providers.Configured] for an already-running and
// preconfigured provider plugin process, or some equivalent of that.
//
// It extends [providers.Configured] with metadata about the provider instance
// it was created for so that operations consuming this type can record which
// provider instance they made use of, such as when tracking the most recently
// used provider instance for a resource instance in the OpenTofu state.
type ProviderClient struct {
	// InstanceAddr is the address of the provider instance this client was
	// configured for.
	InstanceAddr addrs.AbsProviderInstanceCorrect

	// Ops is the object providing the provider protocol operations for this
	// provider.
	Ops providers.Configured
}
