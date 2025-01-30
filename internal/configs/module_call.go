// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"errors"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/getmodules"
)

// ModuleCall represents a "module" block in a module or file.
type ModuleCall struct {
	Name string

	Source        hcl.Expression
	SourceAddrRaw string
	SourceAddr    addrs.ModuleSource
	SourceSet     bool

	// Used when building the corresponding StaticModuleCall
	Variables StaticModuleVariables
	Workspace string

	Config hcl.Body

	VersionAttr *hcl.Attribute
	Version     VersionConstraint

	Count   hcl.Expression
	ForEach hcl.Expression

	Providers []PassedProviderConfig

	DependsOn []hcl.Traversal

	DeclRange hcl.Range
}

func decodeModuleBlock(block *hcl.Block, override bool) (*ModuleCall, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	mc := &ModuleCall{
		DeclRange: block.DefRange,
		Name:      block.Labels[0],
	}

	schema := moduleBlockSchema
	if override {
		schema = schemaForOverrides(schema)
	}

	content, remain, moreDiags := block.Body.PartialContent(schema)
	diags = append(diags, moreDiags...)
	mc.Config = remain

	if !hclsyntax.ValidIdentifier(mc.Name) {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid module instance name",
			Detail:   badIdentifierDetail,
			Subject:  &block.LabelRanges[0],
		})
	}

	if attr, exists := content.Attributes["version"]; exists {
		mc.VersionAttr = attr
	}

	if attr, exists := content.Attributes["source"]; exists {
		mc.SourceSet = true
		mc.Source = attr.Expr
	}

	if attr, exists := content.Attributes["count"]; exists {
		mc.Count = attr.Expr
	}

	if attr, exists := content.Attributes["for_each"]; exists {
		if mc.Count != nil {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  `Invalid combination of "count" and "for_each"`,
				Detail:   `The "count" and "for_each" meta-arguments are mutually-exclusive, only one should be used to be explicit about the number of resources to be created.`,
				Subject:  &attr.NameRange,
			})
		}

		mc.ForEach = attr.Expr
	}

	if attr, exists := content.Attributes["depends_on"]; exists {
		deps, depsDiags := decodeDependsOn(attr)
		diags = append(diags, depsDiags...)
		mc.DependsOn = append(mc.DependsOn, deps...)
	}

	if attr, exists := content.Attributes["providers"]; exists {
		providers, providerDiags := decodePassedProviderConfigs(attr)
		diags = append(diags, providerDiags...)
		mc.Providers = append(mc.Providers, providers...)
	}

	var seenEscapeBlock *hcl.Block
	for _, block := range content.Blocks {
		switch block.Type {
		case "_":
			if seenEscapeBlock != nil {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate escaping block",
					Detail: fmt.Sprintf(
						"The special block type \"_\" can be used to force particular arguments to be interpreted as module input variables rather than as meta-arguments, but each module block can have only one such block. The first escaping block was at %s.",
						seenEscapeBlock.DefRange,
					),
					Subject: &block.DefRange,
				})
				continue
			}
			seenEscapeBlock = block

			// When there's an escaping block its content merges with the
			// existing config we extracted earlier, so later decoding
			// will see a blend of both.
			mc.Config = hcl.MergeBodies([]hcl.Body{mc.Config, block.Body})

		default:
			// All of the other block types in our schema are reserved.
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Reserved block type name in module block",
				Detail:   fmt.Sprintf("The block type name %q is reserved for use by OpenTofu in a future version.", block.Type),
				Subject:  &block.TypeRange,
			})
		}
	}

	return mc, diags
}

func (mc *ModuleCall) decodeStaticFields(eval *StaticEvaluator) hcl.Diagnostics {
	mc.Workspace = eval.call.workspace
	mc.decodeStaticVariables(eval)

	var diags hcl.Diagnostics
	diags = diags.Extend(mc.decodeStaticSource(eval))
	diags = diags.Extend(mc.decodeStaticVersion(eval))
	return diags
}

