// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"fmt"
	"math/big"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	hcljson "github.com/hashicorp/hcl/v2/json"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/hcl2shim"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/lang/evalchecks"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Resource represents a "resource" or "data" block in a module or file.
type Resource struct {
	Mode    addrs.ResourceMode
	Name    string
	Type    string
	Config  hcl.Body
	Count   hcl.Expression
	ForEach hcl.Expression

	ProviderConfigRef *ProviderConfigRefMapping
	Provider          addrs.Provider

	Preconditions  []*CheckRule
	Postconditions []*CheckRule

	DependsOn []hcl.Traversal

	TriggersReplacement []hcl.Expression

	// Managed is populated only for Mode = addrs.ManagedResourceMode,
	// containing the additional fields that apply to managed resources.
	// For all other resource modes, this field is nil.
	Managed *ManagedResource

	// Container links a scoped resource back up to the resources that contains
	// it. This field is referenced during static analysis to check whether any
	// references are also made from within the same container.
	//
	// If this is nil, then this resource is essentially public.
	Container Container

	// IsOverridden indicates if the resource is being overridden. It's used in
	// testing framework to not call the underlying provider.
	IsOverridden bool
	// OverrideValues are only valid if IsOverridden is set to true. The values
	// should be used to compose mock provider response. It is possible to have
	// zero-length OverrideValues even if IsOverridden is set to true.
	OverrideValues map[string]cty.Value

	DeclRange hcl.Range
	TypeRange hcl.Range
}

// ManagedResource represents a "resource" block in a module or file.
type ManagedResource struct {
	Connection   *Connection
	Provisioners []*Provisioner

	CreateBeforeDestroy bool
	PreventDestroy      bool
	IgnoreChanges       []hcl.Traversal
	IgnoreAllChanges    bool

	CreateBeforeDestroySet bool
	PreventDestroySet      bool
}

func (r *Resource) moduleUniqueKey() string {
	return r.Addr().String()
}

// Addr returns a resource address for the receiver that is relative to the
// resource's containing module.
func (r *Resource) Addr() addrs.Resource {
	return addrs.Resource{
		Mode: r.Mode,
		Type: r.Type,
		Name: r.Name,
	}
}

// TODO/Oleksandr: remove this function once merged with Ronny's changes.
func (r *Resource) AnyProviderConfigAddr() addrs.LocalProviderConfig {
	if r.ProviderConfigRef == nil {
		// If no specific "provider" argument is given, we want to look up the
		// provider config where the local name matches the implied provider
		// from the resource type. This may be different from the resource's
		// provider type.
		return addrs.LocalProviderConfig{
			LocalName: r.Addr().ImpliedProvider(),
		}
	}

	if r.ProviderConfigRef.HasInstanceRefsInAlias() {
		// This branch must return the same (first) value every time.
		// It is used in multiple places before and after graph execution.
		// Anyway, this function only mimics real behaviour and should be removed.
		aliases := make([]string, 0, len(r.ProviderConfigRef.Aliases))
		for _, alias := range r.ProviderConfigRef.Aliases {
			aliases = append(aliases, alias)
		}
		slices.Sort(aliases)
		return addrs.LocalProviderConfig{
			LocalName: r.ProviderConfigRef.Name,
			Alias:     aliases[0],
		}
	}

	return addrs.LocalProviderConfig{
		LocalName: r.ProviderConfigRef.Name,
		Alias:     r.ProviderConfigRef.Aliases[addrs.NoKey],
	}
}

// ProviderConfigName returns configuration name of resource provider without an alias.
func (r *Resource) ProviderConfigName() string {
	if r.ProviderConfigRef == nil {
		// If no specific "provider" argument is given, we want to look up the
		// provider config where the local name matches the implied provider
		// from the resource type. This may be different from the resource's
		// provider type.
		return r.Addr().ImpliedProvider()
	}

	return r.ProviderConfigRef.Name
}

// HasCustomConditions returns true if and only if the resource has at least
// one author-specified custom condition.
func (r *Resource) HasCustomConditions() bool {
	return len(r.Postconditions) != 0 || len(r.Preconditions) != 0
}

func (r *Resource) decodeStaticFields(eval *StaticEvaluator) hcl.Diagnostics {
	if r.ProviderConfigRef != nil {
		return r.ProviderConfigRef.decodeStaticAlias(eval, r.Count, r.ForEach)
	}
	return nil
}

