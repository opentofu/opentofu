// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package depsrccfgs

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty/gocty"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

type ProviderPackageRule struct {
	MatchPattern ProviderAddrPattern
	Mapper       ProviderPackageMapper

	DeclRange hcl.Range
}

func decodeProviderPackageRuleBlock(block *hcl.Block) (*ProviderPackageRule, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	pattern, err := ParseProviderAddrPattern(block.Labels[0])
	if err != nil {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid provider address pattern",
			Detail:   fmt.Sprintf("Cannot parse %q as a provider address pattern: %s.", block.Labels[0], err),
			Subject:  block.LabelRanges[0].Ptr(),
		})
		return nil, diags
	}

	ret := &ProviderPackageRule{
		MatchPattern: pattern,
		DeclRange:    block.DefRange,
	}

	content, hclDiags := block.Body.Content(providerPackageRuleSchema)
	diags = diags.Append(hclDiags)
	if hclDiags.HasErrors() {
		return ret, diags
	}

	for _, block := range content.Blocks {
		if ret.Mapper != nil {
			// Only one nested block is expected in each rule, with the type
			// specifying which mapper to use.
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Too many address mapping blocks",
				Detail:   fmt.Sprintf("The mapping for this provider address pattern was already defined at %s.", ret.Mapper.DeclRange().StartString()),
				Subject:  block.DefRange.Ptr(),
			})
			continue
		}

		switch block.Type {
		case "oci_repository":
			mapper, moreDiags := decodeProviderPackageOCIMapperBlock(block)
			diags = diags.Append(moreDiags)
			ret.Mapper = mapper
		case "opentofu_network_mirror":
			mapper, moreDiags := decodeProviderPackageNetworkMirrorMapperBlock(block)
			diags = diags.Append(moreDiags)
			ret.Mapper = mapper
		case "direct":
			mapper, moreDiags := decodeProviderPackageDirectMapperBlock(block)
			diags = diags.Append(moreDiags)
			ret.Mapper = mapper
		default:
			panic(fmt.Sprintf("unhandled block type %q", block.Type))
		}
	}

	return ret, diags
}

type ProviderPackageMapper interface {
	DeclRange() tfdiags.SourceRange
	providerPackageMapper() // sealed interface; implementations in this package only
}

type ProviderPackageOCIMapper struct {
	RepositoryAddr hcl.Expression
	declRange      hcl.Range
}

var _ ProviderPackageMapper = (*ProviderPackageOCIMapper)(nil)

func decodeProviderPackageOCIMapperBlock(block *hcl.Block) (ProviderPackageMapper, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	content, hclDiags := block.Body.Content(&hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "repository_addr", Required: true},
		},
	})
	diags = diags.Append(hclDiags)
	if hclDiags.HasErrors() {
		return nil, diags
	}

	return &ProviderPackageOCIMapper{
		RepositoryAddr: content.Attributes["repository_addr"].Expr,
		declRange:      block.DefRange,
	}, diags
}

// DeclRange implements ProviderPackageMapper.
func (p *ProviderPackageOCIMapper) DeclRange() tfdiags.SourceRange {
	return tfdiags.SourceRangeFromHCL(p.declRange)
}

// providerPackageMapper implements ProviderPackageMapper.
func (p *ProviderPackageOCIMapper) providerPackageMapper() {}

type ProviderPackageNetworkMirrorMapper struct {
	// With our network mirror protocol the server is already aware of the
	// heirarchical provider source address scheme and so we need only a
	// base URL here, with the final URL constructed in a fixed scheme
	// based on the requested source address.
	BaseURL string

	declRange hcl.Range
}

var _ ProviderPackageMapper = (*ProviderPackageNetworkMirrorMapper)(nil)

func decodeProviderPackageNetworkMirrorMapperBlock(block *hcl.Block) (ProviderPackageMapper, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	content, hclDiags := block.Body.Content(&hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "url", Required: true},
		},
	})
	diags = diags.Append(hclDiags)
	if hclDiags.HasErrors() {
		return nil, diags
	}

	urlVal, hclDiags := content.Attributes["url"].Expr.Value(nil)
	diags = diags.Append(hclDiags)
	if hclDiags.HasErrors() {
		return nil, diags
	}

	ret := &ProviderPackageNetworkMirrorMapper{
		declRange: block.DefRange,
	}
	err := gocty.FromCtyValue(urlVal, &ret.BaseURL)
	if err != nil {
		// TODO: better diagnostic
		diags = diags.Append(err)
		return nil, diags
	}

	return ret, diags
}

// DeclRange implements ProviderPackageMapper.
func (p *ProviderPackageNetworkMirrorMapper) DeclRange() tfdiags.SourceRange {
	return tfdiags.SourceRangeFromHCL(p.declRange)
}

// providerPackageMapper implements ProviderPackageMapper.
func (p *ProviderPackageNetworkMirrorMapper) providerPackageMapper() {}

type ProviderPackageDirectMapper struct {
	// This mapper takes no arguments at all, because all of the information
	// it needs comes from the provider source address and ambient service
	// discovery configuration.
	declRange hcl.Range
}

var _ ProviderPackageMapper = (*ProviderPackageDirectMapper)(nil)

func decodeProviderPackageDirectMapperBlock(block *hcl.Block) (ProviderPackageMapper, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	_, hclDiags := block.Body.Content(&hcl.BodySchema{})
	diags = diags.Append(hclDiags)
	if hclDiags.HasErrors() {
		return nil, diags
	}

	return &ProviderPackageDirectMapper{
		declRange: block.DefRange,
	}, diags
}

// DeclRange implements ProviderPackageMapper.
func (p *ProviderPackageDirectMapper) DeclRange() tfdiags.SourceRange {
	return tfdiags.SourceRangeFromHCL(p.declRange)
}

// providerPackageMapper implements ProviderPackageMapper.
func (p *ProviderPackageDirectMapper) providerPackageMapper() {}

var providerPackageRuleSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		// Exactly one of the following "mapper configuration" blocks is
		// required in each provider package rule.
		{Type: "oci_repository"},
		{Type: "opentofu_network_mirror"},
		{Type: "direct"},
	},
}
