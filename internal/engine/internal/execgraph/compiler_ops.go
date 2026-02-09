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
		if !waitForDeps(ctx) {
			return nil, false, diags
		}
		addr, ok, moreDiags := getAddr(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}

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
		config, ok, moreDiags := getConfig(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}

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
		// We intentionally ignore results here because we want to close the
		// provider even if one of its users fails.
		waitForUsers(ctx)

		providerClient, ok, moreDiags := getProviderClient(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
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

		if !waitForDeps(ctx) {
			return nil, false, diags
		}
		instAddr, ok, moreDiags := getInstAddr(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}

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

		instAddr, ok, moreDiags := getInstAddr(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}

		ret, moreDiags := ops.ResourceInstancePrior(ctx, instAddr)
		diags = diags.Append(moreDiags)
		return ret, !diags.HasErrors(), diags
	}
}

func (c *compiler) compileOpManagedFinalPlan(operands *compilerOperands) nodeExecuteRaw {
	getDesired := nextOperand[*eval.DesiredResourceInstance](operands)
	getPrior := nextOperand[*exec.ResourceInstanceObject](operands)
	getInitialPlanned := nextOperand[cty.Value](operands)
	getProviderClient := nextOperand[*exec.ProviderClient](operands)
	diags := operands.Finish()
	c.diags = c.diags.Append(diags)
	if diags.HasErrors() {
		return nil
	}
	ops := c.ops

	return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
		providerClient, ok, moreDiags := getProviderClient(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}
		desired, ok, moreDiags := getDesired(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}
		prior, ok, moreDiags := getPrior(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}
		initialPlanned, ok, moreDiags := getInitialPlanned(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}

		ret, moreDiags := ops.ManagedFinalPlan(ctx, desired, prior, initialPlanned, providerClient)
		diags = diags.Append(moreDiags)
		return ret, !diags.HasErrors(), diags
	}
}

func (c *compiler) compileOpManagedApply(operands *compilerOperands) nodeExecuteRaw {
	getFinalPlan := nextOperand[*exec.ManagedResourceObjectFinalPlan](operands)
	getFallback := nextOperand[*exec.ResourceInstanceObject](operands)
	getProviderClient := nextOperand[*exec.ProviderClient](operands)
	waitForDeps := operands.OperandWaiter()
	diags := operands.Finish()
	c.diags = c.diags.Append(diags)
	if diags.HasErrors() {
		return nil
	}
	ops := c.ops

	return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
		var diags tfdiags.Diagnostics
		if !waitForDeps(ctx) {
			return nil, false, diags
		}
		providerClient, ok, moreDiags := getProviderClient(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}
		finalPlan, ok, moreDiags := getFinalPlan(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}
		fallback, ok, moreDiags := getFallback(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}

		ret, moreDiags := ops.ManagedApply(ctx, finalPlan, fallback, providerClient)
		diags = diags.Append(moreDiags)
		// TODO: Also call ops.ResourceInstancePostconditions if we produced a non-nil result
		log.Printf("[WARN] opManagedApply doesn't yet handle postconditions")
		return ret, !diags.HasErrors(), diags
	}
}

func (c *compiler) compileOpManagedDepose(operands *compilerOperands) nodeExecuteRaw {
	ops := c.ops
	getCurrentObj := nextOperand[*exec.ResourceInstanceObject](operands)
	waitForDeps := operands.OperandWaiter()
	diags := operands.Finish()
	c.diags = c.diags.Append(diags)
	if diags.HasErrors() {
		return nil
	}

	return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
		var diags tfdiags.Diagnostics
		if !waitForDeps(ctx) {
			return nil, false, diags
		}

		currentObj, ok, moreDiags := getCurrentObj(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}

		ret, moreDiags := ops.ManagedDepose(ctx, currentObj)
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

		instAddr, ok, moreDiags := getInstAddr(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}
		deposedKey, ok, moreDiags := getDeposedKey(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}

		ret, moreDiags := ops.ManagedAlreadyDeposed(ctx, instAddr, deposedKey)
		diags = diags.Append(moreDiags)
		return ret, !diags.HasErrors(), diags
	}
}

func (c *compiler) compileOpManagedChangeAddr(operands *compilerOperands) nodeExecuteRaw {
	ops := c.ops
	getCurrentObj := nextOperand[*exec.ResourceInstanceObject](operands)
	getNewAddr := nextOperand[addrs.AbsResourceInstance](operands)
	diags := operands.Finish()
	c.diags = c.diags.Append(diags)
	if diags.HasErrors() {
		return nil
	}

	return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
		var diags tfdiags.Diagnostics

		currentObj, ok, moreDiags := getCurrentObj(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}
		newAddr, ok, moreDiags := getNewAddr(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}

		ret, moreDiags := ops.ManagedChangeAddr(ctx, currentObj, newAddr)
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
		providerClient, ok, moreDiags := getProviderClient(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}
		desired, ok, moreDiags := getDesired(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}
		initialPlanned, ok, moreDiags := getInitialPlanned(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}

		ret, moreDiags := ops.DataRead(ctx, desired, initialPlanned, providerClient)
		diags = diags.Append(moreDiags)
		// TODO: Also call ops.ResourceInstancePostconditions
		log.Printf("[WARN] opDataRead doesn't yet handle postconditions")
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
		providerClient, ok, moreDiags := getProviderClient(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}
		desired, ok, moreDiags := getDesired(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}

		ret, moreDiags := ops.EphemeralOpen(ctx, desired, providerClient)
		diags = diags.Append(moreDiags)
		return ret, !diags.HasErrors(), diags
	}
}

func (c *compiler) compileOpEphemeralState(operands *compilerOperands) nodeExecuteRaw {
	getEphemeral := nextOperand[*exec.OpenEphemeralResourceInstance](operands)
	diags := operands.Finish()
	c.diags = c.diags.Append(diags)
	if diags.HasErrors() {
		return nil
	}
	ops := c.ops

	return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
		ephemeral, ok, moreDiags := getEphemeral(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}

		ret, moreDiags := ops.EphemeralState(ctx, ephemeral)
		diags = diags.Append(moreDiags)
		// TODO: Also call ops.ResourceInstancePostconditions
		log.Printf("[WARN] opEphemeralState doesn't yet handle postconditions")
		return ret, !diags.HasErrors(), diags
	}
}

func (c *compiler) compileOpEphemeralClose(operands *compilerOperands) nodeExecuteRaw {
	getEphemeral := nextOperand[*exec.OpenEphemeralResourceInstance](operands)
	waitForUsers := operands.OperandWaiter()
	diags := operands.Finish()
	c.diags = c.diags.Append(diags)
	if diags.HasErrors() {
		return nil
	}
	ops := c.ops

	return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
		// We intentionally ignore results here because we want to close the
		// ephemeral even if one of its users fails.
		waitForUsers(ctx)

		ephemeral, ok, moreDiags := getEphemeral(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}

		moreDiags = ops.EphemeralClose(ctx, ephemeral)
		diags = diags.Append(moreDiags)
		return struct{}{}, !diags.HasErrors(), diags
	}
}
