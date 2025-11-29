// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/communicator/shared"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/dag"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// traceNameValidateResource is a standardized trace span name we use for the
// overall execution of all graph nodes that somehow represent the planning
// phase for a resource instance.
const traceNameValidateResource = "Validate resource configuration"

// traceAttrConfigResourceAddr is a standardized trace span attribute name that we
// use for recording the address of the main resource that a particular span is
// concerned with.
//
// The value of this should be populated by calling the String method on
// a value of type [addrs.ConfigResource]. DO NOT use this with results from
// [addrs.AbsResourceInstance]; use [traceAttrResourceInstanceAddr] instead
// for that address type.
const traceAttrConfigResourceAddr = "opentofu.resource.address"

// ConcreteResourceNodeFunc is a callback type used to convert an
// abstract resource to a concrete one of some type.
type ConcreteResourceNodeFunc func(*NodeAbstractResource) dag.Vertex

// GraphNodeConfigResource is implemented by any nodes that represent a resource.
// The type of operation cannot be assumed, only that this node represents
// the given resource.
type GraphNodeConfigResource interface {
	ResourceAddr() addrs.ConfigResource
}

// ConcreteResourceInstanceNodeFunc is a callback type used to convert an
// abstract resource instance to a concrete one of some type.
type ConcreteResourceInstanceNodeFunc func(*NodeAbstractResourceInstance) dag.Vertex

// GraphNodeResourceInstance is implemented by any nodes that represent
// a resource instance. A single resource may have multiple instances if,
// for example, the "count" or "for_each" argument is used for it in
// configuration.
type GraphNodeResourceInstance interface {
	ResourceInstanceAddr() addrs.AbsResourceInstance

	// StateDependencies returns any inter-resource dependencies that are
	// stored in the state.
	StateDependencies() []addrs.ConfigResource
}

// NodeAbstractResource represents a resource that has no associated
// operations. It registers all the interfaces for a resource that common
// across multiple operation types.
type NodeAbstractResource struct {
	Addr addrs.ConfigResource

	// The fields below will be automatically set using the Attach
	// interfaces if you're running those transforms, but also be explicitly
	// set if you already have that information.

	Schema        *configschema.Block // Schema for processing the configuration body
	SchemaVersion uint64              // Schema version of "Schema", as decided by the provider
	Config        *configs.Resource   // Config is the resource in the config

	// ProviderMetas is the provider_meta configs for the module this resource belongs to
	ProviderMetas map[addrs.Provider]*configs.ProviderMeta

	ProvisionerSchemas map[string]*configschema.Block

	// Set from GraphNodeTargetable
	Targets []addrs.Targetable

	// Set from GraphNodeTargetable
	Excludes []addrs.Targetable

	// Set from AttachResourceDependsOn
	dependsOn      []addrs.ConfigResource
	forceDependsOn bool

	// The address of the provider this resource will use
	ResolvedProvider ResolvedProvider

	// storedProviderConfig is the provider address retrieved from the
	// state. This is defined here for access within the ProvidedBy method, but
	// will be set from the embedding instance type when the state is attached.
	storedProviderConfig ResolvedProvider

	// This resource may expand into instances which need to be imported.
	importTargets []*ImportTarget

	// generateConfigPath tells this node which file to write generated config
	// into. If empty, then config should not be generated.
	generateConfigPath string

	// removedBlockProvisioners holds any possibly existing configs.Provisioner configs that could be defined by using
	// removed.provisioner configuration. If the field "Config.Managed.Provisioners" is having no provisioners, then
	// these provisioners should be used instead.
	removedBlockProvisioners []*configs.Provisioner
}

