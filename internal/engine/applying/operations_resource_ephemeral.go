// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package applying

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/shared"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type ephemerals struct {
	closers   addrs.Map[addrs.AbsResourceInstance, shared.EphemeralCloseFunc]
	closersMu sync.Mutex
}

func newEphemerals() *ephemerals {
	return &ephemerals{
		closers: addrs.MakeMap[addrs.AbsResourceInstance, shared.EphemeralCloseFunc](),
	}
}

// EphemeralOpen implements [exec.Operations].
func (ops *execOperations) EphemeralOpen(
	ctx context.Context,
	inst *eval.DesiredResourceInstance,
	providerClient *exec.ProviderClient,
) (*exec.ResourceInstanceObject, tfdiags.Diagnostics) {
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

	ops.ephemerals.closersMu.Lock()
	ops.ephemerals.closers.Put(inst.Addr, closeFunc)
	ops.ephemerals.closersMu.Unlock()

	return &exec.ResourceInstanceObject{
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
	}, diags
}

// EphemeralClose implements [exec.Operations].
func (ops *execOperations) EphemeralClose(
	ctx context.Context,
	inst *exec.ResourceInstanceObject,
	providerClient *exec.ProviderClient,
) tfdiags.Diagnostics {
	log.Printf("[TRACE] apply phase: EphemeralClose %s using %s", inst.InstanceAddr, providerClient.InstanceAddr)

	ops.ephemerals.closersMu.Lock()
	closeFunc := ops.ephemerals.closers.Get(inst.InstanceAddr)
	ops.ephemerals.closersMu.Unlock()

	return closeFunc()
}
