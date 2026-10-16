// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type Import struct {
	// Addr is the static address we can infer from the configuration
	// without performing evaluation.
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
	// Only the Instances method accesses this directly. Use that
	// method to obtain the coalesced result for use elsewhere.
	instancesResult grapheval.Once[*compiledInstances[*ImportInstance]]
}

// Instances returns the instances that are selected for this import in its
// configuration, without evaluating their configuration objects yet.
func (i *Import) Instances(ctx context.Context) (map[addrs.InstanceKey]*ImportInstance, tfdiags.Diagnostics) {
	result, diags := i.instancesResult.Do(ctx, func(ctx context.Context) (*compiledInstances[*ImportInstance], tfdiags.Diagnostics) {
		return compileInstances(ctx, i.InstanceSelector, i.CompileImportInstance)
	}, &i.DeclRange, "import instances of %s", i.Addr)
	if diags.HasErrors() {
		return nil, diags
	}
	return result.Instances, diags
}

// CheckAll implements allChecker.
func (i *Import) CheckAll(ctx context.Context) tfdiags.Diagnostics {
	instances, diags := i.Instances(ctx)
	var cg CheckGroup
	for _, inst := range instances {
		cg.CheckChild(ctx, inst)
	}
	return diags.Append(cg.Complete(ctx))
}
