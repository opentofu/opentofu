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

type scope struct {
	vars  map[string]map[string]result[cty.Value]
	types map[string]result[typeWithDefault]
	funcs map[string]result[function.Function]

	builtinFuncs map[string]function.Function
	symbols      map[string]*scope

	requests map[workgraph.RequestID]ident
}

type typeWithDefault struct {
	ty  cty.Type
	def *typeexpr.Defaults
}

func newScope(builtinFuncs map[string]function.Function) *scope {
	return &scope{
		vars:    map[string]map[string]result[cty.Value]{"value": {}},
		types:   map[string]result[typeWithDefault]{},
		funcs:   map[string]result[function.Function]{},
		symbols: map[string]*scope{},

		builtinFuncs: builtinFuncs,

		requests: map[workgraph.RequestID]ident{},
	}
}

type ident struct {
	name string
	src  *hcl.Range
}

func once[V any](id ident, s *scope, fn result[V]) result[V] {
	var mu sync.Mutex
	type T withDiags[V]
	var promise workgraph.Promise[T]
	var resolver workgraph.Resolver[T]
	needsSetup := true

	return func(w *workgraph.Worker) (V, hcl.Diagnostics) {
		mu.Lock()
		if needsSetup {
			resolver, promise = workgraph.NewRequest[T](w)
			s.requests[resolver.RequestID()] = id

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
					if info, ok := s.requests[reqID]; ok {
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
}

func (s *scope) clone() *scope {
	ns := &scope{
		vars:    map[string]map[string]result[cty.Value]{},
		types:   s.types,
		funcs:   s.funcs,
		symbols: s.symbols,

		builtinFuncs: s.builtinFuncs,

		requests: map[workgraph.RequestID]ident{},
	}

	for rk, rv := range s.vars {
		ns.vars[rk] = map[string]result[cty.Value]{}
		maps.Copy(ns.vars[rk], rv)
	}

	maps.Copy(ns.requests, s.requests)

	return ns
}

func (s *scope) typeContext(w *workgraph.Worker) typeexpr.TypeContext {
	sep := "::"
	suffix := "types"
	selfNs := TypeSymbols + sep + suffix

	return typeexpr.TypeContext{
		TypeFunc: func(call *hcl.StaticCall) (*cty.Type, *typeexpr.Defaults, hcl.Diagnostics) {
			kw := hcl.ExprAsKeyword(call.Arguments[0])
			ns := call.Name

			var types map[string]result[typeWithDefault]

			if ns == selfNs {
				types = s.types
			} else {
				for lname, lib := range s.symbols {
					lNs := TypeSymbols + sep + lname + sep + suffix
					if ns == lNs {
						types = lib.types
						break
					}
				}
			}

			fn, ok := types[kw]
			if !ok {
				return nil, nil, nil
			}

			val, diags := fn(w)
			if diags.HasErrors() {
				return nil, nil, diags
			}
			return &(val.ty), val.def, diags
		},
	}
}

func (s *scope) evalContext(w *workgraph.Worker, expr hcl.Expression) (*hcl.EvalContext, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	vars := map[string]map[string]cty.Value{
		TypeSymbols: {},
	}

	for lname, lib := range s.symbols {
		// TODO opt with expr deps
		values := map[string]cty.Value{}
		for cn, fn := range lib.vars["value"] {
			val, vDiags := fn(w)
			diags = diags.Extend(vDiags)
			values[cn] = val
		}
		// TODO values in name?
		vars[TypeSymbols][lname] = cty.ObjectVal(values)
	}

	if expr != nil {
		for _, trav := range expr.Variables() {
			if len(trav) < 2 {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "TODO unformed1",
					Detail:   fmt.Sprintf("%#v", trav),
					Subject:  trav.SourceRange().Ptr(),
				})
				continue
			}

			root := trav[0].(hcl.TraverseRoot)

			if root.Name == TypeSymbols {
				continue
			}

			attr, ok := trav[1].(hcl.TraverseAttr)
			if !ok {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "TODO unformed2",
					Detail:   fmt.Sprintf("%#v", trav),
					Subject:  trav.SourceRange().Ptr(),
				})
				continue
			}

			vRes := s.vars[root.Name][attr.Name]
			if vRes == nil {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "TODO unformed3",
					Detail:   fmt.Sprintf("%s.%s", root.Name, attr.Name),
					Subject:  trav.SourceRange().Ptr(),
				})
				continue
			}

			val, vDiags := vRes(w)
			diags = diags.Extend(vDiags)

			rootVars, ok := vars[root.Name]
			if !ok {
				rootVars = map[string]cty.Value{}
				vars[root.Name] = rootVars
			}

			rootVars[attr.Name] = val
		}
	} else {
		for rootName, entries := range s.vars {
			vars[rootName] = map[string]cty.Value{}
			for varName, entry := range entries {
				val, vDiags := entry(w)
				diags = diags.Extend(vDiags)
				vars[rootName][varName] = val
			}
		}
	}

	evalCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{},
		Functions: map[string]function.Function{},
	}
	maps.Copy(evalCtx.Functions, s.builtinFuncs)

	for name, vars := range vars {
		evalCtx.Variables[name] = cty.ObjectVal(vars)
	}

	// Recursion is not allowed (hcl limitation)
	for funcName, fn := range s.funcs {
		funcIdent := TypeSymbols + "::" + funcName
		impl, fDiags := fn(w)
		diags = diags.Extend(fDiags)
		evalCtx.Functions[funcIdent] = impl
	}

	for libName, lib := range s.symbols {
		for funcName, fn := range lib.funcs {
			funcIdent := TypeSymbols + "::" + libName + "::" + funcName
			impl, fDiags := fn(w)
			diags = diags.Extend(fDiags)
			evalCtx.Functions[funcIdent] = impl
		}
	}

	return evalCtx, diags
}

func (s *scope) addType(name string, typeExpr hcl.Expression) {
	id := ident{TypeSymbols + "::" + "types(" + name + ")", typeExpr.Range().Ptr()}
	s.types[name] = once(id, s, func(w *workgraph.Worker) (typeWithDefault, hcl.Diagnostics) {
		typeCtx := s.typeContext(w)
		varType, typeDefault, diags := typeCtx.TypeConstraintWithDefaults(typeExpr)
		return typeWithDefault{varType, typeDefault}, diags
	})
}

func (s *scope) addVar(namespace string, name string, expr hcl.Expression) {
	ns, ok := s.vars[namespace]
	if !ok {
		ns = map[string]result[cty.Value]{}
		s.vars[namespace] = ns
	}

	id := ident{namespace + "." + name, expr.Range().Ptr()}
	ns[name] = once(id, s, func(w *workgraph.Worker) (cty.Value, hcl.Diagnostics) {
		evalCtx, diags := s.evalContext(w, expr)

		val, vDiags := expr.Value(evalCtx)
		diags = diags.Extend(vDiags)

		return val, diags
	})
}

func (s *scope) addFunction(name string, fn func(*workgraph.Worker, *scope) (function.Function, hcl.Diagnostics)) {
	id := ident{TypeSymbols + "::" + name + "()", nil}
	s.funcs[name] = once(id, s, func(w *workgraph.Worker) (function.Function, hcl.Diagnostics) {
		return fn(w, s)
	})
}

type result[T any] func(w *workgraph.Worker) (T, hcl.Diagnostics)
type withDiags[T any] struct {
	value T
	diags hcl.Diagnostics
}
