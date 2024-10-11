// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"log"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/dag"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func transformProviders(concrete ConcreteProviderNodeFunc, config *configs.Config) GraphTransformer {
	return GraphTransformMulti(
		// Add providers from the config
		&providerConfigTransformer{
			config:           config,
			concreteProvider: concrete,
		},
		// Add any remaining missing providers
		&MissingProviderInstanceTransformer{
			Config:   config,
			Concrete: concrete,
		},
		// Connect the providers
		&ProviderInstanceTransformer{
			Config: config,
		},
		// The following comment shows what must be added to the transformer list after the schema transformer
		// After schema transformer, we can add function references
		//  &ProviderFunctionTransformer{Config: config},
		// Remove unused providers and proxies
		//  &PruneProviderInstanceTransformer{},
	)
}

// GraphNodeProviderInstance is an interface that nodes that can be a provider
// must implement.
//
// ProviderAddr returns the address of the provider configuration this
// satisfies, which is always in the path returned by method Path().
//
// Name returns the full name of the provider in the config.
type GraphNodeProviderInstance interface {
	GraphNodeModulePath
	ProviderAddr() addrs.ConfigProviderInstance
	Name() string
}

// GraphNodeCloseProviderInstance is an interface that nodes that can be a close
// provider must implement. The CloseProviderName returned is the name of
// the provider they satisfy.
type GraphNodeCloseProviderInstance interface {
	GraphNodeModulePath
	CloseProviderAddr() addrs.ConfigProviderInstance
}

// GraphNodeProviderInstanceConsumer is an interface that nodes that require
// a provider must implement. ProvidedBy must return the address of the provider
// to use, which will be resolved to a configuration either in the same module
// or in an ancestor module, with the resulting absolute address passed to
// SetProvider.
type GraphNodeProviderInstanceConsumer interface {
	GraphNodeModulePath
	// ProvidedBy returns the address of the provider configuration the node
	// refers to, if available. The following value types may be returned:
	//
	//   nil + exact true: the node does not require a provider
	// * addrs.LocalProviderConfig: the provider was set in the resource config
	// * addrs.AbsProviderConfig + exact true: the provider configuration was
	//   taken from the instance state.
	// * addrs.AbsProviderConfig + exact false: no config or state; the returned
	//   value is a default provider configuration address for the resource's
	//   Provider
	ProvidedBy() (addr addrs.ProviderInstance, exact bool)

	// Provider() returns the Provider FQN for the node.
	Provider() (provider addrs.Provider)

	// Set the resolved provider address for this resource.
	SetProvider(addrs.ConfigProviderInstance)
}

// ProviderInstanceTransformer is a GraphTransformer that maps resources to providers
// within the graph. This will error if there are any resources that don't map
// to proper resources.
type ProviderInstanceTransformer struct {
	Config *configs.Config
}

