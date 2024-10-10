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
		&ProviderConfigTransformer{
			Config:   config,
			Concrete: concrete,
		},
		// Add any remaining missing providers
		&MissingProviderTransformer{
			Config:   config,
			Concrete: concrete,
		},
		// Connect the providers
		&ProviderTransformer{
			Config: config,
		},
		// The following comment shows what must be added to the transformer list after the schema transformer
		// After schema transformer, we can add function references
		//  &ProviderFunctionTransformer{Config: config},
		// Remove unused providers and proxies
		//  &PruneProviderTransformer{},
	)
}

// GraphNodeProvider is an interface that nodes that can be a provider
// must implement.
//
// ProviderAddr returns the address of the provider configuration this
// satisfies, which is relative to the path returned by method Path().
//
// Name returns the full name of the provider in the config.
type GraphNodeProvider interface {
	GraphNodeModulePath
	ProviderAddr() addrs.AbsProviderConfig
	Name() string
}

// GraphNodeCloseProvider is an interface that nodes that can be a close
// provider must implement. The CloseProviderName returned is the name of
// the provider they satisfy.
type GraphNodeCloseProvider interface {
	GraphNodeModulePath
	CloseProviderAddr() addrs.AbsProviderConfig
}

type ExactProvider struct {
	isResourceProvider bool
	provider           addrs.AbsProviderConfig
}

// GraphNodeProviderConsumer is an interface that nodes that require
// a provider must implement. ProvidedBy must return the address of the provider
// to use, which will be resolved to a configuration either in the same module
// or in an ancestor module, with the resulting absolute address passed to
// SetProvider.
type GraphNodeProviderConsumer interface {
	GraphNodeModulePath
	// ProvidedBy returns the address of the provider configuration the node
	// refers to, if available. The following value types may be returned:
	//
	//   nil + empty ExactProvider{}: the node does not require a provider
	// * addrs.LocalProviderConfig + empty ExactProvider{}: the provider was set in the resource config
	// * empty addrs.AbsProviderConfig + non-empty ExactProvider{}: the provider configuration was
	//   taken from the instance state or previously calculated in previous runs of the provider transformer.
	// * addrs.AbsProviderConfig + empty ExactProvider{}: no config or state; the returned
	//   value is a default provider configuration address for the resource's
	//   Provider
	ProvidedBy() (addr map[addrs.InstanceKey]addrs.ProviderConfig, exactProvider ExactProvider)

	// Provider returns the Provider FQN for the node.
	Provider() (provider addrs.Provider)

	// SetProvider Set the resolved provider address for the resource / instance
	SetProvider(provider addrs.AbsProviderConfig, isResourceProvider bool)

	// SetPotentialProviders sets the potential providers to be resolved later, after the expansion of instances.
	// Sometimes we won't know the exact provider, and will have a list of potential providers that can be resolved once
	// the instances are expanded and known.
	SetPotentialProviders(potentialProviders ResourceInstanceProviderResolver)
}

// ProviderTransformer is a GraphTransformer that maps resources to providers
// within the graph. This will error if there are any resources that don't map
// to proper resources.
type ProviderTransformer struct {
	Config *configs.Config
}

