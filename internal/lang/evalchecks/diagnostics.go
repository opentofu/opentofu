// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package evalchecks

import (
	"github.com/apparentlymart/go-shquot/shquot"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// This file contains some package-local helpers for working with diagnostics.
// For the main diagnostics API, see the separate "tfdiags" package.

// diagnosticCausedByUnknown is an implementation of
// tfdiags.DiagnosticExtraBecauseUnknown which we can use in the "Extra" field
// of a diagnostic to indicate that the problem was caused by unknown values
// being involved in an expression evaluation.
//
// When using this, set the Extra to diagnosticCausedByUnknown(true) and also
// populate the EvalContext and Expression fields of the diagnostic so that
// the diagnostic renderer can use all of that information together to assist
// the user in understanding what was unknown.
type DiagnosticCausedByUnknown bool

var _ tfdiags.DiagnosticExtraBecauseUnknown = DiagnosticCausedByUnknown(true)

func (e DiagnosticCausedByUnknown) DiagnosticCausedByUnknown() bool {
	return bool(e)
}

// diagnosticCausedBySensitive is an implementation of
// tfdiags.DiagnosticExtraBecauseSensitive which we can use in the "Extra" field
// of a diagnostic to indicate that the problem was caused by sensitive values
// being involved in an expression evaluation.
//
// When using this, set the Extra to diagnosticCausedBySensitive(true) and also
// populate the EvalContext and Expression fields of the diagnostic so that
// the diagnostic renderer can use all of that information together to assist
// the user in understanding what was sensitive.
type DiagnosticCausedBySensitive bool

var _ tfdiags.DiagnosticExtraBecauseSensitive = DiagnosticCausedBySensitive(true)

func (e DiagnosticCausedBySensitive) DiagnosticCausedBySensitive() bool {
	return bool(e)
}

// commandLineArgumentsSuggestion returns a representation of the given command line
// arguments that includes suitable quoting or escaping to make it more likely to work
// unaltered if copy-pasted into the command line on the current host system.
//
// We don't try to determine exactly what shell someone is using, so this won't be
// 100% correct in all cases but will at least deal with the most common hazards of
// quoting strings that contain spaces, and escaping some metacharacters.
//
// The "goos" argument should be the value of [runtime.GOOS] when calling this from
// real code, but can be fixed to a particular value when writing unit tests to ensure
// that they behave the same regardless of what platform is being used for development.
func commandLineArgumentsSuggestion(args []string, goos string) string {
	// For now we assume that there are only two possibilities: Windows or "POSIX-ish".
	// We use the "shquot" library for this, since we already have it as
	// a dependency for other purposes anyway.
	var quot shquot.QS
	switch goos {
	case "windows":
		// "WindowsArgvSplit" produces something that's compatible with
		// the de-facto standard argument parsing implemented by Microsoft's
		// Visual C++ runtime library, which the Go runtime mimics and
		// is therefore the most likely to succeed for running OpenTofu.
		//
		// Unfortunately running normal programs through PowerShell adds
		// some additional quoting/escaping hazards which we don't attend
		// to here because doing so tends to make the result invalid
		// for use outside of PowerShell.
		quot = shquot.WindowsArgvSplit
	default:
		// We'll assume that all other platforms we support are compatible
		// with the POSIX shell escaping rules.
		quot = shquot.POSIXShellSplit
	}

	// We're only interested in the "arguments" part of the result, but
	// the shquot library wants us to provide an argv[0] anyway so we'll
	// hard-code that as "tofu" but then ignore it altogether in the
	// result.
	cmdLine := append([]string{"tofu"}, args...)
	_, ret := quot(cmdLine)
	return ret
}
