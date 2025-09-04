// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package exprs

import (
	"context"
	"iter"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Evaluate attempts to evaluate the given [Evaluable] in the given [Scope],
// either returning the resulting value or some error diagnostics describing
// problems that prevented successful evaluation.
//
// Some [Evaluable] implementations (or the symbols they refer to) can block
// on potentially-time-consuming operations, in which case they should respond
// gracefully to cancellation of the given context.
//
// It's valid to pass a nil Scope, representing that no symbols or functions
// are available at all. Note that HCL's JSON syntax treats that situation
// quite differently by taking JSON strings totally literally instead of
// trying to interpret them as HCL templates, and so switching to or from
// a nil scope is typically a breaking change for what's allowed in a
// particular position.
func Evaluate(ctx context.Context, what Evalable, scope Scope) (cty.Value, tfdiags.Diagnostics) {
	hclCtx, diags := buildHCLEvalContext(ctx, what, scope)
	if diags.HasErrors() {
		return cty.DynamicVal.Mark(EvalError), diags
	}
	val, moreDiags := what.Evaluate(ctx, hclCtx)
	diags = diags.Append(moreDiags)
	return EvalResult(val, diags)
}

func buildHCLEvalContext(ctx context.Context, what Evalable, scope Scope) (*hcl.EvalContext, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	ret := &hcl.EvalContext{}
	if scope == nil {
		// A nil scope represents that nothing at all is available, which HCL
		// represents as an EvalContext with nothing defined inside it.
		// Note that this causes significantly different behavior for HCL's
		// JSON syntax.
		return ret, diags
	}

	symbols, moreDiags := buildSymbolTable(ctx, what.References(), scope)
	ret.Variables = symbols
	diags = diags.Append(moreDiags)

	funcs, moreDiags := buildFunctionTable(ctx, what.FunctionCalls(), scope)
	ret.Functions = funcs
	diags = diags.Append(moreDiags)

	return ret, diags
}

func buildSymbolTable(ctx context.Context, refs iter.Seq[hcl.Traversal], scope Scope) (map[string]cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// We'll first build an intermediate representation of the nested symbol
	// tables, because a cty.ObjectVal result is immutable and so we need to
	// collect up all of the attribute values in a mutable data structure
	// and then freeze it into an immutable tree once complete.
	nodes := make(map[string]*symbolTableTempNode)
References:
	for traversal := range refs {
		currentChildren := nodes
		currentTable := SymbolTable(scope)
		for i, step := range traversal {
			// For our purposes here the distinction between TraverseAttr
			// and TraverseRoot is unimportant, so we'll normalize to
			// TraverseAttr.
			if rootStep, ok := step.(hcl.TraverseRoot); ok {
				step = hcl.TraverseAttr{
					Name:     rootStep.Name,
					SrcRange: rootStep.SrcRange,
				}
			}

			switch step := step.(type) {
			case hcl.TraverseAttr:
				attr, moreDiags := currentTable.ResolveAttr(step)
				diags = diags.Append(moreDiags)
				if moreDiags.HasErrors() {
					continue
				}

				switch attr := attr.(type) {
				case nestedSymbolTable:
					if _, ok := currentChildren[step.Name]; !ok {
						currentChildren[step.Name] = &symbolTableTempNode{
							children: make(map[string]*symbolTableTempNode),
						}
					}
					currentChildren = currentChildren[step.Name].children
					currentTable = attr.SymbolTable
					continue
				case valueOf:
					currentChildren[step.Name] = &symbolTableTempNode{
						val:    attr.Valuer,
						remain: traversal[i+1:],
					}
					continue References // any remaining steps are dynamic steps through the final value, captured in "remain" above
				}
			default:
				moreDiags := currentTable.HandleInvalidStep(tfdiags.SourceRangeFromHCL(step.SourceRange()))
				diags = diags.Append(moreDiags)
				continue References
			}
		}
		// If we get here then we ran out of steps before we encountered a
		// leaf Valuer, so this reference is incomplete and therefore invalid.
		moreDiags := currentTable.HandleInvalidStep(tfdiags.SourceRangeFromHCL(traversal.SourceRange()))
		diags = diags.Append(moreDiags)
	}

	ret, moreDiags := valuesForSymbolTableTempNodes(ctx, nodes)
	diags = diags.Append(moreDiags)
	return ret, diags
}

// symbolTableTempNode is an implementation detail of [buildSymbolTable] used
// as a mutable intermediate representation of the symbol table so that we
// can gradually assemble nested symbol tables and then turn them into
// immutable cty object values only at the end when the work is complete.
//
// A value of this type has EITHER val+remain or children alone populated.
type symbolTableTempNode struct {
	val    Valuer
	remain hcl.Traversal

	children map[string]*symbolTableTempNode
}

// valuesForSymbolTableTempNodes returns a map of [cty.Value] providing a
// frozen representation of the symbol tree rooted at the given map.
func valuesForSymbolTableTempNodes(ctx context.Context, symbols map[string]*symbolTableTempNode) (map[string]cty.Value, tfdiags.Diagnostics) {
	if len(symbols) == 0 {
		return nil, nil
	}

	var diags tfdiags.Diagnostics
	ret := make(map[string]cty.Value, len(symbols))
	for name, node := range symbols {
		if node.val != nil {
			// This is a leaf node, with a dynamic value associated with it.
			moreDiags := node.val.StaticCheckTraversal(node.remain)
			diags = diags.Append(moreDiags)
			if moreDiags.HasErrors() {
				ret[name] = AsEvalError(cty.DynamicVal)
				continue
			}

			// When we take the value of the object being referred to we
			// intentionally ignore any _indirect_ diagnostics that might
			// cause because we only want to report diagnostics that are
			// directly related to the expression we're currently
			// evaluating. Whatever problems might exist in the definition
			// of what we're referring to must be caught by visiting
			// that thing and evaluating it directly. This is safe to do
			// because the definition of Valuer requires that Value must
			// always return some reasonable placeholder value to use even
			// when an error occurs.
			val, _ := node.val.Value(ctx)
			ret[name] = val
			continue
		}

		// This is a nested symbol table, so we'll analyze its content recursively.
		childVals, moreDiags := valuesForSymbolTableTempNodes(ctx, node.children)
		diags = diags.Append(moreDiags)
		ret[name] = cty.ObjectVal(childVals)
	}

	return ret, diags
}

func buildFunctionTable(_ context.Context, calls iter.Seq[*hcl.StaticCall], scope Scope) (map[string]function.Function, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	ret := make(map[string]function.Function)
	for call := range calls {
		funcName := call.Name
		impl, moreDiags := scope.ResolveFunc(call)
		diags = diags.Append(moreDiags)
		if moreDiags.HasErrors() {
			continue
		}
		ret[funcName] = impl
	}
	return ret, diags
}
