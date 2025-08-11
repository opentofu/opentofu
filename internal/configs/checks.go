// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/parser"
	"github.com/opentofu/opentofu/internal/lang"
)

// CheckRule represents a configuration-defined validation rule, precondition,
// or postcondition. Blocks of this sort can appear in a few different places
// in configuration, including "validation" blocks for variables,
// and "precondition" and "postcondition" blocks for resources.
type CheckRule struct {
	// Condition is an expression that must evaluate to true if the condition
	// holds or false if it does not. If the expression produces an error then
	// that's considered to be a bug in the module defining the check.
	//
	// The available variables in a condition expression vary depending on what
	// a check is attached to. For example, validation rules attached to
	// input variables can only refer to the variable that is being validated.
	Condition hcl.Expression

	// ErrorMessage should be one or more full sentences, which should be in
	// English for consistency with the rest of the error message output but
	// can in practice be in any language. The message should describe what is
	// required for the condition to return true in a way that would make sense
	// to a caller of the module.
	//
	// The error message expression has the same variables available for
	// interpolation as the corresponding condition.
	ErrorMessage hcl.Expression

	DeclRange hcl.Range
}

// validateSelfReferences looks for references in the check rule matching the
// specified resource address, returning error diagnostics if such a reference
// is found.
func (cr *CheckRule) validateSelfReferences(checkType string, addr addrs.Resource) hcl.Diagnostics {
	var diags hcl.Diagnostics
	exprs := []hcl.Expression{
		cr.Condition,
		cr.ErrorMessage,
	}
	for _, expr := range exprs {
		if expr == nil {
			continue
		}
		refs, _ := lang.References(addrs.ParseRef, expr.Variables())
		for _, ref := range refs {
			var refAddr addrs.Resource

			switch rs := ref.Subject.(type) {
			case addrs.Resource:
				refAddr = rs
			case addrs.ResourceInstance:
				refAddr = rs.Resource
			default:
				continue
			}

			if refAddr.Equal(addr) {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  fmt.Sprintf("Invalid reference in %s", checkType),
					Detail:   fmt.Sprintf("Configuration for %s may not refer to itself.", addr.String()),
					Subject:  expr.Range().Ptr(),
				})
				break
			}
		}
	}
	return diags
}

// decodeCheckRuleBlock decodes the contents of the given block as a check rule.
//
// Unlike most of our "decode..." functions, this one can be applied to blocks
// of various types as long as their body structures are "check-shaped". The
// function takes the containing block only because some error messages will
// refer to its location, and the returned object's DeclRange will be the
// block's header.
func decodeCheckRuleBlock(block *parser.CheckRule, blockType string, override bool) (*CheckRule, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	cr := &CheckRule{
		DeclRange: block.DefRange,
	}

	if override {
		// For now we'll just forbid overriding check blocks, to simplify
		// the initial design. If we can find a clear use-case for overriding
		// checks in override files and there's a way to define it that
		// isn't confusing then we could relax this.
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf("Can't override %s blocks", blockType),
			Detail:   fmt.Sprintf("Override files cannot override %q blocks.", blockType),
			Subject:  cr.DeclRange.Ptr(),
		})
		return cr, diags
	}

	cr.Condition = block.Condition

	if len(cr.Condition.Variables()) == 0 {
		// A condition expression that doesn't refer to any variable is
		// pointless, because its result would always be a constant.
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf("Invalid %s expression", blockType),
			Detail:   "The condition expression must refer to at least one object from elsewhere in the configuration, or else its result would not be checking anything.",
			Subject:  cr.Condition.Range().Ptr(),
		})
	}

	cr.ErrorMessage = block.ErrorMessage

	return cr, diags
}

var checkRuleBlockSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{
			Name:     "condition",
			Required: true,
		},
		{
			Name:     "error_message",
			Required: true,
		},
	},
}

// Check represents a configuration defined check block.
//
// A check block contains 0-1 data blocks, and 0-n assert blocks. The check
// block will load the data block, and execute the assert blocks as check rules
// during the plan and apply OpenTofu operations.
type Check struct {
	Name string

	DataResource *Resource
	Asserts      []*CheckRule

	DeclRange hcl.Range
}

func (c Check) Addr() addrs.Check {
	return addrs.Check{
		Name: c.Name,
	}
}

func (c Check) Accessible(addr addrs.Referenceable) bool {
	if check, ok := addr.(addrs.Check); ok {
		return check.Equal(c.Addr())
	}
	return false
}

func decodeCheckBlock(block *parser.Check, override bool) (*Check, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	check := &Check{
		Name:      block.Name,
		DeclRange: block.DefRange,
	}

	if override {
		// For now we'll just forbid overriding check blocks, to simplify
		// the initial design. If we can find a clear use-case for overriding
		// checks in override files and there's a way to define it that
		// isn't confusing then we could relax this.
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Can't override check blocks",
			Detail:   "Override files cannot override check blocks.",
			Subject:  check.DeclRange.Ptr(),
		})
		return check, diags
	}

	if !hclsyntax.ValidIdentifier(check.Name) {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid check block name",
			Detail:   badIdentifierDetail,
			Subject:  &block.NameRange,
		})
	}

	for _, block := range block.DataResource {
		if check.DataResource != nil {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Multiple data resource blocks",
				Detail:   fmt.Sprintf("This check block already has a data resource defined at %s.", check.DataResource.DeclRange.Ptr()),
				Subject:  block.DefRange.Ptr(),
			})
			continue
		}

		data, moreDiags := decodeDataBlock(block, override, true)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			// Connect this data block back up to this check block.
			data.Container = check

			// Finally, save the data block.
			check.DataResource = data
		}
	}
	for _, block := range block.Asserts {
		assert, moreDiags := decodeCheckRuleBlock(block, "assert", override)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			check.Asserts = append(check.Asserts, assert)
		}
	}

	if len(check.Asserts) == 0 {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Zero assert blocks",
			Detail:   "Check blocks must have at least one assert block.",
			Subject:  check.DeclRange.Ptr(),
		})
	}

	return check, diags
}
