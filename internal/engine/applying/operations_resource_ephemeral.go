// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package applying

import (
	"context"
	"log"

	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// EphemeralOpen implements [exec.Operations].
func (ops *execOperations) EphemeralOpen(
	ctx context.Context,
	inst *eval.DesiredResourceInstance,
	providerClient *exec.ProviderClient,
) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
	log.Printf("[TRACE] apply phase: EphemeralOpen %s using %s", inst.Addr, providerClient.InstanceAddr)

	var diags tfdiags.Diagnostics

	validateDiags := ops.plugins.ValidateResourceConfig(ctx, inst.Provider, inst.Addr.Resource.Resource.Mode, inst.Addr.Resource.Resource.Type, inst.ConfigVal)
	diags = diags.Append(validateDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	resp := providerClient.Ops.OpenEphemeralResource(ctx, providers.OpenEphemeralResourceRequest{
		TypeName: inst.Addr.Resource.Resource.Type,
		Config:   inst.ConfigVal,
	})
	diags = diags.Append(resp.Diagnostics)
	if diags.HasErrors() {
		return nil, diags
	}
	// TODO refresher
	/*ops.ephemeralInstancesMu.Lock()
	ops.ephemeralInstances.Put(inst.Addr, &ephemeralInstance{})
	ops.ephemeralInstancesMu.Unlock()*/

	return &exec.ResourceInstanceObject{
		InstanceAddr: inst.Addr,
		State: &states.ResourceInstanceObjectFull{
			Status:  states.ObjectReady,
			Value:   resp.Result,
			Private: resp.Private,
			// TODO Not sure these fields are needed
			ResourceType:         inst.Addr.Resource.Resource.Type,
			ProviderInstanceAddr: providerClient.InstanceAddr,
			//SchemaVersion:        uint64(schema.Version),
		},
	}, diags
}

// EphemeralClose implements [exec.Operations].
func (ops *execOperations) EphemeralClose(
	ctx context.Context,
	inst *exec.ResourceInstanceObject,
	providerClient *exec.ProviderClient,
) tfdiags.Diagnostics {
	log.Printf("[TRACE] apply phase: EphemeralClose %s using %s", inst.InstanceAddr, providerClient.InstanceAddr)

	/*ops.ephemeralInstancesMu.Lock()
	instance := ops.ephemeralInstances.Get(inst.InstanceAddr)
	ops.ephemeralInstancesMu.Unlock()*/

	closeResp := providerClient.Ops.CloseEphemeralResource(ctx, providers.CloseEphemeralResourceRequest{
		TypeName: inst.InstanceAddr.Resource.Resource.Type,
		Private:  inst.State.Private,
	})
	return closeResp.Diagnostics
}
