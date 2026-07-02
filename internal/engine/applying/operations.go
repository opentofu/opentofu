// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package applying

import (
	"context"
	"fmt"
	"log"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/engine/internal/execgraph"
	"github.com/opentofu/opentofu/internal/engine/plugins"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

type execOperations struct {
	// priorState is the state that was current at the end of the planning
	// phase, which the actions in the execution graph are intended to start
	// from. This must remain unmodified throughout graph execution.
	//
	// FIXME: This is using states.SyncState just because it's a convenient
	// type we already have available which allows us to access the "full"
	// representation of resource instance objects, but this is intended to
	// be an immutable data structure so we don't actually need mutex guards
	// on access to it, so in future we should replace this with a more direct
	// data structure.
	priorState *states.SyncState

	// configOracle gives us access to certain information we need that is
	// produced by the configuration evaluator.
	configOracle *eval.ApplyOracle

	// workingState is the state we modify during execution.
	//
	// TODO: Do we want to continue using states.SyncState for this, or should
	// we adopt a new strategy where we're writing directly to the real state
	// storage as we go along? A more direct approach would be better if we
	// want to do "granular state storage" in future, but is overkill for now
	// when we are only able to flush whole state snapshots anyway.
	//
	// TODO: Need to figure out some equivalent to the traditional runtime's
	// periodic snapshotting of state to remote storage throughout the apply
	// phase. Right now we just wait until the end and save everything at once,
	// but that's risky because the session we use to write to state storage
	// might become invalid before the apply phase is complete.
	workingState *states.SyncState

	// plugins are the provider and provisioner plugins we have available for
	// use during the apply phase.
	plugins plugins.Plugins

	// destroying is a flag to alter behavior for non-resource actions during apply.
	destroying bool
}

// The main operation methods of execOperations are spread across the separate
// files matching operations_*.go, grouped by what kinds of object they each
// primarily interact with.

var _ exec.Operations = (*execOperations)(nil)

func compileExecutionGraph(ctx context.Context, plan *plans.Plan, oracle *eval.ApplyOracle, plugins plugins.Plugins) (*execgraph.Graph, *execgraph.CompiledGraph, *execOperations, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	execGraph, err := execgraph.UnmarshalGraph(plan.ExecutionGraph)
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid execution graph in plan",
			fmt.Sprintf("Failed to unmarshal the execution graph: %s.", tfdiags.FormatError(err)),
		))
		return nil, nil, nil, diags
	}
	if logging.IsDebugOrHigher() {
		// FIXME: The DebugRepr currently includes ctydebug representations of
		// values, which means it'd expose sensitive values. We should consider
		// changing it to use a different notation that redacts sensitive
		// values, perhaps in a similar way to how our plan renderer behaves.
		log.Println("[DEBUG] Execution graph:\n" + logging.Indent(execGraph.DebugRepr()))
	}

	// The graph compiler promises not to call any methods of the given ops
	// during compilation, so we'll leave it unpopulated here and then fill
	// in its fields afterwards to make the combined system ready to execute.
	ops := &execOperations{}
	compiledGraph, moreDiags := execGraph.Compile(ops)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return nil, nil, nil, diags
	}

	ops.priorState = plan.PriorState.SyncWrapper()
	ops.workingState = plan.PriorState.DeepCopy().SyncWrapper()
	ops.configOracle = oracle
	ops.plugins = plugins
	ops.destroying = plan.Destroying

	return execGraph, compiledGraph, ops, diags
}

// Finish closes the operations object and returns its final state snapshot.
//
// This function must be called only once execution is complete and no other
// calls to methods of this type are running concurrently. After calling this
// function the operations object is invalid and must not be used anymore.
func (ops *execOperations) Finish(ctx context.Context) (*states.State, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	var rootOutputs map[string]cty.Value
	if !ops.destroying {
		// Save the root outputs before closing the config oracle
		rootOutputs = ops.configOracle.RootOutputs(ctx)
	}

	// Close the config oracle (checkAll and providers close)
	moreDiags := ops.configOracle.Close(ctx)
	diags = diags.Append(moreDiags)

	// Snapshot the working SyncState
	finalState := ops.workingState.Close()

	// This operations object is now invalid and must not be used any further,
	// which we'll reinforce by disconnecting it from everything else so any
	// future call is likely to panic.
	ops.workingState = nil
	ops.priorState = nil
	ops.plugins = nil
	ops.configOracle = nil

	// Clear out the existing output values from the state as they are
	// all now invalid.
	for _, mod := range finalState.Modules {
		clear(mod.OutputValues)
	}
	// If we are not destroying, we need to add the root module outputs.
	if !ops.destroying {
		for k, v := range rootOutputs {
			// Known is a placeholder for if the output is available due to deferring or targeting (TODO better signal for this)
			outputNotDeferred := v.IsWhollyKnown()
			if outputNotDeferred {
				if v.IsNull() {
					// Not saved
					continue
				}
				unmarkedVal, _ := v.UnmarkDeep()
				sensitive := v.HasMark(marks.Sensitive)
				deprecated := "" // TODO
				finalState.EnsureModule(addrs.RootModuleInstance).SetOutputValue(k, unmarkedVal, sensitive, deprecated)
			} else {
				prev := ops.priorState.OutputValue(addrs.AbsOutputValue{OutputValue: addrs.OutputValue{Name: k}})
				if prev != nil {
					finalState.EnsureModule(addrs.RootModuleInstance).SetOutputValue(k, prev.Value, prev.Sensitive, prev.Deprecated)
				}
			}
		}
	}

	// This function returns diagnostics to reserve the right to do fallible
	// encoding or flushing operations here in future, but for right now we're
	// just returning the state data structure directly and so this cannot fail.
	return finalState, diags
}
