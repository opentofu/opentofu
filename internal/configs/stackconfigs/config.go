// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package stackconfigs

import (
	"fmt"
	"os"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type Config struct {
	// MainModule represents the call to the root module of the OpenTofu
	// configuration that this stack uses.
	MainModule *MainModule

	// InitInputVariables are input variables that must be set during
	// "tofu init" when selecting this stack configuration. The values for
	// these are saved as part of the initialized working directory state
	// so that they can be assumed to not change except during initalization,
	// and so they can safely be used for defining dependency-related
	// information that needs to be resolved during init such as module source
	// addresses.
	InitInputVariables map[string]*configs.Variable

	// RuntimeInputVariables are input variables that can be set on a per-round
	// basis. These should be used only for unusual situations with values that
	// need to vary per round, such as an instruction to restore something from
	// a specific backup.
	//
	// Values that are fixed for a particular stack and only need to vary
	// between stacks should have their values assigned directly in the
	// root_module block instead.
	RuntimeInputVariables map[string]*configs.Variable

	// LocalValues are arbitrary local values defined within the stack
	// configuration. OpenTofu assigns no special meaning to these but they
	// might be useful for intermediate data transformations to populate
	// other arguments in the configuration.
	LocalValues map[string]*configs.Local

	// ProviderConfigs are the provider configurations declared in the stack
	// configuration. Currently this is only for the root module state storage,
	// but it would be nice in future to support passing these into the root
	// module as inputs so that it's possible to have a single provider
	// configuration for both state storage and resource management when that's
	// appropriate.
	ProviderConfigs map[string]*configs.Provider

	// RequiredProviders are the provider requirements for the stack
	// configuration in particular. Currently this is used only for the
	// provider that the state storage implementation belongs to.
	RequiredProviders *configs.RequiredProviders

	Filename string
}

func LoadConfig(src []byte, filename string, startPos hcl.Pos) (*Config, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	file, hclDiags := hclsyntax.ParseConfig(src, filename, startPos)
	diags = diags.Append(hclDiags)
	if file == nil {
		return nil, diags
	}

	content, hclDiags := file.Body.Content(rootSchema)
	diags = diags.Append(hclDiags)
	if hclDiags.HasErrors() {
		return nil, diags
	}

	ret := &Config{
		InitInputVariables:    make(map[string]*configs.Variable),
		RuntimeInputVariables: make(map[string]*configs.Variable),
		LocalValues:           make(map[string]*configs.Local),
		ProviderConfigs:       make(map[string]*configs.Provider),
		Filename:              filename,
	}

	for _, block := range content.Blocks {
		switch block.Type {
		case "variable", "init_variable":
			vc, hclDiags := configs.DecodeVariableBlock(block)
			diags = diags.Append(hclDiags)
			if hclDiags.HasErrors() {
				continue
			}

			if existing, exists := ret.InitInputVariables[vc.Name]; exists {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate input variable declaration",
					Detail:   fmt.Sprintf("The name var.%s was already declared as an init-time variable at %s.", vc.Name, existing.DeclRange),
					Subject:  vc.DeclRange.Ptr(),
				})
				continue
			}
			if existing, exists := ret.RuntimeInputVariables[vc.Name]; exists {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate input variable declaration",
					Detail:   fmt.Sprintf("The name var.%s was already declared as an runtime variable at %s.", vc.Name, existing.DeclRange),
					Subject:  vc.DeclRange.Ptr(),
				})
				continue
			}
			if block.Type == "init_variable" {
				ret.InitInputVariables[vc.Name] = vc
			} else {
				ret.RuntimeInputVariables[vc.Name] = vc
			}
		case "provider":
			pc, hclDiags := configs.DecodeProviderBlock(block)
			diags = diags.Append(hclDiags)
			if hclDiags.HasErrors() {
				continue
			}
			key := pc.Addr().String()
			if existing, exists := ret.ProviderConfigs[key]; exists {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate provider configuration",
					Detail:   fmt.Sprintf("A provider configuration with key %q was already declared as at %s.", key, existing.DeclRange),
					Subject:  pc.DeclRange.Ptr(),
				})
				continue
			}
		case "required_providers":
			if ret.RequiredProviders != nil {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate required_providers block",
					Detail:   fmt.Sprintf("The required providers for this stack configuration were already declared at %s.", ret.RequiredProviders.DeclRange),
					Subject:  block.DefRange.Ptr(),
				})
				continue
			}
			reqd, hclDiags := configs.DecodeRequiredProvidersBlock(block)
			diags = diags.Append(hclDiags)
			ret.RequiredProviders = reqd
		case "locals":
			decls, hclDiags := configs.DecodeLocalsBlock(block)
			diags = diags.Append(hclDiags)
			for _, decl := range decls {
				if existing, exists := ret.LocalValues[decl.Name]; exists {
					diags = diags.Append(&hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Duplicate local value definition",
						Detail:   fmt.Sprintf("A local value named %q was already defined as at %s.", decl.Name, existing.DeclRange),
						Subject:  decl.DeclRange.Ptr(),
					})
					continue
				}
			}
		case "main_module":
			if ret.MainModule != nil {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate main_module block",
					Detail:   fmt.Sprintf("The main module for this stack configuration was already called at %s.", ret.MainModule.DeclRange),
					Subject:  block.DefRange.Ptr(),
				})
				continue
			}
			call, moreDiags := decodeMainModuleBlock(block)
			diags = diags.Append(moreDiags)
			ret.MainModule = call
		default:
			// Should not get here because the cases above should cover
			// everything listed in rootSchema.
			panic(fmt.Sprintf("unhandled block type %q", block.Type))
		}
	}

	return ret, diags
}

func LoadConfigFile(filename string) (*Config, tfdiags.Diagnostics) {
	src, err := os.ReadFile(filename)
	if err != nil {
		var diags tfdiags.Diagnostics
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to read stack configuration",
			fmt.Sprintf("Cannot read stack configuration file: %s.", err),
		))
		return nil, diags
	}

	return LoadConfig(src, filename, hcl.InitialPos)
}

var rootSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "variable", LabelNames: []string{"name"}},
		{Type: "init_variable", LabelNames: []string{"name"}},
		{Type: "provider", LabelNames: []string{"type"}},
		{Type: "required_providers"},
		{Type: "locals"},
		{Type: "main_module"},
	},
}
