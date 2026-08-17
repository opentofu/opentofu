// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package symlib

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

type ident struct {
	name string
	src  *hcl.Range
}

type compiler struct {
	requests map[workgraph.RequestID]ident
}

type valuer[T any] struct {
	Value func(*workgraph.Worker) (T, hcl.Diagnostics)
	Ident ident
}

func onceValuer[V any](comp compiler, id ident, fn func(*workgraph.Worker) (V, hcl.Diagnostics)) valuer[V] {
	var mu sync.Mutex
	type T struct {
		value V
		diags hcl.Diagnostics
	}

	var promise workgraph.Promise[T]
	var resolver workgraph.Resolver[T]
	needsSetup := true

	onceFn := func(w *workgraph.Worker) (V, hcl.Diagnostics) {
		mu.Lock()
		if needsSetup {
			resolver, promise = workgraph.NewRequest[T](w)
			comp.requests[resolver.RequestID()] = id

			workgraph.WithNewAsyncWorker(func(w *workgraph.Worker) {
				val, diags := fn(w)
				resolver.Report(w, T{val, diags}, nil)
			}, resolver)
		}
		needsSetup = false
		mu.Unlock()

		val, err := promise.Await(w)
		if err != nil {
			if selfDep, ok := err.(workgraph.ErrSelfDependency); ok {
				// Copied from grapheval/diagnostics.go
				reqDescs := make([]string, 0)
				for _, reqID := range selfDep.RequestIDs {
					desc := "<unknown object> (failing to report this is a bug in OpenTofu)"
					if info, ok := comp.requests[reqID]; ok {
						if info.src != nil {
							desc = fmt.Sprintf("%s (%s)", info.name, info.src)
						} else {
							desc = info.name
						}
					}
					reqDescs = append(reqDescs, desc)
				}
				slices.Sort(reqDescs)

				var detailBuf strings.Builder
				detailBuf.WriteString("The following objects in the configuration form a dependency cycle, so there is no valid order to evaluate them in:\n")
				for _, desc := range reqDescs {
					fmt.Fprintf(&detailBuf, "  - %s\n", desc)
				}

				val.diags = val.diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Self-referential expressions",
					Detail:   strings.TrimSpace(detailBuf.String()),
					Subject:  id.src,
				})

			} else {
				val.diags = val.diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Workgraph error",
					Detail:   err.Error(),
					Subject:  id.src,
				})
			}
		}
		return val.value, val.diags
	}

	return valuer[V]{
		Value: onceFn,
		Ident: id,
	}
}

type hclValLookup func(*workgraph.Worker, string, string, hcl.Range) (cty.Value, hcl.Diagnostics)

