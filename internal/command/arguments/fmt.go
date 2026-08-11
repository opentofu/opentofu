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

	// View represents the global view options
	View *View
}

// BindFmt registers CLI arguments, returning a Fmt value and it's corresponding hooks.
func BindFmt(cli *CommandLine) *Fmt {
	ret := Fmt{
		View: BindView(cli, viewFlagNone),
	}

	cli.BoolVar(&ret.List, "list", true, "Don't list files whose formatting differs (always disabled if using STDIN)").SetDisplay("=false")
	cli.BoolVar(&ret.Write, "write", true, "Don't write to source files (always disabled if using STDIN or -check)").SetDisplay("=false")
	cli.BoolVar(&ret.Diff, "diff", false, "Display diffs of formatting changes")
	cli.BoolVar(&ret.Check, "check", false, "Check if the input is formatted. Exit status will be 0 if all input is properly formatted and non-zero otherwise.")
	cli.BoolVar(&ret.Recursive, "recursive", false, "Also process files in subdirectories. By default, only the given directory (or current directory) is processed.")

	cli.VariadicArg(&ret.Paths, "paths")
	cli.PreHook(func() tfdiags.Diagnostics {
		if len(ret.Paths) == 0 {
			ret.Paths = []string{"."}
		} else if ret.Paths[0] == stdinArg {
			ret.Paths = nil
			ret.List = false
			ret.Write = false
		}
		return nil
	})

	return &ret
}

// ParseFmt processes CLI arguments, returning a Fmt value, a closer function, and errors.
// If errors are encountered, a Fmt value is still returned representing
// the best effort interpretation of the arguments.
func ParseFmt(args []string) (*Fmt, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	ret := BindFmt(cli)
	closer, diags := cli.parseWithHooks("fmt", args)
	return ret, closer, diags
}
