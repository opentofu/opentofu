// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"

	"github.com/apparentlymart/go-versions/versions"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ModuleCallInstanceGlue describes a callback API that [ModuleCallInstance]
// objects use to ask the caller questions about the module that's being
// called.
//
// Real implementations of this interface will sometimes block on fetching
// a remote module package for inspection, or on operations caused by
// declarations in the child module. If that external work depends on
// information coming from any other part of this package's API then the
// implementation of that MUST use the mechanisms from package grapheval in
// order to cooperate with the self-dependency detection used by this package to
// prevent deadlocks.
type ModuleCallInstanceGlue interface {
	// ValidateInputs determines whether the given value is a valid
	// representation of the inputs to the target module, returning diagnostics
	// describing any problems.
	//
	// TODO: This probably also needs an argument for describing the
	// "sidechannel" provider instances, as would be specified in the "providers"
	// meta-argument in the current language, so the callee can also check
	// those.
	ValidateInputs(ctx context.Context, inputsVal cty.Value) tfdiags.Diagnostics

	// OutputsValue returns the value representing the outputs of this module
	// instance. This is what should be returned as the value of the module
	// instance.
	//
	// Real implementations of this will tend to indirectly depend on the
	// [ModuleCallInstance.InputsValue] method of the module call instance
	// that this glue object belongs to, but exactly what happens between
	// those two is outside of this package's scope of responsibility.
	OutputsValue(ctx context.Context) (cty.Value, tfdiags.Diagnostics)
}

type ModuleSourceArguments struct {
	// Source is the already-parsed-and-normalized module source address.
	Source addrs.ModuleSource

	// AllowedVersions describes what subset of the available versions are
	// accepted, if the source type is one that supports version constraints.
	//
	// It's the responsibility of the [ModuleCall] logic to reject attempts
	// to set a version constraint for a source type that doesn't support
	// it, so a [ModuleSourceArguments] object should not be constructed
	// with a nonzero value in this field when [Source] is not of a
	// version-aware source type.
	AllowedVersions versions.Set
}
