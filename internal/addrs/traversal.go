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
// This function is primarily to help with including traversal strings in
// the UI, and in particular should not be used for comparing traversals.
// Use [TraversalsEquivalent] to determine whether two traversals have
// the same meaning.
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

// TraversalsEquivalent returns true if the two given traversals represent
// the same meaning to HCL in all contexts.
//
// Unfortunately there is some ambiguity in interpreting traversal equivalence
// because HCL treats them differently depending on the context. If a
// traversal is involved in expression evaluation then the [hcl.Index]
// and [hcl.GetAttr] functions perform some automatic type conversions and
// allow interchanging index vs. attribute syntax for map and object types,
// but when interpreting traversals just for their syntax (as we do in
// [ParseRef], for example) these distinctions can potentially be significant.
//
// This function takes the stricter interpretation of ignoring the automatic
// adaptations made during expression evaluation, and so for example
// foo.bar and foo["bar"] are NOT considered to be equivalent by this function.
func TraversalsEquivalent(a, b hcl.Traversal) bool {
	if len(a) != len(b) {
		return false
	}

	for idx, stepA := range a {
		stepB := b[idx]

		switch stepA := stepA.(type) {
		case hcl.TraverseRoot:
			stepB, ok := stepB.(hcl.TraverseRoot)
			if !ok || stepA.Name != stepB.Name {
				return false
			}
		case hcl.TraverseAttr:
			stepB, ok := stepB.(hcl.TraverseAttr)
			if !ok || stepA.Name != stepB.Name {
				return false
			}
		case hcl.TraverseIndex:
			stepB, ok := stepB.(hcl.TraverseIndex)
			if !ok || stepA.Key.Equals(stepB.Key) != cty.True {
				return false
			}
		default:
			// The above should be exhaustive for all traversal
			// step types that HCL can possibly generate. We'll
			// treat any unsupported stepA types as non-equal
			// because that matches what would happen if
			// any stepB were unsupported: the type assertions
			// in the above cases would fail.
			return false
		}
	}

	return true
}
