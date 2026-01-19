// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package execgraph

import (
	"context"
	"log"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// FIXME: The execution functions below are all currently intentionally very
// chatty in the logs while we're in the early stages of working on this, but
// if this ships "for real" we'll probably want to be more selective in what
// we log so that we're only chattering in there when something has gone wrong
// and we have something to share that might be useful for debugging it.

func (c *compiler) compileOpProviderInstanceConfig(operands *compilerOperands) nodeExecuteRaw {
	getAddr := nextOperand[addrs.AbsProviderInstanceCorrect](operands)
	waitForDeps := operands.OperandWaiter()
	diags := operands.Finish()
	c.diags = c.diags.Append(diags)
	if diags.HasErrors() {
		return nil
	}
	ops := c.ops

	return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
		var diags tfdiags.Diagnostics
		log.Printf("[TRACE] execgraph: opProviderInstanceConfig waiting for dependencies to complete")
		if !waitForDeps(ctx) {
			log.Printf("[TRACE] execgraph: opProviderInstanceConfig upstream dependency failed")
			return nil, false, diags
		}
		addr, ok, moreDiags := getAddr(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			log.Printf("[TRACE] execgraph: opProviderInstanceConfig failed to get provider instance address")
			return nil, false, diags
		}
		log.Printf("[TRACE] execgraph: opProviderInstanceConfig ready to execute")

		ret, moreDiags := ops.ProviderInstanceConfig(ctx, addr)
		diags = diags.Append(moreDiags)
		return ret, !diags.HasErrors(), diags
	}
}

func (c *compiler) compileOpProviderInstanceOpen(operands *compilerOperands) nodeExecuteRaw {
	getConfig := nextOperand[*exec.ProviderInstanceConfig](operands)
	diags := operands.Finish()
	c.diags = c.diags.Append(diags)
	if diags.HasErrors() {
		return nil
	}
	ops := c.ops

	return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
		var diags tfdiags.Diagnostics
		log.Printf("[TRACE] execgraph: opProviderInstanceOpen waiting for configuration value")
		config, ok, moreDiags := getConfig(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			log.Printf("[TRACE] execgraph: opProviderInstanceOpen failed to get configuration value")
			return nil, false, diags
		}
		log.Printf("[TRACE] execgraph: opProviderInstanceOpen ready to execute")

		ret, moreDiags := ops.ProviderInstanceOpen(ctx, config)
		diags = diags.Append(moreDiags)
		return ret, !diags.HasErrors(), diags
	}
}

func (c *compiler) compileOpProviderInstanceClose(operands *compilerOperands) nodeExecuteRaw {
	getProviderClient := nextOperand[*exec.ProviderClient](operands)
	waitForUsers := operands.OperandWaiter()
	diags := operands.Finish()
	c.diags = c.diags.Append(diags)
	if diags.HasErrors() {
		return nil
	}
	ops := c.ops

	return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
		var diags tfdiags.Diagnostics
		log.Printf("[TRACE] execgraph: opProviderInstanceClose waiting for all provider users to finish")
		// We intentionally ignore results here because we want to close the
		// provider even if one of its users fails.
		waitForUsers(ctx)

		log.Printf("[TRACE] execgraph: opProviderInstanceClose waiting for provider client")
		providerClient, ok, moreDiags := getProviderClient(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			log.Printf("[TRACE] execgraph: opProviderInstanceClose failed to get provider client")
			return nil, false, diags
		}

		moreDiags = ops.ProviderInstanceClose(ctx, providerClient)
		diags = diags.Append(moreDiags)
		return struct{}{}, !diags.HasErrors(), diags
	}
}

func (c *compiler) compileOpResourceInstanceDesired(operands *compilerOperands) nodeExecuteRaw {
	ops := c.ops
	getInstAddr := nextOperand[addrs.AbsResourceInstance](operands)
	waitForDeps := operands.OperandWaiter()
	diags := operands.Finish()
	c.diags = c.diags.Append(diags)
	if diags.HasErrors() {
		return nil
	}

	return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
		var diags tfdiags.Diagnostics
		log.Printf("[TRACE] execgraph: opResourceInstanceDesired waiting for dependencies to complete")

		if !waitForDeps(ctx) {
			log.Printf("[TRACE] execgraph: opResourceInstanceDesired upstream dependency failed")
			return nil, false, diags
		}
		instAddr, ok, moreDiags := getInstAddr(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			log.Printf("[TRACE] execgraph: opResourceInstanceDesired failed to get resource instance address")
			return nil, false, diags
		}

		log.Printf("[TRACE] execgraph: opResourceInstanceDesired ready to execute")

		ret, moreDiags := ops.ResourceInstanceDesired(ctx, instAddr)
		diags = diags.Append(moreDiags)
		return ret, !diags.HasErrors(), diags
	}
}

