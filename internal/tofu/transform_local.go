// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
)

// LocalTransformer is a GraphTransformer that adds all the local values
// from the configuration to the graph.
type LocalTransformer struct {
	Config *configs.Config
}

func (t *LocalTransformer) Transform(_ context.Context, g *Graph) error {
	return t.transformModule(g, t.Config)
}

func (t *LocalTransformer) transformModule(g *Graph, c *configs.Config) error {
	if c == nil {
		// Can't have any locals if there's no config
		return nil
	}

	for _, local := range c.Module.Locals {
		addr := addrs.LocalValue{Name: local.Name}
		node := &nodeExpandLocal{
			Addr:   addr,
			Module: c.Path,
			Config: local,
		}
		g.Add(node)
	}

	// Also populate locals for child modules
	for _, cc := range c.Children {
		if err := t.transformModule(g, cc); err != nil {
			return err
		}
	}

	return nil
}
