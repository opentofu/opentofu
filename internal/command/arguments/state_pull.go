// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// StatePull represents the command-line arguments for the 'state pull' command.
type StatePull struct {
	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions

	// Vars are the common extended flags
	Vars *Vars
}

// BindStatePull registers CLI arguments, returning a StatePull value and it's corresponding hooks.
func BindStatePull(cli *CommandLine) *StatePull {
	var ret StatePull

	// we only parse but do not register the views flags since this command does not need it because it already
	// prints the state in json format
	ret.ViewOptions.ParseHook(cli)

	ret.Vars = BindVars(cli)

	return &ret
}

// ParseStatePull processes CLI arguments, returning a StatePull value, a closer function, and errors.
// If errors are encountered, a StatePull value is still returned representing
// the best effort interpretation of the arguments.
func ParseStatePull(args []string) (*StatePull, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	ret := BindStatePull(cli)
	closer, diags := cli.Stdlib("state pull", args)
	return ret, closer, diags
}
