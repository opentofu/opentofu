// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import "github.com/opentofu/opentofu/internal/dag"

// GraphDot returns the dot formatting of a visual representation of
// the given OpenTofu graph.
func GraphDot(g *Graph, opts *dag.DotOpts) (string, error) {
	return string(g.Dot(opts)), nil
}
