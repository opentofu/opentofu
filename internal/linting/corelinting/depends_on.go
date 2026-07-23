// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package corelinting

import (
	"context"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/linting"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// RedundantDependsOn is a core linting rule that checks if the `depends_on` references are redundant.
// This check verifies any reference the targetRes has to other blocks through means other than `depends_on` and if those
// are also configured in the `depends_on` meta-argument, it will generate linting diagnostics for any of those references.
//
// This function receives callbacks for getting the direct references and the ones in the `depends_on` argument.
// This way, when the linting rule is not enabled, we avoid the potential performance penalty that could come from
// generating again all those references.
func RedundantDependsOn(
	ctx context.Context,
	targetRes addrs.ConfigResource,
	targetDeclRange hcl.Range,
	directReferencesCb func() []*addrs.Reference,
	dependsOnReferencesCb func() []*addrs.Reference) tfdiags.Diagnostics {
	exec := func() tfdiags.Diagnostics {
		dependsOnReferences := dependsOnReferencesCb()
		if len(dependsOnReferences) == 0 {
			return nil // There is nothing to check if the resource has no depends_on references
		}
		directReferences := directReferencesCb()
		if len(directReferences) == 0 {
			return nil // There is nothing to check if the resource has no direct references
		}
		dependsOn := map[string]addrs.Reference{}
		for _, ref := range dependsOnReferences {
			if ref == nil {
				continue
			}
			dependsOn[unkeyedAddressReferenceKey(*ref)] = *ref
		}

		var diags tfdiags.Diagnostics
		for _, directRef := range directReferences {
			if directRef == nil {
				continue
			}
			refKey := unkeyedAddressReferenceKey(*directRef)
			// For whatever dependsOnRef is reported to be unused but is used in the diagnostic below
			dependsOnRef, ok := dependsOn[refKey] //nolint:staticcheck
			if !ok {
				continue
			}
			diags = diags.Append(tfdiags.LintMessage(
				ruleIDRedundantDependsOn,
				[]linting.RuleAddr{GroupIDConfusing},
				"Redundant 'depends_on' usage",
				fmt.Sprintf("Resource %q configures %q as 'depends_on'. The configured dependency is already automatically inferred which makes this particular 'depends_on' reference redundant.", targetRes.String(), refKey),
				new(tfdiags.SourceRangeFromHCL(dependsOnRef.SourceRange.ToHCL())),
				// TODO linting - do we really want to include this context here? Depends on where the `depends_on` argument is located inside the `resource` block,
				//  it could show only 2-3 lines of context in the diagnostic, but if it configured at the bottom of the block, it will print the whole `resource` block.
				//  Without this context, it will show only the line and the reference in question. Personally I consider that to be enough. Takes?
				new(tfdiags.SourceRangeFromHCL(targetDeclRange)),
			))
		}
		return diags
	}
	return tfdiags.ExecuteLintRule(ctx, exec, tfdiags.SourceRangeFromHCL(targetDeclRange), ruleIDRedundantDependsOn, GroupIDConfusing)
}

// unkeyedAddressReferenceKey returns a string key of the subject in the reference.
// We want to generate keys only for the unkeyed addresses since any reference of the resource/module
// directly or any an attribute/output of it will have the same side effect: will create a graph
// edge between the resource in question and the one it references through its attributes.
func unkeyedAddressReferenceKey(ref addrs.Reference) string {
	switch v := ref.Subject.(type) {
	case addrs.ModuleCallInstance:
		return addrs.Module{v.Call.Name}.String()
	case addrs.ModuleCallOutput:
		return addrs.Module{v.Call.Name}.String()
	case addrs.ModuleCallInstanceOutput:
		return addrs.Module{v.Call.Call.Name}.String()
	case addrs.Resource:
		return v.String()
	case addrs.ResourceInstance:
		return v.Resource.String()
	default:
		return ref.Subject.String()
	}
}
