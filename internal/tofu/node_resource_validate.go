// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/didyoumean"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/provisioners"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// NodeValidatableResource represents a resource that is used for validation
// only.
type NodeValidatableResource struct {
	*NodeAbstractResource
}

var (
	_ GraphNodeModuleInstance            = (*NodeValidatableResource)(nil)
	_ GraphNodeExecutable                = (*NodeValidatableResource)(nil)
	_ GraphNodeReferenceable             = (*NodeValidatableResource)(nil)
	_ GraphNodeReferencer                = (*NodeValidatableResource)(nil)
	_ GraphNodeConfigResource            = (*NodeValidatableResource)(nil)
	_ GraphNodeAttachResourceConfig      = (*NodeValidatableResource)(nil)
	_ GraphNodeAttachProviderMetaConfigs = (*NodeValidatableResource)(nil)
)

func (n *NodeValidatableResource) Path() addrs.ModuleInstance {
	// There is no expansion during validation, so we evaluate everything as
	// single module instances.
	return n.Addr.Module.UnkeyedInstanceShim()
}

// GraphNodeEvalable
func (n *NodeValidatableResource) Execute(ctx EvalContext, op walkOperation) (diags tfdiags.Diagnostics) {
	if n.Config == nil {
		return diags
	}

	diags = diags.Append(n.validateResource(ctx))

	diags = diags.Append(n.validateCheckRules(ctx, n.Config))

	if managed := n.Config.Managed; managed != nil {
		// Validate all the provisioners
		for _, p := range managed.Provisioners {
			// Create a local shallow copy of the provisioner
			provisioner := *p

			if p.Connection == nil {
				provisioner.Connection = n.Config.Managed.Connection
			} else if n.Config.Managed.Connection != nil {
				// Merge the connection with n.Config.Managed.Connection, but only in
				// our local provisioner, as it will only be used by
				// "validateProvisioner"
				connection := &configs.Connection{}
				*connection = *p.Connection
				connection.Config = configs.MergeBodies(n.Config.Managed.Connection.Config, connection.Config)
				provisioner.Connection = connection
			}

			// Validate Provisioner Config
			diags = diags.Append(n.validateProvisioner(ctx, &provisioner))
			if diags.HasErrors() {
				return diags
			}
		}
	}
	return diags
}

// validateProvisioner validates the configuration of a provisioner belonging to
// a resource. The provisioner config is expected to contain the merged
// connection configurations.
func (n *NodeValidatableResource) validateProvisioner(ctx EvalContext, p *configs.Provisioner) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	provisioner, err := ctx.Provisioner(p.Type)
	if err != nil {
		diags = diags.Append(err)
		return diags
	}

	if provisioner == nil {
		return diags.Append(fmt.Errorf("provisioner %s not initialized", p.Type))
	}
	provisionerSchema, err := ctx.ProvisionerSchema(p.Type)
	if err != nil {
		return diags.Append(fmt.Errorf("failed to read schema for provisioner %s: %w", p.Type, err))
	}
	if provisionerSchema == nil {
		return diags.Append(fmt.Errorf("provisioner %s has no schema", p.Type))
	}

	// Validate the provisioner's own config first
	configVal, _, configDiags := n.evaluateBlock(ctx, p.Config, provisionerSchema)
	diags = diags.Append(configDiags)

	if configVal == cty.NilVal {
		// Should never happen for a well-behaved EvaluateBlock implementation
		return diags.Append(fmt.Errorf("EvaluateBlock returned nil value"))
	}

	// Use unmarked value for validate request
	unmarkedConfigVal, _ := configVal.UnmarkDeep()
	req := provisioners.ValidateProvisionerConfigRequest{
		Config: unmarkedConfigVal,
	}

	resp := provisioner.ValidateProvisionerConfig(req)
	diags = diags.Append(resp.Diagnostics)

	if p.Connection != nil {
		// We can't comprehensively validate the connection config since its
		// final structure is decided by the communicator and we can't instantiate
		// that until we have a complete instance state. However, we *can* catch
		// configuration keys that are not valid for *any* communicator, catching
		// typos early rather than waiting until we actually try to run one of
		// the resource's provisioners.
		_, _, connDiags := n.evaluateBlock(ctx, p.Connection.Config, connectionBlockSupersetSchema)
		diags = diags.Append(connDiags)
	}
	return diags
}

