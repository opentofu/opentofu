// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"log"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/dag"
)

// GraphNodeAttachResourceConfig is an interface that must be implemented by nodes
// that want resource configurations attached.
type GraphNodeAttachResourceConfig interface {
	GraphNodeConfigResource

	// Sets the configuration
	AttachResourceConfig(*configs.Resource)
}

// AttachResourceConfigTransformer goes through the graph and attaches
// resource configuration structures to nodes that implement
// GraphNodeAttachManagedResourceConfig or GraphNodeAttachDataResourceConfig.
//
// The attached configuration structures are directly from the configuration.
// If they're going to be modified, a copy should be made.
type AttachResourceConfigTransformer struct {
	Config *configs.Config // Config is the root node in the config tree
}

func (t *AttachResourceConfigTransformer) Transform(_ context.Context, g *Graph) error {

	// Go through and find GraphNodeAttachResource
	for _, v := range g.Vertices() {
		// Only care about GraphNodeAttachResource implementations
		arn, ok := v.(GraphNodeAttachResourceConfig)
		if !ok {
			continue
		}

		// Determine what we're looking for
		addr := arn.ResourceAddr()

		// Get the configuration.
		config := t.Config.Descendent(addr.Module)
		if config == nil {
			log.Printf("[TRACE] AttachResourceConfigTransformer: %q (%T) has no configuration available", dag.VertexName(v), v)
			continue
		}
		var m map[string]*configs.Resource
		switch addr.Resource.Mode {
		case addrs.ManagedResourceMode:
			m = config.Module.ManagedResources
		case addrs.DataResourceMode:
			m = config.Module.DataResources
		case addrs.EphemeralResourceMode:
			m = config.Module.EphemeralResources
		default:
			panic("unknown resource mode: " + addr.Resource.Mode.String())
		}
		coord := addr.Resource.String()
		if r, ok := m[coord]; ok && r.Addr() == addr.Resource {
			log.Printf("[TRACE] AttachResourceConfigTransformer: attaching to %q (%T) config from %#v", dag.VertexName(v), v, r.DeclRange)
			arn.AttachResourceConfig(r)
			if gnapmc, ok := v.(GraphNodeAttachProviderMetaConfigs); ok {
				log.Printf("[TRACE] AttachResourceConfigTransformer: attaching provider meta configs to %s", dag.VertexName(v))
				if config.Module.ProviderMetas != nil {
					gnapmc.AttachProviderMetaConfigs(config.Module.ProviderMetas)
				} else {
					log.Printf("[TRACE] AttachResourceConfigTransformer: no provider meta configs available to attach to %s", dag.VertexName(v))
				}
			}

			// If the provider config declares all_objects_part_of, resolve and
			// attach those parent addresses so orphan planning can use them.
			if node, ok := v.(interface {
				AttachProviderAllObjectsPartOf([]addrs.ConfigResource)
			}); ok {
				providerAddr := r.ProviderConfigAddr()
				if pc, ok := config.Module.GetProviderConfig(providerAddr.LocalName, providerAddr.Alias); ok && len(pc.AllObjectsPartOf) > 0 {
					parents := resolveProviderParentTraversals(pc.AllObjectsPartOf, addr.Module)
					if len(parents) > 0 {
						log.Printf("[TRACE] AttachResourceConfigTransformer: attaching %d provider parent(s) to %s", len(parents), dag.VertexName(v))
						node.AttachProviderAllObjectsPartOf(parents)
					}
				}
			}
		}
	}

	return nil
}

// resolveProviderParentTraversals converts the hcl.Traversal values from
// all_objects_part_of into ConfigResource addresses within the given module.
// Traversals that cannot be resolved as managed resource references are
// silently dropped; validation should have already caught those cases.
func resolveProviderParentTraversals(traversals []hcl.Traversal, module addrs.Module) []addrs.ConfigResource {
	result := make([]addrs.ConfigResource, 0, len(traversals))
	for _, traversal := range traversals {
		ref, diags := addrs.ParseRef(traversal)
		if diags.HasErrors() {
			log.Printf("[ERROR] resolveProviderParentTraversals: cannot parse traversal as reference: %s", diags.Err())
			continue
		}
		switch subject := ref.Subject.(type) {
		case addrs.Resource:
			result = append(result, subject.InModule(module))
		case addrs.ResourceInstance:
			result = append(result, subject.Resource.InModule(module))
		default:
			log.Printf("[WARN] resolveProviderParentTraversals: all_objects_part_of entry %q is not a resource reference; ignoring", traversal.RootName())
		}
	}
	return result
}
