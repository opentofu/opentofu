// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Taint represents the command-line arguments for the taint and untaint commands.
type Taint struct {
	// TargetAddress is the resource address that is requested to be marked as tainted.
	TargetAddress addrs.AbsResourceInstance
	// AllowMissing can be set to "true" to write a warning instead of an error and to return exit code 0
	// when the TargetAddress points to a missing resource.
	AllowMissing bool

	// View represents the global view options
	View *View

	// Vars holds and provides information for the flags related to variables that a user can give into the process
	Vars *Vars

	// State is used for the state related flags
	State *State
	// Backend is used strictly for the ignore remote version flag
	Backend *Backend
}

// BindTaint registers CLI arguments, returning a Taint value and it's corresponding hooks.
func BindTaint(cli *CommandLine, isTaint bool) *Taint {
	arguments := Taint{
		View:    BindView(cli, viewFlagNoInput),
		Vars:    BindVars(cli),
		Backend: BindBackend(cli),
		State:   BindState(cli, stateFlagAll),
	}

	cli.BoolVar(&arguments.AllowMissing, "allow-missing", false, "If specified, the command will succeed (exit code 0) even if the resource is missing.")

	var rawAddr string
	cli.PositionalArg(&rawAddr, "resource address", false)
	cli.PreHook(func() tfdiags.Diagnostics {
		addr, diags := addrs.ParseAbsResourceInstanceStr(rawAddr)
		arguments.TargetAddress = addr
		if addr.Resource.Resource.Mode != addrs.ManagedResourceMode && isTaint {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid resource address",
				fmt.Sprintf("Resource instance %s cannot be tainted", addr),
			))
		}
		return diags
	})

	return &arguments
}

// ParseTaint processes CLI arguments, returning a Taint value, a closer function, and errors.
// If errors are encountered, a Taint value is still returned representing
// the best effort interpretation of the arguments.
func ParseTaint(isTaint bool, args []string) (*Taint, func(), tfdiags.Diagnostics) {
	cmd := "taint"
	if !isTaint {
		cmd = "untaint"
	}

	cli := new(CommandLine)
	arguments := BindTaint(cli, isTaint)
	closer, diags := cli.parseWithHooks(cmd, args)
	return arguments, closer, diags
}
