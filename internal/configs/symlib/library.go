package symlib

import (
	"fmt"

	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

type Library struct {
	scope *scope

	Consts      map[string]cty.Value
	TypeContext typeexpr.TypeContext
	Functions   map[string]function.Function
}

type LibraryLoader func(*LibraryCall) (*Library, hcl.Diagnostics)
type SymbolsLoader func(*SymbolCall) (*Symbols, hcl.Diagnostics)

const TypeLibrary = "library"
const TypeSymbols = "symbols"

func NewLibrary(contents *LibraryContents, libLoader LibraryLoader, symLoader SymbolsLoader, builtinFuncs map[string]function.Function) (*Library, hcl.Diagnostics) {
	return newLibraryOrSymbols(TypeLibrary, contents, libLoader, symLoader, builtinFuncs)
}
func newLibraryOrSymbols(ltype string, contents *LibraryContents, libLoader LibraryLoader, symLoader SymbolsLoader, builtinFuncs map[string]function.Function) (*Library, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	l := &Library{
		scope: newScope(ltype, builtinFuncs),
	}

	// Load libraries
	for libName, call := range contents.LibraryCalls {
		lib, lDiags := libLoader(call)
		diags = diags.Extend(lDiags)

		if lib == nil {
			// Load failed
			continue
		}

		l.scope.libraries[libName] = lib.scope
	}
	for symName, call := range contents.SymbolCalls {
		lib, lDiags := symLoader(call)
		diags = diags.Extend(lDiags)

		if lib == nil {
			// Load failed
			continue
		}

		l.scope.libraries[symName] = lib.scope
	}

	// Build scope
	for _, typeDef := range contents.TypeDefs {
		l.scope.addType(typeDef.Name, typeDef.TypeExpr)
	}
	for _, fn := range contents.Functions {
		l.scope.addFunction(fn.Name, fn.Impl)
	}
	for _, c := range contents.Consts {
		l.scope.addVar("const", c.Name, c.Expr)
	}

	worker := workgraph.NewWorker()

	typeCtx, mDiags := l.scope.typeContext(worker, nil)
	diags = diags.Extend(mDiags)

	evalCtx, mDiags := l.scope.evalContext(worker, nil)
	diags = diags.Extend(mDiags)

	l.TypeContext = typeCtx
	l.Functions = evalCtx.Functions
	l.Consts = evalCtx.Variables

	return l, diags
}

type LibraryContents struct {
	Consts       map[string]*Const
	Functions    map[string]*Function
	TypeDefs     map[string]*TypeDef
	LibraryCalls map[string]*LibraryCall
	SymbolCalls  map[string]*SymbolCall
}

func NewLibraryContents(files []*SymbolFile) (*LibraryContents, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	l := &LibraryContents{
		Consts:       map[string]*Const{},
		Functions:    map[string]*Function{},
		TypeDefs:     map[string]*TypeDef{},
		LibraryCalls: map[string]*LibraryCall{},
		SymbolCalls:  map[string]*SymbolCall{},
	}

	for _, file := range files {
		for _, o := range file.Consts {
			if existing, exists := l.Consts[o.Name]; exists {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate const definition",
					Detail:   fmt.Sprintf("An const named %q was already defined at %s. Const names must be unique within a module.", existing.Name, existing.DeclRange),
					Subject:  &o.DeclRange,
				})
			}
			l.Consts[o.Name] = o
		}
		for _, o := range file.TypeDefs {
			if existing, exists := l.TypeDefs[o.Name]; exists {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate typedef definition",
					Detail:   fmt.Sprintf("An typedef named %q was already defined at %s. TypeDef names must be unique within a module.", existing.Name, existing.DeclRange),
					Subject:  &o.DeclRange,
				})
			}
			l.TypeDefs[o.Name] = o
		}
		for _, o := range file.Functions {
			if existing, exists := l.Functions[o.Name]; exists {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate function definition",
					Detail:   fmt.Sprintf("An function named %q was already defined at %s. Function names must be unique within a module.", existing.Name, existing.DeclRange),
					Subject:  &o.DeclRange,
				})
			}
			l.Functions[o.Name] = o
		}
		for _, o := range file.LibraryCalls {
			if existing, exists := l.LibraryCalls[o.Name]; exists {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate library definition",
					Detail:   fmt.Sprintf("An library named %q was already defined at %s. LibraryCall names must be unique within a module.", existing.Name, existing.DeclRange),
					Subject:  &o.DeclRange,
				})
			}
			l.LibraryCalls[o.Name] = o
		}
		for _, o := range file.SymbolCalls {
			if existing, exists := l.SymbolCalls[o.Name]; exists {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate symbol definition",
					Detail:   fmt.Sprintf("An symbol named %q was already defined at %s. SymbolCall names must be unique within a module.", existing.Name, existing.DeclRange),
					Subject:  &o.DeclRange,
				})
			}
			l.SymbolCalls[o.Name] = o
		}
	}

	return l, diags
}

// A consistent detail message for all "not a valid identifier" diagnostics.
// Duplicated from the configs package
const badIdentifierDetail = "A name must start with a letter or underscore and may contain only letters, digits, underscores, and dashes."
