// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"log"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/moduletest"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// TestContext wraps a Context, and adds in direct values for the current state,
// most recent plan, and configuration.
//
// This combination allows functions called on the TestContext to create a
// complete scope to evaluate test assertions.
type TestContext struct {
	*Context

	Config    *configs.Config
	State     *states.State
	Plan      *plans.Plan
	Variables InputValues
}

// TestContext creates a TestContext structure that can evaluate test assertions
// against the provided state and plan.
func (c *Context) TestContext(config *configs.Config, state *states.State, plan *plans.Plan, variables InputValues) *TestContext {
	return &TestContext{
		Context:   c,
		Config:    config,
		State:     state,
		Plan:      plan,
		Variables: variables,
	}
}

// EvaluateAgainstState processes the assertions inside the provided
// configs.TestRun against the embedded state.
//
// The provided plan is import as it is needed to evaluate the `plantimestamp`
// function, but no data or changes from the embedded plan is referenced in
// this function.
func (ctx *TestContext) EvaluateAgainstState(run *moduletest.Run) {
	defer ctx.acquireRun("evaluate")()
	ctx.evaluate(ctx.State.SyncWrapper(), plans.NewChanges().SyncWrapper(), run, walkApply)
}

// EvaluateAgainstPlan processes the assertions inside the provided
// configs.TestRun against the embedded plan and state.
func (ctx *TestContext) EvaluateAgainstPlan(run *moduletest.Run) {
	defer ctx.acquireRun("evaluate")()
	ctx.evaluate(ctx.State.SyncWrapper(), ctx.Plan.Changes.SyncWrapper(), run, walkPlan)
}

