// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package format

import (
	"strings"
)

// unicodeControlPicturesStart is the codepoint of the first character in the
// Unicode "Control Pictures" block.
//
// The first 32 codepoints in this block correlate with the control characters
// in the first 32 codepoints of the "Basic Latin" block, so a control character
// codepoint can be translated into its corresponding control picture codepoint
// by adding this constant.
const unicodeControlPicturesStart = rune(0x2400)

const del = rune(0x7f)
const delPicture = rune(0x2421)

// ReplaceControlChars translates 7-bit C0 control characters in the given string
// (character codes less than 32) into their corresponding symbols from the
// Unicode "Control Pictures" block, so that the result can be printed to a
// terminal-like device without affecting the terminal's state machine.
//
// As an exception this does not change control characters that commonly appear
// as part of human-oriented text: newline (0x0a), carriage return (0x0d),
// and horizontal tab (0x09).
//
// We use this when including untrusted data as part of "human-friendly"
// output. We use the Unicode control pictures so that a human reader can
// (with a suitably-equipped terminal font) still identify which specific
// control character appeared, in case that is helpful for debugging, and
// because they are relatively unlikely to appear literally in a string we're
// rendering in the UI.
//
// This is only for arbitrary text strings rendered directly in the UI,
// such as the message portions of rendered diagnostics. We need not use this
// when producing machine-readable output such as JSON representations, or when
// showing a string in a quoted notation that mimics either the HCL or Go string
// syntax, because the control characters are already backslash-escaped by the
// quoting process in those cases. We also don't need to use this for strings
// that are known to contain valid HCL identifiers, because the control
// characters are not valid for use in HCL's identifier tokens.
func ReplaceControlChars(input string) string {
	// In the common case there are no relevant control characters at all, so
	// we'll first scan the string to see if we can return the input verbatim
	// and thus avoid allocating a new copy of that string.
	if !strings.ContainsFunc(input, isFilteredControlChar) {
		return input
	}

	// If we get here then we definitely need to build a new string.
	var buf strings.Builder
	for _, r := range input {
		if !isFilteredControlChar(r) {
			// Writing to a [strings.Builder] never encounters an error.
			_, _ = buf.WriteRune(r)
			continue
		}
		// If we get here then seq is definitely an ineligible C0 control
		// character, so we need to transform it into the 3-byte encoding of the
		// corresponding Control Picture codepoint.
		// Writing to a [strings.Builder] never encounters an error.
		_, _ = buf.WriteRune(controlPicture(r))
	}
	return buf.String()
}

// isFilteredControlChar returns true if and only if the given rune is in the
// range of 7-bit C0 control characters.
func isFilteredControlChar(r rune) bool {
	// Space (0x20) is the first non-control character
	return (r < ' ' && r != '\r' && r != '\n' && r != '\t') || r == del
}

// controlPicture returns the control picture equivalent of the given C0 control
// character, or returns the given character verbatim if it is not actually
// a C0 control character.
func controlPicture(ctrl rune) rune {
	if ctrl < ' ' {
		return ctrl + unicodeControlPicturesStart
	}
	if ctrl == del {
		return delPicture
	}
	return ctrl
}
