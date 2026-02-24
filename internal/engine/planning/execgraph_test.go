// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"bufio"
	"math"
	"strings"
)

// stripCommonLeadingTabs is a helper for tests that include constant strings
// for comparison with the result of a [Graph.DebugRepr] call, so that we can
// write the constant strings indentation to fit well with the surrounding
// context while ignoring the extra leading tabs that causes.
//
// This only considers tabs -- ignoring any other kind of whitespace -- because
// this is narrowly focused only on test code indentation and we always indent
// code using tabs.
func stripCommonLeadingTabs(s string) string {
	// We work in two passes here: first we scan over and find the smallest
	// number of leading tabs that all of the lines of the string have in
	// common, and then we do a second pass to build a new string with that
	// many leading tabs removed from each line.

	sc := bufio.NewScanner(strings.NewReader(s))
	sc.Split(bufio.ScanLines)
	minTabCount := math.MaxInt
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimLeft(line, "\t")
		if len(strings.TrimSpace(trimmed)) == 0 {
			continue // empty lines and lines containing only whitespace are ignored
		}
		tabCount := len(line) - len(trimmed)
		if tabCount < minTabCount {
			minTabCount = tabCount
		}
	}
	// (No error check because strings.Reader reads cannot fail)

	if minTabCount == math.MaxInt {
		// If minTabCount is still the max then that suggests we didn't have
		// any non-empty lines at all, and so there's nothing useful we can do
		// here.
		return ""
	}

	trimmedPrefix := strings.Repeat("\t", minTabCount)
	var buf strings.Builder
	sc = bufio.NewScanner(strings.NewReader(s))
	sc.Split(bufio.ScanLines)
	for sc.Scan() {
		// No error check because strings.Builder writes cannot fail.
		buf.WriteString(strings.TrimPrefix(sc.Text(), trimmedPrefix))
		buf.WriteByte('\n')
	}
	// (No error check because strings.Reader reads cannot fail)

	return buf.String()
}
