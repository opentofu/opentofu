// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"github.com/hashicorp/hcl/v2"

	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Input asks for input to fill unset required arguments in provider
// configurations.
//
// Unlike the other better-behaved operation methods, this one actually
// modifies some internal state inside the receiving context so that the
// captured values will be implicitly available to a subsequent call to Plan,
// or to some other operation entry point. Hopefully a future iteration of
// this will change design to make that data flow more explicit.
//
// Because Input saves the results inside the Context object, asking for
// input twice on the same Context is invalid and will lead to undefined
// behavior.
//
// Once you've called Input with a particular config, it's invalid to call
// any other Context method with a different config, because the aforementioned
// modified internal state won't match. Again, this is an architectural wart
// that we'll hopefully resolve in future.
func (c *Context) Input(config *configs.Config, mode InputMode) tfdiags.Diagnostics {
	// This function used to be responsible for more than it is now, so its
	// interface is more general than its current functionality requires.
	// It now exists only to handle interactive prompts for provider
	// configurations, with other prompts the responsibility of the CLI
	// layer prior to calling in to this package.
	//
	// (Hopefully in future the remaining functionality here can move to the
	// CLI layer too in order to avoid this odd situation where core code
	// produces UI input prompts.)

	var diags tfdiags.Diagnostics
	defer c.acquireRun("input")()

	// FIXME: Figure out what makes sense to do here when we have
	// dynamically-expanded provider instances. Maybe we just only
	// show prompts for the providers whose Alias expression is
	// a static keyword?
	// For now we just don't prompt at all because prompting isn't
	// crucial for OpenTofu's runtime behavior.

	return diags
}

// schemaForInputSniffing returns a transformed version of a given schema
// that marks all attributes as optional, which the Context.Input method can
// use to detect whether a required argument is set without missing arguments
// themselves generating errors.
func schemaForInputSniffing(schema *hcl.BodySchema) *hcl.BodySchema {
	ret := &hcl.BodySchema{
		Attributes: make([]hcl.AttributeSchema, len(schema.Attributes)),
		Blocks:     schema.Blocks,
	}

	for i, attrS := range schema.Attributes {
		ret.Attributes[i] = attrS
		ret.Attributes[i].Required = false
	}

	return ret
}
