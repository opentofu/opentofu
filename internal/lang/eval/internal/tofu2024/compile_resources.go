// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func compileModuleInstanceResources(
	ctx context.Context,
	managedConfigs map[string]*configs.Resource,
	dataConfigs map[string]*configs.Resource,
	ephemeralConfigs map[string]*configs.Resource,
	declScope exprs.Scope,
	moduleProviders configgraph.CompileProviderConfigRef,
	moduleInstanceAddr addrs.ModuleInstance,
	providersSchema evalglue.ProvidersSchema,
	provisionersSchema evalglue.ProvisionersSchema,
	getResultValue func(context.Context, *configgraph.ResourceInstance, cty.Value, exprs.FromValue[*configgraph.ProviderInstance], addrs.Set[addrs.AbsResourceInstance]) (cty.Value, tfdiags.Diagnostics),
	extraMarks cty.ValueMarks,
) map[addrs.Resource]*configgraph.Resource {
	ret := make(map[addrs.Resource]*configgraph.Resource, len(managedConfigs)+len(dataConfigs)+len(ephemeralConfigs))
	for _, rc := range managedConfigs {
		addr, rsrc := compileModuleInstanceResource(ctx, rc, declScope, moduleProviders, moduleInstanceAddr, providersSchema, provisionersSchema, getResultValue, extraMarks)
		ret[addr] = rsrc
	}
	for _, rc := range dataConfigs {
		addr, rsrc := compileModuleInstanceResource(ctx, rc, declScope, moduleProviders, moduleInstanceAddr, providersSchema, nil, getResultValue, extraMarks)
		ret[addr] = rsrc
	}
	for _, rc := range ephemeralConfigs {
		addr, rsrc := compileModuleInstanceResource(ctx, rc, declScope, moduleProviders, moduleInstanceAddr, providersSchema, nil, getResultValue, extraMarks)
		ret[addr] = rsrc
	}
	return ret
}