func decodeResourceBlock(block *hcl.Block, override bool) (*Resource, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	r := &Resource{
		Mode:      addrs.ManagedResourceMode,
		Type:      block.Labels[0],
		Name:      block.Labels[1],
		DeclRange: block.DefRange,
		TypeRange: block.LabelRanges[0],
		Managed:   &ManagedResource{},
	}

	content, remain, moreDiags := block.Body.PartialContent(ResourceBlockSchema)
	diags = append(diags, moreDiags...)
	r.Config = remain

	if !hclsyntax.ValidIdentifier(r.Type) {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid resource type name",
			Detail:   badIdentifierDetail,
			Subject:  &block.LabelRanges[0],
		})
	}
	if !hclsyntax.ValidIdentifier(r.Name) {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid resource name",
			Detail:   badIdentifierDetail,
			Subject:  &block.LabelRanges[1],
		})
	}

	if attr, exists := content.Attributes["count"]; exists {
		r.Count = attr.Expr
	}

	if attr, exists := content.Attributes["for_each"]; exists {
		r.ForEach = attr.Expr
		// Cannot have count and for_each on the same resource block
		if r.Count != nil {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  `Invalid combination of "count" and "for_each"`,
				Detail:   `The "count" and "for_each" meta-arguments are mutually-exclusive, only one should be used to be explicit about the number of resources to be created.`,
				Subject:  &attr.NameRange,
			})
		}
	}

	if attr, exists := content.Attributes["provider"]; exists {
		var providerDiags hcl.Diagnostics
		r.ProviderConfigRef, providerDiags = decodeProviderConfigRefMapping(attr.Expr, "provider")
		diags = append(diags, providerDiags...)
	}

	if attr, exists := content.Attributes["depends_on"]; exists {
		deps, depsDiags := decodeDependsOn(attr)
		diags = append(diags, depsDiags...)
		r.DependsOn = append(r.DependsOn, deps...)
	}

	var seenLifecycle *hcl.Block
	var seenConnection *hcl.Block
	var seenEscapeBlock *hcl.Block
	for _, block := range content.Blocks {
		switch block.Type {
		case "lifecycle":
			if seenLifecycle != nil {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate lifecycle block",
					Detail:   fmt.Sprintf("This resource already has a lifecycle block at %s.", seenLifecycle.DefRange),
					Subject:  &block.DefRange,
				})
				continue
			}
			seenLifecycle = block

			lcContent, lcDiags := block.Body.Content(resourceLifecycleBlockSchema)
			diags = append(diags, lcDiags...)

			if attr, exists := lcContent.Attributes["create_before_destroy"]; exists {
				valDiags := gohcl.DecodeExpression(attr.Expr, nil, &r.Managed.CreateBeforeDestroy)
				diags = append(diags, valDiags...)
				r.Managed.CreateBeforeDestroySet = true
			}

			if attr, exists := lcContent.Attributes["prevent_destroy"]; exists {
				valDiags := gohcl.DecodeExpression(attr.Expr, nil, &r.Managed.PreventDestroy)
				diags = append(diags, valDiags...)
				r.Managed.PreventDestroySet = true
			}

			if attr, exists := lcContent.Attributes["replace_triggered_by"]; exists {
				exprs, hclDiags := decodeReplaceTriggeredBy(attr.Expr)
				diags = diags.Extend(hclDiags)

				r.TriggersReplacement = append(r.TriggersReplacement, exprs...)
			}

			if attr, exists := lcContent.Attributes["ignore_changes"]; exists {

				// ignore_changes can either be a list of relative traversals
				// or it can be just the keyword "all" to ignore changes to this
				// resource entirely.
				//   ignore_changes = [ami, instance_type]
				//   ignore_changes = all
				// We also allow two legacy forms for compatibility with earlier
				// versions:
				//   ignore_changes = ["ami", "instance_type"]
				//   ignore_changes = ["*"]

				kw := hcl.ExprAsKeyword(attr.Expr)

				switch {
				case kw == "all":
					r.Managed.IgnoreAllChanges = true
				default:
					exprs, listDiags := hcl.ExprList(attr.Expr)
					diags = append(diags, listDiags...)

					var ignoreAllRange hcl.Range

					for _, expr := range exprs {

						// our expr might be the literal string "*", which
						// we accept as a deprecated way of saying "all".
						if shimIsIgnoreChangesStar(expr) {
							r.Managed.IgnoreAllChanges = true
							ignoreAllRange = expr.Range()
							diags = append(diags, &hcl.Diagnostic{
								Severity: hcl.DiagError,
								Summary:  "Invalid ignore_changes wildcard",
								Detail:   "The [\"*\"] form of ignore_changes wildcard is was deprecated and is now invalid. Use \"ignore_changes = all\" to ignore changes to all attributes.",
								Subject:  attr.Expr.Range().Ptr(),
							})
							continue
						}

						expr, shimDiags := shimTraversalInString(expr, false)
						diags = append(diags, shimDiags...)

						traversal, travDiags := hcl.RelTraversalForExpr(expr)
						diags = append(diags, travDiags...)
						if len(traversal) != 0 {
							r.Managed.IgnoreChanges = append(r.Managed.IgnoreChanges, traversal)
						}
					}

					if r.Managed.IgnoreAllChanges && len(r.Managed.IgnoreChanges) != 0 {
						diags = append(diags, &hcl.Diagnostic{
							Severity: hcl.DiagError,
							Summary:  "Invalid ignore_changes ruleset",
							Detail:   "Cannot mix wildcard string \"*\" with non-wildcard references.",
							Subject:  &ignoreAllRange,
							Context:  attr.Expr.Range().Ptr(),
						})
					}

				}
			}

			for _, block := range lcContent.Blocks {
				switch block.Type {
				case "precondition", "postcondition":
					cr, moreDiags := decodeCheckRuleBlock(block, override)
					diags = append(diags, moreDiags...)

					moreDiags = cr.validateSelfReferences(block.Type, r.Addr())
					diags = append(diags, moreDiags...)

					switch block.Type {
					case "precondition":
						r.Preconditions = append(r.Preconditions, cr)
					case "postcondition":
						r.Postconditions = append(r.Postconditions, cr)
					}
				default:
					// The cases above should be exhaustive for all block types
					// defined in the lifecycle schema, so this shouldn't happen.
					panic(fmt.Sprintf("unexpected lifecycle sub-block type %q", block.Type))
				}
			}

		case "connection":
			if seenConnection != nil {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate connection block",
					Detail:   fmt.Sprintf("This resource already has a connection block at %s.", seenConnection.DefRange),
					Subject:  &block.DefRange,
				})
				continue
			}
			seenConnection = block

			r.Managed.Connection = &Connection{
				Config:    block.Body,
				DeclRange: block.DefRange,
			}

		case "provisioner":
			pv, pvDiags := decodeProvisionerBlock(block)
			diags = append(diags, pvDiags...)
			if pv != nil {
				r.Managed.Provisioners = append(r.Managed.Provisioners, pv)
			}

		case "_":
			if seenEscapeBlock != nil {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate escaping block",
					Detail: fmt.Sprintf(
						"The special block type \"_\" can be used to force particular arguments to be interpreted as resource-type-specific rather than as meta-arguments, but each resource block can have only one such block. The first escaping block was at %s.",
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
			r.Config = hcl.MergeBodies([]hcl.Body{r.Config, block.Body})

		default:
			// Any other block types are ones we've reserved for future use,
			// so they get a generic message.
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Reserved block type name in resource block",
				Detail:   fmt.Sprintf("The block type name %q is reserved for use by OpenTofu in a future version.", block.Type),
				Subject:  &block.TypeRange,
			})
		}
	}

	// Now we can validate the connection block references if there are any destroy provisioners.
	// TODO: should we eliminate standalone connection blocks?
	if r.Managed.Connection != nil {
		for _, p := range r.Managed.Provisioners {
			if p.When == ProvisionerWhenDestroy {
				diags = append(diags, onlySelfRefs(r.Managed.Connection.Config)...)
				break
			}
		}
	}

	return r, diags
}

