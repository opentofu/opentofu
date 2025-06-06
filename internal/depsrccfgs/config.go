// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package depsrccfgs

import (
	"fmt"
	"os"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

type Config struct {
	// ProviderPackageRules are all of the rules for mapping provider source
	// addresses to provider package source locations.
	ProviderPackageRules []*ProviderPackageRule

	// SourcePackageRules are all of the rules for mapping registry-style
	// module source addresses to physical source package locations.
	SourcePackageRules []*SourcePackageRule

	// Filename is the absolute source path of the file that that this
	// configuration was loaded from.
	Filename string
}

func LoadConfig(src []byte, filename string, startPos hcl.Pos) (*Config, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// TODO: Consider supporting a JSON syntax variant too, in case automation
	// wants to generate these files "just in time". (However, that is partially
	// redundant with OpenTofu's existing support for specifying installation
	// methods for providers in the CLI configuration file, so we should think
	// harder about that before deciding.)
	file, hclDiags := hclsyntax.ParseConfig(src, filename, startPos)
	diags = diags.Append(hclDiags)
	if hclDiags.HasErrors() {
		return nil, diags
	}

	ret := &Config{
		Filename: filename,
	}

	content, hclDiags := file.Body.Content(rootSchema)
	diags = diags.Append(hclDiags)
	if hclDiags.HasErrors() {
		return ret, diags
	}

	for _, block := range content.Blocks {
		switch block.Type {
		case "providers":
			rule, moreDiags := decodeProviderPackageRuleBlock(block)
			diags = diags.Append(moreDiags)
			if moreDiags.HasErrors() {
				continue
			}
			ret.ProviderPackageRules = append(ret.ProviderPackageRules, rule)
		case "sources":
			rule, moreDiags := decodeSourcePackageRuleBlock(block)
			diags = diags.Append(moreDiags)
			if moreDiags.HasErrors() {
				continue
			}
			ret.SourcePackageRules = append(ret.SourcePackageRules, rule)
		default:
			// Should not get here: the cases above should exhaustively
			// cover everything declared in rootSchema.
			panic(fmt.Sprintf("unhandled block type %q", block.Type))
		}
	}

	// TODO: Verify that there are no conflicting rules specifying exactly
	// the same matching pattern. There should be at most one rule per
	// fixed prefix at a given pattern specificity level.

	return ret, diags
}

func LoadConfigFile(filename string) (*Config, tfdiags.Diagnostics) {
	src, err := os.ReadFile(filename)
	if err != nil {
		var diags tfdiags.Diagnostics
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to read dependency sources file",
			fmt.Sprintf("Cannot read dependency sources from %s: %s.", filename, tfdiags.FormatError(err)),
		))
		return nil, diags
	}
	return LoadConfig(src, filename, hcl.InitialPos)
}

var rootSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "providers", LabelNames: []string{"pattern"}},
		{Type: "sources", LabelNames: []string{"pattern"}},
	},
}
