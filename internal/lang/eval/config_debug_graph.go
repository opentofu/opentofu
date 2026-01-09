// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"
	"io"
	"strings"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/dag"
	"github.com/opentofu/opentofu/internal/dag/graphviz"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// WriteGraphvizGraphForDebugging writes to the given writer a description of
// the "resource instance graph" implied by this configuration, in the Graphviz
// language and intended for use with the Graphviz layout engine "dot".
//
// This is intended for human debugging use rather than as a dependable
// machine-readable integration point. The specific ways this function
// represents elements of the graph using Graphviz language features is subject
// to change at any time.
func (c *ConfigInstance) WriteGraphvizGraphForDebugging(ctx context.Context, w io.Writer) (diags tfdiags.Diagnostics) {
	// All of our work will be associated with a workgraph worker that serves
	// as the initial worker node in the work graph.
	ctx = grapheval.ContextWithNewWorker(ctx)

	rootModuleInstance, diags := c.precheckedModuleInstance(ctx)
	if diags.HasErrors() {
		return diags
	}

	// The "resource instance graph" is a reduced graph covering only the
	// relationships between resource instances and their provider instances.
	//
	// The following code therefore emits a node for each provider instance,
	// a node for each resource instance, and then edges between them reflecting
	// how result data flows from one node to another. Note that the edges in
	// this graph are "backwards" compared to a dependency graph: if data flows
	// from A to B then B must depend on A.
	//
	// We use the graph representation from our "dag" package here, and its
	// helpers for Graphviz graphs in particular, just because it contains some
	// algorithms that are helpful in making the internal graph easier for
	// humans to consume, by removing edges that are not strictly necessary.

	g := &dag.AcyclicGraph{}

	// First we'll insert all of the nodes, so that we'll be able to retrieve
	// them from these maps when we're inserting the edges below.
	piNodes := addrs.MakeMap[addrs.AbsProviderInstanceCorrect, graphviz.Node]()
	riNodes := addrs.MakeMap[addrs.AbsResourceInstance, graphviz.Node]()
	for pi := range evalglue.ProviderInstancesDeep(ctx, rootModuleInstance) {
		addr := pi.Addr
		style := "solid"
		if addr.Config.Module.IsPlaceholder() {
			style = "dashed"
		}
		n := graphviz.Node{
			ID: addr.String(),
			Attrs: map[string]graphviz.Value{
				"label": graphviz.Val(providerInstanceGraphvizString(addr)),
				"style": graphviz.Val(style),
			},
		}
		g.Add(n)
		piNodes.Put(addr, n)
	}
	for n := range evalglue.ResourceInstancesDeep(ctx, rootModuleInstance) {
		addr := n.Addr
		style := "solid"
		if addr.IsPlaceholder() {
			style = "dashed"
		}
		n := graphviz.Node{
			ID: addr.String(),
			Attrs: map[string]graphviz.Value{
				"label": graphviz.Val(resourceInstanceGraphvizString(addr)),
				"style": graphviz.Val(style),
			},
		}
		g.Add(n)
		riNodes.Put(addr, n)
	}

	// Now we've generated a [graphviz.Node] for each of our nodes we can
	// generate the edges between them.
	for n := range evalglue.ProviderInstancesDeep(ctx, rootModuleInstance) {
		dstAddr := n.Addr
		for n := range n.ResourceInstanceDependencies(ctx) {
			srcAddr := n.Addr
			src := riNodes.Get(srcAddr)
			dst := piNodes.Get(dstAddr)
			g.Connect(dag.BasicEdge(src, dst))
		}
	}
	for n := range evalglue.ResourceInstancesDeep(ctx, rootModuleInstance) {
		dstAddr := n.Addr
		dst := riNodes.Get(dstAddr)
		if maybeProviderInst, _, diags := n.ProviderInstance(ctx); !diags.HasErrors() {
			if providerInst, ok := configgraph.GetKnown(maybeProviderInst); ok {
				srcAddr := providerInst.Addr
				src := piNodes.Get(srcAddr)
				g.Connect(dag.BasicEdge(src, dst))
			} else {
				// If we don't know which provider instance would be used then
				// we'll insert a placeholder node to communicate that.
				placeholder := graphviz.Node{
					ID: dstAddr.String() + " ?provider",
					Attrs: map[string]graphviz.Value{
						"label": graphviz.Val("unknown instance of\n" + n.Provider.String()),
						"style": graphviz.Val("dashed"),
					},
				}
				g.Add(placeholder)
				g.Connect(dag.BasicEdge(placeholder, dst))
			}
		}
		for n := range n.ResourceInstanceDependencies(ctx) {
			srcAddr := n.Addr
			src := riNodes.Get(srcAddr)
			g.Connect(dag.BasicEdge(src, dst))
		}
	}

	// The resource instance graph produce by the evaluator is comprehensive
	// in that e.g. all resource instances refer to their corresponding provider
	// instance even when they depend on another resource instance that refers
	// to the same provider, and so a direct rendering would typically be
	// overwhelming.
	//
	// We therefore compute a transitive reduction of the graph so that we're
	// only displaying the minimum edges required to represent the
	// reachability of the resource instance graph.
	g.TransitiveReduction()

	gg := &graphviz.Graph{
		Content: &g.Graph,
		Attrs: map[string]graphviz.Value{
			"rankdir": graphviz.Val("TB"),
		},
		DefaultNodeAttrs: map[string]graphviz.Value{
			"shape":    graphviz.Val("rect"),
			"fontname": graphviz.Val("Helvetica"),
			"color":    graphviz.Val("#000000"),
			"bgcolor":  graphviz.Val("#ffffff"),
			"align":    graphviz.Val("left"),
		},
		DefaultEdgeDirectionOut: graphviz.EdgeAttachmentSouth,
		DefaultEdgeDirectionIn:  graphviz.EdgeAttachmentNorth,
	}

	err := graphviz.WriteDirectedGraph(gg, w)
	if err != nil {
		// FIXME: If this codepath survives into a released version then we
		// should make this generate a more user-appropriate diagnostic message,
		// rather than just directly reporting whatever the writer produced.
		diags = diags.Append(err)
	}
	return diags
}

