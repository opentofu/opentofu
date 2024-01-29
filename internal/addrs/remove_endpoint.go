// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// RemoveEndpoint is to ConfigRemovable what Target is to Targetable:
// a wrapping struct that captures the result of decoding an HCL
// traversal representing a relative path from the current module to
// a removable object. It is very similar to MoveEndpoint.
//
// Its purpose is to represent the "from" address in a "removed" block
// in the configuration.
//
// To obtain a full address from a RemoveEndpoint we need to combine it
// with any ancestor modules in the configuration
type RemoveEndpoint struct {
	// SourceRange is the location of the physical endpoint address
	// in configuration, if this RemoveEndpoint was decoded from a
	// configuration expression.
	SourceRange tfdiags.SourceRange

	// the representation of our relative address as a ConfigRemovable
	RelSubject ConfigRemovable
}

func (e *RemoveEndpoint) String() string {
	// Our internal pseudo-AbsMoveable representing the relative
	// address (either ModuleInstance or AbsResourceInstance) is
	// a good enough proxy for the relative move endpoint address
	// serialization.
	return e.RelSubject.String()
}

func (e *RemoveEndpoint) Equal(other *RemoveEndpoint) bool {
	switch {
	case (e == nil) != (other == nil):
		return false
	case e == nil:
		return true
	default:
		// Since we only use ModuleInstance and AbsResourceInstance in our
		// string representation, we have no ambiguity between address types
		// and can safely just compare the string representations to
		// compare the RelSubject values.
		return e.String() == other.String() && e.SourceRange == other.SourceRange
	}
}

// ParseRemoveEndpoint attempts to interpret the given traversal as a
// "remove endpoint" address, which is a relative path from the module containing
// the traversal to a removable object in either the same module or in some
// child module.
//
// This deals only with the syntactic element of a remove endpoint expression
// in configuration. Before the result will be useful you'll need to combine
// it with the address of the module where it was declared in order to get
// an absolute address relative to the root module.
func ParseRemoveEndpoint(traversal hcl.Traversal) (*RemoveEndpoint, tfdiags.Diagnostics) {
	path, remain, diags := parseModulePrefix(traversal)
	if diags.HasErrors() {
		return nil, diags
	}

	rng := tfdiags.SourceRangeFromHCL(traversal.SourceRange())

	if len(remain) == 0 {
		return &RemoveEndpoint{
			RelSubject:  path,
			SourceRange: rng,
		}, diags
	}

	riAddr, moreDiags := parseResourceUnderModule(path, remain)
	diags = diags.Append(moreDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	if riAddr.Resource.Mode == DataResourceMode {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Data source address is not allowed",
			Detail:   "Data sources cannot be destroyed, and therefore, 'removed' blocks are not allowed to target them. To remove data sources from the state, you can remove the data source block from the configuration.",
			Subject:  traversal.SourceRange().Ptr(),
		})

		return nil, diags
	}

	return &RemoveEndpoint{
		RelSubject:  riAddr,
		SourceRange: rng,
	}, diags
}
