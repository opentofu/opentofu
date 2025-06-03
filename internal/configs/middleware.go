// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
)

type Middleware struct {
	Name      string
	NameRange hcl.Range

	Command hcl.Expression
	Args    []hcl.Expression
	Env     map[string]string

	Config    hcl.Body
	DeclRange hcl.Range
}

var middlewareBlockSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{
			Name:     "command",
			Required: true,
		},
		{
			Name:     "args",
			Required: false,
		},
		{
			Name:     "env",
			Required: false,
		},
	},
}

func decodeMiddlewareBlock(block *hcl.Block) (*Middleware, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	content, config, contentDiags := block.Body.PartialContent(middlewareBlockSchema)
	diags = append(diags, contentDiags...)

	mw := &Middleware{
		Name:      block.Labels[0],
		NameRange: block.LabelRanges[0],
		DeclRange: block.DefRange,
		Config:    config,
	}

	if attr, exists := content.Attributes["command"]; exists {
		mw.Command = attr.Expr
	}

	if attr, exists := content.Attributes["args"]; exists {
		exprs, argsDiags := hcl.ExprList(attr.Expr)
		diags = append(diags, argsDiags...)
		mw.Args = exprs
	}

	if attr, exists := content.Attributes["env"]; exists {
		// Evaluate the env attribute as a map of strings
		envVal, envDiags := attr.Expr.Value(nil)
		diags = append(diags, envDiags...)

		if envDiags.HasErrors() {
			return mw, diags
		}

		if !envVal.Type().IsObjectType() && !envVal.Type().IsMapType() {
			// Check if it's a map or object type
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid env attribute",
				Detail:   "The env attribute must be a map of strings.",
				Subject:  attr.Expr.Range().Ptr(),
			})
			return mw, diags
		}

		// Convert to map[string]string
		mw.Env = make(map[string]string)
		for it := envVal.ElementIterator(); it.Next(); {
			k, v := it.Element()

			// Convert key to string
			if k.Type() != cty.String {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid env key",
					Detail:   "Environment variable names must be strings.",
					Subject:  attr.Expr.Range().Ptr(),
				})
				continue
			}

			// Convert value to string
			if v.Type() != cty.String {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid env value",
					Detail:   "Environment variable values must be strings.",
					Subject:  attr.Expr.Range().Ptr(),
				})
				continue
			}

			mw.Env[k.AsString()] = v.AsString()
		}
	}

	return mw, diags
}

// moduleUniqueKey returns a unique key for this middleware within a module
func (m *Middleware) moduleUniqueKey() string {
	return m.Name
}

