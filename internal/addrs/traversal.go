// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"bytes"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
)

// TraversalStr produces a representation of an HCL traversal that is compact,
// resembles HCL native syntax, and is suitable for display in the UI.
//
// This was copied (and simplified) from internal/command/views/json/diagnostic.go.
func TraversalStr(traversal hcl.Traversal) string {
	var buf bytes.Buffer
	for _, step := range traversal {
		switch tStep := step.(type) {
		case hcl.TraverseRoot:
			buf.WriteString(tStep.Name)
		case hcl.TraverseAttr:
			buf.WriteByte('.')
			buf.WriteString(tStep.Name)
		case hcl.TraverseIndex:
			buf.WriteByte('[')
			switch tStep.Key.Type() {
			case cty.String:
				buf.WriteString(fmt.Sprintf("%q", tStep.Key.AsString()))
			case cty.Number:
				bf := tStep.Key.AsBigFloat()
				//nolint:mnd // numerical precision
				buf.WriteString(bf.Text('g', 10))
			default:
				buf.WriteString("...")
			}
			buf.WriteByte(']')
		}
	}
	return buf.String()
}
