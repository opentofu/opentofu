// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"strings"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type PushCommand struct {
	Meta
}

func (c *PushCommand) Run(rawArgs []string) int {
	common, _ := arguments.ParseView(rawArgs)
	c.View.Configure(common)
	// This command is no longer supported, but we'll retain it just to
	// give the user some next-steps after upgrading.
	c.View.Diagnostics(tfdiags.Diagnostics{tfdiags.Sourceless(
		tfdiags.Error,
		"Command \"tofu push\" is no longer supported",
		"This command was used to push configuration to Terraform Enterprise legacy (v1), which has now reached end-of-life. To push configuration to a new cloud backend, use its REST API.",
	)})
	return 1
}

func (c *PushCommand) Help() string {
	helpText := `
Usage: tofu [global options] push [options] [DIR]

  This command was for the legacy version of Terraform Enterprise (v1), which
  has now reached end-of-life. Therefore this command is no longer supported.
`
	return strings.TrimSpace(helpText)
}

func (c *PushCommand) Synopsis() string {
	return "Obsolete command for Terraform Enterprise legacy (v1)"
}