func (n *NodeValidatableResource) evaluateBlock(ctx EvalContext, body hcl.Body, schema *configschema.Block) (cty.Value, hcl.Body, tfdiags.Diagnostics) {
	keyData, selfAddr := n.stubRepetitionData(n.Config.Count != nil, n.Config.ForEach != nil)

	return ctx.EvaluateBlock(body, schema, selfAddr, keyData)
}

// connectionBlockSupersetSchema is a schema representing the superset of all
// possible arguments for "connection" blocks across all supported connection
// types.
//
// This currently lives here because we've not yet updated our communicator
// subsystem to be aware of schema itself. Once that is done, we can remove
// this and use a type-specific schema from the communicator to validate
// exactly what is expected for a given connection type.
var connectionBlockSupersetSchema = &configschema.Block{
	Attributes: map[string]*configschema.Attribute{
		// NOTE: "type" is not included here because it's treated special
		// by the config loader and stored away in a separate field.

		// Common attributes for both connection types
		"host": {
			Type:     cty.String,
			Required: true,
		},
		"type": {
			Type:     cty.String,
			Optional: true,
		},
		"user": {
			Type:     cty.String,
			Optional: true,
		},
		"password": {
			Type:     cty.String,
			Optional: true,
		},
		"port": {
			Type:     cty.Number,
			Optional: true,
		},
		"timeout": {
			Type:     cty.String,
			Optional: true,
		},
		"script_path": {
			Type:     cty.String,
			Optional: true,
		},
		// For type=ssh only (enforced in ssh communicator)
		"target_platform": {
			Type:     cty.String,
			Optional: true,
		},
		"private_key": {
			Type:     cty.String,
			Optional: true,
		},
		"certificate": {
			Type:     cty.String,
			Optional: true,
		},
		"host_key": {
			Type:     cty.String,
			Optional: true,
		},
		"agent": {
			Type:     cty.Bool,
			Optional: true,
		},
		"agent_identity": {
			Type:     cty.String,
			Optional: true,
		},
		"proxy_scheme": {
			Type:     cty.String,
			Optional: true,
		},
		"proxy_host": {
			Type:     cty.String,
			Optional: true,
		},
		"proxy_port": {
			Type:     cty.Number,
			Optional: true,
		},
		"proxy_user_name": {
			Type:     cty.String,
			Optional: true,
		},
		"proxy_user_password": {
			Type:     cty.String,
			Optional: true,
		},
		"bastion_host": {
			Type:     cty.String,
			Optional: true,
		},
		"bastion_host_key": {
			Type:     cty.String,
			Optional: true,
		},
		"bastion_port": {
			Type:     cty.Number,
			Optional: true,
		},
		"bastion_user": {
			Type:     cty.String,
			Optional: true,
		},
		"bastion_password": {
			Type:     cty.String,
			Optional: true,
		},
		"bastion_private_key": {
			Type:     cty.String,
			Optional: true,
		},
		"bastion_certificate": {
			Type:     cty.String,
			Optional: true,
		},

		// For type=winrm only (enforced in winrm communicator)
		"https": {
			Type:     cty.Bool,
			Optional: true,
		},
		"insecure": {
			Type:     cty.Bool,
			Optional: true,
		},
		"cacert": {
			Type:     cty.String,
			Optional: true,
		},
		"use_ntlm": {
			Type:     cty.Bool,
			Optional: true,
		},
	},
}