func (c *compiler) compileOpResourceInstancePrior(operands *compilerOperands) nodeExecuteRaw {
	ops := c.ops
	getInstAddr := nextOperand[addrs.AbsResourceInstance](operands)
	diags := operands.Finish()
	c.diags = c.diags.Append(diags)
	if diags.HasErrors() {
		return nil
	}

	return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
		var diags tfdiags.Diagnostics
		log.Printf("[TRACE] execgraph: opResourceInstancePrior waiting for dependencies to complete")

		instAddr, ok, moreDiags := getInstAddr(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			log.Printf("[TRACE] execgraph: opResourceInstancePrior failed to get resource instance address")
			return nil, false, diags
		}

		log.Printf("[TRACE] execgraph: opResourceInstancePrior ready to execute")

		ret, moreDiags := ops.ResourceInstancePrior(ctx, instAddr)
		diags = diags.Append(moreDiags)
		return ret, !diags.HasErrors(), diags
	}
}

func (c *compiler) compileOpManagedFinalPlan(operands *compilerOperands) nodeExecuteRaw {
	getDesired := nextOperand[*eval.DesiredResourceInstance](operands)
	getPrior := nextOperand[*states.ResourceInstanceObjectFull](operands)
	getInitialPlanned := nextOperand[cty.Value](operands)
	getProviderClient := nextOperand[*exec.ProviderClient](operands)
	diags := operands.Finish()
	c.diags = c.diags.Append(diags)
	if diags.HasErrors() {
		return nil
	}
	ops := c.ops

	return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
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

		ret, moreDiags := ops.ManagedFinalPlan(ctx, desired, prior, initialPlanned, providerClient)
		diags = diags.Append(moreDiags)
		return ret, !diags.HasErrors(), diags
	}
}

func (c *compiler) compileOpManagedApply(operands *compilerOperands) nodeExecuteRaw {
	getFinalPlan := nextOperand[*exec.ManagedResourceObjectFinalPlan](operands)
	getFallback := nextOperand[*states.ResourceInstanceObjectFull](operands)
	getProviderClient := nextOperand[*exec.ProviderClient](operands)
	diags := operands.Finish()
	c.diags = c.diags.Append(diags)
	if diags.HasErrors() {
		return nil
	}
	ops := c.ops

	return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
		var diags tfdiags.Diagnostics
		log.Printf("[TRACE] execgraph: opManagedApply waiting for provider client")
		providerClient, ok, moreDiags := getProviderClient(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			log.Printf("[TRACE] execgraph: opManagedApply failed to get provider client")
			return nil, false, diags
		}
		log.Printf("[TRACE] execgraph: opManagedApply waiting for final plan")
		finalPlan, ok, moreDiags := getFinalPlan(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			log.Printf("[TRACE] execgraph: opManagedApply failed to get final plan")
			return nil, false, diags
		}
		log.Printf("[TRACE] execgraph: opManagedApply waiting for fallback value")
		fallback, ok, moreDiags := getFallback(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			log.Printf("[TRACE] execgraph: opManagedApply failed to get fallback value")
			return nil, false, diags
		}
		log.Printf("[TRACE] execgraph: opManagedFinalPlan ready to execute")

		ret, moreDiags := ops.ManagedApply(ctx, finalPlan, fallback, providerClient)
		diags = diags.Append(moreDiags)
		// TODO: Also call ops.ResourceInstancePostconditions if we produced a non-nil result
		return ret, !diags.HasErrors(), diags
	}
}

func (c *compiler) compileOpManagedDepose(operands *compilerOperands) nodeExecuteRaw {
	ops := c.ops
	getInstAddr := nextOperand[addrs.AbsResourceInstance](operands)
	diags := operands.Finish()
	c.diags = c.diags.Append(diags)
	if diags.HasErrors() {
		return nil
	}

	return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
		var diags tfdiags.Diagnostics
		log.Printf("[TRACE] execgraph: opManagedDepose waiting for dependencies to complete")

		instAddr, ok, moreDiags := getInstAddr(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			log.Printf("[TRACE] execgraph: opManagedDepose failed to get resource instance address")
			return nil, false, diags
		}

		log.Printf("[TRACE] execgraph: opManagedDepose ready to execute")

		ret, moreDiags := ops.ManagedDepose(ctx, instAddr)
		diags = diags.Append(moreDiags)
		return ret, !diags.HasErrors(), diags
	}
}

