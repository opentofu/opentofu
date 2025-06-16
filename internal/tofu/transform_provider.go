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
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/dag"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func transformProviders(concrete ConcreteProviderNodeFunc, config *configs.Config, walkOp walkOperation) GraphTransformer {
	return GraphTransformMulti(
		// Add providers from the config
		&ProviderConfigTransformer{
			Config:    config,
			Concrete:  concrete,
			Operation: walkOp,
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

type RequestedProvider struct {
	ProviderConfig addrs.ProviderConfig
	KeyExpression  hcl.Expression
	KeyModule      addrs.Module
	KeyResource    bool
	KeyExact       addrs.InstanceKey
}

type ResolvedProvider struct {
	ProviderConfig addrs.AbsProviderConfig
	KeyExpression  hcl.Expression
	KeyModule      addrs.Module
	KeyResource    bool
	KeyExact       addrs.InstanceKey
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
	//   nil: the node does not require a provider
	// * addrs.LocalProviderConfig: the provider should be looked up in
	//   the current module path. Only the Alias field is used, the LocalName
	//   is ignored and Provider() is preferred instead.
	//   Examples: config, default
	// * addrs.AbsProviderConfig: the exact provider configuration has been
	//   resolved elsewhere and must be referenced directly. No inheritence
	//   logic is allowed.
	//   Examples: state, resource instance (resolved),
	ProvidedBy() RequestedProvider

	// Provider() returns the Provider FQN for the node.
	Provider() (provider addrs.Provider)

	// Set the resolved provider address for this resource.
	SetProvider(ResolvedProvider)
}

// ProviderTransformer is a GraphTransformer that maps resources to providers
// within the graph. This will error if there are any resources that don't map
// to proper resources.
type ProviderTransformer struct {
	Config *configs.Config
}

func (t *ProviderTransformer) Transform(_ context.Context, g *Graph) error {
	// We need to find a provider configuration address for each resource
	// either directly represented by a node or referenced by a node in
	// the graph, and then create graph edges from provider to provider user
	// so that the providers will get initialized first.

	var diags tfdiags.Diagnostics

	// Now we'll go through all the requested addresses we just collected and
	// figure out which _actual_ config address each belongs to, after resolving
	// for provider inheritance and passing.
	m := providerVertexMap(g)
	for _, v := range g.Vertices() {
		pv, isProviderConsumer := v.(GraphNodeProviderConsumer)
		if !isProviderConsumer {
			continue
		}
		req := pv.ProvidedBy()
		if req.ProviderConfig == nil {
			// no provider is required
			continue
		}
		switch providerAddr := req.ProviderConfig.(type) {
		case addrs.AbsProviderConfig:
			target := m[providerAddr.String()]
			if target == nil {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Provider configuration not present",
					fmt.Sprintf(
						"To work with %s its original provider configuration at %s is required, but it has been removed. This occurs when a provider configuration is removed while objects created by that provider still exist in the state. Re-add the provider configuration to destroy %s, after which you can remove the provider configuration again.",
						dag.VertexName(v), providerAddr, dag.VertexName(v),
					),
				))
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
				// We are ignoring p.keyExpr here for now
			}

			log.Printf("[DEBUG] ProviderTransformer: %q (%T) needs exactly %s", dag.VertexName(v), v, dag.VertexName(target))
			pv.SetProvider(ResolvedProvider{
				ProviderConfig: target.ProviderAddr(),
				// Pass through key data
				KeyExpression: req.KeyExpression,
				KeyModule:     req.KeyModule,
				KeyResource:   req.KeyResource,
				KeyExact:      req.KeyExact,
			})
			g.Connect(dag.BasicEdge(v, target))
		case addrs.LocalProviderConfig:
			// We assume that the value returned from Provider() has already been
			// properly checked during the provider validation logic in the
			// config package and can use that Provider Type directly instead
			// of duplicating provider LocalNames logic.
			fullProviderAddr := addrs.AbsProviderConfig{
				Provider: pv.Provider(),
				Module:   pv.ModulePath(),
				Alias:    providerAddr.Alias,
			}

			target := m[fullProviderAddr.String()]

			if target != nil {
				// Providers with configuration will already exist within the graph and can be directly referenced
				log.Printf("[TRACE] ProviderTransformer: exact match for %s serving %s", fullProviderAddr, dag.VertexName(v))
			} else {
				// if we don't have a provider at this level, walk up the path looking for one,
				// This assumes that the provider has the same LocalName in the module tree
				// If there is ever a desire to support differing local names down the module tree, this will
				// need to be rewritten
				for pp, ok := fullProviderAddr.Inherited(); ok; pp, ok = pp.Inherited() {
					target = m[pp.String()]
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

			resolved := ResolvedProvider{
				KeyResource:   req.KeyResource,
				KeyExpression: req.KeyExpression,
			}

			// see if this is a proxy provider pointing to another concrete config
			if p, ok := target.(*graphNodeProxyProvider); ok {
				target = p.Target()

				targetExpr, targetPath := p.TargetExpr()
				if targetExpr != nil {
					if resolved.KeyResource {
						// Module key and resource key are both required. This is not allowed!
						diags = diags.Append(fmt.Errorf("provider instance key provided for both resource and module at %q, this is a bug and should be reported", dag.VertexName(v)))
						continue
					}
					resolved.KeyExpression = targetExpr
					resolved.KeyModule = targetPath
				}
			}
			resolved.ProviderConfig = target.ProviderAddr()

			log.Printf("[DEBUG] ProviderTransformer: %q (%T) needs %s", dag.VertexName(v), v, dag.VertexName(target))
			pv.SetProvider(resolved)
			g.Connect(dag.BasicEdge(v, target))
		default:
			panic(fmt.Sprintf("BUG: Invalid provider address type %T for %#v", req, req))
		}
	}

	return diags.Err()
}

// ProviderFunctionReference is all the information needed to identify
// the provider required in a given module path. Alternatively, this
// could be seen as a Module path + addrs.LocalProviderConfig.
type ProviderFunctionReference struct {
	ModulePath    string
	ProviderName  string
	ProviderAlias string
}

type FunctionProvidedBy struct {
	Provider      addrs.AbsProviderConfig
	KeyModule     addrs.Module
	KeyExpression hcl.Expression
}

// ProviderFunctionMapping maps a provider used by functions at a given location in the graph to the actual AbsProviderConfig
// that's required. This is due to the provider inheritence logic and proxy logic in the below
// transformer needing to be known in other parts of the application.
// Ideally, this would not be needed and be built like the ProviderTransformer. Unfortunately, it's
// a significant refactor to get to that point which adds a lot of complexity.
type ProviderFunctionMapping map[ProviderFunctionReference]FunctionProvidedBy

func (m ProviderFunctionMapping) Lookup(module addrs.Module, pf addrs.ProviderFunction) (FunctionProvidedBy, bool) {
	providedBy, ok := m[ProviderFunctionReference{
		ModulePath:    module.String(),
		ProviderName:  pf.ProviderName,
		ProviderAlias: pf.ProviderAlias,
	}]
	return providedBy, ok
}

// ProviderFunctionTransformer is a GraphTransformer that maps nodes which reference functions to providers
// within the graph. This will error if there are any provider functions that don't map to known providers.
type ProviderFunctionTransformer struct {
	Config                  *configs.Config
	ProviderFunctionTracker ProviderFunctionMapping
}

func (t *ProviderFunctionTransformer) Transform(_ context.Context, g *Graph) error {
	var diags tfdiags.Diagnostics

	if t.Config == nil {
		// This is probably a test case, inherited from ProviderTransformer
		log.Printf("[WARN] Skipping provider function transformer due to missing config")
		return nil
	}

	// Locate all providerVerts in the graph
	providerVerts := providerVertexMap(g)
	// LuT of provider reference -> provider vertex
	providerReferences := make(map[ProviderFunctionReference]dag.Vertex)

	for _, v := range g.Vertices() {
		// Provider function references
		if nr, ok := v.(GraphNodeReferencer); ok && t.Config != nil {
			for _, ref := range nr.References() {
				if pf, ok := ref.Subject.(addrs.ProviderFunction); ok {
					refPath := nr.ModulePath()

					if outside, isOutside := v.(GraphNodeReferenceOutside); isOutside {
						_, refPath = outside.ReferenceOutside()
					}

					key := ProviderFunctionReference{
						ModulePath:    refPath.String(),
						ProviderName:  pf.ProviderName,
						ProviderAlias: pf.ProviderAlias,
					}

					// We already know about this provider and can link directly
					if provider, ok := providerReferences[key]; ok {
						// Is it worth skipping if we have already connected this provider?
						g.Connect(dag.BasicEdge(v, provider))
						continue
					}

					// Find the config that this node belongs to
					mc := t.Config.Descendent(refPath)
					if mc == nil {
						// I don't think this is possible
						diags = diags.Append(&hcl.Diagnostic{
							Severity: hcl.DiagError,
							Summary:  "Unknown Descendent Module",
							Detail:   refPath.String(),
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
						Module:   refPath,
						Alias:    pf.ProviderAlias,
					}

					log.Printf("[TRACE] ProviderFunctionTransformer: %s in %s is provided by %s", pf, dag.VertexName(v), absPc)

					// Lookup provider via full address
					provider := providerVerts[absPc.String()]

					if provider != nil {
						// Providers with configuration will already exist within the graph and can be directly referenced
						log.Printf("[TRACE] ProviderFunctionTransformer: exact match for %s serving %s", absPc, dag.VertexName(v))
					} else {
						// If this provider doesn't exist, stub it out with an init-only provider node
						// This works for unconfigured functions only, but that validation is elsewhere
						stubAddr := addrs.AbsProviderConfig{
							Module:   addrs.RootModule,
							Provider: absPc.Provider,
						}
						// Try to look up an existing stub
						provider, ok = providerVerts[stubAddr.String()]
						// If it does not exist, create it
						if !ok {
							log.Printf("[TRACE] ProviderFunctionTransformer: creating init-only node for %s", stubAddr)

							provider = &NodeEvalableProvider{
								&NodeAbstractProvider{
									Addr: stubAddr,
								},
							}
							providerVerts[stubAddr.String()] = provider
							g.Add(provider)
						}
					}

					var targetExpr hcl.Expression
					var targetPath addrs.Module

					// see if this is a proxy provider pointing to another concrete config
					if p, ok := provider.(*graphNodeProxyProvider); ok {
						provider = p.Target()
						targetExpr, targetPath = p.TargetExpr()
					}

					log.Printf("[DEBUG] ProviderFunctionTransformer: %q (%T) needs %s", dag.VertexName(v), v, dag.VertexName(provider))
					g.Connect(dag.BasicEdge(v, provider))

					// Save for future lookups
					providerReferences[key] = provider
					t.ProviderFunctionTracker[key] = FunctionProvidedBy{
						Provider:      provider.ProviderAddr(),
						KeyModule:     targetPath,
						KeyExpression: targetExpr,
					}
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

func (t *CloseProviderTransformer) Transform(_ context.Context, g *Graph) error {
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

func (t *MissingProviderTransformer) Transform(_ context.Context, g *Graph) error {
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

func (t *PruneProviderTransformer) Transform(_ context.Context, g *Graph) error {
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
func (n *graphNodeCloseProvider) Execute(ctx context.Context, evalCtx EvalContext, op walkOperation) (diags tfdiags.Diagnostics) {
	return diags.Append(evalCtx.CloseProvider(ctx, n.Addr))
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

// graphNodeProxyProvider is a GraphNodeProvider implementation that is used to
// store the name and value of a provider node for inheritance between modules.
// These nodes are only used to store the data while loading the provider
// configurations, and are removed after all the resources have been connected
// to their providers.
type graphNodeProxyProvider struct {
	addr    addrs.AbsProviderConfig
	target  GraphNodeProvider
	keyExpr hcl.Expression
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

// find the concrete provider instance
func (n *graphNodeProxyProvider) Target() GraphNodeProvider {
	switch t := n.target.(type) {
	case *graphNodeProxyProvider:
		return t.Target()
	default:
		return n.target
	}
}

// Find the *single* keyExpression that is used in the provider
// chain.  This is not ideal, but it works with current constraints on this feature
func (n *graphNodeProxyProvider) TargetExpr() (hcl.Expression, addrs.Module) {
	switch t := n.target.(type) {
	case *graphNodeProxyProvider:
		targetExpr, targetPath := t.TargetExpr()
		if targetExpr != nil && n.keyExpr != nil {
			// This should have already been handled during provider validation
			panic(fmt.Sprintf("BUG: Only one key expression allowed in module provider chain: %q", n.Name()))
		}
		if n.keyExpr != nil {
			return n.keyExpr, n.ModulePath()
		}
		return targetExpr, targetPath
	default:
		return n.keyExpr, n.ModulePath()
	}
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

	// Operation is needed to add workarounds for validate
	Operation walkOperation
}

func (t *ProviderConfigTransformer) Transform(_ context.Context, g *Graph) error {
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

			addNode := func(alias string) {
				abstract := &NodeAbstractProvider{
					Addr: addrs.AbsProviderConfig{
						Provider: addr.Provider,
						Module:   addr.Module,
						Alias:    alias,
					},
				}

				var v dag.Vertex
				if t.Concrete != nil {
					v = t.Concrete(abstract)
				} else {
					v = abstract
				}

				g.Add(v)
				t.providers[abstract.Addr.String()] = v.(GraphNodeProvider)
			}
			// Add unaliased instance for the provider in the root
			addNode("")

			if t.Operation == walkValidate {
				// Add a workaround for validating modules by running them as a root module in `tofu validate`
				// See the discussion in https://github.com/opentofu/opentofu/issues/2862 for more details
				for _, alias := range p.Aliases {
					addNode(alias.Alias)
				}
			}
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

		fullParentAddr := addrs.AbsProviderConfig{
			Provider: fqn,
			Module:   parentPath,
			Alias:    pair.InParent.Addr().Alias,
		}

		fullName := fullAddr.String()
		fullParentName := fullParentAddr.String()

		parentProvider := t.providers[fullParentName]

		if parentProvider == nil {
			return fmt.Errorf("missing provider %s", fullParentName)
		}

		proxy := &graphNodeProxyProvider{
			addr:    fullAddr,
			target:  parentProvider,
			keyExpr: pair.InParent.KeyExpression,
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
