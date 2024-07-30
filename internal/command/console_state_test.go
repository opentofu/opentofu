// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import "testing"

func Test_BracketsOpen(t *testing.T) {
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
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			state := consoleBracketState{}
			state.UpdateState(tc.input)

			actual := state.BracketsOpen()
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
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			state := consoleBracketState{}
			for _, input := range tc.inputs {
				state.UpdateState(input)
			}

			actual := state.BracketsOpen()
			if actual != tc.expected {
				t.Fatalf("Actual: %d, expected %d", actual, tc.expected)
			}
		})
	}
}

func Test_ClearState(t *testing.T) {
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
			expected: 0,
		},
		"invalid parenthesis": {
			inputs:   []string{"(", ")", ")"},
			expected: 0,
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
			expected: 0,
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			state := consoleBracketState{}
			for _, input := range tc.inputs {
				state.UpdateState(input)
			}

			state.ClearState()

			actual := state.BracketsOpen()
			if actual != tc.expected {
				t.Fatalf("Actual: %d, expected %d", actual, tc.expected)
			}
		})
	}
}

func Test_GetFullCommand(t *testing.T) {
	type testCase struct {
		inputs   []string
		expected string
	}

	tests := map[string]testCase{
		"plain braces": {
			inputs:   []string{"{", "}"},
			expected: "{\n}",
		},
		"open brackets": {
			inputs:   []string{"[", "[", "]"},
			expected: "[\n[\n]",
		},
		"invalid parenthesis": {
			inputs:   []string{"(", ")", ")"},
			expected: "(\n)\n)",
		},
		"a fake brace": {
			inputs:   []string{"{", "\"}\"", "}"},
			expected: "{\n\"}\"\n}",
		},
		"a mixed bag": {
			inputs:   []string{"{", "}", "[", "...", "", "()", "]"},
			expected: "{\n}\n[\n...\n()\n]",
		},
		"multiple open": {
			inputs:   []string{"{", "[", "("},
			expected: "{\n[\n(",
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			state := consoleBracketState{}
			for _, input := range tc.inputs {
				state.UpdateState(input)
			}

			actual := state.GetFullCommand()
			if actual != tc.expected {
				t.Fatalf("Actual: %s, expected %s", actual, tc.expected)
			}
		})
	}
}
