package symlib

import (
	"fmt"

	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

type SymbolLibrary struct {
	scope *scope

	Consts      map[string]cty.Value
	TypeContext typeexpr.TypeContext
	Functions   map[string]function.Function
}

type SymbolsLoader func(*SymbolCall) (*SymbolLibrary, hcl.Diagnostics)

const TypeSymbols = "symbols"

func NewSymbolLibrary(files []*SymbolFile, symLoader SymbolsLoader, builtinFuncs map[string]function.Function) (*SymbolLibrary, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	// Combine symbol files into iterable maps

	values := map[string]*Value{}
	functions := map[string]*Function{}
	typeDefs := map[string]*TypeDef{}
	symbolCalls := map[string]*SymbolCall{}

	for _, file := range files {
		for _, o := range file.Consts {
			if existing, exists := values[o.Name]; exists {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate const definition",
					Detail:   fmt.Sprintf("An const named %q was already defined at %s. Const names must be unique within a module.", existing.Name, existing.DeclRange),
					Subject:  &o.DeclRange,
				})
			}
			values[o.Name] = o
		}
		for _, o := range file.TypeDefs {
			if existing, exists := typeDefs[o.Name]; exists {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate typedef definition",
					Detail:   fmt.Sprintf("An typedef named %q was already defined at %s. TypeDef names must be unique within a module.", existing.Name, existing.DeclRange),
					Subject:  &o.DeclRange,
				})
			}
			typeDefs[o.Name] = o
		}
		for _, o := range file.Functions {
			if existing, exists := functions[o.Name]; exists {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate function definition",
					Detail:   fmt.Sprintf("An function named %q was already defined at %s. Function names must be unique within a module.", existing.Name, existing.DeclRange),
					Subject:  &o.DeclRange,
				})
			}
			functions[o.Name] = o
		}
		for _, o := range file.SymbolCalls {
			if existing, exists := symbolCalls[o.Name]; exists {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate symbols definition",
					Detail:   fmt.Sprintf("An symbols named %q was already defined at %s. SymbolCall names must be unique within a module.", existing.Name, existing.DeclRange),
					Subject:  &o.DeclRange,
				})
			}
			symbolCalls[o.Name] = o
		}
	}

	// Build the library and scope

	l := &SymbolLibrary{
		scope: newScope(builtinFuncs),
	}

	// Load symbol calls first
	for symName, call := range symbolCalls {
		lib, lDiags := symLoader(call)
		diags = diags.Extend(lDiags)

		if lib == nil {
			// Load failed
			continue
		}

		l.scope.symbols[symName] = lib.scope
	}

	// Build scope
	for _, typeDef := range typeDefs {
		l.scope.addType(typeDef.Name, typeDef.TypeExpr)
	}
	for _, fn := range functions {
		l.scope.addFunction(fn.Name, fn.Impl)
	}
	for _, c := range values {
		l.scope.addVar("value", c.Name, c.Expr)
	}

	worker := workgraph.NewWorker()

	// Compile the library

	typeCtx, mDiags := l.scope.typeContext(worker, nil)
	diags = diags.Extend(mDiags)

	evalCtx, mDiags := l.scope.evalContext(worker, nil)
	diags = diags.Extend(mDiags)

	l.TypeContext = typeCtx
	l.Functions = evalCtx.Functions
	l.Consts = evalCtx.Variables

	return l, diags
}

// A consistent detail message for all "not a valid identifier" diagnostics.
// Duplicated from the configs package
const badIdentifierDetail = "A name must start with a letter or underscore and may contain only letters, digits, underscores, and dashes."