func (t *ProviderTransformer) Transform(g *Graph) error {
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
		Addr        addrs.AbsProviderConfig
		Exact       ExactProvider // If populated, inheritance from parent modules is not attempted
		instanceKey addrs.InstanceKey
	}
	requested := map[dag.Vertex]map[string]ProviderRequest{}
	needConfigured := map[string]addrs.AbsProviderConfig{}
	for _, v := range g.Vertices() {
		// Does the vertex _directly_ use a provider?
		if pv, ok := v.(GraphNodeProviderConsumer); ok {
			providerAddrs, exactProvider := pv.ProvidedBy()

			if providerAddrs == nil && !exactProvider.provider.IsSet() {
				// no provider is required
				continue
			}

			requested[v] = make(map[string]ProviderRequest)

			if exactProvider.provider.IsSet() {
				providerAddr := exactProvider.provider
				log.Printf("[TRACE] ProviderTransformer: %s is provided by %s exactly", dag.VertexName(v), providerAddr)

				requested[v][providerAddr.String()] = ProviderRequest{
					Addr:  addrs.AbsProviderConfig{},
					Exact: exactProvider,
					// If we are providing an exact provider, we won't need to use the instanceKey prop on the request.
					// That's why we can set addrs.NoKey as the instanceKey.
					instanceKey: addrs.NoKey,
				}

				// Direct references need the provider configured as well as initialized
				needConfigured[providerAddr.String()] = providerAddr
			} else {
				for ik, providerAddr := range providerAddrs {
					var absPc addrs.AbsProviderConfig

					switch p := providerAddr.(type) {
					case addrs.AbsProviderConfig:
						// ProvidedBy() returns an AbsProviderConfig when the provider
						// configuration is implied from the resource, so we do not need to verify
						// the FQN matches.
						absPc = p

					case addrs.LocalProviderConfig:
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
						Addr:        absPc,
						Exact:       ExactProvider{},
						instanceKey: ik,
					}

					// Direct references need the provider configured as well as initialized
					needConfigured[absPc.String()] = absPc
				}
			}
		}
	}

	// Now we'll go through all the requested addresses we just collected and
	// figure out which _actual_ config address each belongs to, after resolving
	// for provider inheritance and passing.
	m := providerVertexMap(g)
	for v, reqs := range requested {
		// Mapping from resource.InstanceKey to one of many ModuleInstancePotentialProvider
		potentialTargets := make(ResourceInstanceProviderResolver)

		for key, req := range reqs {
			p := req.Addr
			target := m[key]

			_, ok := v.(GraphNodeModulePath)
			if !ok && target == nil {
				// No target and no path to traverse up from
				diags = diags.Append(fmt.Errorf("%s: provider %s couldn't be found", dag.VertexName(v), p))
				continue
			}

			// This is a pretty odd edge case.
			// The only way in which this can be triggered (as far as I understand) is
			// by removing a resource and provider from a module, where the resource
			// uses the provider in that module, then altering the module's provider
			// block to supply a *different* provider configuration to perform
			// the deletion. Effectively swapping out the explicit provider that a
			// resource needs by manipulating the provider graph.
			// The resource in the module should be deleted before removing the provider
			// configuration in that module as the state has explicitly said that the
			// exact provider configuration is required to manipulate the resource.
			//
			// We may want to consider removing this option and replace it with an
			// error instead.
			if p, ok := target.(*graphNodeProxyProvider); ok {
				target = p.Target()
			}

			isRequestedExactProvider := req.Exact.provider.IsSet()

			// if we don't have a provider at this level, walk up the path looking for one,
			// unless we were told to be exact.
			if target == nil && !isRequestedExactProvider {
				for pp, ok := p.Inherited(); ok; pp, ok = pp.Inherited() {
					key := pp.String()
					target = m[key]
					if target != nil {
						log.Printf("[TRACE] ProviderTransformer: %s uses inherited configuration %s", dag.VertexName(v), pp)
						break
					}
					log.Printf("[TRACE] ProviderTransformer: looking for %s to serve %s", pp, dag.VertexName(v))
				}
			}

			if target == nil {
				// This error message is possibly misleading?
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Provider configuration not present",
					fmt.Sprintf(
						"To work with %s its original provider configuration at %s is required, but it has been removed. This occurs when a provider configuration is removed while objects created by that provider still exist in the state. Re-add the provider configuration to destroy %s, after which you can remove the provider configuration again.",
						dag.VertexName(v), fullProviderAddr, dag.VertexName(v),
					),
				))
				continue
			}

			// If exact is true, it means we are in one in two scenarios:
			// 1. The provider transformer is running after the resources were expanded to instances, and the instances
			// already resolved their providers and the provider level (provider is set for the resource or for the
			// instance).
			// 2. This resource is not in the configuration, so the providers were taken from the state.
			//
			// In both of those cases, as the providers were previously resolved, we don't need to go through the whole
			// process of resolving them again. We'll set them straight on the resource and use the given
			// "isResourceProvider", to set the provider on the already known provider level.
			// We are also breaking the loop and stopping to process this request, as we don't need to proceed and
			// calculate the potentialTargets, as we already know the resolved provider.
			if isRequestedExactProvider {
				if pv, ok := v.(GraphNodeProviderConsumer); ok {
					log.Printf("[DEBUG] ProviderTransformer: %q (%T) needs %s", dag.VertexName(v), v, dag.VertexName(target))

					if _, ok := target.(*graphNodeProxyProvider); ok {
						panic(fmt.Sprintf("%s: exact provider target cannot be from type graphNodeProxyProvider, it should have already been a concrete provider", dag.VertexName(v)))
					}

					log.Printf("[DEBUG] ProviderTransformer: %q (%T) needs the exact provider %s", dag.VertexName(v), v, dag.VertexName(target))
					g.Connect(dag.BasicEdge(v, target))
					pv.SetProvider(target.ProviderAddr(), req.Exact.isResourceProvider)

					break
				}

			}

			// Module expansions for this particular resource InstanceKey
			var moduleExpansionProviders []ModuleInstancePotentialProvider

			// see if this is a proxy provider pointing to another concrete config
			if p, ok := target.(*graphNodeProxyProvider); ok {
				g.Remove(p)

				moduleExpansionProviders = p.Expanded()
				for _, pp := range moduleExpansionProviders {
					log.Printf("[DEBUG] ProviderTransformer: %q (%T) needs the potential provider %s", dag.VertexName(v), v, dag.VertexName(pp))
					g.Connect(dag.BasicEdge(v, pp.concreteProvider))
				}
			} else {
				log.Printf("[DEBUG] ProviderTransformer: %q (%T) needs the potential provider %s", dag.VertexName(v), v, dag.VertexName(target))
				moduleExpansionProviders = []ModuleInstancePotentialProvider{{
					moduleIdentifier: nil, // TODO should this construct the actual module path here?
					concreteProvider: target,
				}}
				g.Connect(dag.BasicEdge(v, target))
			}

			potentialTargets[req.instanceKey] = moduleExpansionProviders
		}

		if pv, ok := v.(GraphNodeProviderConsumer); ok {
			pv.SetPotentialProviders(potentialTargets)

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
		// This is probably a test case, inherited from ProviderTransformer
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
					absPc := addrs.AbsProviderConfig{
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
						stubAddr := addrs.AbsProviderConfig{
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
					if p, ok := provider.(*graphNodeProxyProvider); ok {
						g.Remove(p)

						potentialProviders := p.Expanded()

						for i := range potentialProviders {
							pp := potentialProviders[i]

							log.Printf("[DEBUG] ProviderFunctionTransformer: %q (%T) needs potential provider %s", dag.VertexName(v), v, dag.VertexName(provider))
							g.Connect(dag.BasicEdge(v, pp.concreteProvider))
						}
					} else {
						log.Printf("[DEBUG] ProviderFunctionTransformer: %q (%T) needs concrete provider %s", dag.VertexName(v), v, dag.VertexName(provider))
						g.Connect(dag.BasicEdge(v, provider))
					}

					// Save for future lookups
					providerReferences[key] = provider
				}
			}
		}
	}

	return diags.Err()
}

