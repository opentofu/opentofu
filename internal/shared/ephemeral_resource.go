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

type EphemeralResourceHooks struct {
	PreOpen   func(addrs.AbsResourceInstance)
	PostOpen  func(addrs.AbsResourceInstance, tfdiags.Diagnostics)
	PreRenew  func(addrs.AbsResourceInstance)
	PostRenew func(addrs.AbsResourceInstance, tfdiags.Diagnostics)
	PreClose  func(addrs.AbsResourceInstance)
	PostClose func(addrs.AbsResourceInstance, tfdiags.Diagnostics)
}

type EphemeralCloseFunc func(context.Context) tfdiags.Diagnostics

func OpenEphemeralResourceInstance(
	ctx context.Context,
	// TODO once we remove the old engine, this can be condensed using the new engine types
	addr addrs.AbsResourceInstance,
	schema *configschema.Block,
	providerAddr addrs.AbsProviderInstanceCorrect,
	provider providers.Interface,
	configVal cty.Value,
	hooks EphemeralResourceHooks,
) (cty.Value, EphemeralCloseFunc, tfdiags.Diagnostics) {
	var newVal cty.Value
	var diags tfdiags.Diagnostics

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

	if hooks.PreOpen != nil {
		hooks.PreOpen(addr)
	}

	openReq := providers.OpenEphemeralResourceRequest{
		TypeName: addr.ContainingResource().Resource.Type,
		Config:   configVal,
	}
	openResp := provider.OpenEphemeralResource(ctx, openReq)
	diags = diags.Append(openResp.Diagnostics)
	if diags.HasErrors() {
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

	if diags.HasErrors() {
		// We have an open ephemeral resource, but don't plan to use it due to validation errors
		// It needs to be closed before we can return

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

	if hooks.PostOpen != nil {
		hooks.PostOpen(addr, diags)
	}

	// Initialize the closing channel and the channel that sends diagnostics back to the close caller.
	closeCh := make(chan context.Context, 1)
	diagsCh := make(chan tfdiags.Diagnostics, 1)
	go func() {
		var diags tfdiags.Diagnostics
		renewAt := openResp.RenewAt
		privateData := openResp.Private

		closeCtx := ctx

		// We have two exit paths that should take the same route
		func() {
			for {
				// Select on nil chan will block until other case close or done
				var renewAtTimer chan time.Time
				if renewAt != nil {
					time.After(time.Until(*renewAt))
				}

				select {
				case <-renewAtTimer:
					if hooks.PreRenew != nil {
						hooks.PreRenew(addr)
					}

					renewReq := providers.RenewEphemeralResourceRequest{
						TypeName: addr.Resource.Resource.Type,
						Private:  privateData,
					}
					renewResp := provider.RenewEphemeralResource(ctx, renewReq)
					diags = diags.Append(renewResp.Diagnostics)
					// TODO consider what happens if renew fails, do we still want to update private?
					renewAt = renewResp.RenewAt

					if hooks.PostRenew != nil {
						hooks.PostRenew(addr, diags)
					}
					privateData = renewResp.Private
				case closeCtx = <-closeCh:
					return
				case <-ctx.Done():
					// Even though the context is "Done" we still want to execute the close operation
					closeCtx = context.WithoutCancel(closeCtx)
					return
				}
			}
		}()

		if hooks.PreClose != nil {
			hooks.PreClose(addr)
		}

		closReq := providers.CloseEphemeralResourceRequest{
			TypeName: addr.Resource.Resource.Type,
			Private:  privateData,
		}
		closeResp := provider.CloseEphemeralResource(closeCtx, closReq)
		diags = diags.Append(closeResp.Diagnostics)

		if hooks.PostClose != nil {
			hooks.PostClose(addr, diags)
		}

		diagsCh <- diags
	}()

	closeFunc := func(ctx context.Context) tfdiags.Diagnostics {
		closeCh <- ctx
		close(closeCh)
		defer func() {
			close(diagsCh)
		}()

		timeout := 10 * time.Second
		select {
		case d := <-diagsCh:
			return d
		case <-time.After(timeout):
			return tfdiags.Diagnostics{}.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Closing ephemeral resource timed out",
				Detail:   fmt.Sprintf("The ephemeral resource %q timed out on closing after %s", addr.String(), timeout),
				//TODO Subject:  n.Config.DeclRange.Ptr(),
			})
		}
	}

	return newVal, closeFunc, diags
}