func (t *ProviderInstanceTransformer) Transform(g *Graph) error {
	// We need to find a provider configuration address for each resource
	// either directly represented by a node or referenced by a node in
	// the graph, and then create graph edges from provider to provider user
	// so that the providers will get initialized first.

	var diags tfdiags.Diagnostics

	// To start, we'll collect the _requested_ provider addresses for each
	// node, which we'll then resolve (handling provider inheritance, etc) in
	// the next step.
	// Our "requested" map is from graph vertices to string representations of
	// provider config addresses (for deduping) to requests.
	type ProviderRequest struct {
		Addr  addrs.ConfigProviderInstance
		Exact bool // If true, inheritance from parent modules is not attempted
	}
	requested := map[dag.Vertex]map[string]ProviderRequest{}
	needConfigured := map[string]addrs.ConfigProviderInstance{}
	for _, v := range g.Vertices() {
		// Does the vertex _directly_ use a provider?
		if pv, ok := v.(GraphNodeProviderInstanceConsumer); ok {
			providerAddr, exact := pv.ProvidedBy()
			if providerAddr == nil && exact {
				// no provider is required
				continue
			}

			requested[v] = make(map[string]ProviderRequest)

			var absPc addrs.ConfigProviderInstance

			switch p := providerAddr.(type) {
			case addrs.ConfigProviderInstance:
				// ProvidedBy() returns an AbsProviderConfig when the provider
				// configuration is set in state, so we do not need to verify
				// the FQN matches.
				absPc = p

				if exact {
					log.Printf("[TRACE] ProviderInstanceTransformer: %s is provided by %s exactly", dag.VertexName(v), absPc)
				}

			case addrs.LocalProviderInstance:
				// ProvidedBy() return a LocalProviderConfig when the resource
				// contains a `provider` attribute
				absPc.Provider = pv.Provider()
				modPath := pv.ModulePath()
				if t.Config == nil {
					absPc.Module = modPath
					absPc.Alias = p.Alias
					break
				}

				absPc.Module = modPath
				absPc.Alias = p.Alias

			default:
				// This should never happen; the case statements are meant to be exhaustive
				panic(fmt.Sprintf("%s: provider for %s couldn't be determined", dag.VertexName(v), absPc))
			}

			requested[v][absPc.String()] = ProviderRequest{
				Addr:  absPc,
				Exact: exact,
			}

			// Direct references need the provider configured as well as initialized
			needConfigured[absPc.String()] = absPc
		}
	}

	// Now we'll go through all the requested addresses we just collected and
	// figure out which _actual_ config address each belongs to, after resolving
	// for provider inheritance and passing.
	m := providerVertexMap(g)
	for v, reqs := range requested {
		for key, req := range reqs {
			p := req.Addr
			target := m[key]

			_, ok := v.(GraphNodeModulePath)
			if !ok && target == nil {
				// No target and no path to traverse up from
				diags = diags.Append(fmt.Errorf("%s: provider %s couldn't be found", dag.VertexName(v), p))
				continue
			}

			if target != nil {
				// Providers with configuration will already exist within the graph and can be directly referenced
				log.Printf("[TRACE] ProviderInstanceTransformer: exact match for %s serving %s", p, dag.VertexName(v))
			}

			// if we don't have a provider at this level, walk up the path looking for one,
			// unless we were told to be exact.
			if target == nil && !req.Exact {
				for pp, ok := p.Inherited(); ok; pp, ok = pp.Inherited() {
					key := pp.String()
					target = m[key]
					if target != nil {
						log.Printf("[TRACE] ProviderInstanceTransformer: %s uses inherited configuration %s", dag.VertexName(v), pp)
						break
					}
					log.Printf("[TRACE] ProviderInstanceTransformer: looking for %s to serve %s", pp, dag.VertexName(v))
				}
			}

			// If this provider doesn't need to be configured then we can just
			// stub it out with an init-only provider node, which will just
			// start up the provider and fetch its schema.
			if _, exists := needConfigured[key]; target == nil && !exists {
				stubAddr := addrs.ConfigProviderInstance{
					Module:   addrs.RootModule,
					Provider: p.Provider,
				}
				stub := &NodeEvalableProvider{
					&NodeAbstractProvider{
						Addr: stubAddr,
					},
				}
				m[stubAddr.String()] = stub
				log.Printf("[TRACE] ProviderInstanceTransformer: creating init-only node for %s", stubAddr)
				target = stub
				g.Add(target)
			}

			if target == nil {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Provider configuration not present",
					fmt.Sprintf(
						"To work with %s its original provider configuration at %s is required, but it has been removed. This occurs when a provider configuration is removed while objects created by that provider still exist in the state. Re-add the provider configuration to destroy %s, after which you can remove the provider configuration again.",
						dag.VertexName(v), p, dag.VertexName(v),
					),
				))
				break
			}

			// see if this is a proxy provider pointing to another concrete config
			if p, ok := target.(*graphNodeProxyProviderInstance); ok {
				g.Remove(p)
				target = p.Target()
			}

			log.Printf("[DEBUG] ProviderInstanceTransformer: %q (%T) needs %s", dag.VertexName(v), v, dag.VertexName(target))
			if pv, ok := v.(GraphNodeProviderInstanceConsumer); ok {
				pv.SetProvider(target.ProviderAddr())
			}
			g.Connect(dag.BasicEdge(v, target))
		}
	}

	return diags.Err()
}