func decodeDataBlock(block *hcl.Block, override, nested bool) (*Resource, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	r := &Resource{
		Mode:      addrs.DataResourceMode,
		Type:      block.Labels[0],
		Name:      block.Labels[1],
		DeclRange: block.DefRange,
		TypeRange: block.LabelRanges[0],
	}

	content, remain, moreDiags := block.Body.PartialContent(dataBlockSchema)
	diags = append(diags, moreDiags...)
	r.Config = remain

	if !hclsyntax.ValidIdentifier(r.Type) {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid data source name",
			Detail:   badIdentifierDetail,
			Subject:  &block.LabelRanges[0],
		})
	}
	if !hclsyntax.ValidIdentifier(r.Name) {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid data resource name",
			Detail:   badIdentifierDetail,
			Subject:  &block.LabelRanges[1],
		})
	}

	if attr, exists := content.Attributes["count"]; exists && !nested {
		r.Count = attr.Expr
	} else if exists && nested {
		// We don't allow count attributes in nested data blocks.
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  `Invalid "count" attribute`,
			Detail:   `The "count" and "for_each" meta-arguments are not supported within nested data blocks.`,
			Subject:  &attr.NameRange,
		})
	}

	if attr, exists := content.Attributes["for_each"]; exists && !nested {
		r.ForEach = attr.Expr
		// Cannot have count and for_each on the same data block
		if r.Count != nil {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  `Invalid combination of "count" and "for_each"`,
				Detail:   `The "count" and "for_each" meta-arguments are mutually-exclusive, only one should be used to be explicit about the number of resources to be created.`,
				Subject:  &attr.NameRange,
			})
		}
	} else if exists && nested {
		// We don't allow for_each attributes in nested data blocks.
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  `Invalid "for_each" attribute`,
			Detail:   `The "count" and "for_each" meta-arguments are not supported within nested data blocks.`,
			Subject:  &attr.NameRange,
		})
	}

	if attr, exists := content.Attributes["provider"]; exists {
		var providerDiags hcl.Diagnostics
		r.ProviderConfigRef, providerDiags = decodeProviderConfigRefMapping(attr.Expr, "provider")
		diags = append(diags, providerDiags...)
	}

	if attr, exists := content.Attributes["depends_on"]; exists {
		deps, depsDiags := decodeDependsOn(attr)
		diags = append(diags, depsDiags...)
		r.DependsOn = append(r.DependsOn, deps...)
	}

	var seenEscapeBlock *hcl.Block
	var seenLifecycle *hcl.Block
	for _, block := range content.Blocks {
		switch block.Type {

		case "_":
			if seenEscapeBlock != nil {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate escaping block",
					Detail: fmt.Sprintf(
						"The special block type \"_\" can be used to force particular arguments to be interpreted as resource-type-specific rather than as meta-arguments, but each data block can have only one such block. The first escaping block was at %s.",
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
			r.Config = hcl.MergeBodies([]hcl.Body{r.Config, block.Body})

		case "lifecycle":
			if nested {
				// We don't allow lifecycle arguments in nested data blocks,
				// the lifecycle is managed by the parent block.
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid lifecycle block",
					Detail:   `Nested data blocks do not support "lifecycle" blocks as the lifecycle is managed by the containing block.`,
					Subject:  block.DefRange.Ptr(),
				})
			}

			if seenLifecycle != nil {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate lifecycle block",
					Detail:   fmt.Sprintf("This resource already has a lifecycle block at %s.", seenLifecycle.DefRange),
					Subject:  block.DefRange.Ptr(),
				})
				continue
			}
			seenLifecycle = block

			lcContent, lcDiags := block.Body.Content(resourceLifecycleBlockSchema)
			diags = append(diags, lcDiags...)

			// All of the attributes defined for resource lifecycle are for
			// managed resources only, so we can emit a common error message
			// for any given attributes that HCL accepted.
			for name, attr := range lcContent.Attributes {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid data resource lifecycle argument",
					Detail:   fmt.Sprintf("The lifecycle argument %q is defined only for managed resources (\"resource\" blocks), and is not valid for data resources.", name),
					Subject:  attr.NameRange.Ptr(),
				})
			}

			for _, block := range lcContent.Blocks {
				switch block.Type {
				case "precondition", "postcondition":
					cr, moreDiags := decodeCheckRuleBlock(block, override)
					diags = append(diags, moreDiags...)

					moreDiags = cr.validateSelfReferences(block.Type, r.Addr())
					diags = append(diags, moreDiags...)

					switch block.Type {
					case "precondition":
						r.Preconditions = append(r.Preconditions, cr)
					case "postcondition":
						r.Postconditions = append(r.Postconditions, cr)
					}
				default:
					// The cases above should be exhaustive for all block types
					// defined in the lifecycle schema, so this shouldn't happen.
					panic(fmt.Sprintf("unexpected lifecycle sub-block type %q", block.Type))
				}
			}

		default:
			// Any other block types are ones we're reserving for future use,
			// but don't have any defined meaning today.
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Reserved block type name in data block",
				Detail:   fmt.Sprintf("The block type name %q is reserved for use by OpenTofu in a future version.", block.Type),
				Subject:  block.TypeRange.Ptr(),
			})
		}
	}

	return r, diags
}

