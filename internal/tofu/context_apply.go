// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tracing"
	"github.com/opentofu/opentofu/internal/tracing/traceattrs"
)

// ApplyOpts are the various options that affect the details of how OpenTofu
// will build a plan.
//
// This structure is created from the PlanOpts since wants some functionality
// that PlanOpts already have.
type ApplyOpts struct {
	// SetVariables are the raw values for root module variables as provided
	// by the user who is requesting the run, prior to any normalization or
	// substitution of defaults.
	// In localRunForPlanFile, where the initialization of this was initially
	// introduced, there is a validation to ensure that the values from the cli
	// do not differ from the ones saved in the plan. The place where this is used,
	// in Context#mergePlanAndApplyVariables, the merging of this with the plan variable values
	// follows the same logic and rules of the validation mentioned above.
	SetVariables InputValues

	// SuppressForgetErrorsDuringDestroy suppresses the error that would otherwise
	// be raised when a destroy operation completes with forgotten instances remaining.
	SuppressForgetErrorsDuringDestroy bool
}

// Apply performs the actions described by the given Plan object and returns
// the resulting updated state.
//
// The given configuration *must* be the same configuration that was passed
// earlier to Context.Plan in order to create this plan.
//
// Even if the returned diagnostics contains errors, Apply always returns the
// resulting state which is likely to have been partially-updated.
func (c *Context) Apply(ctx context.Context, plan *plans.Plan, config *configs.Config, opts *ApplyOpts) (*states.State, tfdiags.Diagnostics) {
	defer c.acquireRun("apply")()

	log.Printf("[DEBUG] Building and walking apply graph for %s plan", plan.UIMode)

	var diags tfdiags.Diagnostics

	ctx, span := tracing.Tracer().Start(
		ctx, "Apply phase",
		tracing.SpanAttributes(
			traceattrs.String("opentofu.plan.mode", plan.UIMode.UIName()),
		),
	)
	defer span.End()

	if plan.Errored {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Cannot apply failed plan",
			`The given plan is incomplete due to errors during planning, and so it cannot be applied.`,
		))
		return nil, diags
	}

	var forgetCount int

	for _, rc := range plan.Changes.Resources {
		// Import is a no-op change during an apply (all the real action happens during the plan) but we'd
		// like to show some helpful output that mirrors the way we show other changes.
		if rc.Importing != nil {
			for _, h := range c.hooks {
				// In the future, we may need to call PostApplyImport separately elsewhere in the apply
				// operation. For now, though, we'll call Pre and Post hooks together.
				_, err := h.PreApplyImport(rc.Addr, *rc.Importing)
				if err != nil {
					return nil, diags.Append(err)
				}
				_, err = h.PostApplyImport(rc.Addr, *rc.Importing)
				if err != nil {
					return nil, diags.Append(err)
				}
			}
		}

		// Following the same logic, we want to show helpful output for forget operations as well.
		if rc.Action == plans.Forget {
			forgetCount++
			for _, h := range c.hooks {
				_, err := h.PreApplyForget(rc.Addr)
				if err != nil {
					return nil, diags.Append(err)
				}
				_, err = h.PostApplyForget(rc.Addr)
				if err != nil {
					return nil, diags.Append(err)
				}
			}
		}
	}

	providerFunctionTracker := make(ProviderFunctionMapping)

	graph, operation, diags := c.applyGraph(ctx, plan, config, providerFunctionTracker, opts)
	if diags.HasErrors() {
		return nil, diags
	}

	workingState := plan.PriorState.DeepCopy()
	walker, walkDiags := c.walk(ctx, graph, operation, &graphWalkOpts{
		Config:     config,
		InputState: workingState,
		Changes:    plan.Changes,

		// We need to propagate the check results from the plan phase,
		// because that will tell us which checkable objects we're expecting
		// to see updated results from during the apply step.
		PlanTimeCheckResults: plan.Checks,

		// We also want to propagate the timestamp from the plan file.
		PlanTimeTimestamp:       plan.Timestamp,
		ProviderFunctionTracker: providerFunctionTracker,
	})
	diags = diags.Append(walker.NonFatalDiagnostics)
	diags = diags.Append(walkDiags)

	// After the walk is finished, we capture a simplified snapshot of the
	// check result data as part of the new state.
	walker.State.RecordCheckResults(walker.Checks)

	newState := walker.State.Close()
	if plan.UIMode == plans.DestroyMode && !diags.HasErrors() {
		// NOTE: This is a vestigial violation of the rule that we mustn't
		// use plan.UIMode to affect apply-time behavior.
		// We ideally ought to just call newState.PruneResourceHusks
		// unconditionally here, but we historically didn't and haven't yet
		// verified that it'd be safe to do so.
		newState.PruneResourceHusks()

		// If this was a destroy operation, and everything else succeeded, but
		// there are instances that were forgotten (not destroyed).
		// Even though this was the intended outcome, some automations may depend on the success of destroy operation
		// to indicate the complete removal of resources
		if forgetCount > 0 {
			suppressError := opts != nil && opts.SuppressForgetErrorsDuringDestroy
			if !suppressError {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Destroy was successful but left behind forgotten instances",
					`As requested, OpenTofu has not deleted some remote objects that are no longer managed by this configuration. Those objects continue to exist in their remote system and so may continue to incur charges. Refer to the original plan for more information.
To suppress this error for the future 'destroy' runs, you can add the CLI flag "-suppress-forget-errors".`,
				))
			}
		}
	}

	if len(plan.TargetAddrs) > 0 || len(plan.ExcludeAddrs) > 0 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Warning,
			"Applied changes may be incomplete",
			`The plan was created with the -target or the -exclude option in effect, so some changes requested in the configuration may have been ignored and the output values may not be fully updated. Run the following command to verify that no other changes are pending:
    tofu plan
	
Note that the -target and -exclude options are not suitable for routine use, and are provided only for exceptional situations such as recovering from errors or mistakes, or when OpenTofu specifically suggests to use it as part of an error message.`,
		))
	}

	// FIXME: we cannot check for an empty plan for refresh-only, because root
	// outputs are always stored as changes. The final condition of the state
	// also depends on some cleanup which happens during the apply walk. It
	// would probably make more sense if applying a refresh-only plan were
	// simply just returning the planned state and checks, but some extra
	// cleanup is going to be needed to make the plan state match what apply
	// would do. For now we can copy the checks over which were overwritten
	// during the apply walk.
	// Despite the intent of UIMode, it must still be used for apply-time
	// differences in destroy plans too, so we can make use of that here as
	// well.
	if plan.UIMode == plans.RefreshOnlyMode {
		newState.CheckResults = plan.Checks.DeepCopy()
	}

	return newState, diags
}