// ProviderFunctionTransformer is a GraphTransformer that maps nodes which reference functions to providers
// within the graph. This will error if there are any provider functions that don't map to known providers.
type ProviderFunctionTransformer struct {
	Config *configs.Config
}

func (t *ProviderFunctionTransformer) Transform(g *Graph) error {
	var diags tfdiags.Diagnostics

	if t.Config == nil {
		// This is probably a test case, inherited from ProviderInstanceTransformer
		log.Printf("[WARN] Skipping provider function transformer due to missing config")
		return nil
	}

	// Locate all providers in the graph
	providers := providerVertexMap(g)

	type providerReference struct {
		path  string
		name  string
		alias string
	}
	// LuT of provider reference -> provider vertex
	providerReferences := make(map[providerReference]dag.Vertex)

	for _, v := range g.Vertices() {
		// Provider function references
		if nr, ok := v.(GraphNodeReferencer); ok && t.Config != nil {
			for _, ref := range nr.References() {
				if pf, ok := ref.Subject.(addrs.ProviderFunction); ok {
					key := providerReference{
						path:  nr.ModulePath().String(),
						name:  pf.ProviderName,
						alias: pf.ProviderAlias,
					}

					// We already know about this provider and can link directly
					if provider, ok := providerReferences[key]; ok {
						// Is it worth skipping if we have already connected this provider?
						g.Connect(dag.BasicEdge(v, provider))
						continue
					}

					// Find the config that this node belongs to
					mc := t.Config.Descendent(nr.ModulePath())
					if mc == nil {
						// I don't think this is possible
						diags = diags.Append(&hcl.Diagnostic{
							Severity: hcl.DiagError,
							Summary:  "Unknown Descendent Module",
							Detail:   nr.ModulePath().String(),
							Subject:  ref.SourceRange.ToHCL().Ptr(),
						})
						continue
					}

					// Find the provider type from required_providers
					pr, ok := mc.Module.ProviderRequirements.RequiredProviders[pf.ProviderName]
					if !ok {
						diags = diags.Append(&hcl.Diagnostic{
							Severity: hcl.DiagError,
							Summary:  "Unknown function provider",
							Detail:   fmt.Sprintf("Provider %q does not exist within the required_providers of this module", pf.ProviderName),
							Subject:  ref.SourceRange.ToHCL().Ptr(),
						})
						continue
					}

					// Build fully qualified provider address
					absPc := addrs.ConfigProviderInstance{
						Provider: pr.Type,
						Module:   nr.ModulePath(),
						Alias:    pf.ProviderAlias,
					}

					log.Printf("[TRACE] ProviderFunctionTransformer: %s in %s is provided by %s", pf, dag.VertexName(v), absPc)

					// Lookup provider via full address
					provider := providers[absPc.String()]

					if provider != nil {
						// Providers with configuration will already exist within the graph and can be directly referenced
						log.Printf("[TRACE] ProviderFunctionTransformer: exact match for %s serving %s", absPc, dag.VertexName(v))
					} else {
						// If this provider doesn't need to be configured then we can just
						// stub it out with an init-only provider node, which will just
						// start up the provider and fetch its schema.
						stubAddr := addrs.ConfigProviderInstance{
							Module:   addrs.RootModule,
							Provider: absPc.Provider,
						}
						if provider, ok = providers[stubAddr.String()]; !ok {
							stub := &NodeEvalableProvider{
								&NodeAbstractProvider{
									Addr: stubAddr,
								},
							}
							providers[stubAddr.String()] = stub
							log.Printf("[TRACE] ProviderFunctionTransformer: creating init-only node for %s", stubAddr)
							provider = stub
							g.Add(provider)
						}
					}

					// see if this is a proxy provider pointing to another concrete config
					if p, ok := provider.(*graphNodeProxyProviderInstance); ok {
						g.Remove(p)
						provider = p.Target()
					}

					log.Printf("[DEBUG] ProviderFunctionTransformer: %q (%T) needs %s", dag.VertexName(v), v, dag.VertexName(provider))
					g.Connect(dag.BasicEdge(v, provider))

					// Save for future lookups
					providerReferences[key] = provider
				}
			}
		}
	}

	return diags.Err()
}

