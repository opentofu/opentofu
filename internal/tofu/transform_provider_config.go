package tofu

import (
	"log"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
)

// providerConfigTransformer is a graph transformer that creates a graph node for each
// "provider" block in the configuration, and possibly also additional nodes for
// implied empty provider instances for unconfigured providers that resources
// nonetheless refer to.
//
// A provider configuration block can produce zero or more provider instances,
// depending on how it's written:
//
//   - Using neither "for_each" nor "alias" represents exactly one default provider
//     configuration, whose instance key is NoKey.
//   - Using "alias" without "for_each" represents exactly one additional provider
//     configuration whose instance key is a StringKey containing the alias string.
//   - Using "for_each" without "alias" represents zero or more additional provider
//     configurations whose instance keys are all StringKey taken from keys given
//     in the for_each expression.
//
// In all but the last case we can predict the instance key in the initial graph.
// The last case is trickier because we won't know the instance keys -- or even how
// many instances there are -- until DynamicExpand is called during the graph walk.
//
// The two different cases (known instance keys vs. unknown instance keys during
// graph construction) is a necessary compromise to retain the ability for a
// provider configuration block to refer to a resource that belongs to an instance
// that was declared in a _different_ provider configuration block, which was
// possible before we supported for_each for provider blocks. To handle this we
// make precise connections between resources and provider blocks when both ends
// are statically configured, but make less-precise connections when either the
// reference to the provider instance is dynamic or the provider instance itself
// is dynamic.
//
// Note also that this compromise means that we cannot necessarily detect a duplicate
// declaration of the same provider instance key across two blocks until DynamicExpand.
// To keep things simple we defer _all_ checking of that until DynamicExpand, even
// though technically we could catch the static cases during initial graph construction.
// Not catching it during graph construction just means that there are some additional
// edges in the graph, and that doesn't affect correctness.
type providerConfigTransformer struct {
	// Config is the node of the configuration tree representing the root module.
	config *configs.Config

	// concreteProvider specifies how the provider instance nodes that'll be
	// eventually constructed during DynamicExpand should be transformed into
	// a "concrete" node type to include in the graph.
	//
	// This allows varying the chosen node type for different walk types that
	// need differing subsets of the services offered by providers.
	concreteProvider concreteProviderInstanceNodeFunc
}

var _ GraphTransformer = (*providerConfigTransformer)(nil)

// Transform implements GraphTransformer with the behavior described in the
// [providerTransformer] documentation.
func (p *providerConfigTransformer) Transform(g *Graph) error {
	// Our strategy here is to walk the configuration tree to find
	// all of the "provider" blocks, and then group them by which
	// provider they woud instantiate. We then produce one initial
	// graph node for each distinct provider that "remembers" each of
	// that provider's configuration blocks, so that we can finally
	// decide the full set of _instances_ for each provider during
	// the execution phase in those nodes' DynamicExpand.
	//
	// Note that provider configuration blocks are unlike most block
	// types in the language in that they don't have unique identifiers
	// of their own. Instead, each provider can have any number of
	// configuration blocks and then we dynamically choose zero or
	// more real provider instances based on those blocks only during
	// DynamicExpand. After expansion, all of the provider _instance_
	// addresses for each provider must be unique, but we don't yet
	// have enough information to decide that in this transformer.

	// collectNodesFromConfigBlocks modifies nodesByProvider in-place.
	nodesByProvider := make(map[addrs.Provider]*nodeProvider)
	p.collectNodesFromConfigBlocks(p.config, nodesByProvider)

	// All of the nodes node need to be added to the graph so that
	// later transformers can add incoming dependency edges to them.
	for _, node := range nodesByProvider {
		log.Printf("[TRACE] providerTransformer: %s has %d configuration blocks across the whole configuration", node.addr, len(node.configs))
		g.Add(node)
	}

	// FIXME: We also need to deal with the possibility of implied empty
	// provider configurations in the root module, which need to have
	// synthetic empty configuration generated for them.

	// FIXME: We also need to deal with the interesting fact that literally
	// any expression anywhere in the configuration can depend on a provider
	// instance by calling a function, though we can hopefully deal with
	// _that_ case in ReferenceTransformer rather than in here.

	return nil
}

// collectNodesFromConfigBlocks performs a recursive walk of the configuration
// tree starting at the given node, adding new graph nodes to "into" as needed
// to eventually capture all of provider config blocks found in the configuration
// across all modules.
func (p *providerConfigTransformer) collectNodesFromConfigBlocks(fromConfigNode *configs.Config, into map[addrs.Provider]*nodeProvider) {
	for _, pc := range fromConfigNode.Module.ProviderConfigs {
		// Unfortunate Note: There has been some terminology drift over time here.
		// Early on, a "local name" was called an "unqualified type", before it became
		// clear that it would occasionally be necessary for the local name to differ
		// from the "type" field of the provider source address, and it also isn't
		// really true to say that explicitly-declared local names are "implying"
		// a provider.
		// FIXME: Update this method name to reflect current terminology.
		providerAddr := fromConfigNode.Module.ImpliedProviderForUnqualifiedType(pc.Name)

		if into[providerAddr] == nil {
			into[providerAddr] = &nodeProvider{
				addr:     providerAddr,
				concrete: p.concreteProvider,
			}
		}

		node := into[providerAddr]
		node.configs = append(node.configs, providerConfigBlock{
			moduleAddr: fromConfigNode.Path,
			config:     pc,
		})
	}

	// We also need to visit all of the child nodes, recursively.
	for _, childNode := range fromConfigNode.Children {
		p.collectNodesFromConfigBlocks(childNode, into)
	}
}
