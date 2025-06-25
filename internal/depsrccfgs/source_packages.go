// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package depsrccfgs

import (
	"fmt"
	"strings"

	"github.com/apparentlymart/go-versions/versions"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
	"github.com/zclconf/go-cty/cty/gocty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type SourcePackageRule struct {
	MatchPattern SourceAddrPattern
	Mapper       SourcePackageMapper

	DeclRange hcl.Range
}

func decodeSourcePackageRuleBlock(block *hcl.Block) (*SourcePackageRule, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	pattern, err := ParseSourceAddrPattern(block.Labels[0])
	if err != nil {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid source address pattern",
			Detail:   fmt.Sprintf("Cannot parse %q as a source address pattern: %s.", block.Labels[0], err),
			Subject:  block.LabelRanges[0].Ptr(),
		})
		return nil, diags
	}

	ret := &SourcePackageRule{
		MatchPattern: pattern,
		DeclRange:    block.DefRange,
	}

	content, hclDiags := block.Body.Content(sourcePackageRuleSchema)
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
			mapper, moreDiags := decodeSourcePackageOCIMapperBlock(block)
			diags = diags.Append(moreDiags)
			ret.Mapper = mapper
		case "git_repository":
			mapper, moreDiags := decodeSourcePackageGitMapperBlock(block)
			diags = diags.Append(moreDiags)
			ret.Mapper = mapper
		case "static":
			mapper, moreDiags := decodeSourcePackageStaticMapperBlock(block)
			diags = diags.Append(moreDiags)
			ret.Mapper = mapper
		case "direct":
			mapper, moreDiags := decodeSourcePackageDirectMapperBlock(block)
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

type SourcePackageMapper interface {
	DeclRange() tfdiags.SourceRange
	sourcePackageMapper() // sealed interface; implementations in this package only
}

// SourcePackageStaticMapper is a transitional [SourcePackageMapper] that
// translates directly to traditional raw source addresses at the expense
// of forcing the author to write a list of all of the available versions
// inline in the configuration, since raw source addresses don't have any
// concept of versions.
//
// If possible it's better to use one of the other mappers that translates
// to a system that is able to determine a list of available versions by
// querying a remote system.
type SourcePackageStaticMapper struct {
	// AvailableVersions is the statically-configured set of available versions,
	// used to compensate for the fact that raw source addresses don't have
	// any "list available versions" operation.
	AvailableVersions versions.List

	// SourceAddrFunc encapsulates the process of rendering the author's
	// address template based on the module address and selected version.
	SourceAddrFunc func(addr addrs.ModuleRegistryPackage, version versions.Version) (addrs.ModuleSourceRemote, error)

	declRange hcl.Range
}

func decodeSourcePackageStaticMapperBlock(block *hcl.Block) (SourcePackageMapper, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	content, hclDiags := block.Body.Content(&hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "versions", Required: true},
			{Name: "source_addr", Required: true},
		},
	})
	diags = diags.Append(hclDiags)
	if hclDiags.HasErrors() {
		return nil, diags
	}

	sourceAddrExpr := content.Attributes["source_addr"].Expr
	versionsVal, hclDiags := content.Attributes["versions"].Expr.Value(nil)
	diags = diags.Append(hclDiags)
	if hclDiags.HasErrors() {
		return nil, diags
	}
	versionsVal, err := convert.Convert(versionsVal, cty.List(cty.String))
	if err != nil {
		// TODO: a proper diagnostic
		diags = diags.Append(err)
		return nil, diags
	}
	var versionStrs []string
	err = gocty.FromCtyValue(versionsVal, &versionStrs)
	if err != nil {
		// TODO: a proper diagnostic
		diags = diags.Append(err)
		return nil, diags
	}
	availableVersions := make(versions.List, len(versionStrs))
	for i, versionStr := range versionStrs {
		version, err := versions.ParseVersion(versionStr)
		if err != nil {
			// TODO: a proper diagnostic, ideally highlighting the specific
			// item that caused the error.
			diags = diags.Append(err)
			return nil, diags
		}
		availableVersions[i] = version
	}

	return &SourcePackageStaticMapper{
		AvailableVersions: availableVersions,
		SourceAddrFunc: func(addr addrs.ModuleRegistryPackage, version versions.Version) (addrs.ModuleSourceRemote, error) {
			// TODO: Before returning this we should validate that the
			// template has substitutions for all of the parts of the
			// module address pattern that were wildcarded, and for
			// the version number in particular.
			hclCtx := &hcl.EvalContext{
				Variables: map[string]cty.Value{
					"hostname":      cty.StringVal(addr.Host.ForDisplay()),
					"namespace":     cty.StringVal(addr.Namespace),
					"name":          cty.StringVal(addr.Name),
					"target_system": cty.StringVal(addr.TargetSystem),
					"version":       cty.StringVal(version.String()),
				},
			}
			val, hclDiags := sourceAddrExpr.Value(hclCtx)
			if hclDiags.HasErrors() {
				// Ideally we should precheck the expression so that there are
				// as few cases as possible where we end up having to stuff
				// diagnostics into an error here. Refer to the oci_mirror
				// handling in CLI configuration for how that's done there.
				var diags tfdiags.Diagnostics
				diags = diags.Append(hclDiags)
				return addrs.ModuleSourceRemote{}, diags.Err()
			}
			var sourceAddrStr string
			err = gocty.FromCtyValue(val, &sourceAddrStr)
			if err != nil {
				return addrs.ModuleSourceRemote{}, fmt.Errorf("invalid source address value: %w", err)
			}
			src, err := addrs.ParseModuleSource(sourceAddrStr)
			if err != nil {
				return addrs.ModuleSourceRemote{}, fmt.Errorf("invalid source address value: %w", err)
			}
			remoteSrc, ok := src.(addrs.ModuleSourceRemote)
			if !ok {
				return addrs.ModuleSourceRemote{}, fmt.Errorf("invalid source address value: must specify a remote source location")
			}
			return remoteSrc, nil
		},
	}, diags
}

