// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"fmt"
	"log"

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
	GraphNodeProvider

	AttachProviderConfigSchema(*configschema.Block)
}

// GraphNodeAttachProvisionerSchema is an interface implemented by node types
// that need one or more provisioner schemas attached.
type GraphNodeAttachProvisionerSchema interface {
	ProvisionedBy() []string

	// SetProvisionerSchema is called during transform for each provisioner
	// type returned from ProvisionedBy, providing the configuration schema
	// for each provisioner in turn. The implementer should save these for
	// later use in evaluating provisioner configuration blocks.
	AttachProvisionerSchema(name string, schema *configschema.Block)
}

// AttachSchemaTransformer finds nodes that implement
// GraphNodeAttachResourceSchema, GraphNodeAttachProviderConfigSchema, or
// GraphNodeAttachProvisionerSchema, looks up the needed schemas for each
// and then passes them to a method implemented by the node.
//
// Some other part of the system must have called
// [contextplugins.LoadProviderSchemas] with the same configuration prior to
// executing this graph transformer, so that the needed schemas will become
// available for this transformer to use.
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

	for _, v := range g.Vertices() {

		if tv, ok := v.(GraphNodeAttachResourceSchema); ok {
			addr := tv.ResourceAddr()
			mode := addr.Resource.Mode
			typeName := addr.Resource.Type
			providerFqn := tv.Provider()

			schema, version, diags := t.Plugins.ResourceTypeSchema(providerFqn, mode, typeName)
			if diags.HasErrors() {
				return diags.Err()
			}
			if schema == nil {
				log.Printf("[ERROR] AttachSchemaTransformer: No resource schema available for %s", addr)
				continue
			}
			log.Printf("[TRACE] AttachSchemaTransformer: attaching resource schema to %s", dag.VertexName(v))
			tv.AttachResourceSchema(schema, version)
		}

		if tv, ok := v.(GraphNodeAttachProviderConfigSchema); ok {
			providerAddr := tv.ProviderAddr()
			schema, diags := t.Plugins.ProviderConfigSchema(providerAddr.Provider)
			if diags.HasErrors() {
				return diags.Err()
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

	return nil
}
