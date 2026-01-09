// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/opentofu/opentofu/internal/addrs"
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

	defer func() {
		// To make the subsequent code easier to scan, within this function
		// we use panics to handle errors from the writer and then turn
		// them back into normal error returns here. This is a local tradeoff
		// just to avoid the printing code in this function being interspersed
		// with repeated identical error-handling branches, but only the
		// printf function below should actually rely on it.
		p := recover()
		if err, ok := p.(error); ok {
			// FIXME: If this survives beyond the experimental phase of the
			// new language runtime then this should return a full error
			// diagnostic, not just the naive automatic transformation of
			// an error into one.
			diags = diags.Append(err)
		} else if p != nil {
			// We're not expecting any other panic here but we'll re-raise
			// it anyway just so wen don't accidentally bury it.
			panic(p)
		}
	}()
	printf := func(format string, args ...any) {
		_, err := fmt.Fprintf(w, format, args...)
		if err != nil {
			panic(err) // recovered and returned by the deferred function above
		}
	}

	// The "resource instance graph" is a simplified graph covering only the
	// relationships between resource instances and their provider instances.
	//
	// The following code therefore emits a node for each provider instance,
	// a node for each resource instance, and then edges between them reflecting
	// how result data flows from one node to another. Note that the edges in
	// this graph are "backwards" compared to a dependency graph: if data flows
	// from A to B then B must depend on A.
	//
	// FIXME: This is a naive implementation that renders all of the edges
	// in the graph, which is overwhelming because e.g. there's an edge from
	// a provider instance to every resource instance that uses it. We should
	// at perform a transitive reduction of the graph to reduce it to as few
	// edges as possible for readability, but that'll require actually
	// capturing all of the edges into a graph data structure first and then
	// rendering that, rather than working directly from the evaluator's
	// results.

	printf("strict digraph {\n")
	printf("  rankdir=TB;\n")
	printf("  node [shape=\"rect\",fontname=\"Helvetica\",color=\"#000000\",bgcolor = \"#ffffff\",align=\"left\"];\n")

	// Nodes for provider instances.
	for n := range evalglue.ProviderInstancesDeep(ctx, rootModuleInstance) {
		addr := n.Addr
		style := "solid"
		if addr.Config.Module.IsPlaceholder() {
			style = "dashed"
		}
		printf("  %s [label=%s,style=%s];\n", quoteStringForGraphviz(addr.String()), providerInstanceGraphvizString(addr), quoteStringForGraphviz(style))
	}
	// Nodes for resource instances.
	for n := range evalglue.ResourceInstancesDeep(ctx, rootModuleInstance) {
		addr := n.Addr
		style := "solid"
		if addr.IsPlaceholder() {
			style = "dashed"
		}
		printf("  %s [label=%s,style=%s];\n", quoteStringForGraphviz(addr.String()), resourceInstanceGraphvizString(addr), quoteStringForGraphviz(style))
	}
	// Edges into provider instances.
	for n := range evalglue.ProviderInstancesDeep(ctx, rootModuleInstance) {
		dstAddr := n.Addr
		for n := range n.ResourceInstanceDependencies(ctx) {
			srcAddr := n.Addr
			printf("  %s:s -> %s:n;\n", quoteStringForGraphviz(srcAddr.String()), quoteStringForGraphviz(dstAddr.String()))
		}
	}
	// Edges into resource instances (both from other resource instances, and from each instance's provider instance)
	for n := range evalglue.ResourceInstancesDeep(ctx, rootModuleInstance) {
		dstAddr := n.Addr
		if maybeProviderInst, _, diags := n.ProviderInstance(ctx); !diags.HasErrors() {
			if providerInst, ok := configgraph.GetKnown(maybeProviderInst); ok {
				srcAddr := providerInst.Addr
				printf("  %s:s -> %s:n;\n", quoteStringForGraphviz(srcAddr.String()), quoteStringForGraphviz(dstAddr.String()))
			} else {
				// If we don't know which provider instance would be used then
				// we'll insert a placeholder node to communicate that.
				placeholderName := dstAddr.String() + " ?provider"
				printf("  %s [label=\"unknown instance of\\n%s\",style=\"dashed\"];\n", quoteStringForGraphviz(placeholderName), escapeInGraphvizString(n.Provider.String()))
				printf("  %s:s -> %s:n;\n", quoteStringForGraphviz(placeholderName), quoteStringForGraphviz(dstAddr.String()))
			}
		}
		for n := range n.ResourceInstanceDependencies(ctx) {
			srcAddr := n.Addr
			printf("  %s:s -> %s:n;\n", quoteStringForGraphviz(srcAddr.String()), quoteStringForGraphviz(dstAddr.String()))
		}
	}

	printf("}\n")

	return nil
}

func providerInstanceGraphvizString(addr addrs.AbsProviderInstanceCorrect) string {
	var buf strings.Builder
	buf.WriteByte('"')
	moduleInstanceWriteGraphvizStringPrefix(addr.Config.Module, &buf)
	buf.WriteString(escapeInGraphvizString(addr.Config.Config.String()))
	if addr.Key != addrs.NoKey {
		buf.WriteString(escapeInGraphvizString(addr.Key.String()))
	}
	buf.WriteByte('"')
	return buf.String()
}

func resourceInstanceGraphvizString(addr addrs.AbsResourceInstance) string {
	var buf strings.Builder
	buf.WriteByte('"')
	moduleInstanceWriteGraphvizStringPrefix(addr.Module, &buf)
	buf.WriteString(escapeInGraphvizString(addr.Resource.String()))
	buf.WriteByte('"')
	return buf.String()
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