func providerInstanceGraphvizString(addr addrs.AbsProviderInstanceCorrect) graphviz.PrequotedValue {
	var buf strings.Builder
	buf.WriteByte('"')
	moduleInstanceWriteGraphvizStringPrefix(addr.Config.Module, &buf)
	buf.WriteString(escapeInGraphvizString(addr.Config.Config.String()))
	if addr.Key != addrs.NoKey {
		buf.WriteString(escapeInGraphvizString(addr.Key.String()))
	}
	buf.WriteByte('"')
	return graphviz.PrequotedValue(buf.String())
}

func resourceInstanceGraphvizString(addr addrs.AbsResourceInstance) graphviz.PrequotedValue {
	var buf strings.Builder
	buf.WriteByte('"')
	moduleInstanceWriteGraphvizStringPrefix(addr.Module, &buf)
	buf.WriteString(escapeInGraphvizString(addr.Resource.String()))
	buf.WriteByte('"')
	return graphviz.PrequotedValue(buf.String())
}

func moduleInstanceWriteGraphvizStringPrefix(addr addrs.ModuleInstance, buf *strings.Builder) {
	// The graphviz layout engine works best when graph nodes are not
	// excessively wide, so when rendering the addresses of items nested
	// inside module instances we stack the module instance steps vertically
	// instead of horizontally. The "\l" escape sequences here are how the
	// Graphviz language expresses "left-align this line and then begin a new
	// line".
	for _, step := range addr {
		buf.WriteString("module.")
		buf.WriteString(escapeInGraphvizString(step.Name))
		if step.InstanceKey != addrs.NoKey {
			buf.WriteString(escapeInGraphvizString(step.InstanceKey.String()))
		}
		buf.WriteString("\\l")
	}
}

func quoteStringForGraphviz(input string) string {
	return `"` + escapeInGraphvizString(input) + `"`
}

func escapeInGraphvizString(input string) string {
	var buf strings.Builder
	buf.Grow(len(input)) // we'll produce at least as many bytes as we were given
	for _, r := range input {
		switch r {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		default:
			buf.WriteRune(r)
		}
	}
	return buf.String()
}
