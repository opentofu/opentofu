package tofu

import (
	"fmt"
	"log"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/lang/evalchecks"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
)

// nodeProvider represents a single provider (as would be addressed
// with [`addrs.Provider`] and expands dynamically during execution to
// represent all of the dynamically-generated instances of the provider
// across all modules.
//
// Each provider can end up with zero or more instances, which are
// represented by some derivation of *NodeAbstractProvider decided
// by the "concrete" callback.
type nodeProvider struct {
	addr     addrs.Provider
	configs  []providerConfigBlock
	concrete concreteProviderInstanceNodeFunc
}

var _ GraphNodeDynamicExpandable = (*nodeProvider)(nil)

func (n *nodeProvider) Name() string {
	return n.addr.ForDisplay() + " (expand)"
}

// DynamicExpand implements GraphNodeDynamicExpandable, finding the
// dynamically-generated set of instances for each provider.
func (n *nodeProvider) DynamicExpand(globalCtx EvalContext) (*Graph, error) {
	var diags tfdiags.Diagnostics

	// There are two different modes of dynamic expansion possible for providers:
	//
	// 1. A provider block can contain its own for_each argument that
	//    causes zero or more additional ("aliased") provider configurations
	//    to be generated in the same module.
	// 2. A provider block can appear inside a module that was itself
	//    called with for_each or count, in which case each instance
	//    of the module has its own provider instance.
	//
	// This function handles both levels of expansion together in the
	// nested loops below.

	g := &Graph{}

	// seenInsts tracks the provider instances we've already encountered, so that
	// we can detect duplicates and return errors instead of constructing an invalid
	// graph.
	seenInsts := addrs.MakeMap[addrs.AbsProviderInstance, *configs.Provider]()
	recordInst := func(instAddr addrs.AbsProviderInstance, config *configs.Provider, keyData instances.RepetitionData) bool {
		log.Printf("[TRACE] nodeProvider: found %s", instAddr)

		if existing, exists := seenInsts.GetOk(instAddr); exists {
			prevRng := existing.DeclRange
			thisRng := config.DeclRange
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Duplicate provider instance",
				Detail:   fmt.Sprintf("A provider instance with the address %s was already declared by the block at %s. Generated provider instance addresses must be unique.", instAddr, prevRng),
				Subject:  thisRng.Ptr(),
			})
			return false
		}
		seenInsts.Put(instAddr, config)

		schema, err := globalCtx.ProviderSchema(instAddr)
		if err != nil {
			// We shouldn't get here and this code is only temporary until we
			// update how schema is represented, so this is intentionally a
			// low-quality message.
			diags = diags.Append(fmt.Errorf("missing schema for %s", instAddr))
			return false
		}

		nodeAbstract := &nodeAbstractProviderInstance{
			Addr:   instAddr,
			Config: config,
			Schema: schema.Provider.Block,

			// TODO: Also retain keyData here so that the node will be able to
			// evaluate its config with each.key/each.value populated, if appropriate.
		}
		node := n.concrete(nodeAbstract)
		g.Add(node)

		return true
	}

	allInsts := globalCtx.InstanceExpander()
	for _, configBlock := range n.configs {
		config := configBlock.config
		moduleInsts := allInsts.ExpandModule(configBlock.moduleAddr)
		for _, moduleInstAddr := range moduleInsts {
			// We need an EvalContext bound to the module where this
			// provider block has been instantiated so that we can
			// evaluate expressions in the correct scope.
			modCtx := globalCtx.WithPath(moduleInstAddr)

			switch {
			case config.Alias == nil && config.ForEach == nil:
				// A single "default" (aka "non-aliased") instance.
				recordInst(addrs.AbsProviderInstance{
					Module:   moduleInstAddr,
					Provider: n.addr,
					Key:      addrs.NoKey, // Default configurations have no instance key
				}, config, EvalDataForNoInstanceKey)
			case config.Alias != nil && config.ForEach == nil:
				// A single "additional" (aka "aliased") instance.
				instKey, moreDiags := evalProviderAlias(config.Alias, modCtx)
				diags = diags.Append(moreDiags)
				if moreDiags.HasErrors() {
					continue
				}
				recordInst(
					addrs.AbsProviderInstance{
						Module:   moduleInstAddr,
						Provider: n.addr,
						Key:      instKey,
					},
					config,
					// In this funny case we don't need each.key or each.value set,
					// even though we do have an instance key, because there is
					// only one instance anyway so no need to differentiate in the
					// main config.
					EvalDataForNoInstanceKey,
				)
			case config.Alias == nil && config.ForEach != nil:
				// Zero or more "additional" (aka "aliased") instances.
				instMap, moreDiags := evaluateForEachExpression(config.ForEach, modCtx)
				diags = diags.Append(moreDiags)
				if moreDiags.HasErrors() {
					continue
				}
				for k := range instMap {
					instKey := addrs.StringKey(k)
					recordInst(
						addrs.AbsProviderInstance{
							Module:   moduleInstAddr,
							Provider: n.addr,
							Key:      instKey,
						},
						config,
						EvalDataForInstanceKey(instKey, instMap),
					)
				}
			default:
				// No other situation should be possible if the config
				// decoder is correctly implemented, so this is just a
				// low-quality error for robustness.
				diags = diags.Append(fmt.Errorf("provider block has invalid combination of alias and for_each arguments; it's a bug that this wasn't caught during configuration loading, so please report it"))
			}
		}
	}

	addRootNodeToGraph(g)
	return g, diags.ErrWithWarnings()
}

