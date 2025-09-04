// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"

	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// ProviderInstance represents the configuration for an instance of a provider.
//
// Note that this type's name is slightly misleading because it does not
// represent an already-running provider that requests can be sent to, but
// rather the configuration that should be sent to a running instance of
// this provider in order to prepare it for use. This package does not deal
// with "configured" providers directly at all, instead expecting its caller
// (e.g. an implementation or the plan or apply phase) to handle the provider
// instance lifecycle.
type ProviderInstance struct {
	// Addr is the absolute address of this specific provider instance.
	Addr addrs.AbsProviderInstanceCorrect

	// ProviderAddr is the address of the provider this is an instance of.
	ProviderAddr addrs.Provider

	// TODO: everything else
}

var _ exprs.Valuer = (*ProviderInstance)(nil)

// StaticCheckTraversal implements exprs.Valuer.
func (p *ProviderInstance) StaticCheckTraversal(traversal hcl.Traversal) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if len(traversal) != 0 {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid reference to provider instance",
			Detail:   "A provider instance reference does not have any attributes or elements.",
			Subject:  traversal.SourceRange().Ptr(),
		})
	}
	return diags
}

// Value implements exprs.Valuer.
func (p *ProviderInstance) Value(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	// FIXME: This should wait for the configuration to have been evaluated
	// and validated and then transfer any marks from the configuration into
	// the resulting value to represent that any values produced by this
	// provider might vary by the provider's configuration.
	return ProviderInstanceRefValue(p), nil
}

// ValueSourceRange implements exprs.Valuer.
func (p *ProviderInstance) ValueSourceRange() *tfdiags.SourceRange {
	// TODO: Does it make sense to return the source range of the provider
	// configuration block here, or is that confusing because our value
	// is a reference to a specific instance?
	return nil
}

func (p *ProviderInstance) AnnounceAllGraphevalRequests(announce func(workgraph.RequestID, grapheval.RequestInfo)) {
	// Nothing to announce here yet.
	// TODO: Add the ConfigValuer here once we have it.
}