func (ctx *TestContext) evaluate(state *states.SyncState, changes *plans.ChangesSync, run *moduletest.Run, operation walkOperation) {
	// The state does not include the module that has no resources, making its outputs unusable.
	// synchronizeStates function synchronizes the state with the planned state, ensuring inclusion of all modules.
	if ctx.Plan != nil && ctx.Plan.PlannedState != nil &&
		len(ctx.State.Modules) != len(ctx.Plan.PlannedState.Modules) {
		state = synchronizeStates(ctx.State, ctx.Plan.PlannedState)
	}

	data := &evaluationStateData{
		Evaluator: &Evaluator{
			Operation: operation,
			Meta:      ctx.meta,
			Config:    ctx.Config,
			Plugins:   ctx.plugins,
			State:     state,
			Changes:   changes,
			VariableValues: func() map[string]map[string]cty.Value {
				variables := map[string]map[string]cty.Value{
					addrs.RootModule.String(): make(map[string]cty.Value),
				}
				for name, variable := range ctx.Variables {
					variables[addrs.RootModule.String()][name] = variable.Value
				}
				return variables
			}(),
			VariableValuesLock: new(sync.Mutex),
			PlanTimestamp:      ctx.Plan.Timestamp,
		},
		ModulePath:      nil, // nil for the root module
		InstanceKeyData: EvalDataForNoInstanceKey,
		Operation:       operation,
	}

	var providerInstanceLock sync.Mutex
	providerInstances := make(map[addrs.Provider]providers.Interface)
	defer func() {
		for addr, inst := range providerInstances {
			log.Printf("[INFO] Shutting down test provider %s", addr)
			inst.Close()
		}
	}()

	providerSupplier := func(addr addrs.Provider) providers.Interface {
		providerInstanceLock.Lock()
		defer providerInstanceLock.Unlock()

		if inst, ok := providerInstances[addr]; ok {
			return inst
		}

		factory, ok := ctx.plugins.providerFactories[addr]
		if !ok {
			log.Printf("[WARN] Unable to find provider %s in test context", addr)
			providerInstances[addr] = nil
			return nil
		}
		log.Printf("[INFO] Starting test provider %s", addr)
		inst, err := factory()
		if err != nil {
			log.Printf("[WARN] Unable to start provider %s in test context", addr)
			providerInstances[addr] = nil
			return nil
		} else {
			log.Printf("[INFO] Shutting down test provider %s", addr)
			providerInstances[addr] = inst
			return inst
		}
	}

	scope := &lang.Scope{
		Data:          data,
		BaseDir:       ".",
		PureOnly:      operation != walkApply,
		PlanTimestamp: ctx.Plan.Timestamp,
		ProviderFunctions: func(pf addrs.ProviderFunction, rng tfdiags.SourceRange) (*function.Function, tfdiags.Diagnostics) {
			// This is a simpler flow than what is allowed during normal exection.
			// We only support non-configured functions here.
			pr, ok := ctx.Config.Module.ProviderRequirements.RequiredProviders[pf.ProviderName]
			if !ok {
				return nil, tfdiags.Diagnostics{}.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Unknown function provider",
					Detail:   fmt.Sprintf("Provider %q does not exist within the required_providers of this module", pf.ProviderName),
					Subject:  rng.ToHCL().Ptr(),
				})
			}

			provider := providerSupplier(pr.Type)

			return evalContextProviderFunction(provider, walkPlan, pf, rng)
		},
	}

	// We're going to assume the run has passed, and then if anything fails this
	// value will be updated.
	run.Status = run.Status.Merge(moduletest.Pass)

	// Now validate all the assertions within this run block.
	for _, rule := range run.Config.CheckRules {
		var diags tfdiags.Diagnostics

		refs, moreDiags := lang.ReferencesInExpr(addrs.ParseRefFromTestingScope, rule.Condition)
		diags = diags.Append(moreDiags)
		moreRefs, moreDiags := lang.ReferencesInExpr(addrs.ParseRefFromTestingScope, rule.ErrorMessage)
		diags = diags.Append(moreDiags)
		refs = append(refs, moreRefs...)

		hclCtx, moreDiags := scope.EvalContext(refs)
		diags = diags.Append(moreDiags)

		errorMessage, moreDiags := evalCheckErrorMessage(rule.ErrorMessage, hclCtx)
		diags = diags.Append(moreDiags)

		runVal, hclDiags := rule.Condition.Value(hclCtx)
		diags = diags.Append(hclDiags)

		run.Diagnostics = run.Diagnostics.Append(diags)
		if diags.HasErrors() {
			run.Status = run.Status.Merge(moduletest.Error)
			continue
		}

		// The condition result may be marked if the expression refers to a
		// sensitive value.
		runVal, _ = runVal.Unmark()

		if runVal.IsNull() {
			run.Status = run.Status.Merge(moduletest.Error)
			run.Diagnostics = run.Diagnostics.Append(&hcl.Diagnostic{
				Severity:    hcl.DiagError,
				Summary:     "Invalid condition run",
				Detail:      "Condition expression must return either true or false, not null.",
				Subject:     rule.Condition.Range().Ptr(),
				Expression:  rule.Condition,
				EvalContext: hclCtx,
			})
			continue
		}

		if !runVal.IsKnown() {
			run.Status = run.Status.Merge(moduletest.Error)
			run.Diagnostics = run.Diagnostics.Append(&hcl.Diagnostic{
				Severity:    hcl.DiagError,
				Summary:     "Unknown condition run",
				Detail:      "Condition expression could not be evaluated at this time.",
				Subject:     rule.Condition.Range().Ptr(),
				Expression:  rule.Condition,
				EvalContext: hclCtx,
			})
			continue
		}

		var err error
		if runVal, err = convert.Convert(runVal, cty.Bool); err != nil {
			run.Status = run.Status.Merge(moduletest.Error)
			run.Diagnostics = run.Diagnostics.Append(&hcl.Diagnostic{
				Severity:    hcl.DiagError,
				Summary:     "Invalid condition run",
				Detail:      fmt.Sprintf("Invalid condition run value: %s.", tfdiags.FormatError(err)),
				Subject:     rule.Condition.Range().Ptr(),
				Expression:  rule.Condition,
				EvalContext: hclCtx,
			})
			continue
		}

		if runVal.False() {
			run.Status = run.Status.Merge(moduletest.Fail)
			run.Diagnostics = run.Diagnostics.Append(&hcl.Diagnostic{
				Severity:    hcl.DiagError,
				Summary:     "Test assertion failed",
				Detail:      errorMessage,
				Subject:     rule.Condition.Range().Ptr(),
				Expression:  rule.Condition,
				EvalContext: hclCtx,
			})
			continue
		}
	}
}

// synchronizeStates compares the planned state to the current state and incorporates any missing modules
// from the planned state into the current state.
//
// If a module has no resources, it is included in the current state to ensure that its output variables are usable.
func synchronizeStates(state, plannedState *states.State) *states.SyncState {
	newState := state.DeepCopy()
	for key, value := range plannedState.Modules {
		if _, exists := newState.Modules[key]; !exists {
			newState.Modules[key] = value
		}
	}
	return newState.SyncWrapper()
}
