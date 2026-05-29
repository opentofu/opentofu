// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package applying

import (
	"context"
	"fmt"
	"log"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/engine/internal/execgraph"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/ctymarks"
)

const resourceDependencyMissingDetail = `The value of resource %s was requested by %s during apply, but was not present in the plan.
This may be caused by one of the following options:
  - Ephemeral values requiring different dependencies between plan and apply (unsupported)
  - An edge case of -target or -exclude
  - A bug in OpenTofu

Please inspect your configuration and open a bug report if nessesary.`

func (ops *execOperations) resourceDependenciesMissingCheck(idType string, idName string, cfgVal cty.Value) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	cfgVal, marks := ops.resourceDependenciesMissingMarks(cfgVal)
	for _, mark := range marks {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			fmt.Sprintf("Invalid %s action", idType),
			fmt.Sprintf(resourceDependencyMissingDetail, mark.Target, idName),
		))
	}

	return cfgVal, diags
}

func (ops *execOperations) resourceDependenciesMissingMarks(cfgVal cty.Value) (cty.Value, []execgraph.ResourceInstanceDependencyMissingMark) {
	marksMap := map[execgraph.ResourceInstanceDependencyMissingMark]struct{}{}

	cfgVal, _ = cfgVal.WrangleMarksDeep(func(mark any, path cty.Path) (ctymarks.WrangleAction, error) {
		if ourMark, isOurMark := mark.(execgraph.ResourceInstanceDependencyMissingMark); isOurMark {
			marksMap[ourMark] = struct{}{}
			return ctymarks.WrangleDrop, nil
		}
		return nil, nil // leave all other marks alone
	})

	var marks []execgraph.ResourceInstanceDependencyMissingMark
	for mark := range marksMap {
		marks = append(marks, mark)
	}

	return cfgVal, marks
}

// ResourceInstanceDesired implements [exec.Operations].
func (ops *execOperations) ResourceInstanceDesired(
	ctx context.Context,
	instAddr addrs.AbsResourceInstance,
) (*eval.DesiredResourceInstance, tfdiags.Diagnostics) {
	log.Printf("[TRACE] apply phase: ResourceInstanceDesired %s", instAddr)
	return ops.configOracle.DesiredResourceInstance(ctx, instAddr)
}

// ResourceInstancePrior implements [exec.Operations].
func (ops *execOperations) ResourceInstancePrior(
	ctx context.Context,
	instAddr addrs.AbsResourceInstance,
) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	log.Printf("[TRACE] apply phase: ResourceInstancePrior %s", instAddr)
	return ops.resourceInstanceStateObject(ctx, ops.priorState, instAddr, states.NotDeposed)
}

// ResourceInstancePostconditions implements [exec.Operations].
func (ops *execOperations) ResourceInstancePostconditions(
	ctx context.Context,
	result *exec.ResourceInstanceObject,
) tfdiags.Diagnostics {
	log.Printf("[TRACE] apply phase: ResourceInstancePostconditions (currently just a noop!)")
	// TODO: Implement this by delegating to a special "run resource instance
	// postconditions" method on ops.configOracle.
	return nil
}

// ResourceInstancePrior implements [exec.Operations].
func (ops *execOperations) resourceInstanceStateObject(
	ctx context.Context,
	fromState *states.SyncState,
	instAddr addrs.AbsResourceInstance,
	deposedKey states.DeposedKey,
) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	src := ops.priorState.ResourceInstanceObjectFull(instAddr.Object(deposedKey))
	if src == nil {
		return nil, diags
	}
	// We must decode the resource-type-specific data using the provider's
	// schema for this resource type.
	providerAddr := src.ProviderInstanceAddr.Config.Config.Provider
	schema, moreDiags := ops.plugins.ResourceTypeSchema(
		ctx,
		providerAddr,
		instAddr.Resource.Resource.Mode, // TODO: Make this a direct field of src, as with src.ResourceType, to centralize this rule
		src.ResourceType,
	)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return nil, diags
	}
	state, err := states.DecodeResourceInstanceObjectFull(src, schema.Block.ImpliedType())
	if err != nil {
		nounPhrase := "a current object"
		if deposedKey != states.NotDeposed {
			nounPhrase = "deposed object " + deposedKey.String()
		}
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid resource instance object in prior state",
			fmt.Sprintf(
				"The prior state for %s has %s that doesn't conform to the resource type schema: %s.",
				instAddr, nounPhrase, tfdiags.FormatError(err),
			),
		))
		return nil, diags
	}
	if state == nil {
		return nil, diags
	}
	return &exec.ResourceInstanceObject{
		Addr:  instAddr.Object(deposedKey),
		State: state,
	}, diags
}
