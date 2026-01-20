// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package execgraph

import (
	"context"
	"fmt"
	"log"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func (c *compiler) compileOpManagedFinalPlan(operands *compilerOperands) nodeExecuteRaw {
	getDesired := nextOperand[*eval.DesiredResourceInstance](operands)
	getPrior := nextOperand[*states.ResourceInstanceObjectFull](operands)
	getInitialPlanned := nextOperand[cty.Value](operands)
	getProviderClient := nextOperand[providers.Configured](operands)
	waitForDeps := operands.OperandWaiter()
	diags := operands.Finish()
	c.diags = c.diags.Append(diags)
	if diags.HasErrors() {
		return nil
	}

	return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
		log.Printf("[TRACE] execgraph: opManagedFinalPlan waiting for dependencies to complete")
		var diags tfdiags.Diagnostics
		if !waitForDeps(ctx) {
			log.Printf("[TRACE] execgraph: opManagedFinalPlan upstream dependency failed")
			return nil, false, diags
		}
		log.Printf("[TRACE] execgraph: opManagedFinalPlan waiting for provider client")
		providerClient, ok, moreDiags := getProviderClient(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			log.Printf("[TRACE] execgraph: opManagedFinalPlan failed to get provider client")
			return nil, false, diags
		}
		log.Printf("[TRACE] execgraph: opManagedFinalPlan waiting for desired state")
		desired, ok, moreDiags := getDesired(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			log.Printf("[TRACE] execgraph: opManagedFinalPlan failed to get desired state")
			return nil, false, diags
		}
		log.Printf("[TRACE] execgraph: opManagedFinalPlan waiting for prior state")
		prior, ok, moreDiags := getPrior(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			log.Printf("[TRACE] execgraph: opManagedFinalPlan failed to get prior state")
			return nil, false, diags
		}
		log.Printf("[TRACE] execgraph: opManagedFinalPlan waiting for initial planned state")
		initialPlanned, ok, moreDiags := getInitialPlanned(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			log.Printf("[TRACE] execgraph: opManagedFinalPlan failed to get planned state")
			return nil, false, diags
		}
		log.Printf("[TRACE] execgraph: opManagedFinalPlan ready to execute")

		var resourceTypeName string
		if desired != nil {
			resourceTypeName = desired.Addr.Resource.Resource.Type
		} else if prior != nil {
			resourceTypeName = prior.ResourceType
		} else {
			// Should not get here: there's no reason to be applying changes
			// for a resource instance that has neither a desired state nor
			// a prior state.
			diags = diags.Append(fmt.Errorf("attempting to apply final plan for resource instance that has neither desired nor prior state (this is a bug in OpenTofu)"))
			return nil, false, diags
		}

		req := providers.PlanResourceChangeRequest{
			TypeName: resourceTypeName,
		}
		if desired != nil {
			req.Config = desired.ConfigVal
		} else {
			req.Config = cty.NullVal(cty.DynamicPseudoType)
		}
		if prior != nil {
			req.PriorState = prior.Value
			req.PriorPrivate = prior.Private
		}
		// TODO: req.ProviderMeta, maybe.
		// TODO: req.ProposedNewState, but we need the provider's schema in
		// here in order to build that with objchange.ProposedNew. :(
		// It sure would be nice if these concerns weren't all so tangled
		// together. Maybe we could address that with a higher-level type
		// for the provider client, instead of using [providers.Configured]
		// directly, which has access to the schema cache and maybe even
		// knows which resource type it's trying to be a client for so it
		// can handle these schema-related details internally itself, since
		// the provider client already has schema information available to
		// it in order to marshal the other values in the request.

		resp := providerClient.PlanResourceChange(ctx, req)
		diags = diags.Append(resp.Diagnostics)
		if resp.Diagnostics.HasErrors() {
			return nil, false, diags
		}

		// TODO: Check whether the final plan is valid in comparison to the
		// initial plan. But again, we need access to the schema here to do
		// that directly, which is annoying since it would be nice if that
		// were all encapsulated away somewhere.
		_ = initialPlanned

		ret := &ManagedResourceObjectFinalPlan{
			ResourceType:  resourceTypeName,
			ConfigVal:     desired.ConfigVal,
			PriorStateVal: req.PriorState,
			PlannedVal:    resp.PlannedState,
		}
		return ret, true, diags
	}
}