// CloseProviderInstanceTransformer is a GraphTransformer that adds nodes to the
// graph that will close any provider instances that aren't needed anymore.
// A provider instance is not needed anymore once all dependent resources
// in the graph have been visited.
type CloseProviderInstanceTransformer struct{}

func (t *CloseProviderInstanceTransformer) Transform(g *Graph) error {
	pm := providerVertexMap(g)
	cpm := make(map[string]*graphNodeCloseProviderInstance)
	var err error

	for _, p := range pm {
		key := p.ProviderAddr().String()

		// get the close provider of this type if we already created it
		closer := cpm[key]

		if closer == nil {
			// create a closer for this provider type
			closer = &graphNodeCloseProviderInstance{Addr: p.ProviderAddr()}
			g.Add(closer)
			cpm[key] = closer
		}

		// Close node depends on the provider itself
		// this is added unconditionally, so it will connect to all instances
		// of the provider. Extra edges will be removed by transitive
		// reduction.
		g.Connect(dag.BasicEdge(closer, p))

		// connect all the provider's resources to the close node
		for _, s := range g.UpEdges(p) {
			if _, ok := s.(GraphNodeProviderInstanceConsumer); ok {
				g.Connect(dag.BasicEdge(closer, s))
			} else if _, ok := s.(GraphNodeReferencer); ok {
				g.Connect(dag.BasicEdge(closer, s))
			}
		}
	}

	return err
}

// MissingProviderInstanceTransformer is a GraphTransformer that adds to the graph
// a node for each default provider instance that is referenced by another
// node but not already present in the graph.
//
// These "default" nodes are always added to the root module, regardless of
// where they are requested. This is important because our inheritance
// resolution behavior in ProviderInstanceTransformer will then treat these as a
// last-ditch fallback after walking up the tree, rather than preferring them
// as it would if they were placed in the same module as the requester.
//
// This transformer may create extra nodes that are not needed in practice,
// due to overriding provider configurations in child modules.
// PruneProviderInstanceTransformer can then remove these once ProviderInstanceTransformer
// has resolved all of the inheritance, etc.
type MissingProviderInstanceTransformer struct {
	// MissingProviderInstanceTransformer needs the config to rule out _implied_ default providers
	Config *configs.Config

	// Concrete, if set, overrides how the providers are made.
	Concrete ConcreteProviderNodeFunc
}

func (t *MissingProviderInstanceTransformer) Transform(g *Graph) error {
	// Initialize factory
	if t.Concrete == nil {
		t.Concrete = func(a *NodeAbstractProvider) dag.Vertex {
			return a
		}
	}

	var err error
	m := providerVertexMap(g)
	for _, v := range g.Vertices() {
		pv, ok := v.(GraphNodeProviderInstanceConsumer)
		if !ok {
			continue
		}

		// For our work here we actually care only about the provider type and
		// we plan to place all default providers in the root module.
		providerFqn := pv.Provider()

		// We're going to create an implicit _default_ configuration for the
		// referenced provider type in the _root_ module, ignoring all other
		// aspects of the resource's declared provider address.
		defaultAddr := addrs.RootModuleInstance.ProviderConfigDefault(providerFqn)
		key := defaultAddr.String()
		provider := m[key]

		if provider != nil {
			// There's already an explicit default configuration for this
			// provider type in the root module, so we have nothing to do.
			continue
		}

		log.Printf("[DEBUG] adding implicit provider configuration %s, implied first by %s", defaultAddr, dag.VertexName(v))

		// create the missing top-level provider
		provider = t.Concrete(&NodeAbstractProvider{
			Addr: defaultAddr,
		}).(GraphNodeProviderInstance)

		g.Add(provider)
		m[key] = provider
	}

	return err
}