func (c *Context) applyGraph(ctx context.Context, plan *plans.Plan, config *configs.Config, providerFunctionTracker ProviderFunctionMapping, applyOpts *ApplyOpts) (*Graph, walkOperation, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	variables, vDiags := c.mergePlanAndApplyVariables(config, plan, applyOpts)
	diags = diags.Append(vDiags)
	if diags.HasErrors() {
		return nil, walkApply, diags
	}

	operation := walkApply
	if plan.UIMode == plans.DestroyMode {
		// FIXME: Due to differences in how objects must be handled in the
		// graph and evaluated during a complete destroy, we must continue to
		// use plans.DestroyMode to switch on this behavior. If all objects
		// which require special destroy handling can be tracked in the plan,
		// then this switch will no longer be needed and we can remove the
		// walkDestroy operation mode.
		// TODO: Audit that and remove walkDestroy as an operation mode.
		operation = walkDestroy
	}

	graph, moreDiags := (&ApplyGraphBuilder{
		Config:                  config,
		Changes:                 plan.Changes,
		State:                   plan.PriorState,
		RootVariableValues:      variables,
		Plugins:                 c.plugins,
		Targets:                 plan.TargetAddrs,
		Excludes:                plan.ExcludeAddrs,
		ForceReplace:            plan.ForceReplaceAddrs,
		Operation:               operation,
		ExternalReferences:      plan.ExternalReferences,
		ProviderFunctionTracker: providerFunctionTracker,
	}).Build(ctx, addrs.RootModuleInstance)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return nil, walkApply, diags
	}

	return graph, operation, diags
}

// ApplyGraphForUI is a last vestige of graphs in the public interface of
// Context (as opposed to graphs as an implementation detail) intended only for
// use by the "tofu graph" command when asked to render an apply-time
// graph.
//
// The result of this is intended only for rendering to the user as a dot
// graph, and so may change in future in order to make the result more useful
// in that context, even if drifts away from the physical graph that OpenTofu
// Core currently uses as an implementation detail of planning.
func (c *Context) ApplyGraphForUI(plan *plans.Plan, config *configs.Config) (*Graph, tfdiags.Diagnostics) {
	// For now though, this really is just the internal graph, confusing
	// implementation details and all.

	var diags tfdiags.Diagnostics

	graph, _, moreDiags := c.applyGraph(context.TODO(), plan, config, make(ProviderFunctionMapping), nil)
	diags = diags.Append(moreDiags)
	return graph, diags
}