// decodeReplaceTriggeredBy decodes and does basic validation of the
// replace_triggered_by expressions, ensuring they only contains references to
// a single resource, and the only extra variables are count.index or each.key.
func decodeReplaceTriggeredBy(expr hcl.Expression) ([]hcl.Expression, hcl.Diagnostics) {
	// Since we are manually parsing the replace_triggered_by argument, we
	// need to specially handle json configs, in which case the values will
	// be json strings rather than hcl. To simplify parsing however we will
	// decode the individual list elements, rather than the entire expression.
	isJSON := hcljson.IsJSONExpression(expr)

	exprs, diags := hcl.ExprList(expr)

	for i, expr := range exprs {
		if isJSON {
			var convertDiags hcl.Diagnostics
			expr, convertDiags = hcl2shim.ConvertJSONExpressionToHCL(expr)
			diags = diags.Extend(convertDiags)
			if diags.HasErrors() {
				continue
			}
			// make sure to swap out the expression we're returning too
			exprs[i] = expr
		}

		refs, refDiags := lang.ReferencesInExpr(addrs.ParseRef, expr)
		for _, diag := range refDiags {
			severity := hcl.DiagError
			if diag.Severity() == tfdiags.Warning {
				severity = hcl.DiagWarning
			}

			desc := diag.Description()

			diags = append(diags, &hcl.Diagnostic{
				Severity: severity,
				Summary:  desc.Summary,
				Detail:   desc.Detail,
				Subject:  expr.Range().Ptr(),
			})
		}

		if refDiags.HasErrors() {
			continue
		}

		resourceCount := 0
		for _, ref := range refs {
			switch sub := ref.Subject.(type) {
			case addrs.Resource, addrs.ResourceInstance:
				resourceCount++

			case addrs.ForEachAttr:
				if sub.Name != "key" {
					diags = append(diags, &hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Invalid each reference in replace_triggered_by expression",
						Detail:   "Only each.key may be used in replace_triggered_by.",
						Subject:  expr.Range().Ptr(),
					})
				}
			case addrs.CountAttr:
				if sub.Name != "index" {
					diags = append(diags, &hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Invalid count reference in replace_triggered_by expression",
						Detail:   "Only count.index may be used in replace_triggered_by.",
						Subject:  expr.Range().Ptr(),
					})
				}
			default:
				// everything else should be simple traversals
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid reference in replace_triggered_by expression",
					Detail:   "Only resources, count.index, and each.key may be used in replace_triggered_by.",
					Subject:  expr.Range().Ptr(),
				})
			}
		}

		switch {
		case resourceCount == 0:
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid replace_triggered_by expression",
				Detail:   "Missing resource reference in replace_triggered_by expression.",
				Subject:  expr.Range().Ptr(),
			})
		case resourceCount > 1:
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid replace_triggered_by expression",
				Detail:   "Multiple resource references in replace_triggered_by expression.",
				Subject:  expr.Range().Ptr(),
			})
		}
	}
	return exprs, diags
}

