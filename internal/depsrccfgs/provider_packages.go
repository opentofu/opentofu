// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package depsrccfgs

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"

	"github.com/opentofu/opentofu/internal/addrs"
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

	if ret.Mapper == nil && !diags.HasErrors() {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Missing address mapping configuration",
			Detail:   "A provider mapping block must include one nested block describing how to map each provider address to an installation method.",
			Subject:  block.Body.MissingItemRange().Ptr(),
		})
	}

	return ret, diags
}

type ProviderPackageMapper interface {
	DeclRange() tfdiags.SourceRange
	providerPackageMapper() // sealed interface; implementations in this package only
}

type ProviderPackageOCIMapper struct {
	RepositoryAddrFunc func(addr addrs.Provider) (registryDomain string, repositoryName string, err error)
	declRange          hcl.Range
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

	repositoryAddrExpr := content.Attributes["repository_addr"].Expr
	return &ProviderPackageOCIMapper{
		RepositoryAddrFunc: func(addr addrs.Provider) (registryDomain string, repositoryName string, err error) {
			// TODO: Before returning this we should validate that the
			// template has substitutions for all of the parts of the
			// provider address pattern that were wildcarded, in a similar
			// way as we do for oci_mirror in the CLI configuration. That
			// then allows us to reject an invalid configuration earlier
			// and return a better error message.
			hclCtx := &hcl.EvalContext{
				Variables: map[string]cty.Value{
					"hostname":  cty.StringVal(addr.Hostname.ForDisplay()),
					"namespace": cty.StringVal(addr.Namespace),
					"type":      cty.StringVal(addr.Type),
				},
			}
			val, hclDiags := repositoryAddrExpr.Value(hclCtx)
			if hclDiags.HasErrors() {
				// Ideally we should precheck the expression so that there are
				// as few cases as possible where we end up having to stuff
				// diagnostics into an error here. Refer to the oci_mirror
				// handling in CLI configuration for how that's done there.
				var diags tfdiags.Diagnostics
				diags = diags.Append(hclDiags)
				return "", "", diags.Err()
			}
			var fullAddr string
			err = gocty.FromCtyValue(val, &fullAddr)
			if err != nil {
				return "", "", fmt.Errorf("invalid repository address value: %w", err)
			}
			// FIXME: There's a better parser for this in the CLI configuration package.
			registryDomain, repositoryName, _ = strings.Cut(fullAddr, "/")
			return registryDomain, repositoryName, nil
		},
		declRange: block.DefRange,
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
	BaseURL *url.URL

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
	var baseURLStr string
	err := gocty.FromCtyValue(urlVal, &baseURLStr)
	if err != nil {
		// TODO: better diagnostic
		diags = diags.Append(err)
		return nil, diags
	}
	baseURL, err := url.Parse(baseURLStr)
	if err != nil {
		// TODO: better diagnostic
		diags = diags.Append(err)
		return nil, diags
	}
	ret.BaseURL = baseURL

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