func CompileLibrary(files []*SymbolFile, loader Loader, builtinFuncs map[string]function.Function) (*Library, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	// Load nested symbol calls as a table
	symbolCalls := map[string]*SymbolCall{}
	for _, file := range files {
		for _, o := range file.SymbolCalls {
			if existing, exists := symbolCalls[o.Name]; exists {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate symbols definition",
					Detail:   fmt.Sprintf("An symbols named %q was already defined at %s. SymbolCall names must be unique within a module.", o.Name, existing.DeclRange),
					Subject:  &o.DeclRange,
				})
			}
			symbolCalls[o.Name] = o
		}
	}

	table, tDiags := BuildTable(slices.Collect(maps.Values(symbolCalls)), loader)
	diags = diags.Extend(tDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	values := map[string]valuer[cty.Value]{}
	functions := map[string]valuer[function.Function]{}
	types := map[string]valuer[typeWithDefault]{}
	comp := compiler{requests: map[workgraph.RequestID]ident{}}

	exprContext := func(w *workgraph.Worker, expr hcl.Expression, additional hclValLookup) (*hcl.EvalContext, hcl.Diagnostics) {
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
			var value cty.Value
			var vDiags hcl.Diagnostics
			if root.Name == TypeSymbols {
				if _, ok = variables[TypeSymbols][attr.Name]; ok {
					// Already Added
					continue
				}
				value, vDiags = table.Value(ValueRef{
					Namespace: attr.Name,
					Range:     trav.SourceRange(),
				})
			} else if root.Name == "value" {
				found, ok := values[attr.Name]
				if !ok {
					diags = diags.Append(&hcl.Diagnostic{
						Summary: "Missing value",
						Detail:  fmt.Sprintf("Value %s not defined in symbol library", attr.Name),
						Subject: trav.SourceRange().Ptr(),
					})
					continue
				}
				value, vDiags = found.Value(w)
			} else if additional != nil {
				// TODO integrate these into workgraph to detect loops
				value, vDiags = additional(w, root.Name, attr.Name, trav.SourceRange())
			} else {
				continue
			}
			diags = diags.Extend(vDiags)
			if value != cty.NilVal {
				if variables[root.Name] == nil {
					variables[root.Name] = map[string]cty.Value{}
				}
				variables[root.Name][attr.Name] = value
			}
		}

		hclCtx.Variables = map[string]cty.Value{}
		for name, entries := range variables {
			hclCtx.Variables[name] = cty.ObjectVal(entries)
		}

		hclCtx.Functions = map[string]function.Function{}
		if fexpr, ok := expr.(hcl.ExpressionWithFunctions); ok {
			for _, fn := range fexpr.Functions() {
				root := fn[0].(hcl.TraverseRoot)
				sp := strings.Split(root.Name, "::")
				if sp[0] != TypeSymbols {
					continue
				}
				switch len(sp) {
				case 2:
					name := sp[1]
					found, ok := functions[name]
					if !ok {
						diags = diags.Append(&hcl.Diagnostic{
							Summary: "Missing function",
							Detail:  fmt.Sprintf("Function %s not defined in symbol library", name),
							Subject: fn.SourceRange().Ptr(),
						})
						continue
					}
					val, vDiags := found.Value(w)
					diags = diags.Extend(vDiags)
					hclCtx.Functions[root.Name] = val
				case 3:
					val, vDiags := table.Function(FunctionRef{
						Namespace: sp[1],
						Name:      sp[2],
					})
					diags = diags.Extend(vDiags)
					hclCtx.Functions[root.Name] = val
				default:
					diags = diags.Append(&hcl.Diagnostic{
						Summary: "Invalid function call structure",
						Detail:  "Expected symbol function call of the form symbols::namespace::name()",
						Subject: fn.SourceRange().Ptr(),
					})
				}
			}
		}

		maps.Copy(hclCtx.Functions, builtinFuncs)

		return hclCtx, diags
	}

	typeContext := func(w *workgraph.Worker) typeexpr.TypeContext {
		return typeexpr.TypeContext{TypeFunc: func(call *hcl.StaticCall) (*cty.Type, *typeexpr.Defaults, hcl.Diagnostics) {
			split := strings.Split(call.Name, "::")
			if split[0] != TypeSymbols {
				return nil, nil, nil
			}

			switch len(split) {
			case 2:
				found, ok := types[split[1]]
				if !ok {
					return &cty.NilType, nil, hcl.Diagnostics{{
						Summary: "Missing type",
						Detail:  fmt.Sprintf("Type %s not defined in symbol library", split[1]),
						Subject: call.NameRange.Ptr(),
					}}
				}
				val, diags := found.Value(w)
				if diags.HasErrors() {
					return &cty.DynamicPseudoType, nil, diags
				}
				return &(val.ty), val.def, diags
			case 3:
				ty, def, diags := table.Type(TypeRef{
					Namespace: split[1],
					Name:      split[2],
					Range:     call.NameRange,
				})
				return &ty, def, diags
			default:
				return nil, nil, nil
			}
		}}
	}

	for _, file := range files {
		for _, o := range file.Consts {
			if existing, exists := values[o.Name]; exists {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate values definition",
					Detail:   fmt.Sprintf("An values named %q was already defined at %s. Const names must be unique within a module.", o.Name, existing.Ident.src),
					Subject:  &o.DeclRange,
				})
			}
			id := ident{name: o.Name, src: &o.DeclRange}
			values[o.Name] = onceValuer[cty.Value](comp, id, func(w *workgraph.Worker) (cty.Value, hcl.Diagnostics) {
				hclCtx, diags := exprContext(w, o.Expr, nil)
				if diags.HasErrors() {
					return cty.NilVal, diags
				}
				val, vDiags := o.Expr.Value(hclCtx)
				diags = diags.Extend(vDiags)
				return val, diags
			})
		}
		for _, o := range file.TypeDefs {
			if existing, exists := types[o.Name]; exists {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate typedef definition",
					Detail:   fmt.Sprintf("An typedef named %q was already defined at %s. TypeDef names must be unique within a module.", o.Name, existing.Ident.src),
					Subject:  &o.DeclRange,
				})
			}
			id := ident{name: o.Name, src: &o.DeclRange}
			types[o.Name] = onceValuer(comp, id, func(w *workgraph.Worker) (typeWithDefault, hcl.Diagnostics) {
				typeCtx := typeContext(w)
				varType, typeDefault, diags := typeCtx.TypeConstraintWithDefaults(o.TypeExpr)
				return typeWithDefault{varType, typeDefault}, diags
			})
		}
		for _, o := range file.Functions {
			if existing, exists := functions[o.Name]; exists {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate function definition",
					Detail:   fmt.Sprintf("An function named %q was already defined at %s. Function names must be unique within a module.", o.Name, existing.Ident.src),
					Subject:  &o.DeclRange,
				})
			}
			id := ident{name: o.Name, src: &o.DeclRange}
			functions[o.Name] = onceValuer(comp, id, func(w *workgraph.Worker) (function.Function, hcl.Diagnostics) {
				return o.Compile(w, typeContext, exprContext)
			})
		}
	}

	// Compile the library
	lib := &Library{
		types:     map[string]typeWithDefault{},
		values:    map[string]cty.Value{},
		functions: map[string]function.Function{},
	}

	w := workgraph.NewWorker()
	for name, once := range types {
		t, tDiags := once.Value(w)
		diags = diags.Extend(tDiags)
		lib.types[name] = t
	}
	for name, once := range values {
		v, vDiags := once.Value(w)
		diags = diags.Extend(vDiags)
		lib.values[name] = v
	}
	for name, once := range functions {
		f, fDiags := once.Value(w)
		diags = diags.Extend(fDiags)
		lib.functions[name] = f
	}

	return lib, diags
}
