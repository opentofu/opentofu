// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package applying

import (
	"context"
	"log"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

func (ops *execOperations) runProvisioner(ctx context.Context, objAddr addrs.AbsResourceInstanceObject, prov eval.Provisioner, selfVal cty.Value) (bool, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	tracer := contextTracer(ctx)

	provConfig, moreDiags := prov.Config(ctx, selfVal)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return false, diags
	}

	// Based on NodeAbstractInstance.applyProvisioners

	// Call pre hook
	if cb := tracer.StartProvisionInstanceStep; cb != nil {
		ctx = cb(ctx, objAddr.InstanceAddr, prov.Type)
	}

	// If our config or connection info contains any marked values, ensure
	// those are stripped out before sending to the provisioner. Unlike
	// resources, we have no need to capture the marked paths and reapply
	// later.
	unmarkedConfig, configMarks := provConfig.Value.UnmarkDeep()
	unmarkedConnInfo, _ := provConfig.Connection.UnmarkDeep()

	// The output function passes the config marks to hooks so they can
	// inspect them (e.g. sensitive) and decide how to handle output.
	outputFn := func(string) {}
	if cb := tracer.ProvisionOutput; cb != nil {
		outputFn = func(msg string) {
			cb(ctx, objAddr.InstanceAddr, prov.Type, msg, configMarks)
		}
	}

	resp := ops.plugins.ProvisionResource(
		ctx,
		prov.Type,
		unmarkedConfig,
		unmarkedConnInfo,
		outputFn,
	)
	applyDiags := resp //.InConfigBody(prov.Config, n.Addr.String())

	// Call post hook
	if cb := tracer.StopProvisionInstanceStep; cb != nil {
		cb(ctx, objAddr.InstanceAddr, prov.Type, applyDiags)
	}

	switch prov.OnFailure {
	case configs.ProvisionerOnFailureContinue:
		if applyDiags.HasErrors() {
			log.Printf("[WARN] Errors while provisioning %s with %q, but continuing as requested in configuration", objAddr, prov.Type)
		} else {
			// Maybe there are warnings that we still want to see
			diags = diags.Append(applyDiags)
		}
	default:
		diags = diags.Append(applyDiags)
		if applyDiags.HasErrors() {
			log.Printf("[WARN] Errors while provisioning %s with %q, so aborting", objAddr, prov.Type)
			return false, diags
		}
	}

	return true, diags
}
