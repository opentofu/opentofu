// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Version represents the command-line arguments for the version command.
type Version struct {
	// View represents the global view options
	View *View
}

// BindVersion registers CLI arguments, returning a Version value and it's corresponding hooks.
func BindVersion(cli *CommandLine) *Version {
	ret := Version{
		View: BindView(cli, viewFlagJson),
	}

	// Enable but ignore the global version cli. In main.go, if any of the
	// arguments are -v, -version, or --version, this command will be called
	// with the rest of the arguments, so we need to be able to cope with
	// those.
	var nop bool
	cli.BoolVar(&nop, "v", true, "version").SetHidden(true)
	cli.BoolVar(&nop, "version", true, "version").SetHidden(true)

	return &ret
}

// ParseVersion processes CLI arguments, returning a Version value, a closer function, and errors.
// If errors are encountered, a Version value is still returned representing
// the best effort interpretation of the arguments.
func ParseVersion(args []string) (*Version, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	ret := BindVersion(cli)
	closer, diags := cli.parseWithHooks("version", args)
	return ret, closer, diags
}