var (
	_ GraphNodeReferenceable             = (*NodeAbstractResource)(nil)
	_ GraphNodeReferencer                = (*NodeAbstractResource)(nil)
	_ GraphNodeProviderConsumer          = (*NodeAbstractResource)(nil)
	_ GraphNodeProvisionerConsumer       = (*NodeAbstractResource)(nil)
	_ GraphNodeConfigResource            = (*NodeAbstractResource)(nil)
	_ GraphNodeAttachResourceConfig      = (*NodeAbstractResource)(nil)
	_ GraphNodeAttachResourceSchema      = (*NodeAbstractResource)(nil)
	_ GraphNodeAttachProvisionerSchema   = (*NodeAbstractResource)(nil)
	_ GraphNodeAttachProviderMetaConfigs = (*NodeAbstractResource)(nil)
	_ GraphNodeTargetable                = (*NodeAbstractResource)(nil)
	_ graphNodeAttachResourceDependsOn   = (*NodeAbstractResource)(nil)
	_ dag.GraphNodeDotter                = (*NodeAbstractResource)(nil)
)

// NewNodeAbstractResource creates an abstract resource graph node for
// the given absolute resource address.
func NewNodeAbstractResource(addr addrs.ConfigResource) *NodeAbstractResource {
	return &NodeAbstractResource{
		Addr: addr,
	}
}

var (
	_ GraphNodeModuleInstance            = (*NodeAbstractResourceInstance)(nil)
	_ GraphNodeReferenceable             = (*NodeAbstractResourceInstance)(nil)
	_ GraphNodeReferencer                = (*NodeAbstractResourceInstance)(nil)
	_ GraphNodeRootReferencer            = (*NodeAbstractResourceInstance)(nil)
	_ GraphNodeProviderConsumer          = (*NodeAbstractResourceInstance)(nil)
	_ GraphNodeProvisionerConsumer       = (*NodeAbstractResourceInstance)(nil)
	_ GraphNodeConfigResource            = (*NodeAbstractResourceInstance)(nil)
	_ GraphNodeResourceInstance          = (*NodeAbstractResourceInstance)(nil)
	_ GraphNodeAttachResourceState       = (*NodeAbstractResourceInstance)(nil)
	_ GraphNodeAttachResourceConfig      = (*NodeAbstractResourceInstance)(nil)
	_ GraphNodeAttachResourceSchema      = (*NodeAbstractResourceInstance)(nil)
	_ GraphNodeAttachProvisionerSchema   = (*NodeAbstractResourceInstance)(nil)
	_ GraphNodeAttachProviderMetaConfigs = (*NodeAbstractResourceInstance)(nil)
	_ GraphNodeTargetable                = (*NodeAbstractResourceInstance)(nil)
	_ dag.GraphNodeDotter                = (*NodeAbstractResourceInstance)(nil)
)

func (n *NodeAbstractResource) Name() string {
	return n.ResourceAddr().String()
}

// GraphNodeModulePath
func (n *NodeAbstractResource) ModulePath() addrs.Module {
	return n.Addr.Module
}

// GraphNodeReferenceable
func (n *NodeAbstractResource) ReferenceableAddrs() []addrs.Referenceable {
	return []addrs.Referenceable{n.Addr.Resource}
}

