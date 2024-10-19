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
//     instance, whose instance key is NoKey.
//   - Using "alias" without "for_each" represents exactly one additional provider
//     instance whose instance key is a StringKey containing the alias string.
//   - Using "alias" and "for_each" together represents zero or more additional provider
//     instances whose instance keys are all StringKey taken from keys given
//     in the for_each expression.
//
// A provider configuration block can also produce multiple instances if it is
// declared inside a module that was itself called using either the count or
// for_each meta-arguments. That is true regardless of whether the provider
// configuration _itself_ is using for_each, and so for simplicity and consistency
// we just avoid dealing with instance keys at all in here and defer all handling
// of them until DynamicExpand.
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
// [providerConfigTransformer] documentation.
func (p *providerConfigTransformer) Transform(g *Graph) error {
	// Our strategy here is to walk the configuration tree to find
	// all of the "provider" blocks, and generate one node for each.
	// We'll also hunt for other configuration blocks that can potentially
	// imply an empty configuration for a provider and add an additional
	// node for each one implied, so that by the time we're done every
	// provider-consuming node should be able to depend on another
	// node that represents its provider configuration.

	// collectNodesFromExplicitBlocks modifies nodesByProviderConfig in-place.
	nodesByProviderConfig := addrs.MakeMap[addrs.ConfigProvider, *nodeProviderConfigExpand]()
	p.collectNodesFromExplicitBlocks(p.config, nodesByProviderConfig)

	// collectNodesForImpliedEmptyConfigs continues to modify nodesByProviderConfig
	// in-place, possibly adding some extra nodes representing implied empty
	// provider configurations. The implied ones can be recognized by their
	// config field being a nil pointer, thus representing the absense of an
	// explicit configuration block.
	p.collectNodesForImpliedEmptyConfigs(p.config, nodesByProviderConfig)

	// All of the nodes node need to be added to the graph so that
	// later transformers can add incoming dependency edges to them.
	for _, elem := range nodesByProviderConfig.Elems {
		node := elem.Value
		if node.config != nil {
			log.Printf("[TRACE] providerConfigTransformer: %s explicitly declared at %s", node.addr, node.config.DeclRange)
		} else {
			log.Printf("[TRACE] providerConfigTransformer: %s is implied by resource declaration(s)", node.addr)
		}
		g.Add(node)
	}

	// TODO: Decide whether this transformer should also be responsible for creating
	// dependency edges from provider-consuming nodes, or if that's better handled
	// by a separate transformer. That process involves a very similar walk to
	// collectNodesForImpliedEmptyConfigs, so at the very least it would be good
	// to share that logic between the two.

	return nil
}

// collectNodesFromExplicitBlocks performs a recursive walk of the configuration
// tree starting at the given node, adding new graph nodes to "into" as needed
// to eventually capture all of the explicit provider config blocks found in the
// configuration across all modules.
//
// This does not consider the need for implied empty configuration blocks for
// simple no-config providers like the "null" provider; another function must
// add those in afterwards.
func (p *providerConfigTransformer) collectNodesFromExplicitBlocks(fromConfigNode *configs.Config, into addrs.Map[addrs.ConfigProvider, *nodeProviderConfigExpand]) {
	for _, pc := range fromConfigNode.Module.ProviderConfigs {
		// Unfortunate Note: There has been some terminology drift over time here.
		// Early on, a "local name" was called an "unqualified type", before it became
		// clear that it would occasionally be necessary for the local name to differ
		// from the "type" field of the provider source address, and it also isn't
		// really true to say that explicitly-declared local names are "implying"
		// a provider.
		// FIXME: Update this method name to reflect current terminology.
		providerAddr := fromConfigNode.Module.ImpliedProviderForUnqualifiedType(pc.Name)

		node := &nodeProviderConfigExpand{
			addr: addrs.ConfigProvider{
				Module:   fromConfigNode.Path,
				Provider: providerAddr,
				Alias:    pc.Alias,
			},
			config:   pc,
			concrete: p.concreteProvider,
		}
		into.Put(node.addr, node)
	}

	// We also need to visit all of the child nodes, recursively.
	for _, childNode := range fromConfigNode.Children {
		p.collectNodesFromExplicitBlocks(childNode, into)
	}
}

// collectNodesForImpliedEmptyConfigs performs a recursive walk of the configuration
// tree starting at the given node, adding new graph nodes to "into" as needed
// to eventually represent all of the implied empty provider configurations discovered
// from resource/etc blocks.
//
// This should run after [providerConfigTransformer.collectNodesFromExplicitBlocks]
// using the same "into" map, so that "into" can also serve as a record of which
// provider configuration addresses already have explicit config blocks.
func (p *providerConfigTransformer) collectNodesForImpliedEmptyConfigs(fromConfigNode *configs.Config, into addrs.Map[addrs.ConfigProvider, *nodeProviderConfigExpand]) {
	// TODO: Implement something in package configs to own the concern
	// of finding all of the connections between resource blocks and
	// potential config blocks, and then here we'll add new nodes
	// only for any "potential config blocks" that aren't already in
	// the "into" map.

	// We also need to visit all of the child nodes, recursively.
	for _, childNode := range fromConfigNode.Children {
		p.collectNodesForImpliedEmptyConfigs(childNode, into)
	}
}