func (c *compiler) compileOpManagedApplyChanges(operands *compilerOperands) nodeExecuteRaw {
	getFinalPlan := nextOperand[*ManagedResourceObjectFinalPlan](operands)
	getProviderClient := nextOperand[providers.Configured](operands)
	diags := operands.Finish()
	c.diags = c.diags.Append(diags)
	if diags.HasErrors() {
		return nil
	}

	return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
		var diags tfdiags.Diagnostics
		log.Printf("[TRACE] execgraph: opManagedApplyChanges waiting for provider client")
		providerClient, ok, moreDiags := getProviderClient(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			log.Printf("[TRACE] execgraph: opManagedApplyChanges failed to get provider client")
			return nil, false, diags
		}
		log.Printf("[TRACE] execgraph: opManagedApplyChanges waiting for final plan")
		finalPlan, ok, moreDiags := getFinalPlan(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			log.Printf("[TRACE] execgraph: opManagedApplyChanges failed to get final plan")
			return nil, false, diags
		}

		log.Printf("[TRACE] execgraph: opManagedApplyChanges ready to apply changes for %q", finalPlan.ResourceType)
		req := providers.ApplyResourceChangeRequest{
			TypeName:     finalPlan.ResourceType,
			PriorState:   finalPlan.PriorStateVal,
			PlannedState: finalPlan.PlannedVal,
			Config:       finalPlan.ConfigVal,
			// TODO: PlannedPrivate
			// TODO: ProviderMeta(?)
		}

		resp := providerClient.ApplyResourceChange(ctx, req)
		diags = diags.Append(resp.Diagnostics)
		if resp.Diagnostics.HasErrors() {
			// FIXME: We need to be able to return a new state object even
			// if the apply failed, because it should be saved -- possibly
			// as tainted, if we were trying to create it -- so that the
			// next round can try to recover from the problem.
			return nil, false, diags
		}

		// TODO: Check whether the final state is valid in comparison to the
		// final plan. But we need access to the schema here to do that
		// directly, which is annoying since it would be nice if that were all
		// encapsulated away somewhere.

		ret := &states.ResourceInstanceObjectFull{
			Value:   resp.NewState,
			Private: resp.Private,
			Status:  states.ObjectReady,
			// TODO: ProviderInstanceAddr, which we don't currently have here
			// because we're just holding an already-open client for that
			// provider. Should we extend [providers.Interface] with a method
			// to find which provider instance the client is acting on behalf of?
			ResourceType: finalPlan.ResourceType,
			// TODO: Dependencies ... they come from the "desired" object
			// so maybe we should send that whole thing over here instead of
			// just the ConfigVal?
			// TODO: CreateBeforeDestroy ... also from the "desired" object.
		}
		return ret, true, diags
	}
}

func (c *compiler) compileOpOpenProvider(operands *compilerOperands) nodeExecuteRaw {
	getProviderAddr := nextOperand[addrs.Provider](operands)
	getConfigVal := nextOperand[cty.Value](operands)
	waitForDeps := operands.OperandWaiter()
	diags := operands.Finish()
	c.diags = c.diags.Append(diags)
	if diags.HasErrors() {
		return nil
	}

	execCtx := c.execCtx

	return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
		var diags tfdiags.Diagnostics
		log.Printf("[TRACE] execgraph: opOpenProvider waiting for provider address")
		providerAddr, ok, moreDiags := getProviderAddr(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			log.Printf("[TRACE] execgraph: opOpenProvider failed to get provider address")
			return nil, false, diags
		}
		log.Printf("[TRACE] execgraph: opOpenProvider %s waiting for dependencies to complete", providerAddr)
		if !waitForDeps(ctx) {
			log.Printf("[TRACE] execgraph: opOpenProvider upstream dependency failed")
			return nil, false, diags
		}
		log.Printf("[TRACE] execgraph: opOpenProvider %s waiting for configuration value", providerAddr)
		configVal, ok, moreDiags := getConfigVal(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			log.Printf("[TRACE] execgraph: opOpenProvider failed to get configuration value")
			return nil, false, diags
		}

		log.Printf("[TRACE] execgraph: opOpenProvider creating a configured client for %s", providerAddr)
		ret, moreDiags := execCtx.NewProviderClient(ctx, providerAddr, configVal)
		diags = diags.Append(moreDiags)
		if moreDiags.HasErrors() {
			return nil, false, diags
		}
		return ret, true, diags
	}
}

func (c *compiler) compileOpCloseProvider(operands *compilerOperands) nodeExecuteRaw {
	getProviderClient := nextOperand[providers.Configured](operands)
	waitForUsers := operands.OperandWaiter()
	diags := operands.Finish()
	c.diags = c.diags.Append(diags)
	if diags.HasErrors() {
		return nil
	}

	return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
		var diags tfdiags.Diagnostics
		log.Printf("[TRACE] execgraph: opCloseProvider waiting for all provider users to finish")
		// We intentionally ignore results here because we want to close the
		// provider even if one of its users fails.
		waitForUsers(ctx)

		log.Printf("[TRACE] execgraph: opCloseProvider waiting for provider client")
		providerClient, ok, moreDiags := getProviderClient(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			log.Printf("[TRACE] execgraph: opCloseProvider failed to get provider client")
			return nil, false, diags
		}

		log.Printf("[TRACE] execgraph: opCloseProvider calling Close on provider")
		err := providerClient.Close(ctx)
		if err != nil {
			log.Printf("[TRACE] execgraph: opCloseProvider failed to close: %s", err)
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Failed to close provider client",
				fmt.Sprintf("Error closing provider client: %s.", tfdiags.FormatError(err)),
			))
			return nil, false, diags
		}
		log.Printf("[TRACE] execgraph: opCloseProvider closed provider successfully")

		// This operation has no real result, and so we use an empty struct
		// value to represent "nothing".
		return struct{}{}, true, diags
	}
}
