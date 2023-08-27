// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package opentf

import (
	"log"

	"github.com/placeholderplaceholderplaceholder/opentf/internal/addrs"
	"github.com/placeholderplaceholderplaceholder/opentf/internal/configs/configschema"
	"github.com/placeholderplaceholderplaceholder/opentf/internal/dag"
)

// ResourceCountTransformer is a GraphTransformer that expands the count
// out for a specific resource.
//
// This assumes that the count is already interpolated.
type ResourceCountTransformer struct {
	Concrete ConcreteResourceInstanceNodeFunc
	Schema   *configschema.Block

	Addr          addrs.ConfigResource
	InstanceAddrs []addrs.AbsResourceInstance
}

func (t *ResourceCountTransformer) Transform(g *Graph) error {
	for _, addr := range t.InstanceAddrs {
		abstract := NewNodeAbstractResourceInstance(addr)
		abstract.Schema = t.Schema
		var node dag.Vertex = abstract
		if f := t.Concrete; f != nil {
			node = f(abstract)
		}

		log.Printf("[TRACE] ResourceCountTransformer: adding %s as %T", addr, node)
		g.Add(node)
	}
	return nil
}