func (mc *ModuleCall) decodeStaticSource(eval *StaticEvaluator) hcl.Diagnostics {
	if mc.Source == nil {
		// This is an invalid module.  We already have error handling that has more context and can produce better errors in this
		// scenario.  Follow the trail of mc.SourceAddr -> req.SourceAddr through the command package.
		return nil
	}

	// Decode source field
	diags := eval.DecodeExpression(mc.Source, StaticIdentifier{Module: eval.call.addr, Subject: fmt.Sprintf("module.%s.source", mc.Name), DeclRange: mc.Source.Range()}, &mc.SourceAddrRaw)
	if !diags.HasErrors() {
		// NOTE: This code was originally executed as part of decodeModuleBlock and is now deferred until we have the config merged and static context built
		var err error
		if mc.VersionAttr != nil {
			mc.SourceAddr, err = addrs.ParseModuleSourceRegistry(mc.SourceAddrRaw)
		} else {
			mc.SourceAddr, err = addrs.ParseModuleSource(mc.SourceAddrRaw)
		}
		if err != nil {
			// NOTE: We leave SourceAddr as nil for any situation where the
			// source attribute is invalid, so any code which tries to carefully
			// use the partial result of a failed config decode must be
			// resilient to that.
			mc.SourceAddr = nil

			// NOTE: In practice it's actually very unlikely to end up here,
			// because our source address parser can turn just about any string
			// into some sort of remote package address, and so for most errors
			// we'll detect them only during module installation. There are
			// still a _few_ purely-syntax errors we can catch at parsing time,
			// though, mostly related to remote package sub-paths and local
			// paths.
			var pathErr *getmodules.MaybeRelativePathErr
			if errors.As(err, &pathErr) {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid module source address",
					Detail: fmt.Sprintf(
						"OpenTofu failed to determine your intended installation method for remote module package %q.\n\nIf you intended this as a path relative to the current module, use \"./%s\" instead. The \"./\" prefix indicates that the address is a relative filesystem path.",
						pathErr.Addr, pathErr.Addr,
					),
					Subject: mc.Source.Range().Ptr(),
				})
			} else {
				if mc.VersionAttr != nil {
					// In this case we'll include some extra context that
					// we assumed a registry source address due to the
					// version argument.
					diags = append(diags, &hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Invalid registry module source address",
						Detail:   fmt.Sprintf("Failed to parse module registry address: %s.\n\nOpenTofu assumed that you intended a module registry source address because you also set the argument \"version\", which applies only to registry modules.", err),
						Subject:  mc.Source.Range().Ptr(),
					})
				} else {
					diags = append(diags, &hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Invalid module source address",
						Detail:   fmt.Sprintf("Failed to parse module source address: %s.", err),
						Subject:  mc.Source.Range().Ptr(),
					})
				}
			}
		}
	}

	return diags
}

func (mc *ModuleCall) decodeStaticVersion(eval *StaticEvaluator) hcl.Diagnostics {
	var diags hcl.Diagnostics

	if mc.VersionAttr == nil {
		return diags
	}

	val, valDiags := eval.Evaluate(mc.VersionAttr.Expr, StaticIdentifier{
		Module:    eval.call.addr,
		Subject:   fmt.Sprintf("module.%s.version", mc.Name),
		DeclRange: mc.VersionAttr.Range,
	})
	diags = diags.Extend(valDiags)
	if diags.HasErrors() {
		return diags
	}

	var verDiags hcl.Diagnostics
	mc.Version, verDiags = decodeVersionConstraintValue(mc.VersionAttr, val)
	return diags.Extend(verDiags)
}

