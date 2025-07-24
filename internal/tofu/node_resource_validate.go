// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	otelAttr "go.opentelemetry.io/otel/attribute"
	otelTrace "go.opentelemetry.io/otel/trace"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/communicator/shared"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/didyoumean"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/provisioners"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tracing"
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
func (n *NodeValidatableResource) Execute(ctx context.Context, evalCtx EvalContext, op walkOperation) (diags tfdiags.Diagnostics) {
	_, span := tracing.Tracer().Start(
		ctx, traceNameValidateResource,
		otelTrace.WithAttributes(
			otelAttr.String(traceAttrConfigResourceAddr, n.Addr.String()),
		),
	)
	defer span.End()

	if n.Config == nil {
		return diags
	}

	diags = diags.Append(n.validateResource(ctx, evalCtx))

	diags = diags.Append(n.validateCheckRules(ctx, evalCtx, n.Config))

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
			diags = diags.Append(n.validateProvisioner(ctx, evalCtx, &provisioner))
			if diags.HasErrors() {
				return diags
			}
		}
	}
	importDiags := n.validateImportIDs(ctx, evalCtx)
	diags = diags.Append(importDiags)

	return diags
}

// validateProvisioner validates the configuration of a provisioner belonging to
// a resource. The provisioner config is expected to contain the merged
// connection configurations.
func (n *NodeValidatableResource) validateProvisioner(ctx context.Context, evalCtx EvalContext, p *configs.Provisioner) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	provisioner, err := evalCtx.Provisioner(p.Type)
	if err != nil {
		diags = diags.Append(err)
		return diags
	}

	if provisioner == nil {
		return diags.Append(fmt.Errorf("provisioner %s not initialized", p.Type))
	}
	provisionerSchema, err := evalCtx.ProvisionerSchema(p.Type)
	if err != nil {
		return diags.Append(fmt.Errorf("failed to read schema for provisioner %s: %w", p.Type, err))
	}
	if provisionerSchema == nil {
		return diags.Append(fmt.Errorf("provisioner %s has no schema", p.Type))
	}

	// Validate the provisioner's own config first
	configVal, _, configDiags := n.evaluateBlock(ctx, evalCtx, p.Config, provisionerSchema)
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
		_, _, connDiags := n.evaluateBlock(ctx, evalCtx, p.Connection.Config, shared.ConnectionBlockSupersetSchema)
		diags = diags.Append(connDiags)
	}
	return diags
}

func (n *NodeValidatableResource) evaluateBlock(ctx context.Context, evalCtx EvalContext, body hcl.Body, schema *configschema.Block) (cty.Value, hcl.Body, tfdiags.Diagnostics) {
	keyData, selfAddr := n.stubRepetitionData(n.Config.Count != nil, n.Config.ForEach != nil)

	return evalCtx.EvaluateBlock(ctx, body, schema, selfAddr, keyData)
}

