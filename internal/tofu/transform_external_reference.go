// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
)

// ExternalReferenceTransformer will add a GraphNodeReferencer into the graph
// that makes no changes to the graph itself but, by referencing the addresses
// within ExternalReferences, ensures that any temporary nodes that are required
// by an external caller, such as the tofu testing framework, are not
// skipped because they are not referenced from within the module.
type ExternalReferenceTransformer struct {
	ExternalReferences []*addrs.Reference
}

func (t *ExternalReferenceTransformer) Transform(_ context.Context, g *Graph) error {
	if len(t.ExternalReferences) == 0 {
		return nil
	}

	g.Add(&nodeExternalReference{
		ExternalReferences: t.ExternalReferences,
	})
	return nil
}
