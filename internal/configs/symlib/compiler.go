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
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// ident represents a human readable name and location for use in
// circular dependency messages and diagnostics
type ident struct {
	name string
	src  *hcl.Range
}

// valuer generically represents an item that can be compiled
type valuer[T any] struct {
	Value func(*workgraph.Worker) (T, hcl.Diagnostics)
	Ident ident
}

// onceValuer constructs a workgraph backed valuer from the given inputs
// It also ensures values are only evaluated once.
func onceValuer[V any](s scope, id ident, fn func(*workgraph.Worker) (V, hcl.Diagnostics)) valuer[V] {
	type T struct {
		value V
		diags hcl.Diagnostics
	}

	w := workgraph.NewWorker()
	resolver, promise := workgraph.NewRequest[T](w)
	s.setRequest(resolver.RequestID(), id)
	runFunc := func() {
		val, diags := fn(w)
		resolver.Report(w, T{val, diags}, nil)
	}

	var mu sync.Mutex
	onceFn := func(w *workgraph.Worker) (V, hcl.Diagnostics) {
		mu.Lock()
		if runFunc != nil {
			go runFunc()
			runFunc = nil
		}
		mu.Unlock()

		val, err := promise.Await(w)
		if err != nil {
			if selfDep, ok := err.(workgraph.ErrSelfDependency); ok {
				// Copied from grapheval/diagnostics.go
				reqDescs := make([]string, 0)
				for _, reqID := range selfDep.RequestIDs {
					desc := "<unknown object> (failing to report this is a bug in OpenTofu)"
					if info, ok := s.getRequest(reqID); ok {
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

	libScope := newSymbolScope(table, builtinFuncs)
	var language *Language

	for _, file := range files {
		for _, o := range file.Consts {
			if existing, exists := libScope.values[o.Name]; exists {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate values definition",
					Detail:   fmt.Sprintf("A value named %q was already defined at %s. Value names must be unique within a module.", o.Name, existing.Ident.src),
					Subject:  &o.DeclRange,
				})
			}
			id := ident{name: o.Name, src: &o.DeclRange}
			libScope.values[o.Name] = onceValuer[cty.Value](libScope, id, func(w *workgraph.Worker) (cty.Value, hcl.Diagnostics) {
				hclCtx, diags := hclContext(w, libScope, o.Expr, nil)
				if diags.HasErrors() {
					return cty.NilVal, diags
				}
				val, vDiags := o.Expr.Value(hclCtx)
				diags = diags.Extend(vDiags)
				return val, diags
			})
		}
		for _, o := range file.TypeDefs {
			if existing, exists := libScope.types[o.Name]; exists {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate typedef definition",
					Detail:   fmt.Sprintf("A typedef named %q was already defined at %s. Typedef names must be unique within a module.", o.Name, existing.Ident.src),
					Subject:  &o.DeclRange,
				})
			}
			id := ident{name: o.Name, src: &o.DeclRange}
			libScope.types[o.Name] = onceValuer(libScope, id, func(w *workgraph.Worker) (typeWithDefault, hcl.Diagnostics) {
				typeCtx := typeContext(w, libScope)
				varType, typeDefault, diags := typeCtx.TypeConstraintWithDefaults(o.TypeExpr)
				return typeWithDefault{varType, typeDefault}, diags
			})
		}
		for _, o := range file.Functions {
			if existing, exists := libScope.functions[o.Name]; exists {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate function definition",
					Detail:   fmt.Sprintf("A function named %q was already defined at %s. Function names must be unique within a module.", o.Name, existing.Ident.src),
					Subject:  &o.DeclRange,
				})
			}
			id := ident{name: o.Name, src: &o.DeclRange}
			libScope.functions[o.Name] = onceValuer(libScope, id, func(w *workgraph.Worker) (functionWithStack, hcl.Diagnostics) {
				return o.Compile(w, libScope)
			})
		}
		for _, o := range file.Languages {
			if language != nil {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate language block",
					Detail:   "Only a single language block is allowed per symbol library",
					Subject:  &o.DeclRange,
				})
			}
			language = o
		}
	}

	if language != nil {
		if language.Edition != "experimental2026" {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid language edition",
				Detail:   "The only valid symbol library language edition is \"experimental2026\"",
				Subject:  &language.DeclRange,
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
	for name, once := range libScope.types {
		t, tDiags := once.Value(w)
		diags = diags.Extend(tDiags)
		lib.types[name] = t
	}
	for name, once := range libScope.values {
		v, vDiags := once.Value(w)
		diags = diags.Extend(vDiags)
		lib.values[name] = v
	}
	for name, once := range libScope.functions {
		f, fDiags := once.Value(w)
		diags = diags.Extend(fDiags)
		lib.functions[name] = f.ForExternalCaller()
	}

	diags = DecompactFunctionErrors(diags)

	return lib, diags
}
