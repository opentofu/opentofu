package symlib

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty/function"
)

type Symbols struct {
	*Library
}

func NewSymbols(file *SymbolFile, libLoader LibraryLoader, symLoader SymbolsLoader, builtinFuncs map[string]function.Function) (*Symbols, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	contents, sDiags := NewLibraryContents([]*SymbolFile{file})
	diags = diags.Extend(sDiags)

	l, lDiags := newLibraryOrSymbols(TypeSymbols, contents, libLoader, symLoader, builtinFuncs)
	diags = diags.Extend(lDiags)

	if l == nil {
		return nil, diags
	}
	return &Symbols{l}, diags
}

type SymbolCall struct {
	Name        string
	Source      hcl.Expression
	VersionAttr *hcl.Attribute
	File        string
	DeclRange   hcl.Range
}

func DecodeSymbolsBlock(block *hcl.Block) (*SymbolCall, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	lc := &SymbolCall{
		DeclRange: block.DefRange,
		Name:      block.Labels[0],
	}

	content, moreDiags := block.Body.Content(symbolBlockSchema)
	diags = append(diags, moreDiags...)

	if !hclsyntax.ValidIdentifier(lc.Name) {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid symbol name",
			Detail:   badIdentifierDetail,
			Subject:  &block.LabelRanges[0],
		})
	}

	if attr, exists := content.Attributes["version"]; exists {
		lc.VersionAttr = attr
	}

	if attr, exists := content.Attributes["source"]; exists {
		lc.Source = attr.Expr
	}

	if attr, exists := content.Attributes["file"]; exists {
		diags = diags.Extend(gohcl.DecodeExpression(attr.Expr, nil, &lc.File))
	}

	return lc, diags
}

var symbolBlockSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{
			Name:     "source",
			Required: true,
		},
		{
			Name: "version",
		},
		{
			Name:     "file",
			Required: true,
		},
	},
}

type SymbolFile struct {
	Consts       []*Const
	Functions    []*Function
	TypeDefs     []*TypeDef
	LibraryCalls []*LibraryCall
	SymbolCalls  []*SymbolCall
}

func LoadSymbolFile(body hcl.Body) (*SymbolFile, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	file := &SymbolFile{}

	content, contentDiags := body.Content(symbolFileSchema)
	diags = append(diags, contentDiags...)

	for _, block := range content.Blocks {
		switch block.Type {
		case "const":
			cfg, cfgDiags := decodeConstBlock(block)
			diags = append(diags, cfgDiags...)
			if cfg != nil {
				file.Consts = append(file.Consts, cfg...)
			}
		case "typedef":
			cfg, cfgDiags := decodeTypeDefBlock(block)
			diags = append(diags, cfgDiags...)
			if cfg != nil {
				file.TypeDefs = append(file.TypeDefs, cfg)
			}
		case "function":
			cfg, cfgDiags := decodeFunctionBlock(block)
			diags = append(diags, cfgDiags...)
			if cfg != nil {
				file.Functions = append(file.Functions, cfg)
			}
		case "library":
			cfg, cfgDiags := decodeLibraryBlock(block)
			diags = append(diags, cfgDiags...)
			if cfg != nil {
				file.LibraryCalls = append(file.LibraryCalls, cfg)
			}
		case "symbols":
			cfg, cfgDiags := DecodeSymbolsBlock(block)
			diags = append(diags, cfgDiags...)
			if cfg != nil {
				file.SymbolCalls = append(file.SymbolCalls, cfg)
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
			Type: "const",
		},
		{
			Type:       "typedef",
			LabelNames: []string{"name"},
		},
		{
			Type:       "function",
			LabelNames: []string{"name"},
		},
		{
			Type:       "library",
			LabelNames: []string{"name"},
		},
		{
			Type:       "symbols",
			LabelNames: []string{"name"},
		},
	},
}
