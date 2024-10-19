package tofu

import (
	"fmt"
	"log"

	"github.com/hashicorp/hcl/v2"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// nodeProviderConfigExpand represents a single "provider" configuration
// block (as would be address with [`addrs.ConfigProvider`] and expands
// dynamically during execution to represent all of the dynamically-generated
// instances declared by that block.
//
// Each configuration can end up with zero or more instances, which are
// represented by some derivation of *NodeAbstractProvider decided
// by the "concrete" callback.
type nodeProviderConfigExpand struct {
	addr     addrs.ConfigProvider
	config   *configs.Provider
	concrete concreteProviderInstanceNodeFunc
}

var _ GraphNodeDynamicExpandable = (*nodeProviderConfigExpand)(nil)

func (n *nodeProviderConfigExpand) Name() string {
	return n.addr.String() + " (expand)"
}

// DynamicExpand implements GraphNodeDynamicExpandable, finding the
// dynamically-generated set of instances for each provider configuration block.
func (n *nodeProviderConfigExpand) DynamicExpand(globalCtx EvalContext) (*Graph, error) {
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
		log.Printf("[TRACE] nodeProviderConfigExpand: found %s", instAddr)

		if existing, exists := seenInsts.GetOk(instAddr); exists {
			// It should not actually be possible to get in here under the
			// current rules because a specific provider block can only have
			// multiple instances if they are in different module instances
			// and/or the block uses for_each. In the former case the
			// namespaces are separate anyway, and in the latter case duplicates
			// are effectively blocked by the fact that the for_each map or set
			// can't possibly have duplicate keys. This check is here just for
			// robustness in case the rules change in future.
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
	config := n.config
	moduleInsts := allInsts.ExpandModule(n.addr.Module)
	for _, moduleInstAddr := range moduleInsts {
		// We need an EvalContext bound to the module where this
		// provider block has been instantiated so that we can
		// evaluate expressions in the correct scope.
		modCtx := globalCtx.WithPath(moduleInstAddr)

		switch {
		case config.Alias == "" && config.ForEach == nil:
			// A single "default" (aka "non-aliased") instance.
			recordInst(addrs.AbsProviderInstance{
				Module:   moduleInstAddr,
				Provider: n.addr.Provider,
				Key:      addrs.NoKey, // Default configurations have no instance key
			}, config, EvalDataForNoInstanceKey)
		case config.Alias != "" && config.ForEach == nil:
			// A single "additional" (aka "aliased") instance.
			recordInst(
				addrs.AbsProviderInstance{
					Module:   moduleInstAddr,
					Provider: n.addr.Provider,
					Alias:    n.addr.Alias,
					Key:      addrs.NoKey,
				},
				config,
				EvalDataForNoInstanceKey,
			)
		case config.Alias != "" && config.ForEach != nil:
			// Zero or more "additional" (aka "aliased") instances, with
			// dynamically-selected instance keys.
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
						Provider: n.addr.Provider,
						Alias:    n.addr.Alias,
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

	addRootNodeToGraph(g)
	return g, diags.ErrWithWarnings()
}
