// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package uritemplates

import (
	"bufio"
	"bytes"
	"fmt"
	"unicode/utf8"
)

// newScanner returns a [bufio.Scanner] configured to tokenize the given template
// string using [uriTemplateSplit].
func newScanner(template string) *bufio.Scanner {
	sc := bufio.NewScanner(bytes.NewReader([]byte(template)))
	sc.Split(uriTemplateSplit)

	// Arbitrary maximum scan buffer size just to limit the impact of malicious
	// input. (Though of course any large input is already buffered into
	// "template" by the time we get here anyway.)
	const maxTokenSize = 1024
	sc.Buffer(nil, maxTokenSize)

	return sc
}

// uriTemplateSplit is a [`bufio.SplitFunc`] that deals with the first level of
// tokenization, splitting the input buffer into literal and expression parts.
//
// An expression part is any token that begins with the byte '{'.
func uriTemplateSplit(data []byte, atEOF bool) (int, []byte, error) {
	if len(data) == 0 {
		return 0, nil, nil // end of input
	}

	if data[0] == '{' {
		// Start of an expression token. We want to find the closing '}'
		// that marks the end of the expression token.
		idx := bytes.IndexByte(data, '}')
		if idx == -1 {
			if atEOF {
				return 0, nil, fmt.Errorf("unclosed URI template expression")
			}
			return 0, nil, nil // we need to buffer more bytes
		}

		// idx marks the closing brace, so our token ends immediately after that
		return idx + 1, data[:idx+1], nil
	}

	// If we start with anything other than { then this is a literal token, and
	// so we're searching for either an opening { or EOF to find the end of the
	// literal token.
	idx := bytes.IndexByte(data, '{')
	if idx == -1 {
		if !atEOF {
			return 0, nil, nil // we need to buffer more bytes
		}
		// The remainder of the data is literal
		return len(data), data, nil
	}

	// idx marks the opening brace, so our token ends immediately before that
	return idx, data[:idx], nil
}

// literalSplit is a [`bufio.SplitFunc`] that tokenizes a literal token previously
// found by uriTemplateSplit into individual UTF-8 sequences, except for percent-encoded
// sequences which are returned as a single token.
func literalSplit(data []byte, atEOF bool) (int, []byte, error) {
	if len(data) == 0 {
		return 0, nil, nil
	}

	if data[0] == '%' {
		if !startsWithValidPctEncoded(data) {
			return 0, nil, fmt.Errorf("invalid percent-encoded character")
		}
		return percentEncodedLength, data[:percentEncodedLength], nil
	}

	// For any other character we'll attempt to consume a whole UTF-8 sequence.
	r, size := utf8.DecodeRune(data)
	if r == utf8.RuneError {
		if atEOF || len(data) > 4 {
			return 0, nil, fmt.Errorf("invalid UTF-8 sequence")
		}
		return 0, nil, nil // we need to buffer more bytes
	}
	// Some characters are disallowed by the "literals" production.
	if r <= 0x1f || r == 0x7f {
		// control characters are all disallowed
		return 0, nil, fmt.Errorf("disallowed control character in literal")
	}
	switch r {
	case ' ', '"', '\'', '%', '<', '>', '\\', '^', '`', '{', '|', '}':
		return 0, nil, fmt.Errorf("disallowed character %q in literal", r)
	default:
		return size, data[:size], nil
	}
}

// variableListLevel1Split is a [`bufio.SplitFunc`] that tokenizes a sequence of
// bytes conforming to the "variable-list" production, yielding one token
// per comma-separated "varspec".
//
// The separating commas are not included in the result. This function only
// supports the level 3 subset of the template language, and so it will return
// an error if any modifiers are given for any of the variables.
func variableListLevel3Split(data []byte, atEOF bool) (int, []byte, error) {
	if len(data) == 0 {
		return 0, nil, nil
	}

	var ret []byte
	var advance int
	idx := bytes.IndexByte(data, ',')
	if idx == -1 {
		if !atEOF {
			return 0, nil, nil // we need to buffer more bytes
		}
		// The rest of the input is a single varspec
		ret = data
		advance = len(data)
	} else {
		// We're only interested in the prefix up to (and not including) the comma.
		ret = data[:idx]
		advance = idx + 1 // we want to advance over the comma, though
	}

	// "ret" should now match the "varname" production
	for remain := ret; len(remain) > 0; {
		switch b := remain[0]; b {
		case '%':
			if !startsWithValidPctEncoded(remain) {
				return 0, nil, fmt.Errorf("invalid percent-encoded character")
			}
		case ':', '*':
			return 0, nil, fmt.Errorf("level 4 modifier %q not allowed", b)
		default:
			if !((b == '_') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')) {
				return 0, nil, fmt.Errorf("invalid symbol %q in variable name", b)
			}
		}
		remain = remain[1:]
	}

	return advance, ret, nil
}

func startsWithValidPctEncoded(data []byte) bool {
	if len(data) < 3 || data[0] != '%' {
		return false
	}
	for _, b := range data[1:3] {
		if !((b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')) {
			return false
		}
	}
	return true
}
