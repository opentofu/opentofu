// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	shellwords "github.com/mattn/go-shellwords"
)

const (
	// EnvCLI is the environment variable name to set additional CLI args.
	EnvCLI = "TF_CLI_ARGS"
)

// expandFileBasedArgs searches the given args for elements whose first character is
// "@" (the "at" sign) and attempts to replace them with zero or more arguments
// parsed from a file named by the remaining characters in the string.
//
// If the remainder of the string cannot be treated as a filename for opening and
// reading then the argument is left unchanged, leaving it up to a subsequent
// codepath to decide how to treat it.
//
// The file contents are parsed using a Unix-shell-like tokenization scheme to
// produce a vector of additional arguments that will replace the original
// argument.
func expandFileBasedArgs(args []string) ([]string, error) {
	// We assume that in the common case there will be no @-based arguments to
	// expand and so we will wait to allocate a new slice until we know we
	// need it. A nil "ret" at the end of this function therefore means that
	// we should return args verbatim.
	var ret []string
	for i, arg := range args {
		if !strings.HasPrefix(arg, "@") {
			if ret != nil { // only if we've already started building a new arg vector on a previous iteration
				ret = append(ret, arg)
			}
			continue
		}
		filename := arg[1:]
		raw, err := os.ReadFile(filename)
		if err != nil {
			// We intentionally ignore errors here and just retain the
			// original argument verbatim, assuming that the user intended
			// to write a literal argument that starts with @.
			if ret != nil { // only if we've already started building a new arg vector on a previous iteration
				ret = append(ret, arg)
			}
			continue
		}
		extra, err := shellwords.Parse(string(raw))
		if err != nil {
			// In this case it seems more likely that the operator _was_ intending
			// for this file to be treated as a set of additional arguments, but
			// it contains some sort of syntax error like an unmatched opening
			// quote. Therefore we'll return an error in this case to hopefully
			// give the operator better feedback about the problem.
			return args, fmt.Errorf("failed to expand %q argument: %w", arg, err)
		}

		// If we've got this far then we're definitely returning a different
		// args vector than we were given, so we'll allocate it now if we
		// didn't already.
		if ret == nil {
			// We know that we need at least enough space for all of the
			// given args and the result of the one expansion we're currently
			// working on, so we'll preallocate that but this might get
			// reallocated by the appends below on a future iteration if
			// there are multiple @-arguments.
			// (-1 below because the "extra" elements are replacing our current "arg")
			ret = make([]string, 0, len(args)+len(extra)-1)
			ret = append(ret, args[:i]...) // start with any arguments that we already passed over
		}
		ret = append(ret, extra...)
	}
	if ret == nil {
		return args, nil // We did not find any expandable arguments, so we'll return what we were given.
	}
	return ret, nil
}

func mergeEnvArgs(envName string, cmd string, args []string) ([]string, error) {
	v := os.Getenv(envName)
	if v == "" {
		return args, nil
	}

	log.Printf("[INFO] %s value: %q", envName, v)
	extra, err := shellwords.Parse(v)
	if err != nil {
		return nil, fmt.Errorf("error parsing extra CLI args from %s: %w", envName, err)
	}

	// Find the command to look for in the args. If there is a space,
	// we need to find the last part.
	search := cmd
	if idx := strings.LastIndex(search, " "); idx >= 0 {
		search = cmd[idx+1:]
	}

	// Find the index to place the flags. We put them exactly
	// after the first non-flag arg.
	idx := -1
	for i, v := range args {
		if v == search {
			idx = i
			break
		}
	}

	// idx points to the exact arg that isn't a flag. We increment
	// by one so that all the copying below expects idx to be the
	// insertion point.
	idx++

	// Copy the args
	newArgs := make([]string, len(args)+len(extra))
	copy(newArgs, args[:idx])
	copy(newArgs[idx:], extra)
	copy(newArgs[len(extra)+idx:], args[idx:])
	return newArgs, nil
}
