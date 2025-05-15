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
	"github.com/opentofu/opentofu/internal/states"
)

// RemovedModuleTransformer implements GraphTransformer to add nodes indicating
// when a module was removed from the configuration.
type RemovedModuleTransformer struct {
	Config *configs.Config // root node in the config tree
	State  *states.State
}

func (t *RemovedModuleTransformer) Transform(_ context.Context, g *Graph) error {
	// nothing to remove if there's no state!
	if t.State == nil {
		return nil
	}

	removed := map[string]addrs.Module{}

	for _, m := range t.State.Modules {
		cc := t.Config.DescendentForInstance(m.Addr)
		if cc != nil {
			continue
		}
		mod := m.Addr.Module()
		removed[mod.String()] = mod
		log.Printf("[DEBUG] %s is no longer in configuration\n", m.Addr)
	}

	// add closers to collect any module instances we're removing
	for _, modAddr := range removed {
		closer := &nodeCloseModule{
			Addr: modAddr,
		}
		g.Add(closer)
	}

	return nil
}
