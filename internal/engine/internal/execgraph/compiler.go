// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package execgraph

import (
	"context"
	"fmt"
	"iter"

	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Compile produces a compiled version of the graph which will, once executed,
// use the given arguments to interact with other parts of the broader system.
//
// The [Graph.Compile] function is guaranteed not call any methods on the given
// [exec.Operations] during compilation: it will be used only once the returned
// [CompiledGraph] is executed. In particular this means that it's okay for
// there to be a cyclic dependency between the Operations and the CompiledGraph
// so that the caller can use [CompiledGraph.ResourceInstanceValue] to satisfy
// requests from the evaluation system for final resource instance values, as
// long as the Operations object is updated with a pointer to the returned
// CompiledGraph object before executing the graph.
func (g *Graph) Compile(ops exec.Operations) (*CompiledGraph, tfdiags.Diagnostics) {
	ret := &CompiledGraph{
		resourceInstanceValues: addrs.MakeMap[addrs.AbsResourceInstance, func(ctx context.Context) cty.Value](),
		cleanupWorker:          workgraph.NewWorker(),
	}
	c := &compiler{
		sourceGraph:   g,
		compiledGraph: ret,
		ops:           ops,
		opResolvers:   make([]workgraph.Resolver[nodeResultRaw], len(g.ops)),
		opResults:     make([]workgraph.Promise[nodeResultRaw], len(g.ops)),
	}
	// We'll prepopulate all of the operation promises, and then the compiler
	// will arrange for them to each get wired where they need to be.
	for i := range c.opResults {
		// The "cleanupWorker" is initially the responsible worker, but
		// the compiler arranges for responsibility to transfer to per-operation
		// workers created dynamically as the graph is executed, so in the
		// happy path cleanupWorker should end up responsible for nothing
		// at the end. (If that isn't true then all of the remaining requests
		// will force-fail when the compiled graph gets garbage collected.)
		resolver, promise := workgraph.NewRequest[nodeResultRaw](ret.cleanupWorker)
		c.opResolvers[i] = resolver
		c.opResults[i] = promise
	}
	return c.Compile()
}

// compiler is a temporary object we use during compilation to coordinate
// between all of the different parts of the compilation process.
//
// After compilation is complete, only the object from the compiledGraph
// field remains as the result.
type compiler struct {
	sourceGraph   *Graph
	compiledGraph *CompiledGraph
	ops           exec.Operations

	// opResolvers and opResults track our requests for our operation results,
	// each of which should be resolved by one of the "steps" in the compiled
	// graph so that the data can then propagate between nodes.
	//
	// The indices of this slice correspond to the indices of sourceGraph.ops.
	// The promises in here are initially owned by compiledGraph.cleanupWorker,
	// but responsibility for them is transferred to the worker for each
	// operation's "step" in the compiled graph once they begin executing.
	opResolvers []workgraph.Resolver[nodeResultRaw]
	opResults   []workgraph.Promise[nodeResultRaw]

	// diags accumulates any problems we detect during the compilation process,
	// which are ultimately returned by [compiler.Compile] so that the caller
	// knows not to even try executing the result graph.
	diags tfdiags.Diagnostics
}

func (c *compiler) Compile() (*CompiledGraph, tfdiags.Diagnostics) {
	// Although the _execution_ of the compiled graph runs all of the steps
	// concurrently, the compiler itself is intentionally written as
	// sequential code in the hope of that making it easier to understand
	// and maintain, since it's inevitably quite self-referential as it
	// turns the source graph into a series of executable functions.

	// The operations are the main part of the graph we actually care about
	// because they represent externally-visible side-effects. We'll use
	// those as our main vehicle for compilation, producing compiled versions
	// of other nodes as we go along only as needed to satisfy the operations.
	opResolvers := c.opResolvers
	for opIdx, opDesc := range c.sourceGraph.ops {
		operands := newCompilerOperands(opDesc.opCode, c.compileOperands(opDesc.operands))
		var compileFunc func(operands *compilerOperands) nodeExecuteRaw
		switch opDesc.opCode {
		case opProviderInstanceConfig:
			compileFunc = c.compileOpProviderInstanceConfig
		case opProviderInstanceOpen:
			compileFunc = c.compileOpProviderInstanceOpen
		case opProviderInstanceClose:
			compileFunc = c.compileOpProviderInstanceClose
		case opResourceInstanceDesired:
			compileFunc = c.compileOpResourceInstanceDesired
		case opResourceInstancePrior:
			compileFunc = c.compileOpResourceInstancePrior
		case opManagedFinalPlan:
			compileFunc = c.compileOpManagedFinalPlan
		case opManagedApply:
			compileFunc = c.compileOpManagedApply
		case opManagedDepose:
			compileFunc = c.compileOpManagedDepose
		case opManagedAlreadyDeposed:
			compileFunc = c.compileOpManagedAlreadyDeposed
		case opManagedChangeAddr:
			compileFunc = c.compileOpManagedChangeAddr
		case opDataRead:
			compileFunc = c.compileOpDataRead
		case opEphemeralOpen:
			compileFunc = c.compileOpEphemeralOpen
		case opEphemeralState:
			compileFunc = c.compileOpEphemeralState
		case opEphemeralClose:
			compileFunc = c.compileOpEphemeralClose
		default:
			c.diags = c.diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Unsupported opcode in execution graph",
				fmt.Sprintf("Execution graph includes %s, but the compiler doesn't know how to handle it. This is a bug in OpenTofu.", opDesc.opCode),
			))
			continue
		}
		// The main execution function deals with the opCode-specific behavior,
		// but we need to wrap it in some general code that arranges for
		// the operation results to propagate through the graph using the
		// promises set up in [Graph.Compile].
		mainExec := compileFunc(operands)
		graphStep := func(parentCtx context.Context) tfdiags.Diagnostics {
			// Each operation's execution must have its own workgraph worker
			// that's responsible for resolving the associated promise, since
			// that allows us to detect if operations try to depend on their
			// own results, or if the implementation panics and thus causes
			// this worker to get garbage-collected.
			resolver := opResolvers[opIdx]
			worker := workgraph.NewWorker(resolver)
			ctx := grapheval.ContextWithWorker(parentCtx, worker)
			trackWorkgraphRequest(ctx, opIdx, resolver.RequestID())
			ret, ok, diags := mainExec(ctx)
			// Resolving the promise might allow dependent operations to begin.
			resolver.ReportSuccess(worker, nodeResultRaw{
				Value:       ret,
				CanContinue: ok,
				Diagnostics: diags,
			})
			return diags
		}
		c.compiledGraph.steps = append(c.compiledGraph.steps, graphStep)
	}
	if c.diags.HasErrors() {
		// Don't expose the likely-invalid compiled graph, then.
		return nil, c.diags
	}

	// Before we return we also need to fill in the resource instance values
	// so that it's possible to get the information needed to satisfy the
	// evaluation system.
	for _, elem := range c.sourceGraph.resourceInstanceResults.Elems {
		instAddr := elem.Key
		ref := elem.Value
		execFunc := c.compileResultRef(ref)
		c.compiledGraph.resourceInstanceValues.Put(instAddr, func(ctx context.Context) cty.Value {
			rawResult, ok, _ := execFunc(ctx)
			if !ok {
				return cty.DynamicVal
			}
			finalStateObj := rawResult.(*exec.ResourceInstanceObject)
			if finalStateObj == nil {
				return cty.NullVal(cty.DynamicPseudoType)
			}
			return finalStateObj.State.Value
		})
	}

	return c.compiledGraph, c.diags
}