// CloseProviderTransformer is a GraphTransformer that adds nodes to the
// graph that will close open provider connections that aren't needed anymore.
// A provider connection is not needed anymore once all depended resources
// in the graph are evaluated.
type CloseProviderTransformer struct{}

func (t *CloseProviderTransformer) Transform(g *Graph) error {
	pm := providerVertexMap(g)
	cpm := make(map[string]*graphNodeCloseProvider)
	var err error

	for _, p := range pm {
		key := p.ProviderAddr().String()

		// get the close provider of this type if we already created it
		closer := cpm[key]

		if closer == nil {
			// create a closer for this provider type
			closer = &graphNodeCloseProvider{Addr: p.ProviderAddr()}
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
			if _, ok := s.(GraphNodeProviderConsumer); ok {
				g.Connect(dag.BasicEdge(closer, s))
			} else if _, ok := s.(GraphNodeReferencer); ok {
				g.Connect(dag.BasicEdge(closer, s))
			}
		}
	}

	return err
}

// MissingProviderTransformer is a GraphTransformer that adds to the graph
// a node for each default provider configuration that is referenced by another
// node but not already present in the graph.
//
// These "default" nodes are always added to the root module, regardless of
// where they are requested. This is important because our inheritance
// resolution behavior in ProviderTransformer will then treat these as a
// last-ditch fallback after walking up the tree, rather than preferring them
// as it would if they were placed in the same module as the requester.
//
// This transformer may create extra nodes that are not needed in practice,
// due to overriding provider configurations in child modules.
// PruneProviderTransformer can then remove these once ProviderTransformer
// has resolved all of the inheritance, etc.
type MissingProviderTransformer struct {
	// MissingProviderTransformer needs the config to rule out _implied_ default providers
	Config *configs.Config

	// Concrete, if set, overrides how the providers are made.
	Concrete ConcreteProviderNodeFunc
}