type ProviderConfigRef struct {
	Name      string
	NameRange hcl.Range
	Alias     string

	// TODO: this may not be set in some cases, so it is not yet suitable for
	// use outside of this package. We currently only use it for internal
	// validation, but once we verify that this can be set in all cases, we can
	// export this so providers don't need to be re-resolved.
	// This same field is also added to the Provider struct.
	providerType addrs.Provider
}

// ProviderConfigRefMapping represents an extended version of ProviderConfigRef,
// that supports a scope of Aliases per instance instead of a single one.
type ProviderConfigRefMapping struct {
	Name      string
	NameRange hcl.Range

	Alias   hcl.Expression
	Aliases map[addrs.InstanceKey]string

	// TODO: this may not be set in some cases, so it is not yet suitable for
	// use outside of this package. We currently only use it for internal
	// validation, but once we verify that this can be set in all cases, we can
	// export this so providers don't need to be re-resolved.
	// This same field is also added to the Provider struct.
	providerType addrs.Provider
}

// HasAlias returns true if the provider is referenced by alias. This function
// has multiple use-cases and in some of them ProviderConfigRefMapping may be nil.
// In this case, we treat absent ProviderConfigRefMapping as such that doesn't have
// an alias.
func (m *ProviderConfigRefMapping) HasAlias() bool {
	return m != nil && len(m.Aliases) != 0
}

