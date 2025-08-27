// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package exprs

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
)

// TraversalStepAttributeName attempts to interpret the given traversal step
// in a manner compatible with how [hcl.Index] would apply it to a value
// of an object type, returning the name of the attribute it would access.
//
// If the second return value is false then the given step is not valid to
// use in that situation.
//
// This is mainly for use in [Valuer.StaticCheckTraversal] implementations
// where the Value method would return an object type, to find out which
// attribute name the first traversal step would ultimately select.
func TraversalStepAttributeName(step hcl.Traverser) (string, bool) {
	switch step := step.(type) {
	case hcl.TraverseAttr:
		return step.Name, true
	case hcl.TraverseRoot:
		return step.Name, true
	case hcl.TraverseIndex:
		v, err := convert.Convert(step.Key, cty.String)
		if err != nil || v.IsNull() {
			return "", false
		}
		if !v.IsKnown() {
			// Unknown values should not typically appear in traversals
			// because HCL itself only builds traversals from static
			// traversal syntax, but this is here just in case we do
			// something weird e.g. in a test that's constructing
			// traversals by hand.
			return "", false
		}
		// Likewise marked values should not typically appear in traversals
		// for the same reason, so we're unmarking for robustness against
		// weird contrived inputs only.
		v, _ = v.Unmark()
		return v.AsString(), true
	default:
		return "", false
	}
}