func (n *NodeValidatableResource) validateResource(ctx context.Context, evalCtx EvalContext) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	provider, providerSchema, err := getProvider(ctx, evalCtx, n.ResolvedProvider.ProviderConfig, addrs.NoKey) // Provider Instance Keys are ignored during validate
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
		countDiags := validateCount(ctx, evalCtx, n.Config.Count)
		diags = diags.Append(countDiags)

	case n.Config.ForEach != nil:
		keyData = InstanceKeyEvalData{
			EachKey:   cty.UnknownVal(cty.String),
			EachValue: cty.UnknownVal(cty.DynamicPseudoType),
		}

		// Evaluate the for_each expression here so we can expose the diagnostics
		forEachDiags := validateForEach(ctx, evalCtx, n.Config.ForEach)
		diags = diags.Append(forEachDiags)
	}

	diags = diags.Append(validateDependsOn(ctx, evalCtx, n.Config.DependsOn))

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
			suggestion := n.noResourceSchemaSuggestion(providerSchema)
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid resource type",
				Detail:   fmt.Sprintf("The provider %s does not support resource type %q.%s", n.Provider().ForDisplay(), n.Config.Type, suggestion),
				Subject:  &n.Config.TypeRange,
			})
			return diags
		}

		configVal, _, valDiags := evalCtx.EvaluateBlock(ctx, n.Config.Config, schema, nil, keyData)
		diags = diags.Append(valDiags.InConfigBody(n.Config.Config, n.Addr.String()))
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

		resp := provider.ValidateResourceConfig(ctx, req)
		diags = diags.Append(resp.Diagnostics.InConfigBody(n.Config.Config, n.Addr.String()))

	case addrs.DataResourceMode:
		schema, _ := providerSchema.SchemaForResourceType(n.Config.Mode, n.Config.Type)
		if schema == nil {
			suggestion := n.noResourceSchemaSuggestion(providerSchema)
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid data source",
				Detail:   fmt.Sprintf("The provider %s does not support data source %q.%s", n.Provider().ForDisplay(), n.Config.Type, suggestion),
				Subject:  &n.Config.TypeRange,
			})
			return diags
		}

		configVal, _, valDiags := evalCtx.EvaluateBlock(ctx, n.Config.Config, schema, nil, keyData)
		diags = diags.Append(valDiags.InConfigBody(n.Config.Config, n.Addr.String()))
		if valDiags.HasErrors() {
			return diags
		}

		// Use unmarked value for validate request
		unmarkedConfigVal, _ := configVal.UnmarkDeep()
		req := providers.ValidateDataResourceConfigRequest{
			TypeName: n.Config.Type,
			Config:   unmarkedConfigVal,
		}

		resp := provider.ValidateDataResourceConfig(ctx, req)
		diags = diags.Append(resp.Diagnostics.InConfigBody(n.Config.Config, n.Addr.String()))
	case addrs.EphemeralResourceMode:
		schema, _ := providerSchema.SchemaForResourceType(n.Config.Mode, n.Config.Type)
		if schema == nil {
			suggestion := n.noResourceSchemaSuggestion(providerSchema)
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid ephemeral resource",
				Detail:   fmt.Sprintf("The provider %s does not support ephemeral resource %q.%s", n.Provider().ForDisplay(), n.Config.Type, suggestion),
				Subject:  &n.Config.TypeRange,
			})
			return diags
		}

		configVal, _, valDiags := evalCtx.EvaluateBlock(ctx, n.Config.Config, schema, nil, keyData)
		diags = diags.Append(valDiags)
		if valDiags.HasErrors() {
			return diags
		}

		// Use unmarked value for validate request
		unmarkedConfigVal, _ := configVal.UnmarkDeep()
		req := providers.ValidateEphemeralConfigRequest{
			TypeName: n.Config.Type,
			Config:   unmarkedConfigVal,
		}

		resp := provider.ValidateEphemeralConfig(ctx, req)
		diags = diags.Append(resp.Diagnostics.InConfigBody(n.Config.Config, n.Addr.String()))
	}

	return diags
}

func (n *NodeValidatableResource) validateImportIDs(ctx context.Context, evalCtx EvalContext) tfdiags.Diagnostics {
	importResolver := evalCtx.ImportResolver()
	var diags tfdiags.Diagnostics
	for _, importTarget := range n.importTargets {
		err := importResolver.ValidateImportIDs(ctx, importTarget, evalCtx)
		if err != nil {
			diags = diags.Append(err)
		}
	}
	return diags
}

// noResourceSchemaSuggestion is trying to generate a suggestion to be appended into the diagnostic that is pointing to the fact
// that the resource indicated by the user does not exist. This is doing its best to find a better alternative:
//   - It is checking if in the provider's schema exists a resource with the same resource type but with a different mode.
//   - If none found at the step above, it tries to determine if the name of the resource is incomplete and tries to recommend the
//     closest resource type name to the one that is already configured.
func (n *NodeValidatableResource) noResourceSchemaSuggestion(providerSchema providers.ProviderSchema) string {
	var suggestion string
	if candidateMode, candidateSchema := nodeValidationAlternateBlockModeSuggestion(providerSchema, n.Config.Mode, n.Config.Type); candidateSchema != nil {
		suggestion = fmt.Sprintf("\n\nDid you intend to use a block of type %q %q? If so, declare this using a block of type %q instead of one of type %q.",
			addrs.ResourceModeBlockName(candidateMode), n.Config.Type, addrs.ResourceModeBlockName(candidateMode), addrs.ResourceModeBlockName(n.Config.Mode))
	} else if len(providerSchema.ResourceTypes) > 0 {
		suggestions := make([]string, 0, len(providerSchema.ResourceTypes))
		for name := range providerSchema.ResourceTypes {
			suggestions = append(suggestions, name)
		}
		if suggestion = didyoumean.NameSuggestion(n.Config.Type, suggestions); suggestion != "" {
			suggestion = fmt.Sprintf(" Did you mean %q?", suggestion)
		}
	}
	return suggestion
}

