// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package applying

import (
	"context"
	"fmt"
	"log"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/resources"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// DataRead implements [exec.Operations].
func (ops *execOperations) DataRead(
	ctx context.Context,
	desired *eval.DesiredResourceInstance,
) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	log.Printf("[TRACE] apply phase: DataRead %s using %s", desired.Addr, desired.ProviderInstance)
	/*
		// TODO consider adding tracer
		tracer := contextTracer(ctx)
		if cb := tracer.StartDataResourceInstancePlanning; cb != nil {
			ctx = cb(ctx, inst.Addr)
		}
		if cb := tracer.EndDataResourceInstancePlanning; cb != nil {
			defer func() { // closure to delay evaluating diags until we return
				cb(ctx, inst.Addr, diags)
			}()
		}
	*/

	providerAddr := *desired.ProviderInstance
	providerClient, moreDiags := ops.configOracle.ProviderInstance(ctx, providerAddr)
	if providerClient == nil {
		moreDiags = moreDiags.Append(tfdiags.AttributeValue(
			tfdiags.Error,
			"Provider instance not available",
			fmt.Sprintf("Cannot apply %s because its associated provider instance %s cannot initialize.", desired.Addr, providerAddr),
			nil,
		))
	}
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return nil, diags
	}

	resourceType := resources.NewDataResourceType(desired.Provider, desired.Addr.Resource.Resource.Type, providerClient)
	schema, schemaDiags := resourceType.LoadSchema(ctx)
	if schemaDiags.HasErrors() {
		// TODO handle schema errors
	}

	// TODO do we actually need to run config validation at this point???
	// validateDiags := resourceType.ValidateConfig(ctx, inst.ConfigVal)

	// TODO run PreApply hook here

	// TODO how the hell do we get this????
	var encryption encryption.Encryption

	resp, readDiags := resourceType.Read(ctx, &resources.DataResourceReadRequest{
		ResourceAddress: desired.Addr,
		ConfigValue:     desired.ConfigVal,

		// TODO: ProviderMeta is a rarely-used feature that only really makes
		// sense when the module and provider are both written by the same
		// party and the module author is using the provider as a way to
		// transport module usage telemetry. We should decide whether we want
		// to keep supporting that, and if so design a way for the relevant
		// meta value to get from the evaluator into here.
		ProviderMetaValue: cty.NullVal(cty.DynamicPseudoType),
	}, desired.Addr.CurrentObject(), encryption)

	diags = diags.Append(readDiags)

	// TODO run PostApply hook here

	// TODO do stuff with resp and turn it into state, then we return that
	if resp.Result == cty.NilVal {
		// TODO handle provider giving us a hard time
	}

	var state *states.ResourceInstanceObjectFull
	if !resp.Result.IsNull() {
		status := states.ObjectTainted
		if !diags.HasErrors() {
			status = states.ObjectReady
		}

		/*

			state := &states.ResourceInstanceObject{
				Value:  newVal,
				Status: states.ObjectReady,
			}
		*/

		state = &states.ResourceInstanceObjectFull{
			Status:               status,
			Value:                resp.Result,
			ProviderInstanceAddr: providerAddr,
			ResourceType:         desired.Addr.Resource.Resource.Type,

			// TODO what should we get for the schema version????
			SchemaVersion: uint64(0),
			// TODO Should we get the plan here, so we can get dependencies?
			// Dependencies: desired.,
		}

		// TODO handle err
		stateSrc, _ := states.EncodeResourceInstanceObjectFull(state, schema.Block.ImpliedType())
		ops.workingState.SetResourceInstanceObjectFull(desired.Addr.CurrentObject(), stateSrc)

	}

	ret := &exec.ResourceInstanceObject{
		Addr:  desired.Addr.CurrentObject(),
		State: state, // nil if the object was deleted
	}

	return ret, diags
}
