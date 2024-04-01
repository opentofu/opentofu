// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package funcs

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

func renderTemplate(expr hcl.Expression, varsVal cty.Value, funcs map[string]function.Function) (cty.Value, error) {
	if varsTy := varsVal.Type(); !(varsTy.IsMapType() || varsTy.IsObjectType()) {
		return cty.DynamicVal, function.NewArgErrorf(1, "invalid vars value: must be a map") // or an object, but we don't strongly distinguish these most of the time
	}

	ctx := &hcl.EvalContext{
		Variables: varsVal.AsValueMap(),
		Functions: funcs,
	}

	// We require all of the variables to be valid HCL identifiers, because
	// otherwise there would be no way to refer to them in the template
	// anyway. Rejecting this here gives better feedback to the user
	// than a syntax error somewhere in the template itself.
	for n := range ctx.Variables {
		if !hclsyntax.ValidIdentifier(n) {
			// This error message intentionally doesn't describe _all_ of
			// the different permutations that are technically valid as an
			// HCL identifier, but rather focuses on what we might
			// consider to be an "idiomatic" variable name.
			return cty.DynamicVal, function.NewArgErrorf(1, "invalid template variable name %q: must start with a letter, followed by zero or more letters, digits, and underscores", n)
		}
	}

	// currFilename stores the filename of the template file, if any.
	currFilename := expr.Range().Filename

	// We'll pre-check references in the template here so we can give a
	// more specialized error message than HCL would by default, so it's
	// clearer that this problem is coming from a templatefile/templatestring call.
	for _, traversal := range expr.Variables() {
		root := traversal.RootName()
		referencedPos := fmt.Sprintf("%q", root)
		if currFilename != templateStringFilename {
			referencedPos = fmt.Sprintf("%q, referenced at %s", root, traversal[0].SourceRange())
		}
		if _, ok := ctx.Variables[root]; !ok {
			return cty.DynamicVal, function.NewArgErrorf(1, "vars map does not contain key %s", referencedPos)
		}
	}

	val, diags := expr.Value(ctx)
	if diags.HasErrors() {
		for _, diag := range diags {
			// Roll up recursive errors
			if extra, ok := diag.Extra.(hclsyntax.FunctionCallDiagExtra); ok {
				if extra.CalledFunctionName() == "templatefile" {
					err := extra.FunctionCallError()
					if err, ok := err.(ErrorTemplateRecursionLimit); ok {
						return cty.DynamicVal, ErrorTemplateRecursionLimit{sources: append(err.sources, diag.Subject.String())}
					}
				}
			}
		}
		return cty.DynamicVal, diags
	}

	return val, nil
}
