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
	metadata *exec.ResourceInstanceObjectMeta,
	desired *eval.DesiredResourceInstance,
) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	ret := &exec.ResourceInstanceObject{
		Addr: desired.Addr.CurrentObject(),
	}
	log.Printf("[TRACE] apply phase: DataRead %s using %s", desired.Addr, metadata.ProviderInstance)
	tracer := contextTracer(ctx)
	if cb := tracer.StartDataResourceInstanceRead; cb != nil {
		ctx = cb(ctx, desired.Addr)
	}
	if cb := tracer.EndDataResourceInstanceRead; cb != nil {
		defer func() { // closure to delay evaluating diags until we return
			resultVal := cty.DynamicVal
			if ret.State != nil {
				resultVal = ret.State.Value
			}
			cb(ctx, desired.Addr, resultVal, diags)
		}()
	}

	providerAddr, ok := metadata.ProviderInstance.ValueOk()
	if !ok {
		return ret, diags
	}
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
		return ret, diags
	}

	resourceType := resources.NewDataResourceType(metadata.Provider, metadata.ResourceType, providerClient)
	schema, schemaDiags := resourceType.LoadSchema(ctx)
	diags = diags.Append(schemaDiags)
	if schemaDiags.HasErrors() {
		return ret, diags
	}

	// Note: we do not need to run config validation at this point;
	// the configuration was validated in the plan phase, which produced
	// the opcode to trigger this operation.

	// TODO run PreApply hook here

	// TODO how the hell do we get this????
	var encryption encryption.Encryption

	resp, readDiags := resourceType.Read(ctx, &resources.DataResourceReadRequest{
		ResourceAddress: desired.Addr,
		ConfigValue:     desired.ConfigVal,
	}, desired.Addr.CurrentObject(), encryption)

	diags = diags.Append(readDiags)

	// TODO run PostApply hook here

	if resp.Result == cty.NilVal {
		// TODO handle provider giving us a hard time;
		// From the OG runtime:
		// This can happen with incompletely-configured mocks. We'll allow it
		// and treat it as an alias for a properly-typed null value.
		// resp.Result = cty.NullVal(schema.Block.ImpliedType())
	}

	var state *states.ResourceInstanceObjectFull
	if !resp.Result.IsNull() {
		status := states.ObjectTainted
		if !diags.HasErrors() {
			status = states.ObjectReady
		}

		state = &states.ResourceInstanceObjectFull{
			Status:               status,
			Value:                resp.Result,
			ProviderInstanceAddr: providerAddr,
			ResourceType:         metadata.ResourceType,

			SchemaVersion: uint64(schema.IdentitySchemaVersion),
			// TODO Should we get the plan here, so we can get dependencies?
			// Dependencies: desired.,
		}

		stateSrc, err := states.EncodeResourceInstanceObjectFull(state, schema.Block.ImpliedType())
		if err != nil {
			// TODO maybe a more elegant error handling
			// like, with tfdiags.FormatError(err)
			diags = diags.Append(err)
			return ret, diags
		}
		ops.workingState.SetResourceInstanceObjectFull(desired.Addr.CurrentObject(), stateSrc)

	}

	ret.State = state // nil if the object was deleted

	return ret, diags
}