func (c *compiler) compileOperands(refs []AnyResultRef) iter.Seq2[AnyResultRef, nodeExecuteRaw] {
	return func(yield func(AnyResultRef, nodeExecuteRaw) bool) {
		for _, ref := range refs {
			exec := c.compileResultRef(ref)
			if !yield(ref, exec) {
				return
			}
		}
	}
}

// compileResultRef transforms a result reference into a function that blocks
// until the associated result is ready and then returns that result as a
// value of type [any], which the caller could then cast into the concrete
// type that the result was expected to produce.
func (c *compiler) compileResultRef(ref AnyResultRef) nodeExecuteRaw {
	// The closures we return should only capture primitive values and
	// pointers to as small a part of the compiler's state as possible, so
	// that the overall compiler object can be garbage-collected once
	// compilation is complete.

	const errSummary = "Invalid execution graph"
	switch ref := ref.(type) {
	case valueResultRef:
		vals := c.sourceGraph.constantVals
		index := ref.index
		return func(_ context.Context) (any, bool, tfdiags.Diagnostics) {
			return vals[index], true, nil
		}
	case resourceInstAddrResultRef:
		resourceInstAddrs := c.sourceGraph.resourceInstAddrs
		index := ref.index
		return func(_ context.Context) (any, bool, tfdiags.Diagnostics) {
			return resourceInstAddrs[index], true, nil
		}
	case providerInstAddrResultRef:
		providerInstAddrs := c.sourceGraph.providerInstAddrs
		index := ref.index
		return func(_ context.Context) (any, bool, tfdiags.Diagnostics) {
			return providerInstAddrs[index], true, nil
		}
	case anyOperationResultRef:
		// Operations have different result types depending on their opcodes,
		// but at this point we just represent everything as "any" and expect
		// that the downstream operations that rely on these results will
		// type-assert them dynamically as needed.
		opResults := c.opResults
		index := ref.operationResultIndex()
		return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
			var diags tfdiags.Diagnostics
			promise := opResults[index]
			resultRaw, err := promise.Await(grapheval.WorkerFromContext(ctx))
			if err != nil {
				// An error here always means that the workgraph library has
				// detected a problem that might have caused a deadlock, which
				// during the apply phase is always a bug in OpenTofu because
				// we should've detected any user-caused problems during the
				// planning phase.
				diags = diags.Append(grapheval.DiagnosticsForWorkgraphError(ctx, err))
				return nil, false, diags
			}
			return resultRaw.Value, resultRaw.CanContinue, resultRaw.Diagnostics
		}
	case waiterResultRef:
		// In this case we'll precompile the results we're waiting for because
		// then we can catch certain graph consistency problems sooner.
		waitForRefs := c.sourceGraph.waiters[ref.index]
		waiters := make([]nodeExecuteRaw, len(waitForRefs))
		for i, waitForRef := range waitForRefs {
			waiters[i] = c.compileResultRef(waitForRef)
		}
		return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
			var diags tfdiags.Diagnostics
			callerCanContinue := true
			for _, waiter := range waiters {
				_, ok, moreDiags := waiter(ctx)
				diags = diags.Append(moreDiags)
				if !ok {
					// We'll remember that the caller is supposed to stop
					// but we'll continue through our set of waiters in case
					// we find any other diagnostics to propagate.
					callerCanContinue = false
				}
			}
			return struct{}{}, callerCanContinue, diags
		}
	case nil:
		return func(ctx context.Context) (any, bool, tfdiags.Diagnostics) {
			// We use nil to represent the zero value of any type.
			return nil, true, nil
		}
	default:
		c.diags = append(c.diags, tfdiags.Sourceless(
			tfdiags.Error,
			errSummary,
			fmt.Sprintf("The execution graph includes %#v, but the compiler doesn't know how to handle it. This is a bug in OpenTofu.", ref),
		))
		return nil
	}
}

// nodeExecuteRaw is the lowest-level representation of producing a result,
// without any static type information yet.
//
// If the returned diagnostics includes errors then the caller must not try
// to type-assert the first result, and should instead just return the
// diagnostics along with its own nil result.
type nodeExecuteRaw = func(ctx context.Context) (any, bool, tfdiags.Diagnostics)

// nodeExecute is the type of a function that blocks until the result of a node
// is available and then returns that result.
//
// The boolean result is true if the caller is allowed to take any action based
// on the result. If it is false then the callers should ignore the T result
// and immediately return, on the assumption that something upstream has failed
// and will have already returned some diagnostics.
type nodeExecute[T any] func(ctx context.Context) (T, bool, tfdiags.Diagnostics)

type nodeResultRaw struct {
	Value       any
	CanContinue bool
	Diagnostics tfdiags.Diagnostics
}
