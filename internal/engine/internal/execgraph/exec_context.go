// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package execgraph

import (
	"context"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Implementations of ExecContext allow a [CompiledGraph] to interact with other
// parts of OpenTofu during execution.
//
// The apply engine has the main implementation of this type, but it's an
// interface to make it possible to test the functionality in this package
// without depending directly on other components.
//
// The conventional variable name for a value of this type is "execCtx".
type ExecContext interface {
	// DesiredResourceInstance returns the [DesiredResourceInstance]
	// representation of the resource instance with the given address, or nil
	// if the requested address is not part of the desired state described
	// by the configuration.
	//
	// If this returns nil during real execution then that suggests a bug in
	// the planning engine, because it should only generate
	// desired-resource-instance operations for resource instances that actually
	// appeared in the desired state during the planning process.
	DesiredResourceInstance(ctx context.Context, addr addrs.AbsResourceInstance) *eval.DesiredResourceInstance

	// ResourceInstancePriorState returns either the current object or a deposed
	// object associated with the given resource instance address, or nil if
	// the requested object was not tracked in the desired state.
	//
	// Set deposedKey to [states.NotDeposed] to retrieve the current object
	// from the prior state.
	//
	// If this returns nil during real execution then that suggests a bug in
	// the planning engine, because it should only generate requests for
	// prior state objects that were present and valid in the refreshed state
	// during the planning step.
	ResourceInstancePriorState(ctx context.Context, addr addrs.AbsResourceInstance, deposedKey states.DeposedKey) *states.ResourceInstanceObject

	// ProviderInstanceConfig returns the value that should be sent when
	// configuring the specified provider instance, or [cty.NilVal] if
	// no such provider instance is declared.
	//
	// If this returns cty.NilVal during real execution then that suggests
	// a bug in the planning engine, because it should not generate an execution
	// graph that attempts to use an undeclared provider instance.
	ProviderInstanceConfig(ctx context.Context, addr addrs.AbsProviderInstanceCorrect) cty.Value

	// NewProviderClient returns a preconfigured client for the given provider,
	// using configVal as its configuration.
	//
	// If the provider refuses the configuration, or launching and configuring
	// the provider fails for any other reason, the returned diagnostics
	// contain end-user-oriented errors describing the problem(s).
	//
	// Each call to NewProviderClient returns a separate provider client. The
	// implementation should not attempt to reuse clients across multiple calls
	// to this method.
	NewProviderClient(ctx context.Context, addr addrs.Provider, configVal cty.Value) (providers.Configured, tfdiags.Diagnostics)
}
