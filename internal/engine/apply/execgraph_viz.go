// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package apply

import (
	"fmt"
	"io"

	"github.com/opentofu/opentofu/internal/engine/internal/execgraph"
)

// WriteExecutionGraphForGraphviz is a debugging helper for generating a
// graphviz-compatible representation of the given execution graph on the
// given writer.
//
// The graph should be provided in the opaque bytestream format we save
// in plan files.
func WriteExecutionGraphForGraphviz(graphRaw []byte, wr io.Writer) error {
	graph, err := execgraph.UnmarshalGraph(graphRaw)
	if err != nil {
		return fmt.Errorf("invalid execution graph: %w", err)
	}

	return graph.WriteGraphvizRepr(wr)
}
