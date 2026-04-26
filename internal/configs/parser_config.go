// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"path/filepath"

	"github.com/hashicorp/hcl/v2"

	"github.com/opentofu/opentofu/internal/encryption/config"
	"github.com/opentofu/opentofu/version"
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

	if test != nil {
		baseDir := filepath.Dir(path)
		for _, mockProvider := range test.MockProviders {
			if mockProvider.Source == "" {
				continue
			}
			sourceDir := filepath.Join(baseDir, mockProvider.Source)
			fromFiles, fileDiags := p.loadMockFilesFromDir(sourceDir, mockProvider.SourceRange)
			diags = append(diags, fileDiags...)
			if fromFiles != nil {
				mockProvider.MockResources = mergeMockResources(mockProvider.MockResources, fromFiles.MockResources)
				mockProvider.OverrideResources = mergeOverrideResources(mockProvider.OverrideResources, fromFiles.OverrideResources)
			}
		}
	}

	return test, diags
}

// LoadMockFile reads the file at the given path and parses it as a mock data
// file (.tfmock.hcl or .tofumock.hcl). Mock files contain mock_resource,
// mock_data, override_resource, and override_data blocks used to populate a
// mock_provider that specifies a source directory.
func (p *Parser) LoadMockFile(path string) (*MockProvider, hcl.Diagnostics) {
	body, diags := p.LoadHCLFile(path)
	if body == nil {
		return nil, diags
	}

	mock, mockDiags := loadMockFileBody(body)
	diags = append(diags, mockDiags...)
	return mock, diags
}

func (p *Parser) loadConfigFile(path string, override bool) (*File, hcl.Diagnostics) {
	body, diags := p.LoadHCLFile(path)
	if body == nil {
		return nil, diags
	}
	ret, moreDiags := loadConfigFileBody(body, path, override)
	diags = append(diags, moreDiags...)
	return ret, diags
}

func loadConfigFileBody(body hcl.Body, _ string, override bool) (*File, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	file := &File{}

	// We check for version compatibility constraints in the module first, using
	// some code designed to be as resilient as possible to unpredictable
	// future extensions to the language, so that we have the best possible
	// chance of returning a version compatibility error if someone intentionally
	// excluded the current version due to the module using newer features.
	reqDiags := checkVersionRequirements(body, version.SemVer)
	diags = append(diags, reqDiags...)

	// We still continue here even if there was a version compatibility problem
	// because we want to gather as complete as possible a map of the content
	// of the valid parts of the module in case a caller wants to use that
	// for careful partial analysis. Note though that if we have at least one
	// version compatibility diagnostic in diags then any other diagnostics
	// added later will eventually be discarded by [finalizeModuleLoadDiagnostics].

	content, contentDiags := body.Content(configFileSchema)
	diags = append(diags, contentDiags...)

	for _, block := range content.Blocks {
		switch block.Type {

		case "language":
			cfgDiags := validateLanguageBlock(block, override)
			diags = append(diags, cfgDiags...)

		case "terraform":
			content, contentDiags := block.Body.Content(terraformBlockSchema)
			diags = append(diags, contentDiags...)

			// We ignore the "required_version", "language" and "experiments"
			// attributes here because checkVersionRequirements above deals
			// with "required_version" and the other two are not relevant
			// to OpenTofu. ("language" blocks contain OpenTofu's equivalents.)

			for _, innerBlock := range content.Blocks {
				switch innerBlock.Type {

				case "backend":
					backendCfg, cfgDiags := decodeBackendBlock(innerBlock)
					diags = append(diags, cfgDiags...)
					if backendCfg != nil {
						file.Backends = append(file.Backends, backendCfg)
					}

				case "cloud":
					cloudCfg, cfgDiags := decodeCloudBlock(innerBlock)
					diags = append(diags, cfgDiags...)
					if cloudCfg != nil {
						file.CloudConfigs = append(file.CloudConfigs, cloudCfg)
					}

				case "required_providers":
					reqs, reqsDiags := decodeRequiredProvidersBlock(innerBlock)
					diags = append(diags, reqsDiags...)
					file.RequiredProviders = append(file.RequiredProviders, reqs)

				case "provider_meta":
					providerCfg, cfgDiags := decodeProviderMetaBlock(innerBlock)
					diags = append(diags, cfgDiags...)
					if providerCfg != nil {
						file.ProviderMetas = append(file.ProviderMetas, providerCfg)
					}

				case "encryption":
					encryptionCfg, cfgDiags := config.DecodeConfig(innerBlock.Body, innerBlock.DefRange)
					diags = append(diags, cfgDiags...)
					if encryptionCfg != nil {
						file.Encryptions = append(file.Encryptions, encryptionCfg)
					}

				default:
					// Should never happen because the above cases should be
					// exhaustive for all block type names in our schema.
					continue

				}
			}

		case "required_providers":
			// required_providers should be nested inside a "terraform" block
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid required_providers block",
				Detail:   "A \"required_providers\" block must be nested inside a \"terraform\" block.",
				Subject:  block.TypeRange.Ptr(),
			})

		case "provider":
			cfg, cfgDiags := decodeProviderBlock(block)
			diags = append(diags, cfgDiags...)
			if cfg != nil {
				file.ProviderConfigs = append(file.ProviderConfigs, cfg)
			}

		case "variable":
			cfg, cfgDiags := decodeVariableBlock(block, override)
			diags = append(diags, cfgDiags...)
			if cfg != nil {
				file.Variables = append(file.Variables, cfg)
			}

		case "locals":
			defs, defsDiags := decodeLocalsBlock(block)
			diags = append(diags, defsDiags...)
			file.Locals = append(file.Locals, defs...)

		case "output":
			cfg, cfgDiags := decodeOutputBlock(block, override)
			diags = append(diags, cfgDiags...)
			if cfg != nil {
				file.Outputs = append(file.Outputs, cfg)
			}

		case "module":
			cfg, cfgDiags := decodeModuleBlock(block, override)
			diags = append(diags, cfgDiags...)
			if cfg != nil {
				file.ModuleCalls = append(file.ModuleCalls, cfg)
			}

		case "resource":
			cfg, cfgDiags := decodeResourceBlock(block, override)
			diags = append(diags, cfgDiags...)
			if cfg != nil {
				file.ManagedResources = append(file.ManagedResources, cfg)
			}

		case "data":
			cfg, cfgDiags := decodeDataBlock(block, override, false)
			diags = append(diags, cfgDiags...)
			if cfg != nil {
				file.DataResources = append(file.DataResources, cfg)
			}

		case "ephemeral":
			cfg, cfgDiags := decodeEphemeralBlock(block, override)
			diags = append(diags, cfgDiags...)
			if cfg != nil {
				file.EphemeralResources = append(file.EphemeralResources, cfg)
			}

		case "moved":
			cfg, cfgDiags := decodeMovedBlock(block)
			diags = append(diags, cfgDiags...)
			if cfg != nil {
				file.Moved = append(file.Moved, cfg)
			}

		case "import":
			cfg, cfgDiags := decodeImportBlock(block)
			diags = append(diags, cfgDiags...)
			if cfg != nil {
				file.Import = append(file.Import, cfg)
			}

		case "check":
			cfg, cfgDiags := decodeCheckBlock(block, override)
			diags = append(diags, cfgDiags...)
			if cfg != nil {
				file.Checks = append(file.Checks, cfg)
			}

		case "removed":
			cfg, cfgDiags := decodeRemovedBlock(block)
			diags = append(diags, cfgDiags...)
			if cfg != nil {
				file.Removed = append(file.Removed, cfg)
			}

		default:
			// Should never happen because the above cases should be exhaustive
			// for all block type names in our schema.
			continue

		}
	}

	return file, diags
}