func (mc *ModuleCall) decodeStaticVariables(eval *StaticEvaluator) {
	attr, _ := mc.Config.JustAttributes()

	mc.Variables = func(variable *Variable) (cty.Value, hcl.Diagnostics) {
		v, ok := attr[variable.Name]
		if !ok {
			if variable.Required() {
				return cty.NilVal, hcl.Diagnostics{&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Missing required variable in module call",
					Subject:  mc.Config.MissingItemRange().Ptr(),
				}}
			}
			return variable.Default, nil
		}

		ident := StaticIdentifier{
			Module:    eval.call.addr.Child(mc.Name),
			Subject:   fmt.Sprintf("var.%s", variable.Name),
			DeclRange: v.Range,
		}
		return eval.Evaluate(v.Expr, ident)
	}
}

// EntersNewPackage returns true if this call is to an external module, either
// directly via a remote source address or indirectly via a registry source
// address.
//
// Other behaviors in OpenTofu may treat package crossings as a special
// situation, because that indicates that the caller and callee can change
// independently of one another and thus we should disallow using any features
// where the caller assumes anything about the callee other than its input
// variables, required provider configurations, and output values.
func (mc *ModuleCall) EntersNewPackage() bool {
	return moduleSourceAddrEntersNewPackage(mc.SourceAddr)
}

// PassedProviderConfig represents a provider config explicitly passed down to
// a child module, possibly giving it a new local address in the process.
type PassedProviderConfig struct {
	InChild  *ProviderConfigRef
	InParent *ProviderConfigRef
}

func decodePassedProviderConfigs(attr *hcl.Attribute) ([]PassedProviderConfig, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	var providers []PassedProviderConfig

	seen := make(map[string]hcl.Range)
	pairs, pDiags := hcl.ExprMap(attr.Expr)
	diags = append(diags, pDiags...)
	for _, pair := range pairs {
		key, keyDiags := decodeProviderConfigRef(pair.Key, "providers")
		diags = append(diags, keyDiags...)
		value, valueDiags := decodeProviderConfigRef(pair.Value, "providers")
		diags = append(diags, valueDiags...)
		if keyDiags.HasErrors() || valueDiags.HasErrors() {
			continue
		}

		matchKey := key.String()
		if prev, exists := seen[matchKey]; exists {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Duplicate provider address",
				Detail:   fmt.Sprintf("A provider configuration was already passed to %s at %s. Each child provider configuration can be assigned only once.", matchKey, prev),
				Subject:  pair.Value.Range().Ptr(),
			})
			continue
		}

		rng := hcl.RangeBetween(pair.Key.Range(), pair.Value.Range())
		seen[matchKey] = rng
		providers = append(providers, PassedProviderConfig{
			InChild:  key,
			InParent: value,
		})
	}
	return providers, diags
}

var moduleBlockSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{
			Name:     "source",
			Required: true,
		},
		{
			Name: "version",
		},
		{
			Name: "count",
		},
		{
			Name: "for_each",
		},
		{
			Name: "depends_on",
		},
		{
			Name: "providers",
		},
	},
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "_"}, // meta-argument escaping block

		// These are all reserved for future use.
		{Type: "lifecycle"},
		{Type: "locals"},
		{Type: "provider", LabelNames: []string{"type"}},
	},
}

func moduleSourceAddrEntersNewPackage(addr addrs.ModuleSource) bool {
	switch addr.(type) {
	case nil:
		// There are only two situations where we should get here:
		// - We've been asked about the source address of the root module,
		//   which is always nil.
		// - We've been asked about a ModuleCall that is part of the partial
		//   result of a failed decode.
		// The root module exists outside of all module packages, so we'll
		// just return false for that case. For the error case it doesn't
		// really matter what we return as long as we don't panic, because
		// we only make a best-effort to allow careful inspection of objects
		// representing invalid configuration.
		return false
	case addrs.ModuleSourceLocal:
		// Local source addresses are the only address type that remains within
		// the same package.
		return false
	default:
		// All other address types enter a new package.
		return true
	}
}
