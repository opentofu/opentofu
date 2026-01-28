// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package execgraph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"iter"
	"maps"
	"slices"
	"strings"

	"github.com/zclconf/go-cty-debug/ctydebug"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/opentofu/opentofu/internal/states"
)

// DebugRepr returns a relatively-concise string representation of the
// graph which includes all of the registered operations and their operands,
// along with any constant values they rely on.
//
// The result is intended primarily for human consumption when testing or
// debugging. It's not an executable or parseable representation and details
// about how it's formatted might change over time.
func (g *Graph) DebugRepr() string {
	var buf strings.Builder
	for idx, val := range g.constantVals {
		fmt.Fprintf(&buf, "v[%d] = %s;\n", idx, strings.TrimSpace(ctydebug.ValueString(val)))
	}
	if len(g.constantVals) != 0 && (len(g.ops) != 0 || g.resourceInstanceResults.Len() != 0) {
		buf.WriteByte('\n')
	}
	for idx, op := range g.ops {
		fmt.Fprintf(&buf, "r[%d] = %s(", idx, strings.TrimLeft(op.opCode.String(), "op"))
		for opIdx, result := range op.operands {
			if opIdx != 0 {
				buf.WriteString(", ")
			}
			buf.WriteString(g.resultDebugRepr(result))
		}
		buf.WriteString(");\n")
	}
	if g.resourceInstanceResults.Len() != 0 && (len(g.ops) != 0 || len(g.constantVals) != 0) {
		buf.WriteByte('\n')
	}
	// We'll sort the resource instance results by instance address key just
	// so that the resulting order is consistent for comparison in tests.
	resourceInstanceResults := make(map[string]string)
	for _, elem := range g.resourceInstanceResults.Elems {
		resourceInstanceResults[elem.Key.String()] = g.resultDebugRepr(elem.Value)
	}
	resourceInstanceAddrs := slices.Collect(maps.Keys(resourceInstanceResults))
	slices.Sort(resourceInstanceAddrs)
	for _, addrStr := range resourceInstanceAddrs {
		fmt.Fprintf(&buf, "%s = %s;\n", addrStr, resourceInstanceResults[addrStr])
	}
	return buf.String()
}

func (g *Graph) resultDebugRepr(result AnyResultRef) string {
	switch result := result.(type) {
	case valueResultRef:
		return fmt.Sprintf("v[%d]", result.index)
	case providerAddrResultRef:
		providerAddr := g.providerAddrs[result.index]
		return fmt.Sprintf("provider(%q)", providerAddr)
	case desiredResourceInstanceResultRef:
		instAddr := g.desiredStateRefs[result.index]
		return fmt.Sprintf("desired(%s)", instAddr)
	case resourceInstancePriorStateResultRef:
		ref := g.priorStateRefs[result.index]
		if ref.DeposedKey != states.NotDeposed {
			return fmt.Sprintf("deposedState(%s, %s)", ref.ResourceInstance, ref.DeposedKey)
		}
		return fmt.Sprintf("priorState(%s)", ref.ResourceInstance)
	case providerInstanceConfigResultRef:
		pInstAddr := g.providerInstConfigRefs[result.index]
		return fmt.Sprintf("providerInstConfig(%s)", pInstAddr)
	case anyOperationResultRef:
		return fmt.Sprintf("r[%d]", result.operationResultIndex())
	case waiterResultRef:
		awaiting := g.waiters[result.index]
		var buf strings.Builder
		buf.WriteString("await(")
		for i, r := range awaiting {
			if i != 0 {
				buf.WriteString(", ")
			}
			buf.WriteString(g.resultDebugRepr(r))
		}
		buf.WriteString(")")
		return buf.String()
	case nil:
		return "nil"
	default:
		// Should try to keep the above cases comprehensive because
		// this default is not very readable and might even be
		// useless if it's a reference into a table we're not otherwise
		// including the output here.
		return fmt.Sprintf("%#v", result)
	}
}