// nodeValidationAlternateBlockModeSuggestion is trying to find an alternative addrs.ResourceMode for the given resourceType in the provider's schema.
// This is needed to be able to provide a suggestion when the user is using a wrong block type for the type of the resource that it's intended
// to be used.
func nodeValidationAlternateBlockModeSuggestion(schema providers.ProviderSchema, mode addrs.ResourceMode, resourceType string) (addrs.ResourceMode, *configschema.Block) {
	filterOnOtherModes := func(targetModes []addrs.ResourceMode) (addrs.ResourceMode, *configschema.Block) {
		for _, candidateMode := range targetModes {
			if b, _ := schema.SchemaForResourceType(candidateMode, resourceType); b != nil {
				return candidateMode, b
			}
		}
		return addrs.InvalidResourceMode, nil
	}

	switch mode {
	case addrs.ManagedResourceMode:
		return filterOnOtherModes([]addrs.ResourceMode{addrs.DataResourceMode, addrs.EphemeralResourceMode})
	case addrs.DataResourceMode:
		return filterOnOtherModes([]addrs.ResourceMode{addrs.ManagedResourceMode, addrs.EphemeralResourceMode})
	case addrs.EphemeralResourceMode:
		return filterOnOtherModes([]addrs.ResourceMode{addrs.ManagedResourceMode, addrs.DataResourceMode})
	}

	return addrs.InvalidResourceMode, nil
}

func (n *NodeValidatableResource) evaluateExpr(ctx context.Context, evalCtx EvalContext, expr hcl.Expression, wantTy cty.Type, self addrs.Referenceable, keyData instances.RepetitionData) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	refs, refDiags := lang.ReferencesInExpr(addrs.ParseRef, expr)
	diags = diags.Append(refDiags)

	scope := evalCtx.EvaluationScope(self, nil, keyData)

	hclCtx, moreDiags := scope.EvalContext(ctx, refs)
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

func (n *NodeValidatableResource) validateCheckRules(ctx context.Context, evalCtx EvalContext, config *configs.Resource) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	keyData, selfAddr := n.stubRepetitionData(n.Config.Count != nil, n.Config.ForEach != nil)

	for _, cr := range config.Preconditions {
		_, conditionDiags := n.evaluateExpr(ctx, evalCtx, cr.Condition, cty.Bool, nil, keyData)
		diags = diags.Append(conditionDiags)

		_, errorMessageDiags := n.evaluateExpr(ctx, evalCtx, cr.ErrorMessage, cty.Bool, nil, keyData)
		diags = diags.Append(errorMessageDiags)
	}

	for _, cr := range config.Postconditions {
		_, conditionDiags := n.evaluateExpr(ctx, evalCtx, cr.Condition, cty.Bool, selfAddr, keyData)
		diags = diags.Append(conditionDiags)

		_, errorMessageDiags := n.evaluateExpr(ctx, evalCtx, cr.ErrorMessage, cty.Bool, selfAddr, keyData)
		diags = diags.Append(errorMessageDiags)
	}

	return diags
}

func validateCount(ctx context.Context, evalCtx EvalContext, expr hcl.Expression) (diags tfdiags.Diagnostics) {
	val, countDiags := evaluateCountExpressionValue(ctx, expr, evalCtx)
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

func validateForEach(ctx context.Context, evalCtx EvalContext, expr hcl.Expression) (diags tfdiags.Diagnostics) {
	const unknownsAllowed = true
	const tupleNotAllowed = false

	val, forEachDiags := evaluateForEachExpressionValue(ctx, expr, evalCtx, unknownsAllowed, tupleNotAllowed, nil)
	// If the value isn't known then that's the best we can do for now, but
	// we'll check more thoroughly during the plan walk
	if !val.IsKnown() {
		return diags
	}

	diags = diags.Append(forEachDiags)

	return diags
}

func validateDependsOn(ctx context.Context, evalCtx EvalContext, dependsOn []hcl.Traversal) (diags tfdiags.Diagnostics) {
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
			scope := evalCtx.EvaluationScope(nil, nil, EvalDataForNoInstanceKey)
			if scope != nil { // sometimes nil in tests, due to incomplete mocks
				_, refDiags = scope.EvalReference(ctx, ref, cty.DynamicPseudoType)
				diags = diags.Append(refDiags)
			}
		}
	}
	return diags
}