// GraphNodeReferencer
func (n *NodeAbstractResource) References() []*addrs.Reference {
	var result []*addrs.Reference
	// If we have a config then we prefer to use that.
	if c := n.Config; c != nil {
		result = append(result, n.DependsOn()...)

		if n.Schema == nil {
			// Should never happen, but we'll log if it does so that we can
			// see this easily when debugging.
			log.Printf("[WARN] no schema is attached to %s, so config references cannot be detected", n.Name())
		}

		refs, _ := lang.ReferencesInExpr(addrs.ParseRef, c.Count)
		result = append(result, refs...)
		refs, _ = lang.ReferencesInExpr(addrs.ParseRef, c.ForEach)
		result = append(result, refs...)
		refs, _ = lang.ReferencesInExpr(addrs.ParseRef, c.Enabled)
		result = append(result, refs...)

		if c.ProviderConfigRef != nil && c.ProviderConfigRef.KeyExpression != nil {
			providerRefs, _ := lang.ReferencesInExpr(addrs.ParseRef, c.ProviderConfigRef.KeyExpression)
			result = append(result, providerRefs...)
		}

		for _, expr := range c.TriggersReplacement {
			refs, _ = lang.ReferencesInExpr(addrs.ParseRef, expr)
			result = append(result, refs...)
		}

		// ReferencesInBlock() requires a schema
		if n.Schema != nil {
			refs, _ = lang.ReferencesInBlock(addrs.ParseRef, c.Config, n.Schema)
			result = append(result, refs...)
		}

		if c.Managed != nil {
			if c.Managed.PreventDestroy != nil {
				refs, _ := lang.ReferencesInExpr(addrs.ParseRef, c.Managed.PreventDestroy)
				result = append(result, refs...)
			}

			if c.Managed.Connection != nil {
				refs, _ = lang.ReferencesInBlock(addrs.ParseRef, c.Managed.Connection.Config, shared.ConnectionBlockSupersetSchema)
				result = append(result, refs...)
			}

			for _, p := range c.Managed.Provisioners {
				if p.When != configs.ProvisionerWhenCreate {
					continue
				}
				if p.Connection != nil {
					refs, _ = lang.ReferencesInBlock(addrs.ParseRef, p.Connection.Config, shared.ConnectionBlockSupersetSchema)
					result = append(result, refs...)
				}

				schema := n.ProvisionerSchemas[p.Type]
				if schema == nil {
					log.Printf("[WARN] no schema for provisioner %q is attached to %s, so provisioner block references cannot be detected", p.Type, n.Name())
				}
				refs, _ = lang.ReferencesInBlock(addrs.ParseRef, p.Config, schema)
				result = append(result, refs...)
			}
		}

		for _, check := range c.Preconditions {
			refs, _ := lang.ReferencesInExpr(addrs.ParseRef, check.Condition)
			result = append(result, refs...)
			refs, _ = lang.ReferencesInExpr(addrs.ParseRef, check.ErrorMessage)
			result = append(result, refs...)
		}
		for _, check := range c.Postconditions {
			refs, _ := lang.ReferencesInExpr(addrs.ParseRef, check.Condition)
			result = append(result, refs...)
			refs, _ = lang.ReferencesInExpr(addrs.ParseRef, check.ErrorMessage)
			result = append(result, refs...)
		}
	}

	return result
}

// DestroyReferences is a _partial_ implementation of [GraphNodeDestroyer]
// providing a default implementation for any embedding node type that has
// its own implementations of all of the other methods of that interface.
func (n *NodeAbstractResource) DestroyReferences() []*addrs.Reference {
	// Config is always optional at destroy time, but if it's present then
	// it might influence how we plan and apply the destroy actions.
	var result []*addrs.Reference
	if c := n.Config; c != nil {
		if c.Managed != nil {
			// The prevent_destroy setting, if present, forces planning to fail
			// if the planned action for any instance of the resource is to
			// destroy it, so we need to be able to evaluate the given expression
			// for destroy nodes too.
			if c.Managed.PreventDestroy != nil {
				refs, _ := lang.ReferencesInExpr(addrs.ParseRef, c.Managed.PreventDestroy)
				result = append(result, refs...)
			}
		}
	}
	return result
}

// referencesInImportAddress find all references relevant to the node in an import target address expression.
// The only references we care about here are the references that exist in the keys of hclsyntax.IndexExpr.
// For example, if the address is module.my_module1[expression1].aws_s3_bucket.bucket[expression2], then we would only
// consider references in expression1 and expression2, as the rest of the expression is the static part of the current
// resource's address
func referencesInImportAddress(expr hcl.Expression) (refs []*addrs.Reference, diags tfdiags.Diagnostics) {
	switch e := expr.(type) {
	case *hclsyntax.IndexExpr:
		r, d := referencesInImportAddress(e.Collection)
		diags = diags.Append(d)
		refs = append(refs, r...)

		r, _ = lang.ReferencesInExpr(addrs.ParseRef, e.Key)
		refs = append(refs, r...)
	case *hclsyntax.RelativeTraversalExpr:
		r, d := referencesInImportAddress(e.Source)
		refs = append(refs, r...)
		diags = diags.Append(d)

		// We don't care about the traversal part of the relative expression
		// as it should not contain any references in the index keys
	case *hclsyntax.ScopeTraversalExpr:
		// Static traversals should not contain any references in the index keys
	default:
		//  This should not happen, as it should have failed validation earlier, in config.absTraversalForImportToExpr
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid import address expression",
			Detail:   "Import address must be a reference to a resource's address, and only allows for indexing with dynamic keys. For example: module.my_module[expression1].aws_s3_bucket.my_buckets[expression2] for resources inside of modules, or simply aws_s3_bucket.my_bucket for a resource in the root module",
			Subject:  expr.Range().Ptr(),
		})
	}
	return
}