// DeclRange implements SourcePackageMapper.
func (m *SourcePackageStaticMapper) DeclRange() tfdiags.SourceRange {
	return tfdiags.SourceRangeFromHCL(m.declRange)
}

// sourcePackageMapper implements SourcePackageMapper.
func (m *SourcePackageStaticMapper) sourcePackageMapper() {}

type SourcePackageOCIMapper struct {
	RepositoryAddrFunc func(addr addrs.ModuleRegistryPackage) (registryDomain string, repositoryName string, err error)
	declRange          hcl.Range
}

var _ SourcePackageMapper = (*SourcePackageOCIMapper)(nil)

func decodeSourcePackageOCIMapperBlock(block *hcl.Block) (SourcePackageMapper, tfdiags.Diagnostics) {
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
	return &SourcePackageOCIMapper{
		RepositoryAddrFunc: func(addr addrs.ModuleRegistryPackage) (registryDomain string, repositoryName string, err error) {
			// TODO: Before returning this we should validate that the
			// template has substitutions for all of the parts of the
			// module address pattern that were wildcarded, in a similar
			// way as we do for provider oci_mirror in the CLI configuration.
			// That then allows us to reject an invalid configuration earlier
			// and return a better error message.
			hclCtx := &hcl.EvalContext{
				Variables: map[string]cty.Value{
					"hostname":      cty.StringVal(addr.Host.ForDisplay()),
					"namespace":     cty.StringVal(addr.Namespace),
					"name":          cty.StringVal(addr.Name),
					"target_system": cty.StringVal(addr.TargetSystem),
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

// DeclRange implements SourcePackageMapper.
func (m *SourcePackageOCIMapper) DeclRange() tfdiags.SourceRange {
	return tfdiags.SourceRangeFromHCL(m.declRange)
}

// sourcePackageMapper implements SourcePackageMapper.
func (m *SourcePackageOCIMapper) sourcePackageMapper() {}

type SourcePackageGitMapper struct {
	RepositoryAddrFunc func(addr addrs.ModuleRegistryPackage) (repositoryURL string, err error)
	TagPrefixFunc      func(addr addrs.ModuleRegistryPackage) (tagPrefix string, err error)
	SubdirFunc         func(addr addrs.ModuleRegistryPackage) (subdir string, present bool, err error)
	declRange          hcl.Range
}

var _ SourcePackageMapper = (*SourcePackageGitMapper)(nil)

func decodeSourcePackageGitMapperBlock(block *hcl.Block) (SourcePackageMapper, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	content, hclDiags := block.Body.Content(&hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "repository_addr", Required: true},
			{Name: "tag_prefix"},
			{Name: "subdirectory"},
		},
	})
	diags = diags.Append(hclDiags)
	if hclDiags.HasErrors() {
		return nil, diags
	}

	makeHCLCtx := func(addr addrs.ModuleRegistryPackage) *hcl.EvalContext {
		// TODO: Before returning from the parent function we should validate
		// that the templates both have substitutions for all of the parts of
		// the module address pattern that were wildcarded, in a similar
		// way as we do for provider oci_mirror in the CLI configuration.
		// That then allows us to reject an invalid configuration earlier
		// and return a better error message.
		return &hcl.EvalContext{
			Variables: map[string]cty.Value{
				"hostname":      cty.StringVal(addr.Host.ForDisplay()),
				"namespace":     cty.StringVal(addr.Namespace),
				"name":          cty.StringVal(addr.Name),
				"target_system": cty.StringVal(addr.TargetSystem),
			},
		}
	}
	evalExpr := func(expr hcl.Expression, addr addrs.ModuleRegistryPackage) (string, bool, error) {
		hclCtx := makeHCLCtx(addr)
		val, hclDiags := expr.Value(hclCtx)
		if hclDiags.HasErrors() {
			// Ideally we should precheck the expression so that there are
			// as few cases as possible where we end up having to stuff
			// diagnostics into an error here. Refer to the oci_mirror
			// handling in CLI configuration for how that's done there.
			var diags tfdiags.Diagnostics
			diags = diags.Append(hclDiags)
			return "", false, diags.Err()
		}
		if val.IsNull() {
			return "", false, nil
		}
		var result string
		err := gocty.FromCtyValue(val, &result)
		if err != nil {
			return "", false, fmt.Errorf("invalid value: %w", err)
		}
		return result, true, nil
	}

	// By default we search for tags whose names simply start with "v", and
	// assume that whatever comes afterwards is inteded to be a version number.
	// Authors can override this if, for example, they have multiple
	// independently-versioned modules in the same repository and so need
	// to include some sort of per-module identifier as part of the prefix.
	tagPrefixFunc := func(addr addrs.ModuleRegistryPackage) (tagPrefix string, err error) {
		return "v", nil
	}
	if attr := content.Attributes["tag_prefix"]; attr != nil {
		tagPrefixFunc = func(addr addrs.ModuleRegistryPackage) (tagPrefix string, err error) {
			ret, present, err := evalExpr(attr.Expr, addr)
			if err != nil {
				return "", err
			}
			if !present {
				return "v", nil
			}
			return ret, nil
		}
	}

	subdirFunc := func(addr addrs.ModuleRegistryPackage) (string, bool, error) {
		return "", false, nil
	}
	if attr := content.Attributes["subdirectory"]; attr != nil {
		subdirFunc = func(addr addrs.ModuleRegistryPackage) (tagPrefix string, present bool, err error) {
			return evalExpr(attr.Expr, addr)
		}
	}

	repositoryAddrFunc := func(addr addrs.ModuleRegistryPackage) (tagPrefix string, err error) {
		ret, present, err := evalExpr(content.Attributes["repository_addr"].Expr, addr)
		if err != nil {
			return "", err
		}
		if !present {
			return "", fmt.Errorf("repository address is required")
		}
		return ret, nil
	}

	return &SourcePackageGitMapper{
		RepositoryAddrFunc: repositoryAddrFunc,
		TagPrefixFunc:      tagPrefixFunc,
		SubdirFunc:         subdirFunc,
		declRange:          block.DefRange,
	}, diags
}

// DeclRange implements SourcePackageMapper.
func (m *SourcePackageGitMapper) DeclRange() tfdiags.SourceRange {
	return tfdiags.SourceRangeFromHCL(m.declRange)
}

// sourcePackageMapper implements SourcePackageMapper.
func (m *SourcePackageGitMapper) sourcePackageMapper() {}

type SourcePackageDirectMapper struct {
	// This mapper takes no arguments at all, because all of the information
	// it needs comes from the module source address and ambient service
	// discovery configuration.
	declRange hcl.Range
}

var _ ProviderPackageMapper = (*ProviderPackageDirectMapper)(nil)

func decodeSourcePackageDirectMapperBlock(block *hcl.Block) (SourcePackageMapper, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	_, hclDiags := block.Body.Content(&hcl.BodySchema{})
	diags = diags.Append(hclDiags)
	if hclDiags.HasErrors() {
		return nil, diags
	}

	return &SourcePackageDirectMapper{
		declRange: block.DefRange,
	}, diags
}

// DeclRange implements ProviderPackageMapper.
func (p *SourcePackageDirectMapper) DeclRange() tfdiags.SourceRange {
	return tfdiags.SourceRangeFromHCL(p.declRange)
}

// providerPackageMapper implements ProviderPackageMapper.
func (p *SourcePackageDirectMapper) sourcePackageMapper() {}

var sourcePackageRuleSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		// Exactly one of the following "mapper configuration" blocks is
		// required in each provider package rule.
		{Type: "oci_repository"},
		{Type: "git_repository"},
		{Type: "static"},
		{Type: "direct"},
	},
}