// HasInstanceRefsInAlias returns true if provider is referenced
// via instance dependend (e.g. `each`) keywords. Note, HasInstanceRefsInAlias
// returns false even if there is a for_each set in module call / resource / data,
// but the provider alias doesn't refer to those instances (i.e. doesn't use `each`).
func (m *ProviderConfigRefMapping) HasInstanceRefsInAlias() bool {
	// There is no alias so it has no instance refs.
	if len(m.Aliases) == 0 {
		return false
	}

	// There is multiple entries, so it must be multiple instance keys.
	if len(m.Aliases) != 1 {
		return true
	}

	// There is a single entry and this entry is NoKey so
	// it has no references to actual instances.
	if _, ok := m.Aliases[addrs.NoKey]; ok {
		return false
	}

	// There is a single entry and this entry references some key.
	return true
}

func providerToConfigRefMapping(p *Provider) *ProviderConfigRefMapping {
	m := &ProviderConfigRefMapping{
		Name:         p.Name,
		NameRange:    p.NameRange,
		providerType: p.providerType,
	}

	if p.Alias == "" {
		return m
	}

	m.Aliases = map[addrs.InstanceKey]string{
		addrs.NoKey: p.Alias,
	}

	return m
}

func (m *ProviderConfigRefMapping) getForEachValues(eval *StaticEvaluator, forEachExpr hcl.Expression) (map[addrs.InstanceKey]map[string]cty.Value, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	forEachID := StaticIdentifier{
		Module:    eval.call.addr,
		Subject:   "for_each", // TODO better message here
		DeclRange: forEachExpr.Range(),
	}
	forEachVals, forEachDiags := evalchecks.EvaluateForEachExpression(forEachExpr, func(refs []*addrs.Reference) (*hcl.EvalContext, tfdiags.Diagnostics) {
		evalCtx, evalDiags := eval.EvalContext(forEachID, refs)
		return evalCtx, tfdiags.Diagnostics{}.Append(evalDiags)
	})
	diags = diags.Extend(forEachDiags.ToHCL())
	if forEachDiags.HasErrors() {
		return nil, diags
	}

	result := make(map[addrs.InstanceKey]map[string]cty.Value)
	for k, v := range forEachVals {
		result[addrs.StringKey(k)] = map[string]cty.Value{
			"each": cty.ObjectVal(map[string]cty.Value{
				"key":   cty.StringVal(k),
				"value": v,
			}),
		}
	}

	return result, diags
}

func (m *ProviderConfigRefMapping) getCountValues(eval *StaticEvaluator, countExpr hcl.Expression) (map[addrs.InstanceKey]map[string]cty.Value, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	countID := StaticIdentifier{
		Module:    eval.call.addr,
		Subject:   "count", // TODO better message here
		DeclRange: countExpr.Range(),
	}

	lastIndex, countDiags := evalchecks.EvaluateCountExpression(countExpr, func(expr hcl.Expression) (cty.Value, tfdiags.Diagnostics) {
		v, evalDiags := eval.Evaluate(expr, countID)
		return v, tfdiags.Diagnostics{}.Append(evalDiags)
	})

	diags = diags.Extend(countDiags.ToHCL())
	if countDiags.HasErrors() {
		return nil, diags
	}
	result := make(map[addrs.InstanceKey]map[string]cty.Value)
	for i := 0; i < lastIndex; i++ {
		result[addrs.IntKey(i)] = map[string]cty.Value{
			"count": cty.ObjectVal(map[string]cty.Value{
				"index": cty.NumberVal(big.NewFloat(float64(i))),
			}),
		}
	}
	return result, diags
}

type nonInstanceRefs struct {
	filtered    []*addrs.Reference
	hasEachRef  bool
	hasCountRef bool
}

func filterNonInstanceRefs(refs []*addrs.Reference) nonInstanceRefs {
	var s nonInstanceRefs

	for _, ref := range refs {
		switch ref.Subject.(type) {
		case addrs.ForEachAttr:
			s.hasEachRef = true
		case addrs.CountAttr:
			s.hasCountRef = true
		default:
			s.filtered = append(s.filtered, ref)
		}
	}

	return s
}

