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

	TypeContext typeexpr.TypeContext
	Functions   map[string]function.Function
}

type LibraryLoader func(*LibraryCall) (*Library, hcl.Diagnostics)

func NewLibrary(contents *LibraryContents, loader LibraryLoader, builtinFuncs map[string]function.Function) (*Library, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	l := &Library{
		scope: newScope(builtinFuncs),

		Functions: map[string]function.Function{},
		TypeContext: typeexpr.TypeContext{
			Types:    map[string]map[string]cty.Type{},
			Defaults: map[string]map[string]*typeexpr.Defaults{},
		},
	}

	// Load libraries
	for libName, call := range contents.LibraryCalls {
		lib, lDiags := loader(call)
		diags = diags.Extend(lDiags)

		if lib == nil {
			// Load failed
			continue
		}

		l.scope.libraries[libName] = lib.scope
	}

	// Build scope
	for _, typeDef := range contents.TypeDefs {
		l.scope.addType(typeDef.Name, typeDef.TypeExpr)
	}
	for _, fn := range contents.Functions {
		l.scope.addFunction(fn.Name, fn.Impl)
	}
	// TODO consts

	worker := workgraph.NewWorker()

	typeCtx, mDiags := l.scope.typeContext(worker, nil)
	diags = diags.Extend(mDiags)

	evalCtx, mDiags := l.scope.evalContext(worker, nil)
	diags = diags.Extend(mDiags)

	l.TypeContext = typeCtx
	l.Functions = evalCtx.Functions

	return l, diags
}

type LibraryContents struct {
	Functions    map[string]*Function
	TypeDefs     map[string]*TypeDef
	LibraryCalls map[string]*LibraryCall
}

func NewLibraryContents(files []*SymbolFile) (*LibraryContents, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	l := &LibraryContents{
		Functions:    map[string]*Function{},
		TypeDefs:     map[string]*TypeDef{},
		LibraryCalls: map[string]*LibraryCall{},
	}

	for _, file := range files {
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
	}

	return l, diags
}

type SymbolFile struct {
	Functions    []*Function
	TypeDefs     []*TypeDef
	LibraryCalls []*LibraryCall
}

func LoadSymbolFile(body hcl.Body) (*SymbolFile, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	file := &SymbolFile{}

	content, contentDiags := body.Content(symbolFileSchema)
	diags = append(diags, contentDiags...)

	for _, block := range content.Blocks {
		switch block.Type {
		case "function":
			cfg, cfgDiags := decodeFunctionBlock(block)
			diags = append(diags, cfgDiags...)
			if cfg != nil {
				file.Functions = append(file.Functions, cfg)
			}
		case "typedef":
			cfg, cfgDiags := decodeTypeDefBlock(block)
			diags = append(diags, cfgDiags...)
			if cfg != nil {
				file.TypeDefs = append(file.TypeDefs, cfg)
			}
		case "library":
			cfg, cfgDiags := decodeLibraryBlock(block)
			diags = append(diags, cfgDiags...)
			if cfg != nil {
				file.LibraryCalls = append(file.LibraryCalls, cfg)
			}
		default:
			// Should never happen because the above cases should be exhaustive
			// for all block type names in our schema.
			continue

		}
	}

	return file, diags
}

var symbolFileSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{
			Type:       "function",
			LabelNames: []string{"name"},
		},
		{
			Type:       "typedef",
			LabelNames: []string{"name"},
		},
		{
			Type:       "library",
			LabelNames: []string{"name"},
		},
	},
}

// A consistent detail message for all "not a valid identifier" diagnostics.
// Duplicated from the configs package
const badIdentifierDetail = "A name must start with a letter or underscore and may contain only letters, digits, underscores, and dashes."