func (n *NodeAbstractResource) RootReferences() []*addrs.Reference {
	var root []*addrs.Reference

	for _, importTarget := range n.importTargets {
		// References are only possible in import targets originating from an import block
		if !importTarget.IsFromImportBlock() {
			continue
		}

		refs, _ := referencesInImportAddress(importTarget.Config.To)
		root = append(root, refs...)

		refs, _ = lang.ReferencesInExpr(addrs.ParseRef, importTarget.Config.ForEach)
		root = append(root, refs...)

		refs, _ = lang.ReferencesInExpr(addrs.ParseRef, importTarget.Config.ID)
		root = append(root, refs...)
	}

	return root
}

func (n *NodeAbstractResource) DependsOn() []*addrs.Reference {
	var result []*addrs.Reference
	if c := n.Config; c != nil {

		for _, traversal := range c.DependsOn {
			ref, diags := addrs.ParseRef(traversal)
			if diags.HasErrors() {
				// We ignore this here, because this isn't a suitable place to return
				// errors. This situation should be caught and rejected during
				// validation.
				log.Printf("[ERROR] Can't parse %#v from depends_on as reference: %s", traversal, diags.Err())
				continue
			}

			result = append(result, ref)
		}
	}
	return result
}

// GraphNodeProviderConsumer
func (n *NodeAbstractResource) SetProvider(resolved ResolvedProvider) {
	n.ResolvedProvider = resolved
}

// GraphNodeProviderConsumer
func (n *NodeAbstractResource) ProvidedBy() RequestedProvider {
	// Once the provider is fully resolved, we can return the known value.
	if n.ResolvedProvider.ProviderConfig.Provider.Type != "" {
		return RequestedProvider{
			ProviderConfig: n.ResolvedProvider.ProviderConfig,
			KeyExpression:  n.ResolvedProvider.KeyExpression,
			KeyModule:      n.ResolvedProvider.KeyModule,
			KeyResource:    n.ResolvedProvider.KeyResource,
			KeyExact:       n.ResolvedProvider.KeyExact,
		}
	}

	// If we have a config we prefer that above all else
	if n.Config != nil {
		result := RequestedProvider{
			ProviderConfig: n.Config.ProviderConfigAddr(),
		}
		if n.Config.ProviderConfigRef != nil && n.Config.ProviderConfigRef.KeyExpression != nil {
			result.KeyResource = true
			result.KeyExpression = n.Config.ProviderConfigRef.KeyExpression
		}
		return result
	}

	// See if we have a valid provider config from the state.
	if n.storedProviderConfig.ProviderConfig.Provider.Type != "" {
		// An address from the state must match exactly, since we must ensure
		// we refresh/destroy a resource with the same provider configuration
		// that created it.
		return RequestedProvider{
			ProviderConfig: n.storedProviderConfig.ProviderConfig,
			KeyExpression:  n.storedProviderConfig.KeyExpression,
			KeyModule:      n.storedProviderConfig.KeyModule,
			KeyResource:    n.storedProviderConfig.KeyResource,
			KeyExact:       n.storedProviderConfig.KeyExact,
		}
	}

	// We might have an import target that is providing a specific provider,
	// this is okay as we know there is nothing else potentially providing a
	// provider configuration.
	if len(n.importTargets) > 0 {
		// The import targets should either all be defined via config or none
		// of them should be. They should also all have the same provider, so it
		// shouldn't matter which we check here, as they'll all give the same.
		if n.importTargets[0].Config != nil && n.importTargets[0].Config.ProviderConfigRef != nil {
			return RequestedProvider{
				ProviderConfig: addrs.LocalProviderConfig{
					LocalName: n.importTargets[0].Config.ProviderConfigRef.Name,
					Alias:     n.importTargets[0].Config.ProviderConfigRef.Alias,
				},
				// This is where we would specify a key expression if that was supported for import blocks
			}
		}
	}

	// No provider configuration found; return a default address
	return RequestedProvider{
		ProviderConfig: addrs.LocalProviderConfig{
			LocalName: n.Addr.Resource.ImpliedProvider(), // Unused, see ProviderTransformer
		},
	}
}

