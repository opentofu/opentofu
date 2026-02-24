// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package applying

import (
	"context"
	"fmt"
	"log"

	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/shared"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// EphemeralOpen implements [exec.Operations].
func (ops *execOperations) EphemeralOpen(
	ctx context.Context,
	inst *eval.DesiredResourceInstance,
	providerClient *exec.ProviderClient,
) (*exec.OpenEphemeralResourceInstance, tfdiags.Diagnostics) {
	log.Printf("[TRACE] apply phase: EphemeralOpen %s using %s", inst.Addr, providerClient.InstanceAddr)

	var diags tfdiags.Diagnostics

	schema, _ := ops.plugins.ResourceTypeSchema(ctx, inst.Provider, inst.Addr.Resource.Resource.Mode, inst.Addr.Resource.Resource.Type)
	if schema == nil || schema.Block == nil {
		// Should be caught during validation, so we don't bother with a pretty error here
		diags = diags.Append(fmt.Errorf("provider %q does not support ephemeral resource %q", inst.ProviderInstance, inst.Addr.Resource.Resource.Type))
		return nil, diags
	}

	newVal, closeFunc, openDiags := shared.OpenEphemeralResourceInstance(
		ctx,
		inst.Addr,
		schema.Block,
		*inst.ProviderInstance,
		providerClient.Ops,
		inst.ConfigVal,
		shared.EphemeralResourceHooks{},
	)
	diags = diags.Append(openDiags)
	if openDiags.HasErrors() {
		return nil, diags
	}

	state := &exec.ResourceInstanceObject{
		InstanceAddr: inst.Addr,
		State: &states.ResourceInstanceObjectFull{
			Status: states.ObjectReady,
			Value:  newVal,
			// TODO Not sure these fields are needed
			ResourceType:         inst.Addr.Resource.Resource.Type,
			ProviderInstanceAddr: providerClient.InstanceAddr,
			//SchemaVersion:        uint64(schema.Version),
			//Private: resp.Private,
		},
	}

	return &exec.OpenEphemeralResourceInstance{
		State: state,
		Close: closeFunc,
	}, diags
}

// EphemeralState implements [exec.Operations]
func (ops *execOperations) EphemeralState(
	ctx context.Context,
	ephemeralInst *exec.OpenEphemeralResourceInstance,
) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	return ephemeralInst.State, nil
}

// EphemeralClose implements [exec.Operations].
func (ops *execOperations) EphemeralClose(
	ctx context.Context,
	ephemeralInst *exec.OpenEphemeralResourceInstance,
) tfdiags.Diagnostics {
	return ephemeralInst.Close(ctx)
}
