// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package shared

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// This may be modified to expedite tests
var EphemeralResourceCloseTimeout = 10 * time.Second

type EphemeralCloseFunc func(context.Context) tfdiags.Diagnostics

func OpenEphemeralResourceInstance(
	ctx context.Context,
	// TODO once we remove the old engine, this can be condensed using the new engine types
	addr addrs.AbsResourceInstance,
	schema *configschema.Block,
	providerAddr addrs.AbsProviderInstanceCorrect,
	provider providers.Interface,
	configVal cty.Value,
) (cty.Value, EphemeralCloseFunc, tfdiags.Diagnostics) {
	var newVal cty.Value
	var diags tfdiags.Diagnostics

	tracer := contextTracer(ctx)

	// Unmark before sending to provider, will re-mark before returning
	configVal, pvm := configVal.UnmarkDeepWithPaths()

	log.Printf("[TRACE] OpenEphemeralResourceInstance: Re-validating config for %s", addr)
	validateResp := provider.ValidateEphemeralConfig(
		ctx,
		providers.ValidateEphemeralConfigRequest{
			TypeName: addr.ContainingResource().Resource.Type,
			Config:   configVal,
		},
	)
	diags = diags.Append(validateResp.Diagnostics)
	if diags.HasErrors() {
		return newVal, nil, diags
	}

	// If we get down here then our configuration is complete and we're ready
	// to actually call the provider to open the ephemeral resource.
	log.Printf("[TRACE] OpenEphemeralResourceInstance: %s configuration is complete, so calling the provider", addr)

	openCtx := ctx
	if cb := tracer.StartEphemeralResourceInstanceOpen; cb != nil {
		openCtx = cb(ctx, addr)
	}

	openReq := providers.OpenEphemeralResourceRequest{
		TypeName: addr.ContainingResource().Resource.Type,
		Config:   configVal,
	}
	openResp := provider.OpenEphemeralResource(openCtx, openReq)
	diags = diags.Append(openResp.Diagnostics)
	if diags.HasErrors() {
		if cb := tracer.EndEphemeralResourceInstanceOpen; cb != nil {
			cb(openCtx, addr, diags)
		}
		return newVal, nil, diags
	}

	newVal = openResp.Result

	// Encapsulate validation for easier close handling
	func() {
		for _, err := range newVal.Type().TestConformance(schema.ImpliedType()) {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Provider produced invalid object",
				fmt.Sprintf(
					"Provider %q produced an invalid value for %s.\n\nThis is a bug in the provider, which should be reported in the provider's own issue tracker.",
					providerAddr, tfdiags.FormatErrorPrefixed(err, addr.String()),
				),
			))
		}
		if diags.HasErrors() {
			return
		}

		if newVal.IsNull() {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Provider produced null object",
				fmt.Sprintf(
					"Provider %q produced a null value for %s.\n\nThis is a bug in the provider, which should be reported in the provider's own issue tracker.",
					providerAddr, addr,
				),
			))
			return
		}

		if !newVal.IsNull() && !newVal.IsWhollyKnown() {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Provider produced invalid object",
				fmt.Sprintf(
					"Provider %q produced a value for %s that is not wholly known.\n\nThis is a bug in the provider, which should be reported in the provider's own issue tracker.",
					providerAddr, addr,
				),
			))
			return
		}
	}()

	if cb := tracer.EndEphemeralResourceInstanceOpen; cb != nil {
		cb(openCtx, addr, diags)
	}

	if diags.HasErrors() {
		// We have an open ephemeral resource, but don't plan to use it due to validation errors
		// It needs to be closed before we can return
		// TODO: We should probably call
		// tracer.StartEphemeralResourceInstanceClose and
		// tracer.EndEphemeralResourceInstanceClose around this early close.

		closReq := providers.CloseEphemeralResourceRequest{
			TypeName: addr.Resource.Resource.Type,
			Private:  openResp.Private,
		}
		closeResp := provider.CloseEphemeralResource(ctx, closReq)
		diags = diags.Append(closeResp.Diagnostics)

		return newVal, nil, diags
	}

	// TODO see if this conflicts with anything in the new engine?
	if len(pvm) > 0 {
		newVal = newVal.MarkWithPaths(pvm)
	}

	// Initialize the closing channel and the channel that sends diagnostics back to the close caller.
	closeCh := make(chan context.Context, 1)
	diagsCh := make(chan tfdiags.Diagnostics, 1)
	go func() {
		// The writer is responsible with closing the channel, not the receiver.
		defer func() {
			close(diagsCh)
		}()
		var diags tfdiags.Diagnostics
		renewAt := openResp.RenewAt
		privateData := openResp.Private

		closeCtx := ctx

		// We have two exit paths that should take the same route
		func() {
			for {
				// This is necessary to block on the select statement in 2 cases:
				//  - if renewAt == nil, then the renewal process is disabled and we
				//    want to wait for the close call or ctx.Done() so we return a nil
				//    chan that will block the select statement for the other cases
				//  - if renewAt != nil, we want to execute the renewal at the given interval
				//    so we return a channel that will trigger after the given interval
				waitForRenewal := func() <-chan time.Time {
					if renewAt != nil {
						return time.After(time.Until(*renewAt))
					}
					return nil
				}
				select {
				case <-waitForRenewal():
					renewCtx := ctx
					if cb := tracer.StartEphemeralResourceInstanceRenew; cb != nil {
						renewCtx = cb(ctx, addr)
					}

					renewReq := providers.RenewEphemeralResourceRequest{
						TypeName: addr.Resource.Resource.Type,
						Private:  privateData,
					}
					renewResp := provider.RenewEphemeralResource(renewCtx, renewReq)
					diags = diags.Append(renewResp.Diagnostics)

					// TODO consider what happens if renew fails, do we still want to update private?
					renewAt = renewResp.RenewAt
					privateData = renewResp.Private
					if cb := tracer.EndEphemeralResourceInstanceRenew; cb != nil {
						cb(renewCtx, addr, diags)
					}
				case closeCtx = <-closeCh:
					return
				case <-ctx.Done():
					// Even though the context is "Done" we still want to execute the close operation
					closeCtx = context.WithoutCancel(closeCtx)
					return
				}
			}
		}()

		if cb := tracer.StartEphemeralResourceInstanceClose; cb != nil {
			closeCtx = cb(closeCtx, addr)
		}

		closReq := providers.CloseEphemeralResourceRequest{
			TypeName: addr.Resource.Resource.Type,
			Private:  privateData,
		}
		closeResp := provider.CloseEphemeralResource(closeCtx, closReq)
		diags = diags.Append(closeResp.Diagnostics)

		if cb := tracer.EndEphemeralResourceInstanceClose; cb != nil {
			cb(closeCtx, addr, diags)
		}

		select {
		case diagsCh <- diags:
		case <-time.After(500 * time.Millisecond):
			log.Printf("[ERROR] OpenEphemeralResourceInstance: Diagnostics not sent fully after closing ephemeral %s", diags)
		}
	}()

	closeFunc := func(ctx context.Context) tfdiags.Diagnostics {
		closeCh <- ctx
		close(closeCh)

		timeout := EphemeralResourceCloseTimeout
		select {
		case d := <-diagsCh:
			return d
		case <-time.After(timeout):
			return tfdiags.Diagnostics{}.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Closing ephemeral resource timed out",
				Detail:   fmt.Sprintf("The ephemeral resource %q timed out on closing after %s", addr.String(), timeout),
				// TODO Subject:  n.Config.DeclRange.Ptr(),
			})
		}
	}

	return newVal, closeFunc, diags
}
