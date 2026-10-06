// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

type Import struct {
	Addr      addrs.ConfigResource
	DeclRange tfdiags.SourceRange

	// InstanceSelector represents a rule for deciding which instances of
	// this import have been declared.
	InstanceSelector InstanceSelector

	// CompileImportInstance is a callback function provided by whatever
	// compiled this [Import] object that knows how to produce a compiled
	// [ImportInstance] object once we know of the instance key and associated
	// repetition data for it.
	//
	// This indirection allows the caller to take into account the same
	// context it had available when it built this [Import] object, while
	// incorporating the new information about this specific instance.
	CompileImportInstance func(ctx context.Context, key addrs.InstanceKey, repData instances.RepetitionData) *ImportInstance

	// instancesResult tracks the process of deciding which instances are
	// currently declared for this import, and the result of that process.
	//
	// Only the decideInstances method accesses this directly. Use that
	// method to obtain the coalesced result for use elsewhere.
	instancesResult grapheval.Once[*compiledInstances[*ImportInstance]]
}

// Instances returns the instances that are selected for this import in its
// configuration, without evaluating their configuration objects yet.
//
// Use this when performing a tree walk to discover import instances to
// make sure that it's possible to tell whatever process is running alongside
// that it needs to produce a result value for a particular import instance
// before we actually request that value.
func (i *Import) Instances(ctx context.Context) (map[addrs.InstanceKey]*ImportInstance, tfdiags.Diagnostics) {
	result, diags := i.instancesResult.Do(ctx, func(ctx context.Context) (*compiledInstances[*ImportInstance], tfdiags.Diagnostics) {
		return compileInstances(ctx, i.InstanceSelector, i.CompileImportInstance)
	})
	if diags.HasErrors() {
		return nil, diags
	}
	return result.Instances, diags
}

func (i *Import) CheckAll(ctx context.Context) tfdiags.Diagnostics {
	instances, diags := i.Instances(ctx)
	var cg CheckGroup
	for _, inst := range instances {
		cg.CheckChild(ctx, inst)
	}
	return diags.Append(cg.Complete(ctx))
}

type ImportInstance struct {
	AddrFunc  func(ctx context.Context) (addrs.AbsResourceInstance, tfdiags.Diagnostics)
	DeclRange tfdiags.SourceRange

	// Provider is the provider that this import's type belongs to. This
	// is the provider to use when asking for config validation, etc.
	Provider addrs.Provider

	// ProviderInstanceValuer is a valuer for producing a value representing
	// the provider instance that this import instance is associated with.
	//
	// This valuer should return a value of the capsule type produced by passing
	// the address from the Provider field into [ProviderInstanceRefType],
	// or else type mismatch errors will be reported during evaluation.
	ProviderInstanceValuer exprs.Valuer

	IDFunc       func(ctx context.Context) (string, tfdiags.Diagnostics)
	IdentityFunc func(ctx context.Context) (cty.Value, tfdiags.Diagnostics)

	result grapheval.Once[*ImportStatement]
}

type ImportStatement struct {
	Addr             addrs.AbsResourceInstance
	DeclRange        tfdiags.SourceRange
	Provider         addrs.Provider
	ProviderInstance exprs.FromValue[*ProviderInstance]
	ID               string
	Identity         cty.Value
}

func (i *ImportInstance) CheckAll(ctx context.Context) tfdiags.Diagnostics {
	_, diags := i.Statement(ctx)
	return diags
}

func (i *ImportInstance) Statement(ctx context.Context) (*ImportStatement, tfdiags.Diagnostics) {
	return i.result.Do(ctx, func(ctx context.Context) (*ImportStatement, tfdiags.Diagnostics) {
		var diags tfdiags.Diagnostics
		statement := &ImportStatement{
			DeclRange: i.DeclRange,
			Provider:  i.Provider,
		}
		// TODO unmarking

		var moreDiags tfdiags.Diagnostics
		statement.Addr, moreDiags = i.AddrFunc(ctx)
		diags = diags.Append(moreDiags)

		if i.ProviderInstanceValuer != nil {
			statement.ProviderInstance, moreDiags = DecodeProviderInstance(ctx, i.ProviderInstanceValuer, i.Provider, statement.Addr.String())
			diags = diags.Append(moreDiags)
		}

		if i.IDFunc != nil {
			statement.ID, moreDiags = i.IDFunc(ctx)
			diags = diags.Append(moreDiags)
		}
		if i.IdentityFunc != nil {
			statement.Identity, moreDiags = i.IdentityFunc(ctx)
			diags = diags.Append(moreDiags)
		}

		if diags.HasErrors() {
			return nil, diags
		}
		return statement, diags
	})
}
