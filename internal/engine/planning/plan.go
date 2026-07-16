// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/collections"
	"github.com/opentofu/opentofu/internal/engine/plugins"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/shared"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// PlanOpts represents our various "planning options" that can change various
// details about how we perform the planning phase, and therefore also what
// actions we might propose to perform during a subsequent applying phase.
type PlanOpts struct {
	// Mode is the planning mode to use.
	//
	// Planning modes are mutually-exclusive and each represent significantly
	// different goals for the planning process. Whereas most other options
	// just change specific details of how we plan, a different planning mode
	// has a far more drastic, cross-cutting effect.
	Mode plans.Mode

	ForceReplace []addrs.AbsResourceInstance
}

// PlanChanges is the main entry point, taking a state snapshot from the end
// of the previous plan/apply round and an instantiated configuration (bound
// to some input variable definitions) and returning a plan containing a set of
// proposed actions.
func PlanChanges(ctx context.Context, opts *PlanOpts, prevRoundState *states.State, configInst *eval.ConfigInstance, providers plugins.Providers) (*plans.Plan, tfdiags.Diagnostics) {
	if opts == nil {
		panic("PlanChanges with nil PlanOpts") // caller must always provide valid PlanOpts
	}

	// We'll make the "shared" tracer also available to everything we call.
	tracer := contextTracer(ctx)
	ctx = shared.ContextWithTracer(ctx, &tracer.Tracer)

	switch opts.Mode {
	case plans.NormalMode:
		return normalPlan(ctx, opts, prevRoundState, configInst, providers)
	case plans.DestroyMode:
		// "Destroy" mode is pretty different in that it focuses mainly on
		// prevRoundState and only uses the config for preparing ephemeral
		// objects, so it has its own separate planning function.
		return destroyPlan(ctx, opts, prevRoundState, configInst, providers)
	case plans.RefreshOnlyMode:
		// TODO: Implement this mode
		// (Undecided yet whether it will get its own function, as with
		// DestroyMode, or if it'll share the normalPlan function and just force
		// the resource instance handling to generate no changes for anything.)
		return nil, tfdiags.New(tfdiags.Sourceless(
			tfdiags.Error,
			"Refresh-only mode not available yet",
			"The new language runtime does not yet support the \"refresh-only\" planning mode.",
		))
	default:
		// Should not get here because the cases above should be exhaustive
		// for all possible planning modes.
		panic(fmt.Sprintf("unrecognized planning mode %s", opts.Mode))
	}
}

func finalizePlan(ctx context.Context, intermediate *planContextResult, providers plugins.Providers) (*plans.Plan, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	effectiveReplaceOrders, selfDeps := findEffectiveReplaceOrders(intermediate.ResourceInstanceObjects)
	if len(selfDeps) != 0 {
		// TODO: _Should_ we return this error here? In principle we should only
		// get here if the evaluator failed to detect a self-reference, but in
		// theory that should be impossible and so maybe this is a "should never
		// happen" case, rather than a normal user-facing error?
		selfDeps := sortedResourceInstanceObjectAddrs(selfDeps.All())
		var detail strings.Builder
		detail.WriteString("The following objects depend on themselves either directly or indirectly:")
		for _, addr := range selfDeps {
			fmt.Fprintf(&detail, "\n  - %s", addr)
		}
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Self-referencing resource instances",
			detail.String(),
		))
	}

	changes, moreDiags := buildPlanChanges(ctx,
		intermediate.ResourceInstanceObjects,
		effectiveReplaceOrders,
		providers,
		intermediate.RootOutput,
	)
	diags = diags.Append(moreDiags)

	// newDeposedKeys tracks any new deposed keys we allocate while constructing
	// the execution graph, so we can avoid returning the same key twice.
	newDeposedKeys := addrs.MakeMap[addrs.AbsResourceInstance, collections.Set[addrs.DeposedKey]]()

	execGraph := buildExecutionGraph(
		intermediate.ResourceInstanceObjects,
		effectiveReplaceOrders,
		intermediate.RootOutput.Current.ResourceDependencies,
		func(instAddr addrs.AbsResourceInstance) addrs.DeposedKey {
			// TODO: We should probably factor this out somewhere else, once
			// the rest of the nearby code has settled down.

			var existingDeposed map[addrs.DeposedKey]*states.ResourceInstanceObjectSrc
			newDeposed := newDeposedKeys.Get(instAddr)
			inst := intermediate.RefreshedState.ResourceInstance(instAddr)
			if inst != nil {
				existingDeposed = inst.Deposed
			}

			// We'll just keep trying to allocate new keys until we get a
			// unique one. Deposed keys are effectively 32-bit unsigned integers
			// and so with 1000 deposed objects per instance there'd only be 0.01%
			// probability of colliding here, and that would be a ridiculous
			// number of deposed objects.
			i := 0
			for {
				i++
				if i == 8192 {
					// Something seems to have gone very wrong! We should not get here.
					panic(fmt.Sprintf("failed to allocate a unique deposed key for %s", instAddr))
				}

				key := addrs.NewDeposedKey()
				if _, exists := existingDeposed[key]; exists {
					continue
				}
				if newDeposed.Has(key) {
					continue
				}
				if newDeposed == nil {
					newDeposed = make(collections.Set[addrs.DeposedKey])
				}
				newDeposed[key] = struct{}{}
				return key
			}
		},
	)
	if logging.IsDebugOrHigher() {
		// FIXME: This can potentially contain sensitive values from the
		// configuration, so we should either remove this or change the
		// value representation to include sensitive value redactions.
		log.Println("[DEBUG] Planned execution graph:\n" + logging.Indent(execGraph.DebugRepr()))
	}

	return &plans.Plan{
		UIMode:       plans.NormalMode, // TODO: [PlanChanges] needs something analogous to [tofu.PlanOpts] for planning mode/options
		Changes:      changes,
		PrevRunState: intermediate.PrevRoundState,
		PriorState:   intermediate.RefreshedState,
		// TODO: various other fields that we need to actually make use
		// of this plan result. But this is intentionally just a partial
		// result for now because it's not clear that we'd even be using
		// plans.Plan in a final version of this new approach.

		// This is a special extra field used only by this new runtime,
		// as a probably-temporary place to keep the serialized execution
		// graph so we can round-trip it through saved plan files while
		// the CLI layer is still working in terms of [plans.Plan].
		ExecutionGraph: execGraph.Marshal(),

		Destroying: intermediate.Destroying,
	}, diags
}