func compileModuleInstanceResource(
	ctx context.Context,
	config *configs.Resource,
	declScope exprs.Scope,
	moduleProviders configgraph.CompileProviderConfigRef,
	moduleInstanceAddr addrs.ModuleInstance,
	providers evalglue.ProvidersSchema,
	provisioners evalglue.ProvisionersSchema,
	getResultValue func(context.Context, *configgraph.ResourceInstance, cty.Value, exprs.FromValue[*configgraph.ProviderInstance], addrs.Set[addrs.AbsResourceInstance]) (cty.Value, tfdiags.Diagnostics),
	extraMarks cty.ValueMarks,
) (addrs.Resource, *configgraph.Resource) {
	resourceAddr := config.Addr()
	absAddr := moduleInstanceAddr.Resource(resourceAddr.Mode, resourceAddr.Type, resourceAddr.Name)

	// We compile the depends_on argument here but don't evaluate it yet.
	// It actually gets evaluated inside the "instance selector" we'll
	// construct below, when it gets asked for its instances, and include
	// the resulting marks on the count.index, each.key, and/or each.value
	// results because that means it'll get evaluated only once per resource
	// instead of separately for each resource instance.
	sharedDeps, compileInstanceDeps := compileDependsOn(config.DependsOn, declScope, extraMarks)

	var configEvalable exprs.Evalable
	resourceTypeSchema, diags := providers.ResourceTypeSchema(ctx,
		config.Provider,
		resourceAddr.Mode,
		resourceAddr.Type,
	)
	if diags.HasErrors() {
		configEvalable = exprs.ForcedErrorEvalable(diags, tfdiags.SourceRangeFromHCL(config.TypeRange))
	} else if resourceTypeSchema == nil {
		suggestion := "TODO suggestion" //TODO NodeValidatableResource.noResourceSchemaSuggestion
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid resource type",
			Detail:   fmt.Sprintf("The provider %s does not support resource type %q.%s", config.Provider.ForDisplay(), resourceAddr.Type, suggestion),
			Subject:  config.TypeRange.Ptr(),
		})
		configEvalable = exprs.ForcedErrorEvalable(diags, tfdiags.SourceRangeFromHCL(config.TypeRange))
	} else if resourceTypeSchema == evalglue.StaticProviderSchema {
		// Assumed static context, I don't love this sort of flagging though
		configEvalable = exprs.EvalableHCLExpression(&hclsyntax.LiteralValueExpr{Val: cty.DynamicVal})
	} else {
		spec := resourceTypeSchema.Block.DecoderSpec()
		configEvalable = exprs.EvalableHCLBodyWithDynamicBlocks(config.Config, spec)
	}

	// Compile Provisioners
	var createProvisioners []compiledProvisioner
	var destroyProvisioners []compiledProvisioner
	if config.Managed != nil {
		var baseConn hcl.Body
		if config.Managed.Connection != nil {
			baseConn = config.Managed.Connection.Config
		}

		for _, prov := range config.Managed.Provisioners {
			provComp, provDiags := compileProvisioner(ctx, prov, baseConn, provisioners)
			diags = diags.Append(provDiags)
			if provDiags.HasErrors() {
				continue
			}

			if provComp == nil {
				continue
			}

			switch prov.When {
			case configs.ProvisionerWhenCreate:
				createProvisioners = append(createProvisioners, provComp)
			case configs.ProvisionerWhenDestroy:
				destroyProvisioners = append(destroyProvisioners, provComp)
			default:
				panic("Unreachable")
			}
		}
	}

	var ignoreChanges []cty.Path
	if config.Managed != nil {
		if config.Managed.IgnoreAllChanges {
			// Represents ignore all
			ignoreChanges = []cty.Path{{}}
		} else {
			// Copied from NodeValidatableResource
			for _, traversal := range config.Managed.IgnoreChanges {
				// validate the ignore_changes traversals apply.
				moreDiags := resourceTypeSchema.Block.StaticValidateTraversal(traversal)
				diags = diags.Append(moreDiags)

				// ignore_changes cannot be used for Computed attributes,
				// unless they are also Optional.
				// If the traversal was valid, convert it to a cty.Path and
				// use that to check whether the Attribute is Computed and
				// non-Optional.
				if !diags.HasErrors() {
					path := traversalToPath(traversal)

					attrSchema := resourceTypeSchema.Block.AttributeByPath(path)

					if attrSchema != nil && !attrSchema.Optional && attrSchema.Computed {
						// ignore_changes uses absolute traversal syntax in config despite
						// using relative traversals, so we strip the leading "." added by
						// FormatCtyPath for a better error message.
						attrDisplayPath := strings.TrimPrefix(tfdiags.FormatCtyPath(path), ".")

						diags = diags.Append(&hcl.Diagnostic{
							Severity: hcl.DiagWarning,
							Summary:  "Redundant ignore_changes element",
							Detail:   fmt.Sprintf("Adding an attribute name to ignore_changes tells OpenTofu to ignore future changes to the argument in configuration after the object has been created, retaining the value originally configured.\n\nThe attribute %s is decided by the provider alone and therefore there can be no configured value to compare with. Including this attribute in ignore_changes has no effect. Remove the attribute from ignore_changes to quiet this warning.", attrDisplayPath),
							Subject:  &config.TypeRange,
						})
					}
				}
			}
			if travs := config.Managed.IgnoreChanges; len(travs) != 0 {
				ignoreChanges = traversalsToPaths(travs)
			}
		}

	}

	var pdValuer *configgraph.OnceValuer
	if config.Managed != nil && config.Managed.PreventDestroy != nil {
		// We currently forbid usage of any instance information in prevent destroy. We could take a similar
		// approach to destroy provisioners eventually, but for now we will match the previous implementation.
		// See https://github.com/opentofu/opentofu/issues/2522 for more details
		pdValuer = configgraph.ValuerOnce(
			exprs.NewClosure(exprs.EvalableHCLExpression(config.Managed.PreventDestroy), declScope),
		)
	}

	if diags.HasErrors() {
		configEvalable = exprs.ForcedErrorEvalable(diags, tfdiags.SourceRangeFromHCL(config.TypeRange))
	}

	ret := &configgraph.Resource{
		Addr:      absAddr,
		DeclRange: tfdiags.SourceRangeFromHCL(config.DeclRange),

		// Our instance selector depends on which of the repetition metaarguments
		// are set, if any. We assume that package configs allows at most one
		// of these to be set for each resource config.
		InstanceSelector: compileInstanceSelector(ctx, declScope, config.ForEach, config.Count, config.Enabled, sharedDeps),

		// The [configgraph.Resource] implementation will call back to this
		// for each child instance it discovers through [InstanceSelector],
		// allowing us to finalize all of the details for a specific instance
		// of this resource.
		CompileResourceInstance: func(ctx context.Context, key addrs.InstanceKey, repData instances.RepetitionData) *configgraph.ResourceInstance {
			var diags tfdiags.Diagnostics

			localScope := instanceLocalScope(declScope, repData)
			providerRef := compileProviderConfigRef(ctx, moduleProviders, config.ProviderConfigAddr(), config.ProviderConfigRef, localScope)
			instanceDeps := compileInstanceDeps(localScope)

			// For now we require a literal boolean constant in
			// create_before_destroy to match how the old implementation treated
			// this, but this is designed to grow to support arbitrary
			// expressions in future once we're ready to implement this issue:
			//    https://github.com/opentofu/opentofu/issues/2523
			var cbdVal cty.Value
			if config.Managed != nil {
				if !config.Managed.CreateBeforeDestroySet {
					cbdVal = cty.NullVal(cty.Bool)
				} else if config.Managed.CreateBeforeDestroy {
					cbdVal = cty.True
				} else {
					cbdVal = cty.False
				}
			}
			var cbdValuer *configgraph.OnceValuer
			if cbdVal != cty.NilVal {
				// We don't currently track the source location of the
				// create_before_destroy argument in particular, but when
				// we make this support arbitrary expressions in future
				// the expression's source range will be used here instead.
				cbdValuer = configgraph.ValuerOnce(
					exprs.ConstantValuerWithSourceRange(cbdVal, tfdiags.SourceRangeFromHCL(config.DeclRange)),
				)
			}

			var provisionerConfigs []configgraph.Provisioner
			var provisionerMarks cty.ValueMarks
			for _, provFn := range createProvisioners {
				provCfg := provFn(ctx, localScope, absAddr.Instance(key))
				provisionerConfigs = append(provisionerConfigs, provCfg)
				for dep := range provCfg.Dependencies {
					if provisionerMarks == nil {
						provisionerMarks = cty.ValueMarks{}
					}
					provisionerMarks[configgraph.NewResourceInstanceMark(dep)] = struct{}{}
				}
			}

			var replaceMarks cty.ValueMarks
			var replaceTriggeredBy []configgraph.ResourceInstanceAttributePath
			if config.Managed != nil {
				// Validate that attribute traversals in replace_triggered_by
				// expressions refer to attributes that exist in the schema.
				// We use evalReplaceTriggeredByExpr to correctly handle JSON
				// syntax and HCL expressions.
				for _, expr := range config.TriggersReplacement {
					// An alternative to this system would be to mark each resource attribute with it's exact path, sort of
					// an ResourceInstanceMarkWithPath so we know exactly what resource and path an expression derives it's
					// value from. I suspect that the performance impact of that would be quite terrible and we should probably
					// not do that.
					//
					// TODO replace evalReplaceTriggeredByExpr with something more tailored to the new engine style of doing things.
					ref, refDiags := evalReplaceTriggeredByExpr(expr, repData)
					if refDiags.HasErrors() {
						diags = diags.Append(refDiags)
						continue
					}

					var refAddr addrs.Resource
					switch rs := ref.Subject.(type) {
					case addrs.Resource:
						refAddr = rs
					case addrs.ResourceInstance:
						refAddr = rs.Resource
					default:
						continue
					}

					refVal, refValDiags := exprs.NewClosure(exprs.EvalableHCLExpression(expr), localScope).Value(ctx)
					diags = diags.Append(refValDiags)
					if refValDiags.HasErrors() {
						// TODO are these errors good enough or should we replace them with something better?
						continue
					}

					path := traversalToPath(ref.Remaining)
					for ri := range configgraph.ContributingResourceInstances(refVal) {
						if !ri.Addr.Resource.Resource.Equal(refAddr) {
							continue
						}

						if replaceMarks == nil {
							replaceMarks = cty.ValueMarks{}
						}
						replaceMarks[configgraph.NewResourceInstanceMark(ri)] = struct{}{}
						replaceTriggeredBy = append(replaceTriggeredBy, configgraph.ResourceInstanceAttributePath{
							ResourceInstance: ri.Addr,
							Path:             path,
						})
					}
				}
			}

			// Note that repetitionMarks also incorporates any marks from the
			// depends_on argument, which got evaluated as part of the instance
			// selector just as a convenient once-per-resource evaluation hook.
			repetitionMarks := repData.AllValueMarks()

			// Some language features related to resource blocks cause extra
			// transformations of the configuration value, so we'll deal
			// with those by transforming what we get from just evaluating
			// the main config body.
			configValuer := configgraph.ValuerOnce(exprs.DerivedValuerContext(
				exprs.NewClosure(configEvalable, localScope),
				func(ctx context.Context, v cty.Value, vDiags tfdiags.Diagnostics) (cty.Value, tfdiags.Diagnostics) {
					// TODO more efficient marking, this does a lot of map garbarge
					// TODO consider moving provider related marks into here, currently partially impl in configgraph.ResourceInstance.Value
					if len(repetitionMarks) != 0 {
						v = v.WithMarks(repetitionMarks)
					}
					instDepMarks, moreDiags := instanceDeps.Marks(ctx)
					vDiags = vDiags.Append(moreDiags)
					if len(instDepMarks) != 0 {
						v = v.WithMarks(instDepMarks)
					}

					if len(provisionerMarks) != 0 {
						v = v.WithMarks(provisionerMarks)
					}

					if len(replaceMarks) != 0 {
						v = v.WithMarks(replaceMarks)
					}

					// Include any additional diags during the instance setup logic
					vDiags = vDiags.Append(diags)

					return v, vDiags
				},
			))

			inst := &configgraph.ResourceInstance{
				Addr:                      absAddr.Instance(key),
				Provider:                  config.Provider,
				ConfigValuer:              configValuer,
				ProviderInstanceValuer:    configgraph.ValuerOnce(providerRef),
				CreateBeforeDestroyValuer: cbdValuer,
				CreateProvisioners:        provisionerConfigs,
				IgnoreChangesPaths:        ignoreChanges,
				ReplaceTriggeredBy:        replaceTriggeredBy,
			}
			// Again the [ResourceInstance] implementation will call back
			// through this object so we can help it interact with the
			// appropriate provider and collect the result of whatever
			// side-effects our caller is doing with this resource instance
			// in the current phase. (The planned new state during the plan
			// phase, for example.)
			inst.Glue = &resourceInstanceGlue{
				getResultValue: func(ctx context.Context, configVal cty.Value, providerInst exprs.FromValue[*configgraph.ProviderInstance], riDeps addrs.Set[addrs.AbsResourceInstance]) (cty.Value, tfdiags.Diagnostics) {
					return getResultValue(ctx, inst, configVal, providerInst, riDeps)
				},
			}
			return inst
		},
		PreventDestroyValuer: pdValuer,
		DestroyProvisioners: func(ctx context.Context, addr addrs.ResourceInstance) []configgraph.Provisioner {
			var repData instances.RepetitionData
			if addr.Key != nil {
				keyValue := addr.Key.Value()
				switch keyValue.Type() {
				case cty.String:
					repData.EachKey = keyValue
					// For a destroy-time provisioner EachValue is intentionally nil here,
					// that's okay because each.value is prohibited for destroy-time provisioners.
				case cty.Number:
					repData.CountIndex = keyValue
				}
			}

			localScope := instanceLocalScope(declScope, repData)

			var provisionerConfigs []configgraph.Provisioner
			for _, provFn := range destroyProvisioners {
				provisionerConfigs = append(provisionerConfigs, provFn(ctx, localScope, addr.Absolute(moduleInstanceAddr)))
			}
			return provisionerConfigs
		},
	}
	return resourceAddr, ret
}