// PruneProviderInstanceTransformer removes any providers that are not actually used by
// anything, and provider proxies. This avoids the provider being initialized
// and configured.  This both saves resources but also avoids errors since
// configuration may imply initialization which may require auth.
type PruneProviderInstanceTransformer struct{}

func (t *PruneProviderInstanceTransformer) Transform(g *Graph) error {
	for _, v := range g.Vertices() {
		// We only care about providers
		_, ok := v.(GraphNodeProviderInstance)
		if !ok {
			continue
		}

		// ProxyProviders will have up edges, but we're now done with them in the graph
		if _, ok := v.(*graphNodeProxyProviderInstance); ok {
			log.Printf("[DEBUG] pruning proxy %s", dag.VertexName(v))
			g.Remove(v)
		}

		// Remove providers with no dependencies.
		if g.UpEdges(v).Len() == 0 {
			log.Printf("[DEBUG] pruning unused %s", dag.VertexName(v))
			g.Remove(v)
		}
	}

	return nil
}

func providerVertexMap(g *Graph) map[string]GraphNodeProviderInstance {
	m := make(map[string]GraphNodeProviderInstance)
	for _, v := range g.Vertices() {
		if pv, ok := v.(GraphNodeProviderInstance); ok {
			addr := pv.ProviderAddr()
			m[addr.String()] = pv
		}
	}

	return m
}

type graphNodeCloseProviderInstance struct {
	Addr addrs.ConfigProviderInstance
}

var (
	_ GraphNodeCloseProviderInstance = (*graphNodeCloseProviderInstance)(nil)
	_ GraphNodeExecutable            = (*graphNodeCloseProviderInstance)(nil)
)

func (n *graphNodeCloseProviderInstance) Name() string {
	return n.Addr.String() + " (close)"
}

// GraphNodeModulePath
func (n *graphNodeCloseProviderInstance) ModulePath() addrs.Module {
	return n.Addr.Module
}

// GraphNodeExecutable impl.
func (n *graphNodeCloseProviderInstance) Execute(ctx EvalContext, op walkOperation) (diags tfdiags.Diagnostics) {
	return diags.Append(ctx.CloseProvider(n.Addr))
}

func (n *graphNodeCloseProviderInstance) CloseProviderAddr() addrs.ConfigProviderInstance {
	return n.Addr
}

// GraphNodeDotter impl.
func (n *graphNodeCloseProviderInstance) DotNode(name string, opts *dag.DotOpts) *dag.DotNode {
	if !opts.Verbose {
		return nil
	}
	return &dag.DotNode{
		Name: name,
		Attrs: map[string]string{
			"label": n.Name(),
			"shape": "diamond",
		},
	}
}

// graphNodeProxyProviderInstance is a GraphNodeProvider implementation that is used to
// store the name and value of a provider node for inheritance between modules.
// These nodes are only used to store the data while loading the provider
// configurations, and are removed after all the resources have been connected
// to their providers.
type graphNodeProxyProviderInstance struct {
	addr   addrs.ConfigProviderInstance
	target GraphNodeProviderInstance
}

var (
	_ GraphNodeModulePath       = (*graphNodeProxyProviderInstance)(nil)
	_ GraphNodeProviderInstance = (*graphNodeProxyProviderInstance)(nil)
)

func (n *graphNodeProxyProviderInstance) ProviderAddr() addrs.ConfigProviderInstance {
	return n.addr
}

func (n *graphNodeProxyProviderInstance) ModulePath() addrs.Module {
	return n.addr.Module
}

func (n *graphNodeProxyProviderInstance) Name() string {
	return n.addr.String() + " (proxy)"
}

// find the concrete provider instance
func (n *graphNodeProxyProviderInstance) Target() GraphNodeProviderInstance {
	switch t := n.target.(type) {
	case *graphNodeProxyProviderInstance:
		return t.Target()
	default:
		return n.target
	}
}
