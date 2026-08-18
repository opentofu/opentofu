// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Refresh represents the command-line arguments for the apply command.
type Refresh struct {
	// State, Operation, and Vars are the common extended flags
	State     *State
	Operation *Operation
	Vars      *Vars

	// View represents the global view options
	View *View
}

// BindRefresh registers CLI arguments, returning a Refresh value and it's corresponding hooks.
func BindRefresh(cli *CommandLine) *Refresh {
	return &Refresh{
		View:      BindView(cli, viewFlagAll),
		Vars:      BindVars(cli),
		Operation: BindOperation(cli),
		State:     BindState(cli, stateFlagAll),
	}
}

// ParseRefresh processes CLI arguments, returning a Refresh value, a closer function, and errors.
// If errors are encountered, a Refresh value is still returned representing
// the best effort interpretation of the arguments.
func ParseRefresh(args []string) (*Refresh, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	refresh := BindRefresh(cli)
	closer, diags := cli.parseWithHooks("refresh", args)
	return refresh, closer, diags
}
