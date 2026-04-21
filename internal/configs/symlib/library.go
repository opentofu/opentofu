package symlib

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

type Library struct {
	functions    map[string]function.Function
	types        map[string]cty.Type
	typeDefaults map[string]*typeexpr.Defaults

	builtinFuncs map[string]function.Function

	TypeContext typeexpr.TypeContext
	Functions   map[string]function.Function
}

type LibraryLoader func(*LibraryCall) (*Library, hcl.Diagnostics)

func NewLibrary(contents *LibraryContents, loader LibraryLoader, builtinFuncs map[string]function.Function) (*Library, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	l := &Library{
		functions:    map[string]function.Function{},
		types:        map[string]cty.Type{},
		typeDefaults: map[string]*typeexpr.Defaults{},

		builtinFuncs: builtinFuncs,

		Functions: map[string]function.Function{},
		TypeContext: typeexpr.TypeContext{
			Types:    map[string]map[string]cty.Type{},
			Defaults: map[string]map[string]*typeexpr.Defaults{},
		},
	}

	// TODO This is where complex interdependencies can happen!
	// TODO We ignore this problem for now!

	// Load libraries
	for libName, call := range contents.LibraryCalls {
		lib, lDiags := loader(call)
		diags = diags.Extend(lDiags)

		if lib == nil {
			// Load failed
			continue
		}

		// Imported functions
		for name, fn := range lib.functions {
			l.Functions["library::"+libName+"::"+name] = fn
		}

		// Imported types
		l.TypeContext.Types["library::"+libName+"::types"] = lib.types
		l.TypeContext.Defaults["library::"+libName+"::types"] = lib.typeDefaults
	}

	// Declared types
	for _, typeDef := range contents.TypeDefs {
		varType, typeDefault, valDiags := l.TypeContext.TypeConstraintWithDefaults(typeDef.TypeExpr)
		diags = diags.Extend(valDiags)

		l.types[typeDef.Name] = varType
		if typeDefault != nil {
			l.typeDefaults[typeDef.Name] = typeDefault
		}
	}
	// Exported types
	l.TypeContext.Types["library::types"] = l.types
	l.TypeContext.Defaults["library::types"] = l.typeDefaults

	// Declared functions
	for _, fn := range contents.Functions {
		impl, moreDiags := fn.Impl(l)
		diags = diags.Extend(moreDiags)

		l.functions[fn.Name] = impl
	}
	// Exported functions
	for name, fn := range l.functions {
		l.Functions["library::"+name] = fn
	}

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
