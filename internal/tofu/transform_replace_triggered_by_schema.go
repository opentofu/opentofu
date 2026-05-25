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
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/dag"
)

// GraphNodeAttachReplaceTriggeredBySchema is an interface implemented by node types
// that need one or more provider schemas attached for the replace_triggered_by.
type GraphNodeAttachReplaceTriggeredBySchema interface {
	ReplaceTriggeredBy() []*addrs.Reference

	// AttachReplaceTriggeredBySchema is called during transform for each resource type
	// type returned from ReplaceTriggeredBy, providing the configuration schema
	// for each referenced provider in turn. The implementer should save these for
	// later use in evaluating replace_triggered_by references.
	AttachReplaceTriggeredBySchema(ref addrs.Reference, schema *configschema.Block)
}

// ReplaceTriggeredBySchemaTransformer checks the GraphNodeAttachReplaceTriggeredBySchema nodes and if any
// reports references from the `replace_triggered_by` expressions, this tries to identify the schema
// of the referenced resource type and attach it to the node containing the reference.
// This way, the node configured with `replace_triggered_by` can validate the expression against the
// actual targeted resource schema.
type ReplaceTriggeredBySchemaTransformer struct {
	Plugins *contextPlugins
}

func (t *ReplaceTriggeredBySchemaTransformer) Transform(ctx context.Context, g *Graph) error {
	vs := g.Vertices()
	for _, v := range vs {
		if tv, ok := v.(GraphNodeAttachReplaceTriggeredBySchema); ok {
			refs := tv.ReplaceTriggeredBy()
			if len(refs) == 0 {
				// Early exit to not uselessly iterate over the dependencies of the current node
				continue
			}
			deps := map[addrs.Resource]GraphNodeProviderConsumer{}
			for _, d := range g.DownEdges(v) {
				// We need the resource address
				crn, ok := d.(GraphNodeConfigResource)
				if !ok {
					continue
				}
				addr := crn.ResourceAddr().Resource
				pcn, ok := d.(GraphNodeProviderConsumer)
				if !ok {
					log.Printf("[TRACE] ReplaceTriggeredBySchemaTransformer: skip processing %s since it's not GraphNodeProviderConsumer", d)
					continue
				}
				deps[addr] = pcn
			}
			for _, ref := range refs {
				var refAddr addrs.Resource
				switch rs := ref.Subject.(type) {
				case addrs.Resource:
					refAddr = rs
				case addrs.ResourceInstance:
					refAddr = rs.Resource
				default:
					continue
				}

				pcn, ok := deps[refAddr]
				if !ok {
					log.Printf("[WARN] ReplaceTriggeredBySchemaTransformer: no node found for %s. It should have been added into the graph by the ReferenceTransformer", refAddr)
					continue
				}
				providerFqn := pcn.Provider()
				schema, _, err := t.Plugins.ResourceTypeSchema(ctx, providerFqn, refAddr.Mode, refAddr.Type)
				if err != nil {
					return fmt.Errorf("failed to read resource schema for %q: %v", refAddr, err)
				}
				if schema == nil {
					log.Printf("[WARN] ReplaceTriggeredBySchemaTransformer: No schema available for %q on %q", refAddr, dag.VertexName(pcn))
					continue
				}
				log.Printf("[TRACE] ReplaceTriggeredBySchemaTransformer: attaching replace_triggered_by %q schema to %s", refAddr, dag.VertexName(v))
				tv.AttachReplaceTriggeredBySchema(*ref, schema)
			}
		}
	}
	return nil
}
