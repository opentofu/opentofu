// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/opentofu/opentofu/internal/configs/parser"
	"github.com/opentofu/opentofu/internal/encryption/config"
)

// LoadConfigFile reads the file at the given path and parses it as a config
// file.
//
// If the file cannot be read -- for example, if it does not exist -- then
// a nil *File will be returned along with error diagnostics. Callers may wish
// to disregard the returned diagnostics in this case and instead generate
// their own error message(s) with additional context.
//
// If the returned diagnostics has errors when a non-nil map is returned
// then the map may be incomplete but should be valid enough for careful
// static analysis.
//
// This method wraps LoadHCLFile, and so it inherits the syntax selection
// behaviors documented for that method.
func (p *Parser) LoadConfigFile(path string) (*File, hcl.Diagnostics) {
	return p.loadConfigFile(path, false)
}

// LoadConfigFileOverride is the same as LoadConfigFile except that it relaxes
// certain required attribute constraints in order to interpret the given
// file as an overrides file.
func (p *Parser) LoadConfigFileOverride(path string) (*File, hcl.Diagnostics) {
	return p.loadConfigFile(path, true)
}

// LoadTestFile reads the file at the given path and parses it as a OpenTofu
// test file.
//
// It references the same LoadHCLFile as LoadConfigFile, so inherits the same
// syntax selection behaviours.
func (p *Parser) LoadTestFile(path string) (*TestFile, hcl.Diagnostics) {
	body, diags := p.LoadHCLFile(path)
	if body == nil {
		return nil, diags
	}

	test, testDiags := loadTestFile(body)
	diags = append(diags, testDiags...)
	return test, diags
}

func (p *Parser) loadConfigFile(path string, override bool) (*File, hcl.Diagnostics) {
	body, diags := p.LoadHCLFile(path)
	if body == nil {
		return nil, diags
	}

	file := &File{}

	var reqDiags hcl.Diagnostics
	file.CoreVersionConstraints, reqDiags = sniffCoreVersionRequirements(body)
	diags = append(diags, reqDiags...)

	// We'll load the experiments first because other decoding logic in the
	// loop below might depend on these experiments.
	var expDiags hcl.Diagnostics
	file.ActiveExperiments, expDiags = sniffActiveExperiments(body, p.allowExperiments)
	diags = append(diags, expDiags...)

	var parsed parser.File
	decodeDiags := gohcl.DecodeBody(body, nil, &parsed)
	diags = append(diags, decodeDiags...)

	for _, product := range parsed.Product {
		if product.Backend != nil {
			backendCfg, cfgDiags := decodeBackendBlock(product.Backend)
			diags = append(diags, cfgDiags...)
			if backendCfg != nil {
				file.Backends = append(file.Backends, backendCfg)
			}
		}

		if product.Cloud != nil {
			cloudCfg, cfgDiags := decodeCloudBlock(product.Cloud)
			diags = append(diags, cfgDiags...)
			if cloudCfg != nil {
				file.CloudConfigs = append(file.CloudConfigs, cloudCfg)
			}
		}

		if product.RequiredProviders != nil {
			reqs, reqsDiags := decodeRequiredProvidersBlock(product.RequiredProviders)
			diags = append(diags, reqsDiags...)
			file.RequiredProviders = append(file.RequiredProviders, reqs)
		}

		for _, providerMeta := range product.ProviderMeta {
			providerCfg, cfgDiags := decodeProviderMetaBlock(providerMeta)
			diags = append(diags, cfgDiags...)
			if providerCfg != nil {
				file.ProviderMetas = append(file.ProviderMetas, providerCfg)
			}
		}

		if product.Encryption != nil {
			encryptionCfg, cfgDiags := config.DecodeConfig(product.Encryption.Body, product.Encryption.DefRange)
			diags = append(diags, cfgDiags...)
			if encryptionCfg != nil {
				file.Encryptions = append(file.Encryptions, encryptionCfg)
			}
		}
	}

	for _, block := range parsed.RequiredProviders {
		// required_providers should be nested inside a "terraform" block
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid required_providers block",
			Detail:   "A \"required_providers\" block must be nested inside a \"terraform\" block.",
			Subject:  block.TypeRange.Ptr(),
		})
	}

	for _, provider := range parsed.ProviderConfigs {
		cfg, cfgDiags := decodeProviderBlock(provider)
		diags = append(diags, cfgDiags...)
		if cfg != nil {
			file.ProviderConfigs = append(file.ProviderConfigs, cfg)
		}
	}

	for _, variable := range parsed.Variables {
		cfg, cfgDiags := decodeVariableBlock(variable, override)
		diags = append(diags, cfgDiags...)
		if cfg != nil {
			file.Variables = append(file.Variables, cfg)
		}
	}

	for _, local := range parsed.Locals {
		defs, defsDiags := decodeLocalsBlock(local)
		diags = append(diags, defsDiags...)
		file.Locals = append(file.Locals, defs...)
	}

	for _, output := range parsed.Outputs {
		cfg, cfgDiags := decodeOutputBlock(output, override)
		diags = append(diags, cfgDiags...)
		if cfg != nil {
			file.Outputs = append(file.Outputs, cfg)
		}
	}

	for _, moduleCall := range parsed.ModuleCalls {
		cfg, cfgDiags := decodeModuleBlock(moduleCall, override)
		diags = append(diags, cfgDiags...)
		if cfg != nil {
			file.ModuleCalls = append(file.ModuleCalls, cfg)
		}
	}

	for _, resource := range parsed.ManagedResources {
		cfg, cfgDiags := decodeResourceBlock(resource, override)
		diags = append(diags, cfgDiags...)
		if cfg != nil {
			file.ManagedResources = append(file.ManagedResources, cfg)
		}
	}
	for _, datasource := range parsed.DataResources {
		cfg, cfgDiags := decodeDataBlock(datasource, override, false)
		diags = append(diags, cfgDiags...)
		if cfg != nil {
			file.DataResources = append(file.DataResources, cfg)
		}
	}
	for _, ephemeral := range parsed.EphemeralResources {
		cfg, cfgDiags := decodeEphemeralBlock(ephemeral, override)
		diags = append(diags, cfgDiags...)
		if cfg != nil {
			file.EphemeralResources = append(file.EphemeralResources, cfg)
		}
	}

	for _, moved := range parsed.Moved {
		cfg, cfgDiags := decodeMovedBlock(moved)
		diags = append(diags, cfgDiags...)
		if cfg != nil {
			file.Moved = append(file.Moved, cfg)
		}
	}

	for _, imp := range parsed.Import {
		cfg, cfgDiags := decodeImportBlock(imp)
		diags = append(diags, cfgDiags...)
		if cfg != nil {
			file.Import = append(file.Import, cfg)
		}
	}

	for _, check := range parsed.Checks {
		cfg, cfgDiags := decodeCheckBlock(check, override)
		diags = append(diags, cfgDiags...)
		if cfg != nil {
			file.Checks = append(file.Checks, cfg)
		}
	}

	for _, removed := range parsed.Removed {
		cfg, cfgDiags := decodeRemovedBlock(removed)
		diags = append(diags, cfgDiags...)
		if cfg != nil {
			file.Removed = append(file.Removed, cfg)
		}
	}

	return file, diags
}

