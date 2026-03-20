// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

const (
	stdinArg = "-"
)

// Fmt represents the command-line arguments for the fmt command.
type Fmt struct {
	// Paths contains the file paths that the formatter will handle.
	// When no arguments given to the command, it will use the current directory.
	// If the first argument is -, it will read the content to format from [os.Stdin].
	Paths []string

	// List controls the output of the formatted list. If disabled, it will not print the
	// names of the formatted files.
	List bool
	// Write controls if the formatter should write the content back to the check file or not.
	Write bool
	// Diff tells to the formatter to print the diff between the before and after formatting
	// process.
	Diff bool
	// Check can be used to instruct the command to return a non-zero error code if it finds
	// any file that is not properly formatted.
	Check bool
	// Recursive indicates that the formatting should be done recursive through all the
	// subdirectories.
	Recursive bool

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions
}

// ParseFmt processes CLI arguments, returning a Fmt value, a closer function, and errors.
// If errors are encountered, a Fmt value is still returned representing
// the best effort interpretation of the arguments.
func ParseFmt(args []string) (*Fmt, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	ret := &Fmt{}

	cmdFlags := defaultFlagSet("fmt")
	cmdFlags.BoolVar(&ret.List, "list", true, "list")
	cmdFlags.BoolVar(&ret.Write, "write", true, "write")
	cmdFlags.BoolVar(&ret.Diff, "diff", false, "diff")
	cmdFlags.BoolVar(&ret.Check, "check", false, "check")
	cmdFlags.BoolVar(&ret.Recursive, "recursive", false, "recursive")

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	args = cmdFlags.Args()
	if len(args) == 0 {
		ret.Paths = []string{"."}
	} else if args[0] == stdinArg {
		ret.List = false
		ret.Write = false
	} else {
		ret.Paths = args
	}

	// we only parse but do not register the views flags since this command does not need it
	closer, moreDiags := ret.ViewOptions.Parse()
	diags = diags.Append(moreDiags)

	return ret, closer, diags
}