func (n *NodeValidatableResource) validateResource(ctx EvalContext) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	provider, providerSchema, err := getProvider(ctx, n.ResolvedProvider.ProviderConfig, addrs.NoKey) // Provider Instance Keys are ignored during validate
	diags = diags.Append(err)
	if diags.HasErrors() {
		return diags
	}

	keyData := EvalDataForNoInstanceKey

	switch {
	case n.Config.Count != nil:
		// If the config block has count, we'll evaluate with an unknown
		// number as count.index so we can still type check even though
		// we won't expand count until the plan phase.
		keyData = InstanceKeyEvalData{
			CountIndex: cty.UnknownVal(cty.Number),
		}

		// Basic type-checking of the count argument. More complete validation
		// of this will happen when we DynamicExpand during the plan walk.
		countDiags := validateCount(ctx, n.Config.Count)
		diags = diags.Append(countDiags)

	case n.Config.ForEach != nil:
		keyData = InstanceKeyEvalData{
			EachKey:   cty.UnknownVal(cty.String),
			EachValue: cty.UnknownVal(cty.DynamicPseudoType),
		}

		// Evaluate the for_each expression here so we can expose the diagnostics
		forEachDiags := validateForEach(ctx, n.Config.ForEach)
		diags = diags.Append(forEachDiags)
	}

	diags = diags.Append(validateDependsOn(ctx, n.Config.DependsOn))

	// Validate the provider_meta block for the provider this resource
	// belongs to, if there is one.
	//
	// Note: this will return an error for every resource a provider
	// uses in a module, if the provider_meta for that module is
	// incorrect. The only way to solve this that we've found is to
	// insert a new ProviderMeta graph node in the graph, and make all
	// that provider's resources in the module depend on the node. That's
	// an awful heavy hammer to swing for this feature, which should be
	// used only in limited cases with heavy coordination with the
	// OpenTofu team, so we're going to defer that solution for a future
	// enhancement to this functionality.
	/*
		if n.ProviderMetas != nil {
			if m, ok := n.ProviderMetas[n.ProviderAddr.ProviderConfig.Type]; ok && m != nil {
				// if the provider doesn't support this feature, throw an error
				if (*n.ProviderSchema).ProviderMeta == nil {
					diags = diags.Append(&hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  fmt.Sprintf("Provider %s doesn't support provider_meta", cfg.ProviderConfigAddr()),
						Detail:   fmt.Sprintf("The resource %s belongs to a provider that doesn't support provider_meta blocks", n.Addr),
						Subject:  &m.ProviderRange,
					})
				} else {
					_, _, metaDiags := ctx.EvaluateBlock(m.Config, (*n.ProviderSchema).ProviderMeta, nil, EvalDataForNoInstanceKey)
					diags = diags.Append(metaDiags)
				}
			}
		}
	*/
	// BUG(paddy): we're not validating provider_meta blocks on EvalValidate right now
	// because the ProviderAddr for the resource isn't available on the EvalValidate
	// struct.

	// Provider entry point varies depending on resource mode, because
	// managed resources and data resources are two distinct concepts
	// in the provider abstraction.
	switch n.Config.Mode {
	case addrs.ManagedResourceMode:
		schema, _ := providerSchema.SchemaForResourceType(n.Config.Mode, n.Config.Type)
		if schema == nil {
			var suggestion string
			if dSchema, _ := providerSchema.SchemaForResourceType(addrs.DataResourceMode, n.Config.Type); dSchema != nil {
				suggestion = fmt.Sprintf("\n\nDid you intend to use the data source %q? If so, declare this using a \"data\" block instead of a \"resource\" block.", n.Config.Type)
			} else if len(providerSchema.ResourceTypes) > 0 {
				suggestions := make([]string, 0, len(providerSchema.ResourceTypes))
				for name := range providerSchema.ResourceTypes {
					suggestions = append(suggestions, name)
				}
				if suggestion = didyoumean.NameSuggestion(n.Config.Type, suggestions); suggestion != "" {
					suggestion = fmt.Sprintf(" Did you mean %q?", suggestion)
				}
			}

			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid resource type",
				Detail:   fmt.Sprintf("The provider %s does not support resource type %q.%s", n.Provider().ForDisplay(), n.Config.Type, suggestion),
				Subject:  &n.Config.TypeRange,
			})
			return diags
		}

		configVal, _, valDiags := ctx.EvaluateBlock(n.Config.Config, schema, nil, keyData)
		diags = diags.Append(valDiags)
		if valDiags.HasErrors() {
			return diags
		}

		if n.Config.Managed != nil { // can be nil only in tests with poorly-configured mocks
			for _, traversal := range n.Config.Managed.IgnoreChanges {
				// validate the ignore_changes traversals apply.
				moreDiags := schema.StaticValidateTraversal(traversal)
				diags = diags.Append(moreDiags)

				// ignore_changes cannot be used for Computed attributes,
				// unless they are also Optional.
				// If the traversal was valid, convert it to a cty.Path and
				// use that to check whether the Attribute is Computed and
				// non-Optional.
				if !diags.HasErrors() {
					path := traversalToPath(traversal)

					attrSchema := schema.AttributeByPath(path)

					if attrSchema != nil && !attrSchema.Optional && attrSchema.Computed {
						// ignore_changes uses absolute traversal syntax in config despite
						// using relative traversals, so we strip the leading "." added by
						// FormatCtyPath for a better error message.
						attrDisplayPath := strings.TrimPrefix(tfdiags.FormatCtyPath(path), ".")

						diags = diags.Append(&hcl.Diagnostic{
							Severity: hcl.DiagWarning,
							Summary:  "Redundant ignore_changes element",
							Detail:   fmt.Sprintf("Adding an attribute name to ignore_changes tells OpenTofu to ignore future changes to the argument in configuration after the object has been created, retaining the value originally configured.\n\nThe attribute %s is decided by the provider alone and therefore there can be no configured value to compare with. Including this attribute in ignore_changes has no effect. Remove the attribute from ignore_changes to quiet this warning.", attrDisplayPath),
							Subject:  &n.Config.TypeRange,
						})
					}
				}
			}
		}

		// Use unmarked value for validate request
		unmarkedConfigVal, _ := configVal.UnmarkDeep()
		req := providers.ValidateResourceConfigRequest{
			TypeName: n.Config.Type,
			Config:   unmarkedConfigVal,
		}

		resp := provider.ValidateResourceConfig(req)
		diags = diags.Append(resp.Diagnostics.InConfigBody(n.Config.Config, n.Addr.String()))

	case addrs.DataResourceMode:
		schema, _ := providerSchema.SchemaForResourceType(n.Config.Mode, n.Config.Type)
		if schema == nil {
			var suggestion string
			if dSchema, _ := providerSchema.SchemaForResourceType(addrs.ManagedResourceMode, n.Config.Type); dSchema != nil {
				suggestion = fmt.Sprintf("\n\nDid you intend to use the managed resource type %q? If so, declare this using a \"resource\" block instead of a \"data\" block.", n.Config.Type)
			} else if len(providerSchema.DataSources) > 0 {
				suggestions := make([]string, 0, len(providerSchema.DataSources))
				for name := range providerSchema.DataSources {
					suggestions = append(suggestions, name)
				}
				if suggestion = didyoumean.NameSuggestion(n.Config.Type, suggestions); suggestion != "" {
					suggestion = fmt.Sprintf(" Did you mean %q?", suggestion)
				}
			}

			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid data source",
				Detail:   fmt.Sprintf("The provider %s does not support data source %q.%s", n.Provider().ForDisplay(), n.Config.Type, suggestion),
				Subject:  &n.Config.TypeRange,
			})
			return diags
		}

		configVal, _, valDiags := ctx.EvaluateBlock(n.Config.Config, schema, nil, keyData)
		diags = diags.Append(valDiags)
		if valDiags.HasErrors() {
			return diags
		}

		// Use unmarked value for validate request
		unmarkedConfigVal, _ := configVal.UnmarkDeep()
		req := providers.ValidateDataResourceConfigRequest{
			TypeName: n.Config.Type,
			Config:   unmarkedConfigVal,
		}

		resp := provider.ValidateDataResourceConfig(req)
		diags = diags.Append(resp.Diagnostics.InConfigBody(n.Config.Config, n.Addr.String()))
	}

	return diags
}

