// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import "testing"

func Test_commandInOpenState(t *testing.T) {
	type testCase struct {
		input    string
		expected int
	}

	tests := map[string]testCase{
		"plain braces": {
			input:    "{}",
			expected: 0,
		},
		"plain brackets": {
			input:    "[]",
			expected: 0,
		},
		"plain parentheses": {
			input:    "()",
			expected: 0,
		},
		"open braces": {
			input:    "{",
			expected: 1,
		},
		"open brackets": {
			input:    "[",
			expected: 1,
		},
		"open parentheses": {
			input:    "(",
			expected: 1,
		},
		"two open braces": {
			input:    "{{",
			expected: 2,
		},
		"two open brackets": {
			input:    "[[",
			expected: 2,
		},
		"two open parentheses": {
			input:    "((",
			expected: 2,
		},
		"open and closed braces": {
			input:    "{{}",
			expected: 1,
		},
		"open and closed brackets": {
			input:    "[[]",
			expected: 1,
		},
		"open and closed parentheses": {
			input:    "(()",
			expected: 1,
		},
		"mix braces and brackets": {
			input:    "{[]",
			expected: 1,
		},
		"mix brackets and parentheses": {
			input:    "[()",
			expected: 1,
		},
		"mix parentheses and braces": {
			input:    "({}",
			expected: 1,
		},
		"invalid braces": {
			input:    "{}}",
			expected: -1,
		},
		"invalid brackets": {
			input:    "[]]",
			expected: -1,
		},
		"invalid parentheses": {
			input:    "())",
			expected: -1,
		},
		"escaped new line": {
			input:    "\\",
			expected: 1,
		},
		"false positive new line": {
			input:    "\\\\",
			expected: 0,
		},
		"mix parentheses and new line": {
			input:    "(\\",
			expected: 2,
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			state := consoleBracketState{}
			_, actual := state.UpdateState(tc.input)
			if actual != tc.expected {
				t.Fatalf("Actual: %d, expected %d", actual, tc.expected)
			}
		})
	}
}

func Test_UpdateState(t *testing.T) {
	type testCase struct {
		inputs   []string
		expected int
	}

	tests := map[string]testCase{
		"plain braces": {
			inputs:   []string{"{", "}"},
			expected: 0,
		},
		"open brackets": {
			inputs:   []string{"[", "[", "]"},
			expected: 1,
		},
		"invalid parenthesis": {
			inputs:   []string{"(", ")", ")"},
			expected: -1,
		},
		"a fake brace": {
			inputs:   []string{"{", "\"}\"", "}"},
			expected: 0,
		},
		"a mixed bag": {
			inputs:   []string{"{", "}", "[", "...", "()", "]"},
			expected: 0,
		},
		"multiple open": {
			inputs:   []string{"{", "[", "("},
			expected: 3,
		},
		"escaped new line": {
			inputs:   []string{"\\"},
			expected: 1,
		},
		"false positive new line": {
			inputs:   []string{"\\\\"},
			expected: 0,
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			actual := 0
			state := consoleBracketState{}
			for _, input := range tc.inputs {
				_, actual = state.UpdateState(input)
			}

			if actual != tc.expected {
				t.Fatalf("Actual: %d, expected %d", actual, tc.expected)
			}
		})
	}
}

func Test_GetFullCommand(t *testing.T) {
	type testCase struct {
		inputs   []string
		expected []string
	}

	tests := map[string]testCase{
		"plain braces": {
			inputs:   []string{"{", "}"},
			expected: []string{"{", "{\n}"},
		},
		"open brackets": {
			inputs:   []string{"[", "[", "]"},
			expected: []string{"[", "[\n[", "[\n[\n]"},
		},
		"invalid parenthesis": {
			inputs:   []string{"(", ")", ")"},
			expected: []string{"(", "(\n)", ")"},
		},
		"a fake brace": {
			inputs:   []string{"{", "\"}\"", "}"},
			expected: []string{"{", "{\n\"}\"", "{\n\"}\"\n}"},
		},
		"a mixed bag": {
			inputs:   []string{"{", "}", "[", "...", "", "()", "]"},
			expected: []string{"{", "{\n}", "[", "[\n...", "[\n...", "[\n...\n()", "[\n...\n()\n]"},
		},
		"multiple open": {
			inputs:   []string{"{", "[", "("},
			expected: []string{"{", "{\n[", "{\n[\n("},
		},
		"escaped new line": {
			inputs:   []string{"\\"},
			expected: []string{""},
		},
		"false positive new line": {
			inputs:   []string{"\\\\"},
			expected: []string{"\\"},
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			state := consoleBracketState{}
			if len(tc.inputs) != len(tc.expected) {
				t.Fatalf("\nthe length of inputs: %d\n and expected: %d don't match", len(tc.inputs), len(tc.expected))
			}

			for i, input := range tc.inputs {
				actual, _ := state.UpdateState(input)
				if actual != tc.expected[i] {
					t.Fatalf("\nActual: %q\nexpected: %q", actual, tc.expected[i])
				}
			}
		})
	}
}
