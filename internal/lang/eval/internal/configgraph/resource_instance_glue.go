// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ResourceInstanceGlue describes a callback API that [ResourceInstance]
// objects use to ask the caller questions about the resource instance whose
// answers vary based on what phase we're currently evaluating for,
// what provider plugins are available, or any other concern that lives
// outside of this package.
//
// Real implementations of these methods are likely to block until some
// side-effects have occured elsewhere, such as asking a provider to produce a
// planned new state. If that external work depends on information coming from
// any other part of this package's API then the implementation of that
// MUST use the mechanisms from package grapheval in order to cooperate
// with the self-dependency detection used by this package to prevent
// deadlocks.
type ResourceInstanceGlue interface {
	// ResultValue returns the results of whatever side-effects are happening
	// for this resource in the current phase, such as getting the "planned new
	// state" of the resource instance during the plan phase, while keeping this
	// package focused only on the general concern of evaluating expressions
	// in the configuration.
	//
	// If this returns error diagnostics then it MUST also return a suitable
	// placeholder unknown value to use when evaluating downstream expressions.
	// If there's not enough information to return anything more precise
	// then returning [cty.DynamicVal] is an acceptable last resort.
	ResultValue(ctx context.Context, configVal cty.Value, providerInst Maybe[*ProviderInstance], riDeps addrs.Set[addrs.AbsResourceInstance]) (cty.Value, tfdiags.Diagnostics)
}