// WriteGraphvizRepr writes to the given writer a Graphviz-compatible
// representation of the execution graph, intended primarily for the purpose of
// debugging by humans and with details subject to change over time in future
// versions.
func (g *Graph) WriteGraphvizRepr(w io.Writer) (err error) {
	defer func() {
		// To make the subsequent code easier to scan, within this function
		// we use panics to handle errors from the writer and then turn
		// them back into normal error returns here. This is a local tradeoff
		// just to avoid the printing code in this function being interspersed
		// with repeated identical error-handling branches, but only the
		// printf function below should actually rely on it.
		p := recover()
		if e, ok := p.(error); ok {
			err = e
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

	printf("digraph {\n")
	printf("  rankdir=LR;\n")
	printf("  node [fontname=\"Helvetica\",color=\"#000000\",bgcolor = \"#ffffff\"];\n")

	for idx, op := range g.ops {
		label := g.operationGraphvizRepr(idx, op)
		printf("  r%d [shape=none,label=<%s>];\n", idx, label)
		_ = op
	}

	// With all of the operations represented as nodes, we can now describe
	// the data-flow edges between them. These arrows reflect the direction
	// of data flow (from result to dependent argument) rather than representing
	// dependencies, so the arrows are intentionally "backwards" compared to a
	// normal dependency graph.
	for opIdx, op := range g.ops {
		for argIdx, anyRef := range op.operands {
			for ref := range g.indirectlyNeededOperationResults(anyRef) {
				refIdx := ref.operationResultIndex()
				// This connects the east edge of the source operation's "result"
				// cell to the west edge of the destination argument's cell.
				printf("  r%d:r:e -> r%d:a%d:w;\n", refIdx, opIdx, argIdx)
			}
		}
	}

	printf("}\n")
	return
}

// operationGraphvizRepr returns a representation of the given operation using
// Graphviz's "HTML-like" node label syntax to produce a vertical-stacked table
// representation of the operation type, arguments, and result.
//
// The result has appropriate escaping to be included directly inside the
// "<" and ">" delimiters used for Graphviz's HTML-like-label syntax, without
// needing any further transformation.
func (g *Graph) operationGraphvizRepr(idx int, op operationDesc) string {
	var buf strings.Builder
	buf.WriteString(`<table border="0" cellborder="1" cellspacing="0">`)

	// Header row, naming the operation type
	// (Some extra spacing around the caption because the Graphviz layout engine
	// can get a little confused by bold text being wider than normal text in
	// the same font family, making the container too small for its content.)
	buf.WriteString(`<tr><td bgcolor="#90c0e0" align="center"><b>  `)
	buf.WriteString(escapeGraphvizHTMLLike(strings.TrimLeft(op.opCode.String(), "op")))
	buf.WriteString(`  </b></td></tr>`)

	// Operand rows, describing each argument in turn
	for argIdx, result := range op.operands {
		desc := g.resultGraphvizRepr(result)
		fmt.Fprintf(&buf, `<tr><td align="left" port="a%d">%s</td></tr>`, argIdx, desc)
	}

	// Result row, giving the identifier for this operation's result
	buf.WriteString(`<tr><td align="right" bgcolor="#eeeeee" port="r">`)
	// If this operation determines the final value of a resource instance
	// then we'll indicate that inline here.
	for _, elem := range g.resourceInstanceResults.Elems {
		if resultRef, ok := elem.Value.(anyOperationResultRef); ok && resultRef.operationResultIndex() == idx {
			fmt.Fprintf(&buf, `%s as `, escapeGraphvizHTMLLike(elem.Key.String()))
			// In principle it's possible that this operation could define
			// more than one resource instance, but we don't generate graphs
			// shaped like that in practice so we won't worry about handling it.
			break
		}
	}
	// TODO: check if this operation determines the final value of any resource
	// instances, and indicate that here if so.
	fmt.Fprintf(&buf, `r[%d]`, idx)
	buf.WriteString(`</td></tr>`)

	buf.WriteString(`</table>`)
	return buf.String()
}

// resultGraphvizRepr returns a representation of the given result using
// Graphviz's "HTML-like" node label syntax, intended for direct inclusion into
// one of the cells produced by [Graph.operationGraphvizRepr].
//
// The result has appropriate escaping to be included directly inside the
// "<td>" and "</td>" tags used for one table cell in the overall operation
// representation, without needing any further transformation.
func (g *Graph) resultGraphvizRepr(result AnyResultRef) string {
	switch result := result.(type) {
	case valueResultRef:
		// For a value we'll just inline a JSON representation of it
		// directly inside the cell, because these values tend to be too
		// bulky to appear as separate graph nodes without making the graph
		// layout very messy.
		val := g.constantVals[result.index]
		vJSON, err := ctyjson.Marshal(cty.UnknownAsNull(val), val.Type())
		if err != nil { // Should not get here because we control the input
			return "(failed to serialize value)"
		}
		var buf bytes.Buffer
		err = json.Indent(&buf, vJSON, "", "  ")
		if err != nil { // Should not get here because we control the input
			return "(failed to pretty-print value)"
		}
		return fmt.Sprintf(`<font face="Courier">%s<br align="left" /></font>`, escapeGraphvizHTMLLike(buf.String()))
	case providerAddrResultRef:
		providerAddr := g.providerAddrs[result.index]
		return escapeGraphvizHTMLLike(providerAddr.String())
	case desiredResourceInstanceResultRef:
		instAddr := g.desiredStateRefs[result.index]
		return fmt.Sprintf("%s<br align=\"left\" />desired state<br align=\"left\" />", escapeGraphvizHTMLLike(instAddr.String()))
	case resourceInstancePriorStateResultRef:
		ref := g.priorStateRefs[result.index]
		if ref.DeposedKey != states.NotDeposed {
			return fmt.Sprintf(
				"%s<br align=\"left\" />deposed %s<br align=\"left\" />prior state<br align=\"left\" />",
				escapeGraphvizHTMLLike(ref.ResourceInstance.String()),
				escapeGraphvizHTMLLike(ref.DeposedKey.String()),
			)
		}
		return fmt.Sprintf("%s<br align=\"left\" />prior state<br align=\"left\" />", escapeGraphvizHTMLLike(ref.ResourceInstance.String()))
	case providerInstanceConfigResultRef:
		pInstAddr := g.providerInstConfigRefs[result.index]
		return escapeGraphvizHTMLLike(pInstAddr.String())
	case anyOperationResultRef:
		return fmt.Sprintf("r[%d]", result.operationResultIndex())
	case waiterResultRef:
		waiter := g.waiters[result.index]
		if len(waiter) == 1 {
			return "<i>(1 dependency)</i>"
		} else if n := len(waiter); n != 0 {
			return fmt.Sprintf("<i>(%d dependencies)</i>", n)
		} else {
			return "<i>(no dependencies)</i>"
		}
	case nil:
		return "nil"
	default:
		// Should try to keep the above cases comprehensive because
		// this default is not very readable and might even be
		// useless if it's a reference into a table we're not otherwise
		// including the output here.
		return fmt.Sprintf("%#v", result)
	}
}

func escapeGraphvizHTMLLike(input string) string {
	return strings.ReplaceAll(html.EscapeString(input), "\n", "<br align=\"left\" />")
}

func (g *Graph) indirectlyNeededOperationResults(ref AnyResultRef) iter.Seq[anyOperationResultRef] {
	var walk func(AnyResultRef, func(anyOperationResultRef) bool)
	walk = func(ref AnyResultRef, yield func(anyOperationResultRef) bool) {
		switch ref := ref.(type) {
		case anyOperationResultRef:
			yield(ref)
		case waiterResultRef:
			for _, dep := range g.waiters[ref.index] {
				walk(dep, yield)
			}
		default:
			// No operations at all in any other case
		}
	}
	return func(yield func(anyOperationResultRef) bool) {
		walk(ref, yield)
	}
}