// sniffCoreVersionRequirements does minimal parsing of the given body for
// "terraform" blocks with "required_version" attributes, returning the
// requirements found.
//
// This is intended to maximize the chance that we'll be able to read the
// requirements (syntax errors notwithstanding) even if the config file contains
// constructs that might've been added in future OpenTofu versions
//
// This is a "best effort" sort of method which will return constraints it is
// able to find, but may return no constraints at all if the given body is
// so invalid that it cannot be decoded at all.
func sniffCoreVersionRequirements(body hcl.Body) ([]VersionConstraint, hcl.Diagnostics) {
	rootContent, _, diags := body.PartialContent(configFileTerraformBlockSniffRootSchema)

	var constraints []VersionConstraint

	for _, block := range rootContent.Blocks {
		content, _, blockDiags := block.Body.PartialContent(configFileVersionSniffBlockSchema)
		diags = append(diags, blockDiags...)

		attr, exists := content.Attributes["required_version"]
		if !exists {
			continue
		}

		constraint, constraintDiags := decodeVersionConstraint(attr)
		diags = append(diags, constraintDiags...)
		if !constraintDiags.HasErrors() {
			constraints = append(constraints, constraint)
		}
	}

	return constraints, diags
}

// configFileTerraformBlockSniffRootSchema is a schema for
// sniffCoreVersionRequirements and sniffActiveExperiments.
var configFileTerraformBlockSniffRootSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{
			Type: "terraform",
		},
	},
}

// configFileVersionSniffBlockSchema is a schema for sniffCoreVersionRequirements
var configFileVersionSniffBlockSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{
			Name: "required_version",
		},
	},
}

// configFileExperimentsSniffBlockSchema is a schema for sniffActiveExperiments,
// to decode a single attribute from inside a "terraform" block.
var configFileExperimentsSniffBlockSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "experiments"},
		{Name: "language"},
	},
}