// resourceInstanceGlue is our implementation of [configgraph.ResourceInstanceGlue],
// allowing our compiled [configgraph.ResourceInstance] objects to call back
// to us for needs that require interacting with outside concerns like
// provider plugins, an active plan or apply process, etc.
type resourceInstanceGlue struct {
	getResultValue func(context.Context, cty.Value, exprs.FromValue[*configgraph.ProviderInstance], addrs.Set[addrs.AbsResourceInstance]) (cty.Value, tfdiags.Diagnostics)
}

// ResultValue implements [configgraph.ResourceInstanceGlue].
func (r *resourceInstanceGlue) ResultValue(ctx context.Context, configVal cty.Value, providerInst exprs.FromValue[*configgraph.ProviderInstance], riDeps addrs.Set[addrs.AbsResourceInstance]) (cty.Value, tfdiags.Diagnostics) {
	return r.getResultValue(ctx, configVal, providerInst, riDeps)
}

// From node_resource_abstract_instance.go

// Convert the hcl.Traversal values we get form the configuration to the
// cty.Path values we need to operate on the cty.Values
func traversalsToPaths(traversals []hcl.Traversal) []cty.Path {
	paths := make([]cty.Path, len(traversals))
	for i, traversal := range traversals {
		path := traversalToPath(traversal)
		paths[i] = path
	}
	return paths
}

func traversalToPath(traversal hcl.Traversal) cty.Path {
	path := make(cty.Path, len(traversal))
	for si, step := range traversal {
		switch ts := step.(type) {
		case hcl.TraverseRoot:
			path[si] = cty.GetAttrStep{
				Name: ts.Name,
			}
		case hcl.TraverseAttr:
			path[si] = cty.GetAttrStep{
				Name: ts.Name,
			}
		case hcl.TraverseIndex:
			path[si] = cty.IndexStep{
				Key: ts.Key,
			}
		default:
			panic(fmt.Sprintf("unsupported traversal step %#v", step))
		}
	}
	return path
}
