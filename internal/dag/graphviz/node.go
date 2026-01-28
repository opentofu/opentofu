// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package graphviz

import (
	"github.com/opentofu/opentofu/internal/dag"
)

type Node struct {
	ID    string
	Attrs Attributes
}

var _ dag.Hashable = Node{}

// Hashcode implements [dag.Hashable], using the value in the ID field as
// the unique identifier for a [Node].
//
// This means that all nodes in a graph must have a distinct ID value, or
// the [dag.Graph] implementation will coalesce any with conflicting IDs with
// unspecified results.
func (n Node) Hashcode() any {
	return n.ID
}