// GraphNodeProviderConsumer
func (n *NodeAbstractResource) Provider() addrs.Provider {
	if n.ResolvedProvider.ProviderConfig.Provider.Type != "" {
		return n.ResolvedProvider.ProviderConfig.Provider
	}
	if n.Config != nil {
		return n.Config.Provider
	}
	if n.storedProviderConfig.ProviderConfig.Provider.Type != "" {
		return n.storedProviderConfig.ProviderConfig.Provider
	}

	if len(n.importTargets) > 0 {
		// The import targets should either all be defined via config or none
		// of them should be. They should also all have the same provider, so it
		// shouldn't matter which we check here, as they'll all give the same.
		if n.importTargets[0].Config != nil {
			return n.importTargets[0].Config.Provider
		}
	}

	return addrs.ImpliedProviderForUnqualifiedType(n.Addr.Resource.ImpliedProvider())
}

// GraphNodeProvisionerConsumer
func (n *NodeAbstractResource) ProvisionedBy() []string {
	// If we have no configuration, then we have no provisioners
	if n.Config == nil || n.Config.Managed == nil {
		return nil
	}

	// Build the list of provisioners we need based on the configuration.
	// It is okay to have duplicates here.
	result := make([]string, len(n.Config.Managed.Provisioners))
	for i, p := range n.Config.Managed.Provisioners {
		result[i] = p.Type
	}

	return result
}

// GraphNodeProvisionerConsumer
func (n *NodeAbstractResource) AttachProvisionerSchema(name string, schema *configschema.Block) {
	if n.ProvisionerSchemas == nil {
		n.ProvisionerSchemas = make(map[string]*configschema.Block)
	}
	n.ProvisionerSchemas[name] = schema
}

// GraphNodeResource
func (n *NodeAbstractResource) ResourceAddr() addrs.ConfigResource {
	return n.Addr
}

// GraphNodeTargetable
func (n *NodeAbstractResource) SetTargets(targets []addrs.Targetable) {
	n.Targets = targets
}

// GraphNodeTargetable
func (n *NodeAbstractResource) SetExcludes(excludes []addrs.Targetable) {
	n.Excludes = excludes
}

// graphNodeAttachResourceDependsOn
func (n *NodeAbstractResource) AttachResourceDependsOn(deps []addrs.ConfigResource, force bool) {
	n.dependsOn = deps
	n.forceDependsOn = force
}

// GraphNodeAttachResourceConfig
func (n *NodeAbstractResource) AttachResourceConfig(c *configs.Resource) {
	n.Config = c
}

// GraphNodeAttachResourceSchema impl
func (n *NodeAbstractResource) AttachResourceSchema(schema *configschema.Block, version uint64) {
	n.Schema = schema
	n.SchemaVersion = version
}

// GraphNodeAttachProviderMetaConfigs impl
func (n *NodeAbstractResource) AttachProviderMetaConfigs(c map[addrs.Provider]*configs.ProviderMeta) {
	n.ProviderMetas = c
}

// GraphNodeDotter impl.
func (n *NodeAbstractResource) DotNode(name string, opts *dag.DotOpts) *dag.DotNode {
	return &dag.DotNode{
		Name: name,
		Attrs: map[string]string{
			"label": n.Name(),
			"shape": "box",
		},
	}
}

