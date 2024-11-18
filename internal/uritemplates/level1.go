// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package uritemplates

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

// ExpandLevel1 performs the "expansion" process, described in  [RFC 6570] section 3,
// on the level 1 template given in template, using the given variables.
//
// If the given template is invalid then this returns a partial expansion along with
// an error. If the template has multiple problems then it's unspecified which
// one this function will prefer to describe in its return value.
func ExpandLevel1(template string, vars map[string]string) (string, error) {
	var buf strings.Builder
	sc := newScanner(template)

	for sc.Scan() {
		tok := sc.Bytes()
		switch {
		case len(tok) > 0 && tok[0] == '{':
			if err := expandLevel1Expression(tok, vars, &buf); err != nil {
				return buf.String(), err
			}
		default:
			if err := expandLevel1Literal(tok, &buf); err != nil {
				return buf.String(), err
			}
		}
	}
	return buf.String(), sc.Err()
}

func expandLevel1Expression(tok []byte, vars map[string]string, into *strings.Builder) error {
	// We'll use our validate function to deal with the various ways the
	// input might be invalid.
	if err := validateLevel1Expression(tok); err != nil {
		return err
	}

	// We can now assume that we're holding a valid level 1 expression,
	// which means that everything between the brace delimiters would
	// be a single valid variable name.
	val := vars[string(tok[1:len(tok)-1])] // undefined variables are treated as empty string, per the spec
	into.Write(escapeVariableValue(val))
	return nil
}

func expandLevel1Literal(tok []byte, into *strings.Builder) error {
	sc := bufio.NewScanner(bytes.NewReader(tok))
	sc.Split(literalSplit)
	for sc.Scan() {
		// seq is one literal UTF-8 sequence or percent-encoding token
		// from the literal.
		seq := sc.Bytes()

		if seq[0] == '%' {
			// Percent-encoded sequences are copied verbatim.
			into.Write(seq)
			continue
		}

		// For literal UTF-8 sequences we can copy _most_ directly
		// to the destination, but the spec requires us to perform
		// percent-encoding of any character that wouldn't normally
		// be valid in a URI.
		into.Write(escapeLiteral(seq))
	}
	return sc.Err()
}

// ValidateLevel1 checks whether the given template is valid for URI Templates Level 1,
// as defined in [RFC 6570], returning an error if not.
//
// If this function returns nil then the template uses valid syntax and uses only the
// subset of template features defined for level 1.
//
// If the given template has multiple problems then it's unspecified which one this
// function will prefer to describe in its return value.
func ValidateLevel1(template string) error {
	sc := newScanner(template)

	for sc.Scan() {
		tok := sc.Bytes()
		switch {
		case len(tok) > 0 && tok[0] == '{':
			if err := validateLevel1Expression(tok); err != nil {
				return err
			}
		default:
			if err := validateLevel1Literal(tok); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}

func validateLevel1Expression(tok []byte) error {
	inner := tok[1 : len(tok)-1] // trim the surrounding braces that are always present
	if len(inner) == 0 {
		return fmt.Errorf("zero-length expression sequence")
	}

	// Level 1 templates support only a single named variable, with no operators.
	// To give a more helpful error message we'll recognize the specific operators
	// from higher spec levels and explicitly report that those levels are not
	// supported.
	switch op := inner[0]; op {
	case '+', '#':
		return fmt.Errorf("level 2 template expression operator %q not allowed; only level 1 templates are supported", op)
	case '.', '/', ';', '?', '&':
		return fmt.Errorf("level 3 template expression operator %q not allowed; only level 1 templates are supported", op)
	case '=', ',', '!', '@', '|':
		return fmt.Errorf("reserved template expression operator %q not allowed", op)
	}

	// The token is a valid expression if the variableListLevel3Split function
	// yields exactly one token without errors.
	sc := bufio.NewScanner(bytes.NewReader(inner))
	sc.Split(variableListLevel3Split)
	count := 0
	for sc.Scan() {
		count++
		if count > 1 {
			break // if we find more than one token then we're definitely invalid
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("expression may include only one variable name")
	}
	return nil
}

func validateLevel1Literal(tok []byte) error {
	// The token is a valid literal if we can scan it fully using
	// literalSplit without encountering any errors.
	sc := bufio.NewScanner(bytes.NewReader(tok))
	sc.Split(literalSplit)
	for sc.Scan() {
		// (we don't actually care about the content of the literals here)
	}
	return sc.Err()
}