func (c *compiler) compileOpManagedAlreadyDeposed(operands *compilerOperands) nodeExecuteRaw {
	ops := c.ops
	getInstAddr := nextOperand[addrs.AbsResourceInstance](operands)
	getDeposedKey := nextOperand[states.DeposedKey](operands)
	diags := operands.Finish()
	c.diags = c.diags.Append(diags)
	if diags.HasErrors() {
		return nil
	}

	return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
		var diags tfdiags.Diagnostics
		log.Printf("[TRACE] execgraph: opManagedAlreadyDeposed waiting for dependencies to complete")

		instAddr, ok, moreDiags := getInstAddr(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			log.Printf("[TRACE] execgraph: opManagedAlreadyDeposed failed to get resource instance address")
			return nil, false, diags
		}
		deposedKey, ok, moreDiags := getDeposedKey(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			log.Printf("[TRACE] execgraph: opManagedAlreadyDeposed failed to get deposed key")
			return nil, false, diags
		}

		log.Printf("[TRACE] execgraph: opManagedAlreadyDeposed ready to execute")

		ret, moreDiags := ops.ManagedAlreadyDeposed(ctx, instAddr, deposedKey)
		diags = diags.Append(moreDiags)
		return ret, !diags.HasErrors(), diags
	}
}

func (c *compiler) compileOpDataRead(operands *compilerOperands) nodeExecuteRaw {
	getDesired := nextOperand[*eval.DesiredResourceInstance](operands)
	getInitialPlanned := nextOperand[cty.Value](operands)
	getProviderClient := nextOperand[*exec.ProviderClient](operands)
	diags := operands.Finish()
	c.diags = c.diags.Append(diags)
	if diags.HasErrors() {
		return nil
	}
	ops := c.ops

	return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
		log.Printf("[TRACE] execgraph: opDataRead waiting for provider client")
		providerClient, ok, moreDiags := getProviderClient(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			log.Printf("[TRACE] execgraph: opDataRead failed to get provider client")
			return nil, false, diags
		}
		log.Printf("[TRACE] execgraph: opDataRead waiting for desired state")
		desired, ok, moreDiags := getDesired(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			log.Printf("[TRACE] execgraph: opDataRead failed to get desired state")
			return nil, false, diags
		}
		log.Printf("[TRACE] execgraph: opDataRead waiting for initial planned state")
		initialPlanned, ok, moreDiags := getInitialPlanned(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			log.Printf("[TRACE] execgraph: opDataRead failed to get planned state")
			return nil, false, diags
		}
		log.Printf("[TRACE] execgraph: opDataRead ready to execute")

		ret, moreDiags := ops.DataRead(ctx, desired, initialPlanned, providerClient)
		diags = diags.Append(moreDiags)
		// TODO: Also call ops.ResourceInstancePostconditions
		return ret, !diags.HasErrors(), diags
	}
}

func (c *compiler) compileOpEphemeralOpen(operands *compilerOperands) nodeExecuteRaw {
	getDesired := nextOperand[*eval.DesiredResourceInstance](operands)
	getProviderClient := nextOperand[*exec.ProviderClient](operands)
	diags := operands.Finish()
	c.diags = c.diags.Append(diags)
	if diags.HasErrors() {
		return nil
	}
	ops := c.ops

	return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
		log.Printf("[TRACE] execgraph: opEphemeralOpen waiting for provider client")
		providerClient, ok, moreDiags := getProviderClient(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			log.Printf("[TRACE] execgraph: opEphemeralOpen failed to get provider client")
			return nil, false, diags
		}
		log.Printf("[TRACE] execgraph: opEphemeralOpen waiting for desired state")
		desired, ok, moreDiags := getDesired(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			log.Printf("[TRACE] execgraph: opEphemeralOpen failed to get desired state")
			return nil, false, diags
		}
		log.Printf("[TRACE] execgraph: opEphemeralOpen ready to execute")

		ret, moreDiags := ops.EphemeralOpen(ctx, desired, providerClient)
		diags = diags.Append(moreDiags)
		// TODO: Also call ops.ResourceInstancePostconditions
		return ret, !diags.HasErrors(), diags
	}
}

func (c *compiler) compileOpEphemeralClose(operands *compilerOperands) nodeExecuteRaw {
	getObject := nextOperand[*states.ResourceInstanceObjectFull](operands)
	getProviderClient := nextOperand[*exec.ProviderClient](operands)
	diags := operands.Finish()
	c.diags = c.diags.Append(diags)
	if diags.HasErrors() {
		return nil
	}
	ops := c.ops

	return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
		log.Printf("[TRACE] execgraph: opEphemeralClose waiting for provider client")
		providerClient, ok, moreDiags := getProviderClient(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			log.Printf("[TRACE] execgraph: opEphemeralClose failed to get provider client")
			return nil, false, diags
		}
		log.Printf("[TRACE] execgraph: opEphemeralClose waiting for desired state")
		object, ok, moreDiags := getObject(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			log.Printf("[TRACE] execgraph: opEphemeralClose failed to get object to close")
			return nil, false, diags
		}
		log.Printf("[TRACE] execgraph: opEphemeralClose ready to execute")

		moreDiags = ops.EphemeralClose(ctx, object, providerClient)
		diags = diags.Append(moreDiags)
		return struct{}{}, !diags.HasErrors(), diags
	}
}