func buildPlanChanges(
	ctx context.Context,
	objs *resourceInstanceObjects,
	effectiveReplaceOrders addrs.Map[addrs.AbsResourceInstanceObject, resourceInstanceReplaceOrder],
	providers plugins.Providers,
	rootOutput rootOutput,
) (*plans.Changes, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	changes := plans.NewChanges().SyncWrapper()

	for addr, obj := range objs.All() {
		change := obj.PlannedChange
		if change == nil {
			// We're only interested in the subset of objects that actually
			// have planned changes.
			continue
		}

		schema, moreDiags := providers.ResourceTypeSchema(ctx,
			obj.Provider,
			obj.Addr.InstanceAddr.Resource.Resource.Mode,
			obj.Addr.InstanceAddr.Resource.Resource.Type,
		)
		diags = diags.Append(moreDiags)
		if moreDiags.HasErrors() {
			continue // can't encode a change without a schema
		}

		changeSrc, err := change.Encode(schema)
		if err != nil {
			// TODO: Make this a proper diagnostic, since this can potentially
			// be user-facing if the provider returned something that's somehow
			// invalid. (That can only happen for built-in providers, because
			// for plugin-based providers we would already have used the schema
			// to decode the wire representation of this object.)
			diags = diags.Append(err)
			continue
		}
		if changeSrc.Action.IsReplace() {
			// We substitute the final effective change action now, to describe
			// the change accurately to the end-user.
			changeSrc.Action = effectiveReplaceOrders.Get(addr).ChangeAction()
		}

		changes.AppendResourceInstanceChange(changeSrc)
	}

	for name, value := range rootOutput.Current.OutputValues {
		absAddr := addrs.AbsOutputValue{OutputValue: addrs.OutputValue{Name: name}}
		valueSensitive := value.HasMark(marks.Sensitive)
		valueDeprecated := "" // TODO

		// Copied from node_output.go

		// if this is a root module, try to get a before value from the state for
		// the diff
		sensitiveBefore := false
		deprecatedBefore := ""
		before := cty.NullVal(cty.DynamicPseudoType)

		// is this output new to our state?
		newOutput := true

		prevValue, ok := rootOutput.Previous[name]
		if ok {
			before = prevValue.Value
			sensitiveBefore = prevValue.Sensitive
			deprecatedBefore = prevValue.Deprecated
			newOutput = false
		}

		// We will not show the value if either the before or after are marked
		// as sensitive. We can show the value again once sensitivity is
		// removed from both the config and the state.
		sensitiveChange := sensitiveBefore || valueSensitive

		// strip any marks here just to be sure we don't panic on the True comparison
		unmarkedVal, _ := value.UnmarkDeep()

		action := plans.Update
		switch {
		case value.IsNull() && before.IsNull() &&
			valueDeprecated == deprecatedBefore:
			// This is separate from the NoOp case below, since we can ignore
			// sensitivity here when there are only null values.
			// However, we still need to ensure deprecation update is going to
			// be written.
			action = plans.NoOp

		case newOutput:
			// This output was just added to the configuration
			action = plans.Create

		case value.IsWhollyKnown() &&
			unmarkedVal.Equals(before).True() &&
			valueSensitive == sensitiveBefore &&
			valueDeprecated == deprecatedBefore:
			// Sensitivity and deprecation must also match to be a NoOp.
			// Theoretically marks may not match here, but sensitivity is the
			// only one we can act on, and the state will have been loaded
			// without any marks to consider.
			action = plans.NoOp
		}

		change := &plans.OutputChange{
			Addr:      absAddr,
			Sensitive: sensitiveChange,
			Change: plans.Change{
				Action: action,
				Before: before,
				After:  value,
			},
		}

		cs, err := change.Encode()
		if err != nil {
			// Should never happen, since we just constructed this right above
			panic(fmt.Sprintf("planned change for %s could not be encoded: %s", absAddr, err))
		}
		log.Printf("[TRACE] Saving %s change for %s in changeset", change.Action, absAddr)
		changes.AppendOutputChange(cs) // add the new planned change

	}

	for name, prevValue := range rootOutput.Previous {
		absAddr := addrs.AbsOutputValue{OutputValue: addrs.OutputValue{Name: name}}

		value, ok := rootOutput.Current.OutputValues[name]
		if !ok || value.IsNull() {
			// Copied from node_output.go
			change := &plans.OutputChange{
				Addr:      absAddr,
				Sensitive: prevValue.Sensitive,
				Change: plans.Change{
					Action: plans.Delete,
					Before: prevValue.Value,
					After:  cty.NullVal(cty.DynamicPseudoType),
				},
			}

			cs, err := change.Encode()
			if err != nil {
				// Should never happen, since we just constructed this right above
				panic(fmt.Sprintf("planned change for %s could not be encoded: %s", absAddr, err))
			}
			log.Printf("[TRACE] Saving %s change for %s in changeset", change.Action, absAddr)

			changes.AppendOutputChange(cs)
		}
	}

	return changes.Close(), diags
}
