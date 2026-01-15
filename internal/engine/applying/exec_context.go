// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package applying

import (
	"context"
	"fmt"
	"log"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/execgraph"
	"github.com/opentofu/opentofu/internal/engine/plugins"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type executionContext struct {
	// priorState is the state that was current at the end of the planning
	// phase, which the actions in the execution graph are intended to start
	// from. This must remain unmodified throughout graph execution.
	//
	// FIXME: This uses a syncState type because in the first iteration of this
	// we also had a mutable "working state". However, that's now gone because
	// the compiled graph already tracks enough information to act as the working
	// state, and so we could simplify this to be just an immutable map of
	// pre-decoded state objects without any mutex.
	priorState *syncState
	oracle     *eval.ApplyOracle
	plugins    plugins.Plugins

	// graph refers back to the execution graph that this object was
	// instantiated for.
	//
	// The compiled graph also has a pointer to the execution context, so this
	// field must be populated only after the graph has already been
	// successfully compiled.
	graph *execgraph.CompiledGraph
}

func compileExecutionGraph(ctx context.Context, plan *plans.Plan, oracle *eval.ApplyOracle, plugins plugins.Plugins) (*execgraph.CompiledGraph, *executionContext, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	execGraph, err := execgraph.UnmarshalGraph(plan.ExecutionGraph)
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid execution graph in plan",
			fmt.Sprintf("Failed to unmarshal the execution graph: %s.", tfdiags.FormatError(err)),
		))
		return nil, nil, diags
	}
	if logging.IsDebugOrHigher() {
		// FIXME: The DebugRepr currently includes ctydebug representations of
		// values, which means it'd expose sensitive values. We should consider
		// changing it to use a different notation that redacts sensitive
		// values, perhaps in a similar way to how our plan renderer behaves.
		log.Println("[DEBUG] Execution graph:\n" + logging.Indent(execGraph.DebugRepr()))
	}

	// There is a cyclic dependency between the execution context and the
	// compiled execution graph, so we compile the graph against an unpopulated
	// execution context first and then populate it afterwards. This is okay
	// because the execution graph compiler guarantees that it won't call
	// any methods on the execution context until the graph is actually
	// executed.
	execCtx := &executionContext{}
	compiledGraph, moreDiags := execGraph.Compile(execCtx)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return nil, nil, diags
	}
	execCtx.graph = compiledGraph

	// We pre-populate a bunch of other information here too so that we can
	// have a single place to catch errors involving inconsistencies in the
	// input, missing provider dependencies, etc.
	priorState, moreDiags := syncStateFromPriorState(ctx, plan.PriorState, plugins)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return nil, nil, diags
	}
	execCtx.priorState = priorState

	execCtx.oracle = oracle
	execCtx.plugins = plugins

	return compiledGraph, nil, diags
}

// DesiredResourceInstance implements [execgraph.ExecContext].
func (c *executionContext) DesiredResourceInstance(ctx context.Context, addr addrs.AbsResourceInstance) *eval.DesiredResourceInstance {
	// FIXME: We're currently just ignoring the diagnostics completely here
	// because this API was modelled after the apply phase where we proactively
	// walk the entire configuration collecting diagnostics for everything,
	// but we don't do that during the apply phase so we should instead model
	// the "desired resource instance" request as a fallible operation here.
	ret, _ := c.oracle.DesiredResourceInstance(ctx, addr)
	return ret
}

// NewProviderClient implements [execgraph.ExecContext].
func (c *executionContext) NewProviderClient(ctx context.Context, addr addrs.Provider, configVal cty.Value) (providers.Configured, tfdiags.Diagnostics) {
	return c.plugins.NewConfiguredProvider(ctx, addr, configVal)
}

// ProviderInstanceConfig implements [execgraph.ExecContext].
func (c *executionContext) ProviderInstanceConfig(ctx context.Context, addr addrs.AbsProviderInstanceCorrect) cty.Value {
	// FIXME: We're currently just ignoring the diagnostics completely here
	// because this API was modelled after the apply phase where we proactively
	// walk the entire configuration collecting diagnostics for everything,
	// but we don't do that during the apply phase so we should instead model
	// the "provider instance config" request as a fallible operation here.
	ret, _ := c.oracle.ProviderInstanceConfig(ctx, addr)
	return ret
}

// ResourceInstancePriorState implements [execgraph.ExecContext].
func (c *executionContext) ResourceInstancePriorState(ctx context.Context, addr addrs.AbsResourceInstance, deposedKey states.DeposedKey) *states.ResourceInstanceObjectFull {
	return c.priorState.ResourceInstanceObject(addr, deposedKey)
}

func (c *executionContext) Finish(ctx context.Context) (*states.State, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	ret := states.NewState()
	// TODO: Collect up all of the results from the compiled execution graph,
	// which should by now have finished executing and so should have
	// final values for anything it was able to apply. If execution ended early
	// due to an error then we should also preserve the relevant resource
	// instance objects from the prior state here.
	return ret, diags
}

var _ execgraph.ExecContext = (*executionContext)(nil)