func (t *MissingProviderTransformer) Transform(g *Graph) error {
	// Initialize factory
	if t.Concrete == nil {
		t.Concrete = func(a *NodeAbstractProvider) dag.Vertex {
			return a
		}
	}

	var err error
	m := providerVertexMap(g)
	for _, v := range g.Vertices() {
		pv, ok := v.(GraphNodeProviderConsumer)
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
		}).(GraphNodeProvider)

		g.Add(provider)
		m[key] = provider
	}

	return err
}

// PruneProviderTransformer removes any providers that are not actually used by
// anything, and provider proxies. This avoids the provider being initialized
// and configured.  This both saves resources but also avoids errors since
// configuration may imply initialization which may require auth.
type PruneProviderTransformer struct{}

func (t *PruneProviderTransformer) Transform(g *Graph) error {
	for _, v := range g.Vertices() {
		// We only care about providers
		_, ok := v.(GraphNodeProvider)
		if !ok {
			continue
		}

		// ProxyProviders will have up edges, but we're now done with them in the graph
		if _, ok := v.(*graphNodeProxyProvider); ok {
			log.Printf("[DEBUG] pruning proxy %s", dag.VertexName(v))
			g.Remove(v)
			continue
		}

		// Remove providers with no dependencies.
		if g.UpEdges(v).Len() == 0 {
			log.Printf("[DEBUG] pruning unused %s", dag.VertexName(v))
			g.Remove(v)
		}
	}

	return nil
}

func providerVertexMap(g *Graph) map[string]GraphNodeProvider {
	m := make(map[string]GraphNodeProvider)
	for _, v := range g.Vertices() {
		if pv, ok := v.(GraphNodeProvider); ok {
			addr := pv.ProviderAddr()
			m[addr.String()] = pv
		}
	}

	return m
}

type graphNodeCloseProvider struct {
	Addr addrs.AbsProviderConfig
}

var (
	_ GraphNodeCloseProvider = (*graphNodeCloseProvider)(nil)
	_ GraphNodeExecutable    = (*graphNodeCloseProvider)(nil)
)

func (n *graphNodeCloseProvider) Name() string {
	return n.Addr.String() + " (close)"
}

// GraphNodeModulePath
func (n *graphNodeCloseProvider) ModulePath() addrs.Module {
	return n.Addr.Module
}

// GraphNodeExecutable impl.
func (n *graphNodeCloseProvider) Execute(ctx EvalContext, op walkOperation) (diags tfdiags.Diagnostics) {
	return diags.Append(ctx.CloseProvider(n.Addr))
}

func (n *graphNodeCloseProvider) CloseProviderAddr() addrs.AbsProviderConfig {
	return n.Addr
}