// decodeStaticAlias decodes alias using static evaluation with count or for_each from instanceExpr.
func (m *ProviderConfigRefMapping) decodeStaticAlias(eval *StaticEvaluator, countExpr, forEachExpr hcl.Expression) hcl.Diagnostics {
	if m.Alias == nil {
		return nil
	}

	var diags hcl.Diagnostics

	unfilteredRefs, refDiags := lang.ReferencesInExpr(addrs.ParseRef, m.Alias)
	diags = diags.Extend(refDiags.ToHCL())
	if refDiags.HasErrors() {
		return diags
	}

	// We don't want to try evaluate 'each' / 'count' references so
	// we filter them out to process separately via child evaluation
	// contexts.
	refInfo := filterNonInstanceRefs(unfilteredRefs)

	if refInfo.hasEachRef && refInfo.hasCountRef {
		return diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid provider alias reference",
			Detail:   "Alias expression cannot reference both 'count' and 'each'.",
			Subject:  m.Alias.Range().Ptr(),
		})
	}

	if refInfo.hasCountRef && countExpr == nil {
		didYouMean := ""
		if forEachExpr != nil {
			didYouMean = " Did you mean to use 'each' instead?"
		}

		return diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid provider alias reference",
			Detail:   fmt.Sprintf("Alias expression references 'count' without the declaration.%v", didYouMean),
			Subject:  m.Alias.Range().Ptr(),
		})
	}

	if refInfo.hasEachRef && forEachExpr == nil {
		didYouMean := ""
		if countExpr != nil {
			didYouMean = " Did you mean to use 'count' instead?"
		}

		return diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid provider alias reference",
			Detail:   fmt.Sprintf("Alias expression references 'each' without the 'for_each' declaration.%v", didYouMean),
			Subject:  m.Alias.Range().Ptr(),
		})
	}

	var instanceVariableMap map[addrs.InstanceKey]map[string]cty.Value
	var instanceDiags hcl.Diagnostics

	switch {
	case refInfo.hasCountRef:
		instanceVariableMap, instanceDiags = m.getCountValues(eval, countExpr)
	case refInfo.hasEachRef:
		instanceVariableMap, instanceDiags = m.getForEachValues(eval, forEachExpr)
	default:
		instanceVariableMap = map[addrs.InstanceKey]map[string]cty.Value{addrs.NoKey: nil}
	}

	diags = diags.Extend(instanceDiags)
	if instanceDiags.HasErrors() {
		return diags
	}

	evalCtx, evalCtxDiags := eval.EvalContext(StaticIdentifier{
		Module:    eval.call.addr,
		Subject:   "providers.alias",
		DeclRange: m.Alias.Range(),
	}, refInfo.filtered)
	diags = diags.Extend(evalCtxDiags)
	if evalCtxDiags.HasErrors() {
		return diags
	}
	for k, v := range instanceVariableMap {
		instanceEvalCtx := evalCtx.NewChild()
		instanceEvalCtx.Variables = v

		var alias string
		aliasDiags := gohcl.DecodeExpression(m.Alias, instanceEvalCtx, &alias)
		diags = diags.Extend(aliasDiags)
		if aliasDiags.HasErrors() {
			continue
		}

		m.Aliases[k] = alias
	}

	return diags
}

func decodeIndexProviderConfigRefMapping(expr *hclsyntax.IndexExpr, argName string) (*ProviderConfigRefMapping, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	name, nameDiags := hcl.AbsTraversalForExpr(expr.Collection)
	diags = append(diags, nameDiags...)
	if diags.HasErrors() {
		return nil, diags
	}

	if len(name) != 1 {
		return nil, append(diags, invalidProviderReferenceDiag(argName, expr.Range().Ptr()))
	}

	root, ok := name[0].(hcl.TraverseRoot)
	if !ok {
		return nil, append(diags, invalidProviderReferenceDiag(argName, expr.Range().Ptr()))
	}

	return &ProviderConfigRefMapping{
		Name:      root.Name,
		NameRange: expr.Collection.Range(),
		Alias:     expr.Key,
	}, diags
}

func decodeProviderConfigRefMapping(expr hcl.Expression, argName string) (*ProviderConfigRefMapping, hcl.Diagnostics) {
	if expr, ok := expr.(*hclsyntax.IndexExpr); ok {
		return decodeIndexProviderConfigRefMapping(expr, argName)
	}

	ref, diags := decodeProviderConfigRef(expr, argName)
	if diags.HasErrors() {
		return nil, diags
	}

	m := &ProviderConfigRefMapping{
		Name:         ref.Name,
		NameRange:    ref.NameRange,
		providerType: ref.providerType,
	}

	if ref.Alias != "" {
		m.Aliases = map[addrs.InstanceKey]string{
			addrs.NoKey: ref.Alias,
		}
	}

	return m, diags
}