func (n *NodeValidatableResource) evaluateExpr(ctx EvalContext, expr hcl.Expression, wantTy cty.Type, self addrs.Referenceable, keyData instances.RepetitionData) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	refs, refDiags := lang.ReferencesInExpr(addrs.ParseRef, expr)
	diags = diags.Append(refDiags)

	scope := ctx.EvaluationScope(self, nil, keyData)

	hclCtx, moreDiags := scope.EvalContext(refs)
	diags = diags.Append(moreDiags)

	result, hclDiags := expr.Value(hclCtx)
	diags = diags.Append(hclDiags)

	return result, diags
}

func (n *NodeValidatableResource) stubRepetitionData(hasCount, hasForEach bool) (instances.RepetitionData, addrs.Referenceable) {
	keyData := EvalDataForNoInstanceKey
	selfAddr := n.ResourceAddr().Resource.Instance(addrs.NoKey)

	if n.Config.Count != nil {
		// For a resource that has count, we allow count.index but don't
		// know at this stage what it will return.
		keyData = InstanceKeyEvalData{
			CountIndex: cty.UnknownVal(cty.Number),
		}

		// "self" can't point to an unknown key, but we'll force it to be
		// key 0 here, which should return an unknown value of the
		// expected type since none of these elements are known at this
		// point anyway.
		selfAddr = n.ResourceAddr().Resource.Instance(addrs.IntKey(0))
	} else if n.Config.ForEach != nil {
		// For a resource that has for_each, we allow each.value and each.key
		// but don't know at this stage what it will return.
		keyData = InstanceKeyEvalData{
			EachKey:   cty.UnknownVal(cty.String),
			EachValue: cty.DynamicVal,
		}

		// "self" can't point to an unknown key, but we'll force it to be
		// key "" here, which should return an unknown value of the
		// expected type since none of these elements are known at
		// this point anyway.
		selfAddr = n.ResourceAddr().Resource.Instance(addrs.StringKey(""))
	}

	return keyData, selfAddr
}