// GraphNodeDotter impl.
func (n *graphNodeCloseProvider) DotNode(name string, opts *dag.DotOpts) *dag.DotNode {
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

type ResourceInstanceProviderResolver map[addrs.InstanceKey]ModuleInstanceProviderResolver

func (r ResourceInstanceProviderResolver) Any() addrs.AbsProviderConfig {
	for _, m := range r {
		for _, p := range m {
			return p.concreteProvider.ProviderAddr()
		}
	}
	panic("unreachable")
}

func (r ResourceInstanceProviderResolver) Resolve(addr addrs.AbsResourceInstance) addrs.AbsProviderConfig {
	// First check for an exact match on the resource instance key
	if p, ok := r[addr.Resource.Key]; ok {
		return p.Resolve(addr.Module)
	}
	// If the resource instance key is not an exact match, the other possibility is that there is no
	// alternate mapping to ModuleInstanceProviderResovlers based on the provider key
	if p, ok := r[addrs.NoKey]; ok {
		return p.Resolve(addr.Module)
	}
	panic("TODO better message here")
}

type ModuleInstanceProviderResolver []ModuleInstancePotentialProvider

func (m ModuleInstanceProviderResolver) Resolve(addr addrs.ModuleInstance) addrs.AbsProviderConfig {
	for _, pr := range m {
		if pr.ModuleInstanceMatches(addr) {
			return pr.concreteProvider.ProviderAddr()
		}
	}
	panic("TODO better message here")
}

// ModuleInstancePotentialProvider is a representation of a concrete provider and identifiers that match the provider to a
// specific module instance. Because we might have for_each or count on providers in the module, we cannot
// calculate the concrete provider per instance in stages of building the graph; we can only do that after the
// expansion of instances (while walking the graph). So, this structure will be passed on to the resource as potential
// providers and resolved using IsResourceInstanceMatching() once we get to the expansion stage.
type ModuleInstancePotentialProvider struct {
	moduleIdentifier []addrs.ModuleInstanceStep
	concreteProvider GraphNodeProvider // Ronny TODO: I don't like that this GraphNodeProvider type is eventually leaking into the node abstract resource itself
}

func (d *ModuleInstancePotentialProvider) AddModuleIdentifierToTheEnd(step addrs.ModuleInstanceStep) {
	d.moduleIdentifier = append(d.moduleIdentifier, step)
}

func (d *ModuleInstancePotentialProvider) ModuleInstanceMatches(moduleInstance addrs.ModuleInstance) bool {
	for i, moduleInstanceStep := range d.moduleIdentifier {
		parallelResourceModuleAddress := moduleInstance[i]

		if moduleInstanceStep.Name != parallelResourceModuleAddress.Name {
			return false
		}

		if moduleInstanceStep.InstanceKey != nil && moduleInstanceStep.InstanceKey != parallelResourceModuleAddress.InstanceKey {
			return false
		}
	}

	return true
}

// graphNodeProxyProvider is a GraphNodeProvider implementation that is used to
// store the name and value of a provider node for inheritance between modules.
// These nodes are only used to store the data while loading the provider
// configurations, and are removed after all the resources have been connected
// to their providers.
type graphNodeProxyProvider struct {
	addr    addrs.AbsProviderConfig
	targets map[addrs.InstanceKey]GraphNodeProvider
}

var (
	_ GraphNodeModulePath = (*graphNodeProxyProvider)(nil)
	_ GraphNodeProvider   = (*graphNodeProxyProvider)(nil)
)

func (n *graphNodeProxyProvider) ProviderAddr() addrs.AbsProviderConfig {
	return n.addr
}

func (n *graphNodeProxyProvider) ModulePath() addrs.Module {
	return n.addr.Module
}

func (n *graphNodeProxyProvider) Name() string {
	return n.addr.String() + " (proxy)"
}

// Expanded recurses over the graphNodeProxyProvider, trying to find all the possible concrete providers.
// Because we might have for_each on providers in the resource/module, we cannot calculate the concrete provider per
// instance in this part of building the graph, we can only do that after the expansion of instances. That is why, in
// here, we are returning an array of ModuleInstancePotentialProvider, each with identifiers that later will help us match a
// specific resource instance to a specific concrete provider.
func (n *graphNodeProxyProvider) Expanded() []ModuleInstancePotentialProvider {
	var concrete []ModuleInstancePotentialProvider

	for ik, target := range n.targets {
		var targetConcrete []ModuleInstancePotentialProvider

		if t, ok := target.(*graphNodeProxyProvider); ok {
			// We want to add all the modules we went through during the recursion as part of the ModuleIdentifier
			currModuleIdentifier := addrs.ModuleInstanceStep{
				Name:        n.ModulePath()[len(n.ModulePath())-1],
				InstanceKey: ik,
			}

			providers := t.Expanded()
			for i := range providers {
				providers[i].AddModuleIdentifierToTheEnd(currModuleIdentifier)
			}
			targetConcrete = append(targetConcrete, providers...)
		} else {
			// We hit a concrete provider
			modulePath := addrs.ModuleInstanceStep{
				Name:        n.ModulePath()[len(n.ModulePath())-1],
				InstanceKey: ik,
			}
			targetConcrete = []ModuleInstancePotentialProvider{{
				moduleIdentifier: []addrs.ModuleInstanceStep{modulePath},
				concreteProvider: target,
			}}
		}

		concrete = append(concrete, targetConcrete...)
	}

	return concrete
}

// ProviderConfigTransformer adds all provider nodes from the configuration and
// attaches the configs.
type ProviderConfigTransformer struct {
	Concrete ConcreteProviderNodeFunc

	// each provider node is stored here so that the proxy nodes can look up
	// their targets by name.
	providers map[string]GraphNodeProvider
	// record providers that can be overridden with a proxy
	proxiable map[string]bool

	// Config is the root node of the configuration tree to add providers from.
	Config *configs.Config
}

func (t *ProviderConfigTransformer) Transform(g *Graph) error {
	// If no configuration is given, we don't do anything
	if t.Config == nil {
		return nil
	}

	t.providers = make(map[string]GraphNodeProvider)
	t.proxiable = make(map[string]bool)

	// Start the transformation process
	if err := t.transform(g, t.Config); err != nil {
		return err
	}

	// finally attach the configs to the new nodes
	return t.attachProviderConfigs(g)
}

func (t *ProviderConfigTransformer) transform(g *Graph, c *configs.Config) error {
	// If no config, do nothing
	if c == nil {
		return nil
	}

	// Add our resources
	if err := t.transformSingle(g, c); err != nil {
		return err
	}

	// Transform all the children.
	for _, cc := range c.Children {
		if err := t.transform(g, cc); err != nil {
			return err
		}
	}
	return nil
}

func (t *ProviderConfigTransformer) transformSingle(g *Graph, c *configs.Config) error {
	// Get the module associated with this configuration tree node
	mod := c.Module
	path := c.Path

	// If this is the root module, we can add nodes for required providers that
	// have no configuration, equivalent to having an empty configuration
	// block. This will ensure that a provider node exists for modules to
	// access when passing around configuration and inheritance.
	if path.IsRoot() && c.Module.ProviderRequirements != nil {
		for name, p := range c.Module.ProviderRequirements.RequiredProviders {
			if _, configured := mod.ProviderConfigs[name]; configured {
				continue
			}

			addr := addrs.AbsProviderConfig{
				Provider: p.Type,
				Module:   path,
			}

			if _, ok := t.providers[addr.String()]; ok {
				// The config validation warns about this too, but we can't
				// completely prevent it in v1.
				log.Printf("[WARN] ProviderConfigTransformer: duplicate required_providers entry for %s", addr)
				continue
			}

			abstract := &NodeAbstractProvider{
				Addr: addr,
			}

			var v dag.Vertex
			if t.Concrete != nil {
				v = t.Concrete(abstract)
			} else {
				v = abstract
			}

			g.Add(v)
			t.providers[addr.String()] = v.(GraphNodeProvider)
		}
	}

	// add all providers from the configuration
	for _, p := range mod.ProviderConfigs {
		fqn := mod.ProviderForLocalConfig(p.Addr())
		addr := addrs.AbsProviderConfig{
			Provider: fqn,
			Alias:    p.Alias,
			Module:   path,
		}

		if _, ok := t.providers[addr.String()]; ok {
			// The abstract provider node may already have been added from the
			// provider requirements.
			log.Printf("[WARN] ProviderConfigTransformer: provider node %s already added", addr)
			continue
		}

		abstract := &NodeAbstractProvider{
			Addr: addr,
		}
		var v dag.Vertex
		if t.Concrete != nil {
			v = t.Concrete(abstract)
		} else {
			v = abstract
		}

		// Add it to the graph
		g.Add(v)
		key := addr.String()
		t.providers[key] = v.(GraphNodeProvider)

		// While deprecated, we still accept empty configuration blocks within
		// modules as being a possible proxy for passed configuration.
		if !path.IsRoot() {
			// A provider configuration is "proxyable" if its configuration is
			// entirely empty. This means it's standing in for a provider
			// configuration that must be passed in from the parent module.
			// We decide this by evaluating the config with an empty schema;
			// if this succeeds, then we know there's nothing in the body.
			_, diags := p.Config.Content(&hcl.BodySchema{})
			t.proxiable[key] = !diags.HasErrors()
		}
	}

	// Now replace the provider nodes with proxy nodes if a provider was being
	// passed in, and create implicit proxies if there was no config. Any extra
	// proxies will be removed in the prune step.
	return t.addProxyProviders(g, c)
}

func (t *ProviderConfigTransformer) addProxyProviders(g *Graph, c *configs.Config) error {
	path := c.Path

	// can't add proxies at the root
	if path.IsRoot() {
		return nil
	}

	parentPath, callAddr := path.Call()
	parent := c.Parent
	if parent == nil {
		return nil
	}

	callName := callAddr.Name
	var parentCfg *configs.ModuleCall
	for name, mod := range parent.Module.ModuleCalls {
		if name == callName {
			parentCfg = mod
			break
		}
	}

	if parentCfg == nil {
		// this can't really happen during normal execution.
		return fmt.Errorf("parent module config not found for %s", c.Path.String())
	}

	// Go through all the providers the parent is passing in, and add proxies to
	// the parent provider nodes.
	for _, pair := range parentCfg.Providers {
		fqn := c.Module.ProviderForLocalConfig(pair.InChild.Addr())
		fullAddr := addrs.AbsProviderConfig{
			Provider: fqn,
			Module:   path,
			Alias:    pair.InChild.Addr().Alias,
		}

		fullName := fullAddr.String()

		proxy := &graphNodeProxyProvider{
			addr:    fullAddr,
			targets: make(map[addrs.InstanceKey]GraphNodeProvider),
		}

		// Build the proxy provider
		if len(pair.InParentMapping.Aliases) > 0 {
			// If we have aliases set in InParentMapping, we'll calculate a target for each one
			for key, alias := range pair.InParentMapping.Aliases {
				fullParentAddr := addrs.AbsProviderConfig{
					Provider: fqn,
					Module:   parentPath,
					Alias:    alias,
				}
				fullParentName := fullParentAddr.String()

				parentProvider := t.providers[fullParentName]

				if parentProvider == nil {
					return fmt.Errorf("missing provider %s", fullParentName)
				}

				proxy.targets[key] = parentProvider
			}
		} else {
			// If we have no aliases in InParentMapping, we still need to calculate a single target
			fullParentAddr := addrs.AbsProviderConfig{
				Provider: fqn,
				Module:   parentPath,
				Alias:    "",
			}

			fullParentName := fullParentAddr.String()
			parentProvider := t.providers[fullParentName]

			if parentProvider == nil {
				return fmt.Errorf("missing provider %s", fullParentName)
			}

			proxy.targets[addrs.NoKey] = parentProvider
		}

		concreteProvider := t.providers[fullName]

		// replace the concrete node with the provider passed in only if it is
		// proxyable
		if concreteProvider != nil {
			if t.proxiable[fullName] {
				g.Replace(concreteProvider, proxy)
				t.providers[fullName] = proxy
			}
			continue
		}

		// There was no concrete provider, so add this as an implicit provider.
		// The extra proxy will be pruned later if it's unused.
		g.Add(proxy)
		t.providers[fullName] = proxy
	}

	return nil
}

func (t *ProviderConfigTransformer) attachProviderConfigs(g *Graph) error {
	for _, v := range g.Vertices() {
		// Only care about GraphNodeAttachProvider implementations
		apn, ok := v.(GraphNodeAttachProvider)
		if !ok {
			continue
		}

		// Determine what we're looking for
		addr := apn.ProviderAddr()

		// Get the configuration.
		mc := t.Config.Descendent(addr.Module)
		if mc == nil {
			log.Printf("[TRACE] ProviderConfigTransformer: no configuration available for %s", addr.String())
			continue
		}

		// Find the localName for the provider fqn
		localName := mc.Module.LocalNameForProvider(addr.Provider)

		// Go through the provider configs to find the matching config
		for _, p := range mc.Module.ProviderConfigs {
			if p.Name == localName && p.Alias == addr.Alias {
				log.Printf("[TRACE] ProviderConfigTransformer: attaching to %q provider configuration from %s", dag.VertexName(v), p.DeclRange)
				apn.AttachProvider(p)
				break
			}
		}
	}

	return nil
}
