// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package symlib

import (
	"fmt"
	"strings"
	"sync"

	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

func hclContext(w *workgraph.Worker, s scope, expr hcl.Expression) (*hcl.EvalContext, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	hclCtx := &hcl.EvalContext{}

	variables := map[string]map[string]cty.Value{}
	for _, trav := range expr.Variables() {
		if len(trav) < 2 {
			continue
		}
		root, ok := trav[0].(hcl.TraverseRoot)
		if !ok {
			continue
		}
		attr, ok := trav[1].(hcl.TraverseAttr)
		if !ok {
			continue
		}

		value, vDiags := s.valueFor(w, root.Name, attr.Name, trav.SourceRange())
		diags = diags.Extend(vDiags)
		if variables[root.Name] == nil {
			variables[root.Name] = map[string]cty.Value{}
		}
		variables[root.Name][attr.Name] = value
	}

	hclCtx.Variables = map[string]cty.Value{}
	for name, entries := range variables {
		hclCtx.Variables[name] = cty.ObjectVal(entries)
	}

	hclCtx.Functions = map[string]function.Function{}
	if fexpr, ok := expr.(hcl.ExpressionWithFunctions); ok {
		for _, fn := range fexpr.Functions() {
			found, fDiags := s.functionFor(w, fn)
			if found != nil {
				hclCtx.Functions[fn[0].(hcl.TraverseRoot).Name] = *found
			}
			diags = diags.Extend(fDiags)
		}
	}

	return hclCtx, diags
}

func typeContext(w *workgraph.Worker, s scope) typeexpr.TypeContext {
	return typeexpr.TypeContext{TypeFunc: func(call *hcl.StaticCall) (*cty.Type, *typeexpr.Defaults, hcl.Diagnostics) {
		ty, def, diags := s.typeForStaticCall(w, call)
		if ty == cty.NilType {
			return nil, nil, diags
		}
		return &ty, def, diags
	}}
}

type scope interface {
	valueFor(*workgraph.Worker, string, string, hcl.Range) (cty.Value, hcl.Diagnostics)
	functionFor(*workgraph.Worker, hcl.Traversal) (*function.Function, hcl.Diagnostics)
	typeForStaticCall(*workgraph.Worker, *hcl.StaticCall) (cty.Type, *typeexpr.Defaults, hcl.Diagnostics)
}

type symbolScope struct {
	table Table

	builtinFuncs map[string]function.Function

	requestMu sync.Mutex
	requests  map[workgraph.RequestID]ident

	values    map[string]valuer[cty.Value]
	functions map[string]valuer[function.Function]
	types     map[string]valuer[typeWithDefault]
}

func newSymbolScope(table Table, builtinFuncs map[string]function.Function) *symbolScope {
	return &symbolScope{
		table:        table,
		builtinFuncs: builtinFuncs,

		requests: map[workgraph.RequestID]ident{},

		values:    map[string]valuer[cty.Value]{},
		functions: map[string]valuer[function.Function]{},
		types:     map[string]valuer[typeWithDefault]{},
	}
}

func (s *symbolScope) valueFor(w *workgraph.Worker, rootName string, attrName string, rng hcl.Range) (cty.Value, hcl.Diagnostics) {
	if rootName == TypeSymbols {
		return s.table.Value(ValueRef{
			Namespace: attrName,
			Range:     rng,
		})
	}
	if rootName == "value" {
		found, ok := s.values[attrName]
		if !ok {
			return cty.NilVal, hcl.Diagnostics{{
				Summary: "Missing value",
				Detail:  fmt.Sprintf("Value %s not defined in symbol library", attrName),
				Subject: rng.Ptr(),
			}}
		}
		return found.Value(w)
	}
	return cty.NilVal, nil
}

func (s *symbolScope) functionFor(w *workgraph.Worker, fn hcl.Traversal) (*function.Function, hcl.Diagnostics) {
	root := fn[0].(hcl.TraverseRoot)
	sp := strings.Split(root.Name, "::")
	if len(sp) == 1 {
		// Builtin funcs
		builtin, ok := s.builtinFuncs[root.Name]
		if ok {
			return &builtin, nil
		}
		return nil, nil
	}
	// TODO core NS funcs?
	if sp[0] != TypeSymbols {
		return nil, nil
	}
	switch len(sp) {
	case 2:
		name := sp[1]
		found, ok := s.functions[name]
		if !ok {
			return nil, hcl.Diagnostics{{
				Summary: "Missing function",
				Detail:  fmt.Sprintf("Function %s not defined in symbol library", name),
				Subject: fn.SourceRange().Ptr(),
			}}
		}
		fn, diags := found.Value(w)
		return &fn, diags
	case 3:
		return s.table.Function(FunctionRef{
			Namespace: sp[1],
			Name:      sp[2],
		})
	default:
		return nil, hcl.Diagnostics{{
			Summary: "Invalid function call structure",
			Detail:  "Expected symbol function call of the form symbols::namespace::name()",
			Subject: fn.SourceRange().Ptr(),
		}}
	}
}

func (s *symbolScope) typeForStaticCall(w *workgraph.Worker, call *hcl.StaticCall) (cty.Type, *typeexpr.Defaults, hcl.Diagnostics) {
	split := strings.Split(call.Name, "::")
	if split[0] != TypeSymbols {
		return cty.NilType, nil, nil
	}

	switch len(split) {
	case 2:
		found, ok := s.types[split[1]]
		if !ok {
			return cty.NilType, nil, hcl.Diagnostics{{
				Summary: "Missing type",
				Detail:  fmt.Sprintf("Type %s not defined in symbol library", split[1]),
				Subject: call.NameRange.Ptr(),
			}}
		}
		val, diags := found.Value(w)
		return val.ty, val.def, diags
	case 3:
		return s.table.Type(TypeRef{
			Namespace: split[1],
			Name:      split[2],
			Range:     call.NameRange,
		})
	default:
		return cty.NilType, nil, nil
	}
}

type functionScope struct {
	*symbolScope

	name   string
	params map[string]cty.Value
	locals map[string]valuer[cty.Value]
}

func newFunctionScope(s *symbolScope, name string, params map[string]cty.Value) *functionScope {
	return &functionScope{
		symbolScope: s,
		name:        name,
		params:      params,
		// Locals intentionally uninitialized as those are added part way though function setup
	}
}

func (f *functionScope) valueFor(w *workgraph.Worker, rootName string, attrName string, rng hcl.Range) (cty.Value, hcl.Diagnostics) {
	if rootName == "param" {
		val, ok := f.params[attrName]
		if !ok {
			return cty.NilVal, hcl.Diagnostics{{
				Severity: hcl.DiagError,
				Summary:  "Unknown reference",
				Detail:   fmt.Sprintf("Param %q does not exist within function %s", attrName, f.name),
				Subject:  rng.Ptr(),
			}}
		}
		return val, nil
	}
	if rootName == "local" {
		if f.locals == nil {
			return cty.NilVal, hcl.Diagnostics{{
				Severity: hcl.DiagError,
				Summary:  "Invalid reference",
				Detail:   "Locals not allowed here",
				Subject:  rng.Ptr(),
			}}
		}
		local, ok := f.locals[attrName]
		if !ok {
			return cty.NilVal, hcl.Diagnostics{{
				Severity: hcl.DiagError,
				Summary:  "Unknown reference",
				Detail:   fmt.Sprintf("Local %q does not exist within function %s", attrName, f.name),
				Subject:  rng.Ptr(),
			}}
		}
		return local.Value(w)
	}
	return f.symbolScope.valueFor(w, rootName, attrName, rng)
}
