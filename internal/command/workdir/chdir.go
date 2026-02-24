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
	// The arguments can begin with a -chdir option to ask OpenTofu to switch
	// to a different working directory for the rest of its work. If that
	// option is present then extractChdirOption returns a trimmed args with that option removed.
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
// TODO meta-refactor: remove this once the current CLI library is replaced.
func extractChdirOption(args []string) (string, []string, error) {
	if len(args) == 0 {
		return "", args, nil
	}

	const argName = "-chdir"
	const argPrefix = argName + "="
	var argValue string
	var argPos int

	for i, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			// Because the chdir option is a subcommand-agnostic one, we require
			// it to appear before any subcommand argument, so if we find a
			// non-option before we find -chdir then we are finished.
			break
		}
		if arg == argName || arg == argPrefix {
			return "", args, fmt.Errorf("must include an equals sign followed by a directory path, like -chdir=example")
		}
		if strings.HasPrefix(arg, argPrefix) {
			argPos = i
			argValue = arg[len(argPrefix):]
		}
	}

	// When we fall out here, we'll have populated argValue with a non-empty
	// string if the -chdir=... option was present and valid, or left it
	// empty if it wasn't present.
	if argValue == "" {
		return "", args, nil
	}

	// If we did find the option then we'll need to produce a new args that
	// doesn't include it anymore.
	if argPos == 0 {
		// Easy case: we can just slice off the front
		return argValue, args[1:], nil
	}
	// Otherwise we need to construct a new array and copy to it.
	newArgs := make([]string, len(args)-1)
	copy(newArgs, args[:argPos])
	copy(newArgs[argPos:], args[argPos+1:])
	return argValue, newArgs, nil
}