func evalProviderAlias(expr hcl.Expression, ctx EvalContext) (addrs.InstanceKey, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	scope := ctx.EvaluationScope(nil, nil, EvalDataForNoInstanceKey)

	refs, moreDiags := lang.ReferencesInExpr(addrs.ParseRef, expr)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return nil, diags
	}
	evalCtx, moreDiags := scope.EvalContext(refs)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return nil, diags
	}
	aliasVal, hclDiags := expr.Value(evalCtx)
	diags = diags.Append(hclDiags)
	if hclDiags.HasErrors() {
		return nil, diags
	}
	aliasVal, err := convert.Convert(aliasVal, cty.String)
	if err != nil {
		diags = diags.Append(&hcl.Diagnostic{
			Severity:    hcl.DiagError,
			Summary:     "Invalid provider alias expression",
			Detail:      fmt.Sprintf("Unsuitable value for provider alias: %s", tfdiags.FormatError(err)),
			Subject:     expr.Range().Ptr(),
			Expression:  expr,
			EvalContext: evalCtx,
		})
		return nil, diags
	}
	if !aliasVal.IsKnown() {
		diags = diags.Append(&hcl.Diagnostic{
			Severity:    hcl.DiagError,
			Summary:     "Invalid provider alias expression",
			Detail:      "In order to determine which provider instance to use to work with each resource instance, OpenTofu needs to determine the final alias for each provider block during the planning phase, but this expression is derived from a value that won't be known until the apply phase.",
			Subject:     expr.Range().Ptr(),
			Expression:  expr,
			EvalContext: evalCtx,
			Extra:       evalchecks.DiagnosticCausedByUnknown(true),
		})
		return nil, diags
	}
	if !aliasVal.IsNull() {
		diags = diags.Append(&hcl.Diagnostic{
			Severity:    hcl.DiagError,
			Summary:     "Invalid provider alias expression",
			Detail:      "The alias for an additional provider configuration must not be null.",
			Subject:     expr.Range().Ptr(),
			Expression:  expr,
			EvalContext: evalCtx,
		})
		return nil, diags
	}
	if aliasVal.HasMark(marks.Sensitive) {
		diags = diags.Append(&hcl.Diagnostic{
			Severity:    hcl.DiagError,
			Summary:     "Invalid provider alias expression",
			Detail:      "The alias for a provider configuration cannot be derived from a sensitive value, because references to the provider instance in the UI would need to disclose the value.",
			Subject:     expr.Range().Ptr(),
			Expression:  expr,
			EvalContext: evalCtx,
			Extra:       evalchecks.DiagnosticCausedBySensitive(true),
		})
		return nil, diags
	}
	if aliasVal.IsMarked() {
		// We don't currently have any other marks at the time of writing this, but we'll
		// check this here just in case we add another in the future and forget to update this.
		// If execution gets here then we should consider it a bug and add some reasonable
		// handling for whatever new mark we've run into.
		diags = diags.Append(&hcl.Diagnostic{
			Severity:    hcl.DiagError,
			Summary:     "Invalid provider alias expression",
			Detail:      fmt.Sprintf("The alias expression produced a value with unhandled marks: %#v. This is a bug in OpenTofu, so please report it.", aliasVal),
			Subject:     expr.Range().Ptr(),
			Expression:  expr,
			EvalContext: evalCtx,
		})
		return nil, diags
	}
	// If we get here then we can safely assume that we have a known, non-null, unmarked string
	aliasStr := aliasVal.AsString()
	if !hclsyntax.ValidIdentifier(aliasStr) {
		// To avoid upsetting too many assumptions from older versions where
		// alias was always required to be a statically-configured identifier,
		// we're continuing to require only identifier characters here. We
		// might relax this in future if we can be confident that it won't
		// break anything, but for now we'll return an error.
		diags = diags.Append(&hcl.Diagnostic{
			Severity:    hcl.DiagError,
			Summary:     "Invalid provider alias",
			Detail:      "The alias for a provider instance must be a valid identifier.", // FIXME: More elaborate error message?
			Subject:     expr.Range().Ptr(),
			Expression:  expr,
			EvalContext: evalCtx,
		})
		return nil, diags
	}

	// If we get _here_ then we've finally got ourselves a valid alias.
	return addrs.StringKey(aliasStr), diags
}

// providerConfigBlock associates a *configs.Provider with the
// address of the static module it was declared in.
//
// This is used for collaboration between [providerTransformer] and
// the [nodeExpandProvider] nodes it generates, to allow deferring the
// actual final expansion of provider _instances_ until the execution
// phase when we can start evaluating for_each expressions, etc.
type providerConfigBlock struct {
	moduleAddr addrs.Module
	config     *configs.Provider
}