// mergePlanAndApplyVariables is meant to prepare InputValues for the apply phase.
//
// # Context:
// As requested in opentofu/opentofu#1922, we had to add the ability to specify variable's values
// during the apply too, not only during the plan command.
// Therefore, when a plan is created via `tofu plan -out <planfile>` and then applied with `tofu apply <planfile>`
// we need to be able to specify -var/-var-file/etc to allow configuring the variables that are not kept in the
// plan (encryption configuration, ephemeral variables).
//
// # mergePlanAndApplyVariables
// This gets the plan and the *ApplyOpts and builds the InputValues. The values saved in the plan have
// priority *when defined*, but the variables marked as ephemeral in the plan and values for those are searched in the ApplyOpts.
// The implementation is an incremental check from the basic value to the most specific one:
// * First, the initial value is cty.NilVal that will force later the variable node to check for its default value
// * Second, it tries to find the value of the variable in the ApplyOpts#SetVariables, and if it does, it overrides the value from the previous step with it
// * Third, it tries to find the value of the variable in the plans.Plan#VariableValues, and if it does, it overrides the value from the previous step with it
// * Last, it executed two validations to ensure that the resulted value matches its configuration and the plan content.
func (c *Context) mergePlanAndApplyVariables(config *configs.Config, plan *plans.Plan, opts *ApplyOpts) (InputValues, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	variables := map[string]*InputValue{}

	var inputVars map[string]*InputValue
	if opts != nil && opts.SetVariables != nil {
		inputVars = opts.SetVariables
	}

	// Check for variables not in configuration (bug)
	for name := range plan.VariableValues {
		if _, ok := config.Module.Variables[name]; !ok {
			// This should already be validated elsewhere, but we have this here just in case
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Missing variable in configuration",
				fmt.Sprintf("Plan variable %q not found in the given configuration", name),
			))
		}
	}
	for name := range inputVars {
		if _, ok := config.Module.Variables[name]; !ok {
			// This should already be validated elsewhere, but we have this here just in case
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Missing variable in configuration",
				fmt.Sprintf("Variable %q not found in the given configuration", name),
			))
		}
	}
	// no reason to process more if there are already errors
	if diags.HasErrors() {
		return nil, diags
	}

	for name, cfg := range config.Module.Variables {
		// The plan.VariableValues field only records variables that were actually
		// set by the caller in the PlanOpts, so we may need to provide
		// placeholders for any other variables that the user didn't set, in
		// which case OpenTofu will once again use the default value from the
		// configuration when we visit these variables during the graph walk.
		variables[name] = &InputValue{
			Value:      cty.NilVal,
			SourceType: ValueFromPlan,
		}

		// Pull the var value from the input vars
		var inputValue cty.Value
		inputVar, inputOk := inputVars[name]
		if inputOk {
			inputValue = inputVar.Value

			// Record the var in our return value
			variables[name] = inputVar
		}

		// Pull the var value from the plan vars
		var planValue cty.Value
		planVar, planOk := plan.VariableValues[name]
		if planOk {
			val, err := planVar.Decode(cty.DynamicPseudoType)
			if err != nil {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Invalid variable value in plan",
					fmt.Sprintf("Invalid value for variable %q recorded in plan file: %s.", name, err),
				))
				continue
			}
			planValue = val

			// Record the var in our return value (potentially overriding the above set)
			variables[name] = &InputValue{
				Value:      val,
				SourceType: ValueFromPlan,
			}
		}

		// If both are set, ensure they are identical.
		// This is applicable only for non-ephemeral variables, ephemeral values can be only in one of the source at once:
		// * Will be in the plan when `tofu apply` will be executed without a plan file
		// * Will be in the applyOpts when `tofu apply` will be executed with a plan file
		if planOk && inputOk {
			if inputValue.Equals(planValue).False() {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Mismatch between input and plan variable value",
					fmt.Sprintf("Value saved in the plan file for variable %q is different from the one given to the current command.", name),
				))
				continue
			}
		}

		// If an ephemeral variable have no default value configured and there is no value for it in plan or input,
		// then the value for this is required so ask for it.
		if plan.EphemeralVariables[name] && cfg.Required() && !inputOk && !planOk {
			// Ephemeral variables are not saved into the plan so these need to be passed during the apply too.
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  `No value for required variable`,
				Detail:   fmt.Sprintf("Variable %q is configured as ephemeral. This type of variables need to be given a value during `tofu plan` and also during `tofu apply`.", name),
				Subject:  cfg.DeclRange.Ptr(),
			})
			continue
		}
	}

	return variables, diags
}
