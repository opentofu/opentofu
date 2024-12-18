// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"iter"
	"log"
	"sync"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/dag"
	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Graph represents the graph that OpenTofu uses to represent resources
// and their dependencies.
type Graph struct {
	// Graph is the actual DAG. This is embedded so you can call the DAG
	// methods directly.
	dag.AcyclicGraph

	// Path is the path in the module tree that this Graph represents.
	Path addrs.ModuleInstance
}

func (g *Graph) DirectedGraph() dag.Grapher {
	return &g.AcyclicGraph
}

// Walk walks the graph with the given walker for callbacks. The graph
// will be walked with full parallelism, so the walker should expect
// to be called in concurrently.
func (g *Graph) Walk(ctx context.Context, walker GraphWalker) tfdiags.Diagnostics {
	return g.walk(ctx, walker)
}

func (g *Graph) walk(_ context.Context, walker GraphWalker) tfdiags.Diagnostics {
	// The callbacks for enter/exiting a graph
	evalCtx := walker.EvalContext()

	// We explicitly create the panicHandler before
	// spawning many go routines for vertex evaluation
	// to minimize the performance impact of capturing
	// the stack trace.
	panicHandler := logging.PanicHandlerWithTraceFn()

	// Walk the graph.
	walkFn := func(v dag.Vertex) (diags tfdiags.Diagnostics) {
		// the walkFn is called asynchronously, and needs to be recovered
		// separately in the case of a panic.
		defer panicHandler()

		log.Printf("[TRACE] vertex %q: starting visit (%T)", dag.VertexName(v), v)

		defer func() {
			if diags.HasErrors() {
				for _, diag := range diags {
					if diag.Severity() == tfdiags.Error {
						desc := diag.Description()
						log.Printf("[ERROR] vertex %q error: %s", dag.VertexName(v), desc.Summary)
					}
				}
				log.Printf("[TRACE] vertex %q: visit complete, with errors", dag.VertexName(v))
			} else {
				log.Printf("[TRACE] vertex %q: visit complete", dag.VertexName(v))
			}
		}()

		// vertexCtx is the context that we use when evaluating. This
		// is normally the context of our graph but can be overridden
		// with a GraphNodeModuleInstance impl.
		vertexCtx := evalCtx
		if pn, ok := v.(GraphNodeModuleInstance); ok {
			vertexCtx = walker.EnterPath(pn.Path())
			defer walker.ExitPath(pn.Path())
		}

		// If the node is exec-able, then execute it.
		if ev, ok := v.(GraphNodeExecutable); ok {
			diags = diags.Append(walker.Execute(vertexCtx, ev))
			if diags.HasErrors() {
				return
			}
		}

		return diags
	}

	return g.AcyclicGraph.Walk(walkFn)
}

// executeGraphNodes calls the Execute method of each [GraphNodeExecutable] in the
// given sequence, which must be finite, and returns all of the collected diagnostics.
//
// The Execute methods across all of the nodes are called concurrently. This function
// returns once all of the inner calls have returned.
//
// This is a temporary helper used in the early stages of our migration away from
// GraphNodeDynamicExpandable and the special idea of "subgraphs" that was formerly
// implemented in Graph.walk. This function is designed to allow a tail call (or similar)
// to this function to replace a previous use of DynamicExpand with only minimal changes
// to the original DynamicExpand logic, to reduce the risk of the change. All uses of
// this function should eventually be replaced by something simpler that doesn't involve
// the instantiation of any new graph nodes.
//
// Calling code that is being migrated away from this function MUST use evalCtx.PerformIO
// directly itself to limit the concurrency of any I/O operations, since that is no longer
// handled automatically by the graph walk machinery. Although this does now spread the
// responsibility more than it was before, it also gives the individual graph node
// implementations more flexibility in deciding exactly what is and is not considered to
// be an "I/O operation", including skipping the semaphore entirely when performing fast,
// CPU-bound work that has no need for limited concurrency.
func executeGraphNodes(nodes iter.Seq[GraphNodeExecutable], evalCtx EvalContext, op walkOperation) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	var diagsMu sync.Mutex
	var wg sync.WaitGroup

	for node := range nodes {
		wg.Add(1)
		// In Go 1.22 and later, "node" is a separate symbol in each
		// iteration of this loop and so is safe to capture into the
		// closure below. This would not have been safe in Go 1.21 and
		// earlier: https://go.dev/blog/loopvar-preview
		go func() {
			// If the node has its own module instance then we need to provide it an
			// EvalContext that is bound to that module instance. Otherwise we'll
			// just use the EvalContext we were given directly, so the node will
			// execute under the same context as its caller.
			localCtx := evalCtx
			if n, ok := node.(GraphNodeModuleInstance); ok {
				moduleInstAddr := n.Path()
				localCtx = localCtx.WithPath(moduleInstAddr)
			}

			nodeName := dag.VertexName(node)
			// We use evalCtx.PerformIO here to ensure that these not-yet-updated "subgraph"
			// implementations still respect the concurrency semaphore that was previously
			// enforced centrally by the graph walker.
			//
			// TODO: In future we'll plumb context.Context into here through changes to
			// the GraphNodeExecutable interface, but for now we just stub it since
			// we know GraphNodeExecutable implementers can't possibly accept a Context
			// anyway.
			moreDiags := evalCtx.PerformIO(context.TODO(), func(_ context.Context) tfdiags.Diagnostics {
				log.Printf("[TRACE] executeGraphNodes: executing %s", nodeName)
				moreDiags := node.Execute(localCtx, op)
				if moreDiags.HasErrors() {
					log.Printf("[TRACE] executeGraphNodes: execution of %s returned errors", nodeName)
				} else {
					log.Printf("[TRACE] executeGraphNodes: successfully executed %s", nodeName)
				}
				return moreDiags
			})
			diagsMu.Lock()
			diags = diags.Append(moreDiags)
			diagsMu.Unlock()
			wg.Done()
		}()
	}

	wg.Wait()
	return diags
}