// writeResourceState ensures that a suitable resource-level state record is
// present in the state, if that's required for the "each mode" of that
// resource.
//
// This is important primarily for the situation where count = 0, since this
// eval is the only change we get to set the resource "each mode" to list
// in that case, allowing expression evaluation to see it as a zero-element list
// rather than as not set at all.
func (n *NodeAbstractResource) writeResourceState(ctx context.Context, evalCtx EvalContext, addr addrs.AbsResource) (diags tfdiags.Diagnostics) {
	state := evalCtx.State()

	// We'll record our expansion decision in the shared "expander" object
	// so that later operations (i.e. DynamicExpand and expression evaluation)
	// can refer to it. Since this node represents the abstract module, we need
	// to expand the module here to create all resources.
	expander := evalCtx.InstanceExpander()

	switch {
	case n.Config != nil && n.Config.Count != nil:
		count, countDiags := evaluateCountExpression(ctx, n.Config.Count, evalCtx, addr)
		diags = diags.Append(countDiags)
		if countDiags.HasErrors() {
			return diags
		}

		state.SetResourceProvider(addr, n.ResolvedProvider.ProviderConfig)
		expander.SetResourceCount(addr.Module, n.Addr.Resource, count)

	case n.Config != nil && n.Config.Enabled != nil:
		enabled, enabledDiags := evaluateEnabledExpression(ctx, n.Config.Enabled, evalCtx)
		diags = diags.Append(enabledDiags)
		if enabledDiags.HasErrors() {
			return diags
		}

		state.SetResourceProvider(addr, n.ResolvedProvider.ProviderConfig)
		expander.SetResourceEnabled(addr.Module, n.Addr.Resource, enabled)

	case n.Config != nil && n.Config.ForEach != nil:
		forEach, forEachDiags := evaluateForEachExpression(ctx, n.Config.ForEach, evalCtx, addr)
		diags = diags.Append(forEachDiags)
		if forEachDiags.HasErrors() {
			return diags
		}

		// This method takes care of all of the business logic of updating this
		// while ensuring that any existing instances are preserved, etc.
		state.SetResourceProvider(addr, n.ResolvedProvider.ProviderConfig)
		expander.SetResourceForEach(addr.Module, n.Addr.Resource, forEach)

	default:
		state.SetResourceProvider(addr, n.ResolvedProvider.ProviderConfig)
		expander.SetResourceSingle(addr.Module, n.Addr.Resource)
	}

	return diags
}

func isResourceMovedToDifferentType(newAddr, oldAddr addrs.AbsResourceInstance) bool {
	return newAddr.Resource.Resource.Type != oldAddr.Resource.Resource.Type
}

// readResourceInstanceState reads the current object for a specific instance in
// the state.
func (n *NodeAbstractResourceInstance) readResourceInstanceState(ctx context.Context, evalCtx EvalContext, addr addrs.AbsResourceInstance) (*states.ResourceInstanceObject, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	provider, providerSchema, err := getProvider(ctx, evalCtx, n.ResolvedProvider.ProviderConfig, n.ResolvedProviderKey)
	if err != nil {
		return nil, diags.Append(err)
	}

	log.Printf("[TRACE] readResourceInstanceState: reading state for %s", addr)

	src := evalCtx.State().ResourceInstanceObject(addr, states.CurrentGen)
	if src == nil {
		// Presumably we only have deposed objects, then.
		log.Printf("[TRACE] readResourceInstanceState: no state present for %s", addr)
		return nil, nil
	}

	schema, currentVersion := (providerSchema).SchemaForResourceAddr(addr.Resource.ContainingResource())
	if schema == nil {
		// Shouldn't happen since we should've failed long ago if no schema is present
		return nil, diags.Append(fmt.Errorf("no schema available for %s while reading state; this is a bug in OpenTofu and should be reported", addr))
	}

	// prevAddr will match the newAddr if the resource wasn't moved (prevRunAddr checks move results)
	prevAddr := n.prevRunAddr(evalCtx)
	transformArgs := stateTransformArgs{
		currentAddr:          addr,
		prevAddr:             prevAddr,
		provider:             provider,
		objectSrc:            src,
		currentSchema:        schema,
		currentSchemaVersion: currentVersion,
	}
	if isResourceMovedToDifferentType(addr, prevAddr) {
		src, diags = moveResourceState(transformArgs)
	} else {
		src, diags = upgradeResourceState(transformArgs)
	}

	if n.Config != nil {
		diags = diags.InConfigBody(n.Config.Config, addr.String())
	}
	if diags.HasErrors() {
		return nil, diags
	}

	obj, err := src.Decode(schema.ImpliedType())
	if err != nil {
		diags = diags.Append(err)
	}

	return obj, diags
}

