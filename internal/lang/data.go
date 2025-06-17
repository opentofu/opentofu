// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package lang

import (
	"context"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Data is an interface whose implementations can provide cty.Value
// representations of objects identified by referenceable addresses from
// the addrs package.
//
// This interface will grow each time a new type of reference is added, and so
// implementations outside of the OpenTofu codebases are not advised.
//
// Each method returns a suitable value and optionally some diagnostics. If the
// returned diagnostics contains errors then the type of the returned value is
// used to construct an unknown value of the same type which is then used in
// place of the requested object so that type checking can still proceed. In
// cases where it's not possible to even determine a suitable result type,
// cty.DynamicVal is returned along with errors describing the problem.
type Data interface {
	StaticValidateReferences(ctx context.Context, refs []*addrs.Reference, self addrs.Referenceable, source addrs.Referenceable) tfdiags.Diagnostics

	GetCountAttr(context.Context, addrs.CountAttr, tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics)
	GetForEachAttr(context.Context, addrs.ForEachAttr, tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics)
	GetResource(context.Context, addrs.Resource, tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics)
	GetLocalValue(context.Context, addrs.LocalValue, tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics)
	GetModule(context.Context, addrs.ModuleCall, tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics)
	GetPathAttr(context.Context, addrs.PathAttr, tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics)
	GetTerraformAttr(context.Context, addrs.TerraformAttr, tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics)
	GetInputVariable(context.Context, addrs.InputVariable, tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics)
	GetOutput(context.Context, addrs.OutputValue, tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics)
	GetCheckBlock(context.Context, addrs.Check, tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics)
}
