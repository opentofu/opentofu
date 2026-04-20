package configs

import (
	"context"
	"fmt"

	"github.com/hashicorp/hcl/v2"
)

type LibraryCall struct {
	*ModuleCall
}

func decodeLibraryBlock(block *hcl.Block, override bool) (*LibraryCall, hcl.Diagnostics) {
	mc, diags := decodeModuleBlock(block, override)
	// Static eval not allowed
	diags = diags.Extend(mc.decodeStaticFields(context.TODO(), NewStaticEvaluator(&Module{}, RootModuleCallForTesting())))
	return &LibraryCall{mc}, diags
}

func (l *LibraryCall) merge(other *LibraryCall) hcl.Diagnostics {
	return l.ModuleCall.merge(other.ModuleCall)
}

type Library struct {
	Functions map[string]*Function
	TypeDefs  map[string]*TypeDef
}

func NewLibrary(files []*SymbolFile) (*Library, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	l := &Library{
		Functions: map[string]*Function{},
		TypeDefs:  map[string]*TypeDef{},
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
			println("REG " + o.Name)
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
	}
	return l, diags
}

type SymbolFile struct {
	Functions []*Function
	TypeDefs  []*TypeDef
}

func (p *Parser) loadSymbolFile(path string) (*SymbolFile, hcl.Diagnostics) {
	body, diags := p.LoadHCLFile(path)
	if body == nil {
		return nil, diags
	}
	ret, moreDiags := loadSymbolFileBody(body, path)
	diags = append(diags, moreDiags...)
	return ret, diags
}

func loadSymbolFileBody(body hcl.Body, _ string) (*SymbolFile, hcl.Diagnostics) {
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
	},
}
