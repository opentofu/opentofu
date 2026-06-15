// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package evalglue

import (
	"context"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Glue is an interface used internally by the lang/eval packages to deal
// with situations where evaluation relies on the results of side-effects
// managed outside of this package, so that our generalized evaluation
// logic can get the information it needs in a generic way.
//
// We export higher-level interfaces like PlanGlue and ApplyGlue that
// are more tailored for specific operations, and then we use implementations
// of [Glue] to adapt that into the minimal set of operations
// that are needed regardless of what overall operation we're currently driving.
type Glue interface {
	// ProviderFunction constructs a cty function given a provider and a function address.
	//
	// This is a bit odd due to how we support functions on configured providers. We pass in both
	// a provider address and a provider instance, preferring a call on the configured provider
	// instance if available.
	ProviderFunction(ctx context.Context, provider addrs.Provider, providerInst configgraph.Maybe[*configgraph.ProviderInstance], pf addrs.ProviderFunction, rng hcl.Range) (function.Function, tfdiags.Diagnostics)

	// ResourceInstanceValue returns the result value for the given resource
	// instance.
	//
	// What "result value" means depends on the phase. For example, during
	// the planning phase it's the "planned new state".
	//
	// The given configVal is the result of calling ConfigValue on the given
	// resource instance object, but guaranteed to have already been validated.
	// The implementation of this method should not call ConfigValue again
	// and should instead just trust the value given as an argument.
	ResourceInstanceValue(ctx context.Context, ri *configgraph.ResourceInstance, configVal cty.Value, providerInst configgraph.Maybe[*configgraph.ProviderInstance], riDeps addrs.Set[addrs.AbsResourceInstance]) (cty.Value, tfdiags.Diagnostics)
}
