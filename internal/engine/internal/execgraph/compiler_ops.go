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
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

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
	diags := operands.Finish()
	c.diags = c.diags.Append(diags)
	if diags.HasErrors() {
		return nil
	}
	ops := c.ops

	return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
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

		ret, moreDiags := ops.ManagedFinalPlan(ctx, desired, prior, initialPlanned)
		diags = diags.Append(moreDiags)
		return ret, !diags.HasErrors(), diags
	}
}

func (c *compiler) compileOpManagedApply(operands *compilerOperands) nodeExecuteRaw {
	getFinalPlan := nextOperand[*exec.ManagedResourceObjectFinalPlan](operands)
	getFallback := nextOperand[*exec.ResourceInstanceObject](operands)
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

		ret, moreDiags := ops.ManagedApply(ctx, finalPlan, fallback)
		diags = diags.Append(moreDiags)
		// TODO: Also call ops.ResourceInstancePostconditions if we produced a non-nil result
		log.Printf("[WARN] opManagedApply doesn't yet handle postconditions")

		return ret, !diags.HasErrors(), diags
	}
}

func (c *compiler) compileOpManagedPrepareDepose(operands *compilerOperands) nodeExecuteRaw {
	getFinalPlan := nextOperand[*exec.ManagedResourceObjectFinalPlan](operands)
	getDeposedKey := nextOperand[addrs.DeposedKey](operands)
	diags := operands.Finish()
	c.diags = c.diags.Append(diags)
	if diags.HasErrors() {
		return nil
	}

	// This operation is an intrinsic, which means that its behavior is
	// fixed directly inline here rather than being delegated to the
	// [exec.Operations] object in c.ops. We use an intrinsic here because
	// this operation doesn't have any externally-visible side effects and so
	// there's no need for its behavior to vary; if implemented as a real
	// operation then every test using mock operations would need to
	// re-implement essentially the same logic.
	return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
		var diags tfdiags.Diagnostics
		finalPlan, ok, moreDiags := getFinalPlan(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}

		// The following checks are just to catch situations where the execution
		// graph was constructed incorrectly. No user input (valid or otherwise)
		// should cause these situations to arise, so if either of these
		// messages appear then that suggests a bug in the planning engine.
		const errSummary = "Invalid execution graph"
		if finalPlan.Addr.IsDeposed() {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				errSummary,
				fmt.Sprintf("Operation ManagedPrepareDeposed was called with a plan for already-deposed object %s. This is a bug in OpenTofu.", finalPlan.Addr),
			))
		}
		if !finalPlan.PlannedVal.IsNull() {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				errSummary,
				fmt.Sprintf("Operation ManagedPrepareDeposed was called with a non-destroy plan for %s. This is a bug in OpenTofu.", finalPlan.Addr),
			))
		}

		deposedKey, ok, moreDiags := getDeposedKey(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}
		ret := finalPlan.IntoDeposed(deposedKey)
		return ret, !diags.HasErrors(), diags
	}
}

func (c *compiler) compileOpManagedPerformDepose(operands *compilerOperands) nodeExecuteRaw {
	ops := c.ops
	getCurrentObj := nextOperand[*exec.ResourceInstanceObject](operands)
	getDeletePlan := nextOperand[*exec.ManagedResourceObjectFinalPlan](operands)
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
		deletePlan, ok, moreDiags := getDeletePlan(ctx)
		diags = diags.Append(moreDiags)
		if !ok {
			return nil, false, diags
		}

		ret, moreDiags := ops.ManagedPerformDepose(ctx, currentObj, deletePlan)
		diags = diags.Append(moreDiags)

		if !diags.HasErrors() {
			// Some correctness checks just to help us catch bugs in the
			// operations implementation before they cause confusion downstream.
			if !ret.Addr.IsDeposed() {
				diags = diags.Append(fmt.Errorf("opManagedPerformDepose result has non-deposed object address %s; this is a bug in OpenTofu", ret.Addr))
			}
			if !ret.Addr.InstanceAddr.Equal(currentObj.Addr.InstanceAddr) {
				diags = diags.Append(fmt.Errorf("opManagedPerformDepose for %s result has wrong instance address %s; this is a bug in OpenTofu", currentObj.Addr.InstanceAddr, ret.Addr.InstanceAddr))
			}
		}
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

		if !diags.HasErrors() {
			// Some correctness checks just to help us catch bugs in the
			// operations implementation before they cause confusion downstream.
			if !ret.Addr.IsDeposed() {
				diags = diags.Append(fmt.Errorf("opManagedAlreadyDeposed result has non-deposed object address %s; this is a bug in OpenTofu", ret.Addr))
			}
			if !ret.Addr.InstanceAddr.Equal(instAddr) {
				diags = diags.Append(fmt.Errorf("opManagedAlreadyDeposed for %s result has wrong instance address %s; this is a bug in OpenTofu", instAddr.Object(deposedKey), ret.Addr.InstanceAddr))
			}
		}
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
	diags := operands.Finish()
	c.diags = c.diags.Append(diags)
	if diags.HasErrors() {
		return nil
	}
	ops := c.ops

	return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
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

		ret, moreDiags := ops.DataRead(ctx, desired, initialPlanned)
		diags = diags.Append(moreDiags)
		// TODO: Also call ops.ResourceInstancePostconditions
		log.Printf("[WARN] opDataRead doesn't yet handle postconditions")
		return ret, !diags.HasErrors(), diags
	}
}