func (n *NodeValidatableResource) validateCheckRules(ctx EvalContext, config *configs.Resource) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	keyData, selfAddr := n.stubRepetitionData(n.Config.Count != nil, n.Config.ForEach != nil)

	for _, cr := range config.Preconditions {
		_, conditionDiags := n.evaluateExpr(ctx, cr.Condition, cty.Bool, nil, keyData)
		diags = diags.Append(conditionDiags)

		_, errorMessageDiags := n.evaluateExpr(ctx, cr.ErrorMessage, cty.Bool, nil, keyData)
		diags = diags.Append(errorMessageDiags)
	}

	for _, cr := range config.Postconditions {
		_, conditionDiags := n.evaluateExpr(ctx, cr.Condition, cty.Bool, selfAddr, keyData)
		diags = diags.Append(conditionDiags)

		_, errorMessageDiags := n.evaluateExpr(ctx, cr.ErrorMessage, cty.Bool, selfAddr, keyData)
		diags = diags.Append(errorMessageDiags)
	}

	return diags
}

func validateCount(ctx EvalContext, expr hcl.Expression) (diags tfdiags.Diagnostics) {
	val, countDiags := evaluateCountExpressionValue(expr, ctx)
	// If the value isn't known then that's the best we can do for now, but
	// we'll check more thoroughly during the plan walk
	if !val.IsKnown() {
		return diags
	}

	if countDiags.HasErrors() {
		diags = diags.Append(countDiags)
	}

	return diags
}

func validateForEach(ctx EvalContext, expr hcl.Expression) (diags tfdiags.Diagnostics) {
	const unknownsAllowed = true
	const tupleNotAllowed = false

	val, forEachDiags := evaluateForEachExpressionValue(expr, ctx, unknownsAllowed, tupleNotAllowed, nil)
	// If the value isn't known then that's the best we can do for now, but
	// we'll check more thoroughly during the plan walk
	if !val.IsKnown() {
		return diags
	}

	if forEachDiags.HasErrors() {
		diags = diags.Append(forEachDiags)
	}

	return diags
}

func validateDependsOn(ctx EvalContext, dependsOn []hcl.Traversal) (diags tfdiags.Diagnostics) {
	for _, traversal := range dependsOn {
		ref, refDiags := addrs.ParseRef(traversal)
		diags = diags.Append(refDiags)
		if !refDiags.HasErrors() && len(ref.Remaining) != 0 {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid depends_on reference",
				Detail:   "References in depends_on must be to a whole object (resource, etc), not to an attribute of an object.",
				Subject:  ref.Remaining.SourceRange().Ptr(),
			})
		}

		// The ref must also refer to something that exists. To test that,
		// we'll just eval it and count on the fact that our evaluator will
		// detect references to non-existent objects.
		if !diags.HasErrors() {
			scope := ctx.EvaluationScope(nil, nil, EvalDataForNoInstanceKey)
			if scope != nil { // sometimes nil in tests, due to incomplete mocks
				_, refDiags = scope.EvalReference(ref, cty.DynamicPseudoType)
				diags = diags.Append(refDiags)
			}
		}
	}
	return diags
}