func decodeProviderConfigRef(expr hcl.Expression, argName string) (*ProviderConfigRef, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	var shimDiags hcl.Diagnostics
	expr, shimDiags = shimTraversalInString(expr, false)
	diags = append(diags, shimDiags...)

	traversal, travDiags := hcl.AbsTraversalForExpr(expr)

	// AbsTraversalForExpr produces only generic errors, so we'll discard
	// the errors given and produce our own with extra context. If we didn't
	// get any errors then we might still have warnings, though.
	if !travDiags.HasErrors() {
		diags = append(diags, travDiags...)
	}

	if len(traversal) < 1 || len(traversal) > 2 {
		// A provider reference was given as a string literal in the legacy
		// configuration language and there are lots of examples out there
		// showing that usage, so we'll sniff for that situation here and
		// produce a specialized error message for it to help users find
		// the new correct form.
		if exprIsNativeQuotedString(expr) {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid provider configuration reference",
				Detail:   "A provider configuration reference must not be given in quotes.",
				Subject:  expr.Range().Ptr(),
			})
			return nil, diags
		}

		diags = append(diags, invalidProviderReferenceDiag(argName, expr.Range().Ptr()))
		return nil, diags
	}

	// verify that the provider local name is normalized
	name := traversal.RootName()
	nameDiags := checkProviderNameNormalized(name, traversal[0].SourceRange())
	diags = append(diags, nameDiags...)
	if diags.HasErrors() {
		return nil, diags
	}

	ret := &ProviderConfigRef{
		Name:      name,
		NameRange: traversal[0].SourceRange(),
	}

	if len(traversal) > 1 {
		switch aliasStep := traversal[1].(type) {
		case hcl.TraverseAttr:
			ret.Alias = aliasStep.Name
		case hcl.TraverseIndex:
			ret.Alias = aliasStep.Key.AsString()
		default:
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid provider configuration reference",
				Detail:   "Provider name must either stand alone or be followed by a period and then a configuration alias.",
				Subject:  traversal[1].SourceRange().Ptr(),
			})
			return ret, diags
		}
	}

	return ret, diags
}

// Addr returns the provider config address corresponding to the receiving
// config reference.
//
// This is a trivial conversion, essentially just discarding the source
// location information and keeping just the addressing information.
func (r *ProviderConfigRef) Addr() addrs.LocalProviderConfig {
	return addrs.LocalProviderConfig{
		LocalName: r.Name,
		Alias:     r.Alias,
	}
}

func (r *ProviderConfigRef) String() string {
	if r == nil {
		return "<nil>"
	}
	if r.Alias != "" {
		return fmt.Sprintf("%s.%s", r.Name, r.Alias)
	}
	return r.Name
}

func invalidProviderReferenceDiag(argName string, sub *hcl.Range) *hcl.Diagnostic {
	return &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Invalid provider configuration reference",
		Detail:   fmt.Sprintf("The %s argument requires a provider type name with an optional alias specified after a dot ('provider.alias') or in square brackets ('provider[\"alias\"]').", argName),
		Subject:  sub,
	}
}

var commonResourceAttributes = []hcl.AttributeSchema{
	{
		Name: "count",
	},
	{
		Name: "for_each",
	},
	{
		Name: "provider",
	},
	{
		Name: "depends_on",
	},
}

// ResourceBlockSchema is the schema for a resource or data resource type within
// OpenTofu.
//
// This schema is public as it is required elsewhere in order to validate and
// use generated config.
var ResourceBlockSchema = &hcl.BodySchema{
	Attributes: commonResourceAttributes,
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "locals"}, // reserved for future use
		{Type: "lifecycle"},
		{Type: "connection"},
		{Type: "provisioner", LabelNames: []string{"type"}},
		{Type: "_"}, // meta-argument escaping block
	},
}

var dataBlockSchema = &hcl.BodySchema{
	Attributes: commonResourceAttributes,
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "lifecycle"},
		{Type: "locals"}, // reserved for future use
		{Type: "_"},      // meta-argument escaping block
	},
}

var resourceLifecycleBlockSchema = &hcl.BodySchema{
	// We tell HCL that these elements are all valid for both "resource"
	// and "data" lifecycle blocks, but the rules are actually more restrictive
	// than that. We deal with that after decoding so that we can return
	// more specific error messages than HCL would typically return itself.
	Attributes: []hcl.AttributeSchema{
		{
			Name: "create_before_destroy",
		},
		{
			Name: "prevent_destroy",
		},
		{
			Name: "ignore_changes",
		},
		{
			Name: "replace_triggered_by",
		},
	},
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "precondition"},
		{Type: "postcondition"},
	},
}
