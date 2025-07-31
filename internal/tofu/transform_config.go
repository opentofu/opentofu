// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"log"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/dag"
)

// ConfigTransformer is a GraphTransformer that adds all the resources
// from the configuration to the graph.
//
// The module used to configure this transformer must be the root module.
//
// Only resources are added to the graph. Variables, outputs, and
// providers must be added via other transforms.
//
// Unlike ConfigTransformerOld, this transformer creates a graph with
// all resources including module resources, rather than creating module
// nodes that are then "flattened".
type ConfigTransformer struct {
	Concrete ConcreteResourceNodeFunc

	// Module is the module to add resources from.
	Config *configs.Config

	// Mode will only add resources that match the given mode
	ModeFilter bool
	Mode       addrs.ResourceMode

	// Do not apply this transformer.
	skip bool

	// importTargets specifies a slice of addresses that will have state
	// imported for them.
	importTargets []*ImportTarget

	// generateConfigPathForImportTargets tells the graph where to write any
	// generated config for import targets that are not contained within config.
	//
	// If this is empty and an import target has no config, the graph will
	// simply import the state for the target and any follow-up operations will
	// try to delete the imported resource unless the config is updated
	// manually.
	generateConfigPathForImportTargets string
}

func (t *ConfigTransformer) Transform(_ context.Context, g *Graph) error {
	if t.skip {
		return nil
	}

	// If no configuration is available, we don't do anything
	if t.Config == nil {
		return nil
	}

	// Start the transformation process
	return t.transform(g, t.Config, t.generateConfigPathForImportTargets)
}

func (t *ConfigTransformer) transform(g *Graph, config *configs.Config, generateConfigPath string) error {
	// If no config, do nothing
	if config == nil {
		return nil
	}

	// If the module is being overridden, do nothing. We don't want to create anything
	// from the underlying module.
	if config.Module.IsOverridden {
		return nil
	}

	// Add our resources
	if err := t.transformSingle(g, config, generateConfigPath); err != nil {
		return err
	}

	// Transform all the children without generating config.
	for _, c := range config.Children {
		if err := t.transform(g, c, ""); err != nil {
			return err
		}
	}

	return nil
}

func (t *ConfigTransformer) transformSingle(g *Graph, config *configs.Config, generateConfigPath string) error {
	path := config.Path
	module := config.Module
	log.Printf("[TRACE] ConfigTransformer: Starting for path: %v", path)

	allResources := make([]*configs.Resource, 0, len(module.ManagedResources)+len(module.DataResources))
	for _, r := range module.ManagedResources {
		allResources = append(allResources, r)
	}
	for _, r := range module.DataResources {
		allResources = append(allResources, r)
	}

	// Take a copy of the import targets, so we can edit them as we go.
	// Only include import targets that are targeting the current module.
	var importTargets []*ImportTarget
	for _, target := range t.importTargets {
		if targetModule := target.StaticAddr().Module; targetModule.Equal(config.Path) {
			importTargets = append(importTargets, target)
		}
	}

	for _, r := range allResources {
		relAddr := r.Addr()

		if t.ModeFilter && relAddr.Mode != t.Mode {
			// Skip non-matching modes
			continue
		}

		// If any of the import targets can apply to this node's instances,
		// filter them down to the applicable addresses.
		var imports []*ImportTarget
		configAddr := relAddr.InModule(path)

		var matchedIndices []int
		for ix, i := range importTargets {
			if target := i.StaticAddr(); target.Equal(configAddr) {
				// This import target has been claimed by an actual resource,
				// let's make a note of this to remove it from the targets.
				matchedIndices = append(matchedIndices, ix)
				imports = append(imports, i)
			}
		}

		for ix := len(matchedIndices) - 1; ix >= 0; ix-- {
			tIx := matchedIndices[ix]

			// We do this backwards, since it means we don't have to adjust the
			// later indices as we change the length of import targets.
			//
			// We need to do this separately, as a single resource could match
			// multiple import targets.
			importTargets = append(importTargets[:tIx], importTargets[tIx+1:]...)
		}

		abstract := &NodeAbstractResource{
			Addr: addrs.ConfigResource{
				Resource: relAddr,
				Module:   path,
			},
			importTargets: imports,
		}

		var node dag.Vertex = abstract
		if f := t.Concrete; f != nil {
			node = f(abstract)
		}

		g.Add(node)
	}

	// If any import targets were not claimed by resources, then let's add them
	// into the graph now.
	//
	// We should only reach this point if config generation is enabled, since we validate that all import targets have
	// a resource in validateImportTargets when config generation is disabled
	//
	// We'll add the nodes that we know will fail, and catch them again later
	// in the processing when we are in a position to raise a much more helpful
	// error message.
	for _, i := range importTargets {
		if len(generateConfigPath) > 0 {
			// Create a node with the resource and import target. This node will take care of the config generation
			abstract := &NodeAbstractResource{
				// We've already validated in validateImportTargets that the address is fully resolvable
				Addr:               i.ResolvedAddr().ConfigResource(),
				importTargets:      []*ImportTarget{i},
				generateConfigPath: generateConfigPath,
			}

			var node dag.Vertex = abstract
			if f := t.Concrete; f != nil {
				node = f(abstract)
			}

			g.Add(node)
		} else {
			// Technically we shouldn't reach this point, as we've already validated that a resource exists
			// in validateImportTargets
			return importResourceWithoutConfigDiags(i.StaticAddr().String(), i.Config)
		}
	}

	return nil
}