// readResourceInstanceStateDeposed reads the deposed object for a specific
// instance in the state.
func (n *NodeAbstractResourceInstance) readResourceInstanceStateDeposed(ctx context.Context, evalCtx EvalContext, addr addrs.AbsResourceInstance, key states.DeposedKey) (*states.ResourceInstanceObject, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	provider, providerSchema, err := getProvider(ctx, evalCtx, n.ResolvedProvider.ProviderConfig, n.ResolvedProviderKey)
	if err != nil {
		diags = diags.Append(err)
		return nil, diags
	}

	if key == states.NotDeposed {
		return nil, diags.Append(fmt.Errorf("readResourceInstanceStateDeposed used with no instance key; this is a bug in OpenTofu and should be reported"))
	}

	log.Printf("[TRACE] readResourceInstanceStateDeposed: reading state for %s deposed object %s", addr, key)

	src := evalCtx.State().ResourceInstanceObject(addr, key)
	if src == nil {
		// Presumably we only have deposed objects, then.
		log.Printf("[TRACE] readResourceInstanceStateDeposed: no state present for %s deposed object %s", addr, key)
		return nil, diags
	}

	schema, currentVersion := (providerSchema).SchemaForResourceAddr(addr.Resource.ContainingResource())
	if schema == nil {
		// Shouldn't happen since we should've failed long ago if no schema is present
		return nil, diags.Append(fmt.Errorf("no schema available for %s while reading state; this is a bug in OpenTofu and should be reported", addr))
	}
	// prevAddr will match the newAddr if the resource wasn't moved (prevRunAddr checks move results)
	prevAddr := n.prevRunAddr(evalCtx)
	transformArgs := stateTransformArgs{
		currentAddr:          addr,
		prevAddr:             prevAddr,
		provider:             provider,
		objectSrc:            src,
		currentSchema:        schema,
		currentSchemaVersion: currentVersion,
	}
	if isResourceMovedToDifferentType(addr, prevAddr) {
		src, diags = moveResourceState(transformArgs)
	} else {
		src, diags = upgradeResourceState(transformArgs)
	}

	if n.Config != nil {
		diags = diags.InConfigBody(n.Config.Config, addr.String())
	}
	if diags.HasErrors() {
		// Note that we don't have any channel to return warnings here. We'll
		// accept that for now since warnings during a schema upgrade would
		// be pretty weird anyway, since this operation is supposed to seem
		// invisible to the user.
		return nil, diags
	}

	obj, err := src.Decode(schema.ImpliedType())
	if err != nil {
		diags = diags.Append(err)
	}

	return obj, diags
}

// graphNodesAreResourceInstancesInDifferentInstancesOfSameModule is an
// annoyingly-task-specific helper function that returns true if and only if
// the following conditions hold:
//   - Both of the given vertices represent specific resource instances, as
//     opposed to unexpanded resources or any other non-resource-related object.
//   - The module instance addresses for both of the resource instances belong
//     to the same static module.
//   - The module instance addresses for both of the resource instances are
//     not equal, indicating that they belong to different instances of the
//     same module.
//
// This result can be used as a way to compensate for the effects of
// conservative analysis passes in our graph builders which make their
// decisions based only on unexpanded addresses, often so that they can behave
// correctly for interactions between expanded and not-yet-expanded objects.
//
// Callers of this helper function will typically skip adding an edge between
// the two given nodes if this function returns true.
func graphNodesAreResourceInstancesInDifferentInstancesOfSameModule(a, b dag.Vertex) bool {
	aRI, aOK := a.(GraphNodeResourceInstance)
	bRI, bOK := b.(GraphNodeResourceInstance)
	if !aOK || !bOK {
		return false
	}
	aModInst := aRI.ResourceInstanceAddr().Module
	bModInst := bRI.ResourceInstanceAddr().Module
	if !aModInst.HasSameModule(bModInst) {
		return false
	}
	return !aModInst.Equal(bModInst)
}
