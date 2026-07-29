// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package applying

import (
	"context"
	"log"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func (ops *execOperations) runProvisioner(ctx context.Context, objAddr addrs.AbsResourceInstanceObject, prov *eval.ResourceProvisioner, selfVal cty.Value) (bool, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	tracer := contextTracer(ctx)
	log.Printf("[TRACE] apply phase: running %q provisioner for %s", prov.Type, objAddr)

	provConfig, moreDiags := prov.BuildConfig(ctx, selfVal)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return false, diags
	}

	// Call pre hook
	if cb := tracer.StartProvisionInstanceStep; cb != nil {
		ctx = cb(ctx, objAddr.InstanceAddr, prov.Type)
	}

	// If our config or connection info contains any marked values, ensure
	// those are stripped out before sending to the provisioner. Unlike
	// resources, we have no need to capture the marked paths and reapply
	// later.
	unmarkedConfig, configMarks := provConfig.MainConfig.UnmarkDeep()
	unmarkedConnInfo, _ := provConfig.ConnectionConfig.UnmarkDeep()

	// The output function passes the config marks to hooks so they can
	// inspect them (e.g. sensitive) and decide how to handle output.
	outputFn := func(string) {}
	if cb := tracer.ProvisionOutput; cb != nil {
		outputFn = func(msg string) {
			cb(ctx, objAddr.InstanceAddr, prov.Type, msg, configMarks)
		}
	}

	// FIXME: this function is allowed to return contextual diagnostics
	// that use attribute paths instead of source locations, and so we
	// ought to actually resolve them to their approximate source locations
	// before we return here. We don't have direct access to the HCL body
	// but perhaps we could add a new callback to [eval.ResourceProvisioner]
	// that is responsible for finalizing the diagnostics, and then the
	// tofu2024 package can implement that in terms of the real HCL body?
	applyDiags := ops.plugins.ProvisionResource(
		ctx,
		prov.Type,
		unmarkedConfig,
		unmarkedConnInfo,
		outputFn,
	)

	// Call post hook
	if cb := tracer.StopProvisionInstanceStep; cb != nil {
		cb(ctx, objAddr.InstanceAddr, prov.Type, applyDiags)
	}

	if prov.ContinueOnFailure {
		if applyDiags.HasErrors() {
			log.Printf("[WARN] Errors while provisioning %s with %q, but continuing as requested in configuration", objAddr, prov.Type)
		} else {
			// Maybe there are warnings that we still want to see
			diags = diags.Append(applyDiags)
		}
		return true, diags
	}

	diags = diags.Append(applyDiags)
	if applyDiags.HasErrors() {
		log.Printf("[WARN] Errors while provisioning %s with %q, so aborting", objAddr, prov.Type)
		return false, diags
	}
	return true, diags
}
