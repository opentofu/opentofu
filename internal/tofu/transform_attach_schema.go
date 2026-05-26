// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"fmt"
	"log"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/dag"
)

// GraphNodeAttachResourceSchema is an interface implemented by node types
// that need a resource schema attached.
type GraphNodeAttachResourceSchema interface {
	GraphNodeConfigResource
	GraphNodeProviderConsumer

	AttachResourceSchema(schema *configschema.Block, version uint64)
}

// GraphNodeAttachProviderConfigSchema is an interface implemented by node types
// that need a provider configuration schema attached.
type GraphNodeAttachProviderConfigSchema interface {
	ProviderAddr() addrs.AbsProviderConfig

	AttachProviderConfigSchema(*configschema.Block)
}

// GraphNodeAttachProvisionerSchema is an interface implemented by node types
// that need one or more provisioner schemas attached.
type GraphNodeAttachProvisionerSchema interface {
	ProvisionedBy() []string

	// AttachProvisionerSchema is called during transform for each provisioner
	// type returned from ProvisionedBy, providing the configuration schema
	// for each provisioner in turn. The implementer should save these for
	// later use in evaluating provisioner configuration blocks.
	AttachProvisionerSchema(name string, schema *configschema.Block)
}

// GraphNodeAttachReplaceTriggeredBySchema is an interface implemented by node types
// that need one or more provider schemas attached for the replace_triggered_by.
type GraphNodeAttachReplaceTriggeredBySchema interface {
	GraphNodeConfigResource
	ReplaceTriggeredBy() []*addrs.Reference

	// AttachReplaceTriggeredBySchema is called during transform for each resource type
	// type returned from ReplaceTriggeredBy, providing the configuration schema
	// for each referenced provider in turn. The implementer should save these for
	// later use in evaluating replace_triggered_by references.
	AttachReplaceTriggeredBySchema(ref addrs.Resource, schema *configschema.Block)
}

// AttachSchemaTransformer finds nodes that implement
// GraphNodeAttachResourceSchema, GraphNodeAttachProviderConfigSchema, or
// GraphNodeAttachProvisionerSchema, looks up the needed schemas for each
// and then passes them to a method implemented by the node.
type AttachSchemaTransformer struct {
	Plugins *contextPlugins
	Config  *configs.Config
}

func (t *AttachSchemaTransformer) Transform(ctx context.Context, g *Graph) error {
	if t.Plugins == nil {
		// Should never happen with a reasonable caller, but we'll return a
		// proper error here anyway so that we'll fail gracefully.
		return fmt.Errorf("AttachSchemaTransformer used with nil Plugins")
	}

	refMap := addrs.MakeMap[addrs.ConfigResource, *configschema.Block]()

	for _, v := range g.Vertices() {

		if tv, ok := v.(GraphNodeAttachResourceSchema); ok {
			addr := tv.ResourceAddr()
			mode := addr.Resource.Mode
			typeName := addr.Resource.Type
			providerFqn := tv.Provider()

			// TODO: Plumb a useful context.Context through to here.
			schema, version, diags := t.Plugins.ResourceTypeSchema(ctx, providerFqn, mode, typeName)
			if diags.HasErrors() {
				return fmt.Errorf("failed to read schema for %s in %s: %w", addr, providerFqn, diags.Err())
			}
			if schema == nil {
				log.Printf("[ERROR] AttachSchemaTransformer: No resource schema available for %s", addr)
				continue
			}
			log.Printf("[TRACE] AttachSchemaTransformer: attaching resource schema to %s", dag.VertexName(v))
			tv.AttachResourceSchema(schema, version)

			refMap.Put(addr, schema)
		}

		if tv, ok := v.(GraphNodeAttachProviderConfigSchema); ok {
			providerAddr := tv.ProviderAddr()
			// TODO: Plumb a useful context.Context through to here.
			schema, diags := t.Plugins.ProviderConfigSchema(ctx, providerAddr.Provider)
			if diags.HasErrors() {
				return fmt.Errorf("failed to read provider configuration schema for %s: %w", providerAddr.Provider, diags.Err())
			}
			if schema == nil {
				log.Printf("[ERROR] AttachSchemaTransformer: No provider config schema available for %s", providerAddr)
				continue
			}
			log.Printf("[TRACE] AttachSchemaTransformer: attaching provider config schema to %s", dag.VertexName(v))
			tv.AttachProviderConfigSchema(schema)
		}

		if tv, ok := v.(GraphNodeAttachProvisionerSchema); ok {
			names := tv.ProvisionedBy()
			for _, name := range names {
				schema, err := t.Plugins.ProvisionerSchema(name)
				if err != nil {
					return fmt.Errorf("failed to read provisioner configuration schema for %q: %w", name, err)
				}
				if schema == nil {
					log.Printf("[ERROR] AttachSchemaTransformer: No schema available for provisioner %q on %q", name, dag.VertexName(v))
					continue
				}
				log.Printf("[TRACE] AttachSchemaTransformer: attaching provisioner %q config schema to %s", name, dag.VertexName(v))
				tv.AttachProvisionerSchema(name, schema)
			}
		}
	}

	for _, v := range g.Vertices() {
		if tv, ok := v.(GraphNodeAttachReplaceTriggeredBySchema); ok {
			module := tv.ResourceAddr().Module
			refs := tv.ReplaceTriggeredBy()
			for _, ref := range refs {
				var refAddr addrs.ConfigResource
				switch rs := ref.Subject.(type) {
				case addrs.Resource:
					refAddr = rs.InModule(module)
				case addrs.ResourceInstance:
					refAddr = rs.Resource.InModule(module)
				default:
					continue
				}
				schema, ok := refMap.GetOk(refAddr)
				if ok {
					log.Printf("[TRACE] AttachSchemaTransformer: attaching replace_triggered_by %q config schema to %s", refAddr, dag.VertexName(v))
					tv.AttachReplaceTriggeredBySchema(refAddr.Resource, schema)
				} else {
					log.Printf("[WARN] AttachSchemaTransformer: missing replace_triggered_by %q in %s", refAddr, dag.VertexName(v))
				}
			}
		}
	}

	return nil
}