// configFileSchema is the schema for the top-level of a config file. We use
// the low-level HCL API for this level so we can easily deal with each
// block type separately with its own decoding logic.
var configFileSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{
			Type: "language",
		},
		{
			// This one is not really valid, but we include it here so we
			// can create a specialized error message hinting the user to
			// nest it inside a "terraform" block.
			Type: "required_providers",
		},
		{
			Type:       "provider",
			LabelNames: []string{"name"},
		},
		{
			Type:       "variable",
			LabelNames: []string{"name"},
		},
		{
			Type: "locals",
		},
		{
			Type:       "output",
			LabelNames: []string{"name"},
		},
		{
			Type:       "module",
			LabelNames: []string{"name"},
		},
		{
			Type:       "resource",
			LabelNames: []string{"type", "name"},
		},
		{
			Type:       "data",
			LabelNames: []string{"type", "name"},
		},
		{
			Type:       "ephemeral",
			LabelNames: []string{"type", "name"},
		},
		{
			Type: "moved",
		},
		{
			Type: "import",
		},
		{
			Type:       "check",
			LabelNames: []string{"name"},
		},
		{
			Type: "removed",
		},
		{
			Type: "terraform",
		},
	},
}

// terraformBlockSchema is the schema for a top-level "terraform" block in
// a configuration file.
var terraformBlockSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		// This argument is accepted in any file, but ignored unless it appears
		// in a file named with a ".tofu" or similar suffix that indicates
		// it's intended for OpenTofu rather than its predecessor.
		{Name: "required_version"},

		// The following two are included for compatibility with modules
		// written by OpenTofu's predecessor, but are ignored when present
		// because we cannot predict what any future experiment or language
		// edition keywords in our predecessor might represent.
		//
		// The equivalents of these arguments for OpenTofu are inside top-level
		// "language" blocks, which are handled elsewhere in this package.
		{Name: "experiments"},
		{Name: "language"},
	},
	Blocks: []hcl.BlockHeaderSchema{
		{
			Type:       "backend",
			LabelNames: []string{"type"},
		},
		{
			Type: "cloud",
		},
		{
			Type: "required_providers",
		},
		{
			Type:       "provider_meta",
			LabelNames: []string{"provider"},
		},
		{
			Type: "encryption",
		},
	},
}
