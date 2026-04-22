package symlib

import (
	"fmt"
	"maps"
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
	libraries    map[string]*scope
	// TODO parent scope for functions
}

type typeWithDefault struct {
	ty  cty.Type
	def *typeexpr.Defaults
}

func newScope(builtinFuncs map[string]function.Function) *scope {
	return &scope{
		vars:      map[string]map[string]result[cty.Value]{"const": {}},
		types:     map[string]result[typeWithDefault]{},
		funcs:     map[string]result[function.Function]{},
		libraries: map[string]*scope{},

		builtinFuncs: builtinFuncs,
	}
}

func once[V any](fn result[V]) result[V] {
	var mu sync.Mutex
	type T withDiags[V]
	var promise workgraph.Promise[T]
	var resolver workgraph.Resolver[T]
	needsSetup := true

	return func(w *workgraph.Worker) (V, hcl.Diagnostics) {
		mu.Lock()
		if needsSetup {
			resolver, promise = workgraph.NewRequest[T](w)
		}
		neededSetup := needsSetup
		needsSetup = false
		mu.Unlock()

		if neededSetup {
			val, diags := fn(w)
			resolver.Report(w, T{val, diags}, nil)
		}
		val, err := promise.Await(w)
		if err != nil {
			val.diags = val.diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Workgraph error",
				Detail:   err.Error(),
				Extra:    err,
			})
		}
		return val.value, val.diags
	}
}

func (s *scope) clone() *scope {
	ns := &scope{
		vars:      map[string]map[string]result[cty.Value]{},
		types:     s.types,
		funcs:     s.funcs,
		libraries: s.libraries,

		builtinFuncs: s.builtinFuncs,
	}

	for rk, rv := range s.vars {
		ns.vars[rk] = map[string]result[cty.Value]{}
		maps.Copy(ns.vars[rk], rv)
	}

	return ns
}

func (s *scope) typeContext(w *workgraph.Worker, typeExpr hcl.Expression) (typeexpr.TypeContext, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	prefix := "library::"
	suffix := "types"
	selfNs := prefix + suffix

	typeCtx := typeexpr.TypeContext{
		Types:    map[string]map[string]cty.Type{selfNs: {}},
		Defaults: map[string]map[string]*typeexpr.Defaults{selfNs: {}},
	}
	// Add all libraries, we could do less work based on missing below
	// Minor opt at this point, not worth it
	for lname, lib := range s.libraries {
		lNs := prefix + lname + "::" + suffix
		typeCtx.Types[lNs] = map[string]cty.Type{}
		typeCtx.Defaults[lNs] = map[string]*typeexpr.Defaults{}
		for tname, fn := range lib.types {
			val, vDiags := fn(w)
			diags = diags.Extend(vDiags)
			typeCtx.Types[lNs][tname] = val.ty
			typeCtx.Defaults[lNs][tname] = val.def
		}
	}

	if typeExpr != nil {
		missing, mDiags := typeCtx.TypeDependencies(typeExpr)
		diags = diags.Extend(mDiags)
		for _, ty := range missing[selfNs] {
			if fn, ok := s.types[ty]; ok {
				val, vDiags := fn(w)
				diags = diags.Extend(vDiags)
				typeCtx.Types[selfNs][ty] = val.ty
				typeCtx.Defaults[selfNs][ty] = val.def
			}
		}
	} else {
		for tn, fn := range s.types {
			val, vDiags := fn(w)
			diags = diags.Extend(vDiags)
			typeCtx.Types[selfNs][tn] = val.ty
			typeCtx.Defaults[selfNs][tn] = val.def
		}
	}

	return typeCtx, diags
}

func (s *scope) evalContext(w *workgraph.Worker, expr hcl.Expression) (*hcl.EvalContext, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	vars := map[string]map[string]cty.Value{"library": {}}

	for lname, lib := range s.libraries {
		// TODO opt with expr deps
		consts := map[string]cty.Value{}
		for cn, fn := range lib.vars["const"] {
			val, vDiags := fn(w)
			diags = diags.Extend(vDiags)
			consts[cn] = val
		}
		// TODO consts in name?
		vars["library"][lname] = cty.ObjectVal(consts)
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

			if root.Name == "library" {
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

	if expr != nil {
		for _, trav := range expr.(hcl.ExpressionWithFunctions).Functions() {
			funcIdent := trav.RootName()
			parts := strings.Split(funcIdent, "::")

			if parts[0] == "library" {
				// library::func
				if len(parts) == 2 {
					funcName := parts[1]
					fn, ok := s.funcs[funcName]
					if ok {
						impl, fDiags := fn(w)
						diags = diags.Extend(fDiags)
						evalCtx.Functions[funcIdent] = impl
					}
				}
				// library::lib::func
				if len(parts) == 3 {
					libName := parts[1]
					funcName := parts[2]

					if lib, ok := s.libraries[libName]; ok {
						if fn, ok := lib.funcs[funcName]; ok {
							impl, fDiags := fn(w)
							diags = diags.Extend(fDiags)
							evalCtx.Functions[funcIdent] = impl
						}
					}
				}
			}
		}
	} else {
		for funcName, fn := range s.funcs {
			funcIdent := "library::" + funcName
			impl, fDiags := fn(w)
			diags = diags.Extend(fDiags)
			evalCtx.Functions[funcIdent] = impl
		}

		for libName, lib := range s.libraries {
			for funcName, fn := range lib.funcs {
				funcIdent := "library::" + libName + "::" + funcName
				impl, fDiags := fn(w)
				diags = diags.Extend(fDiags)
				evalCtx.Functions[funcIdent] = impl
			}
		}
	}

	return evalCtx, diags
}

func (s *scope) addType(name string, typeExpr hcl.Expression) {
	s.types[name] = once(func(w *workgraph.Worker) (typeWithDefault, hcl.Diagnostics) {
		typeCtx, diags := s.typeContext(w, typeExpr)

		varType, typeDefault, vDiags := typeCtx.TypeConstraintWithDefaults(typeExpr)
		diags = diags.Extend(vDiags)

		return typeWithDefault{varType, typeDefault}, diags
	})
}

func (s *scope) addVar(namespace string, name string, expr hcl.Expression) {
	ns, ok := s.vars[namespace]
	if !ok {
		ns = map[string]result[cty.Value]{}
		s.vars[namespace] = ns
	}

	ns[name] = once(func(w *workgraph.Worker) (cty.Value, hcl.Diagnostics) {
		evalCtx, diags := s.evalContext(w, expr)

		val, vDiags := expr.Value(evalCtx)
		diags = diags.Extend(vDiags)

		return val, diags
	})
}

func (s *scope) addFunction(name string, fn func(*workgraph.Worker, *scope) (function.Function, hcl.Diagnostics)) {
	s.funcs[name] = once(func(w *workgraph.Worker) (function.Function, hcl.Diagnostics) {
		return fn(w, s)
	})
}

type result[T any] func(w *workgraph.Worker) (T, hcl.Diagnostics)
type withDiags[T any] struct {
	value T
	diags hcl.Diagnostics
}
