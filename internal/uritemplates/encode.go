// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package uritemplates

import (
	"regexp"
)

const hexChars = "0123456789abcdef"
const percentEncodedLength = 3

// literalRequiringEscape is a regular expression pattern that matches byte sequences
// that require percent-encoding if they appear in a literal, as defined in [RFC 6570]
// section 3.1.
//
// This matches any sequence that isn't one of the "reserved" or "unreserved" characters
// defined in [RFC 3986].
var literalRequiringEscape = regexp.MustCompile(`[^A-Za-z0-9\-._~:/?#[\]@!$&'()*+,;=]+`)

// variableRequiringEscape is a regular expression pattern that matches byte sequences
// that require percent-encoding if they appear in a literal or matched variable value,
// as defined in [RFC 6570] section 3.1.
//
// This matches any sequence that isn't already percent-encoded or one of the "unreserved"
// characters defined in [RFC 3986]. In particular notice that this causes the "reserved"
// characters to be escaped to ensure that they are interpreted literally rather than as
// meaningful URI punctuation.
var variableRequiringEscape = regexp.MustCompile(`[^A-Za-z0-9\-._~]+`)

// escapeLiteral returns an escaped version of the given literal byte sequence, ready to
// be inserted verbatim into the result of template expansion.
//
// The given bytes must _actually_ be literal; any percent-encoded sequences in the original
// input must not be passed here unless their percent signs are to be taken literally.
func escapeLiteral(src []byte) []byte {
	return literalRequiringEscape.ReplaceAllFunc(src, percentEncode)
}

// escapeVariableValue returns an escaped version of the given variable value, ready
// to be inserted verbatim into the result of template expansion.
func escapeVariableValue(src string) []byte {
	return variableRequiringEscape.ReplaceAllFunc([]byte(src), percentEncode)
}

func percentEncode(src []byte) []byte {
	const hexDigitCount = len(hexChars)

	ret := make([]byte, len(src)*percentEncodedLength)
	for idx, b := range src {
		part := ret[idx*percentEncodedLength : idx*percentEncodedLength+percentEncodedLength]
		part[0] = '%'
		part[1] = hexChars[int(b)/hexDigitCount]
		part[2] = hexChars[int(b)%hexDigitCount]
	}
	return ret
}
