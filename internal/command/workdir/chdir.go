// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package workdir

import (
	"fmt"
	"os"
	"strings"
)

// runChdir gets the args that the program was executed with, extracts the
// -chdir flag from it and runs [os.Chdir] with the associated flag value, if any provided.
//
// It returns back the list of arguments containing without the -chdir flag and value to be used
// later by the invoked command.
// TODO meta-refactor: this is a temporary solution until we will switch the CLI library that
// will be able to handle root-only flags in a similar fashion with what we are doing here manually.
func runChdir(args []string) ([]string, error) {
	overrideWd, args, err := extractChdirOption(args)
	if err != nil {
		return nil, fmt.Errorf("Invalid -chdir option: %s", err)
	}
	if overrideWd != "" {
		err := os.Chdir(overrideWd)
		if err != nil {
			return nil, fmt.Errorf("Error handling -chdir option: %s", err)
		}
	}
	return args, nil
}

// extractChdirOption extracts the -chdir flag together with the associated flag value, from the given args,
// if any provided.
//
// It returns back the list of arguments without the -chdir flag and value to be used later by the invoked command.
// This returns also an error in case the -chdir flag is given with the wrong values.
//
// Supported syntaxes:
//   - -chdir=<dir>
//   - --chdir=<dir>
//   - -chdir <dir>
//   - --chdir <dir>
//
// TODO meta-refactor: remove this once the current CLI library is replaced.
func extractChdirOption(args []string) (string, []string, error) {
	if len(args) == 0 {
		return "", args, nil
	}

	const argNameSingle = "-chdir"
	const argNameDouble = "--chdir"
	const argPrefixSingle = argNameSingle + "="
	const argPrefixDouble = argNameDouble + "="

	var argValue string
	var argPos int
	var argLen int // 1 for -chdir=x, 2 for -chdir x

	for i, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			// chdir must appear before any subcommand, so stop
			// as soon as we see a non-option argument.
			break
		}

		// Handle -chdir=<dir> and --chdir=<dir>
		if strings.HasPrefix(arg, argPrefixSingle) || strings.HasPrefix(arg, argPrefixDouble) {
			value := arg[strings.Index(arg, "=")+1:]
			if value == "" {
				return "", args, fmt.Errorf("must include a directory path after the equals sign, like -chdir=example")
			}
			argPos = i
			argValue = value
			argLen = 1
			break
		}

		// Handle bare -chdir or --chdir (space separated)
		if arg == argNameSingle || arg == argNameDouble {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return "", args, fmt.Errorf("must include a directory path, like -chdir=example or -chdir example")
			}
			argPos = i
			argValue = args[i+1]
			argLen = 2
			break
		}
	}

	if argValue == "" {
		return "", args, nil
	}

	// Build new args slice with the -chdir flag (and value if space-separated) removed.
	newArgs := make([]string, 0, len(args)-argLen)
	newArgs = append(newArgs, args[:argPos]...)
	newArgs = append(newArgs, args[argPos+argLen:]...)

	return argValue, newArgs, nil
}