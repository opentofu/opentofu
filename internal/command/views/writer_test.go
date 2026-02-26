// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0
package views

import "testing"

func TestWriter(t *testing.T) {
	in := []string{
		"line with no new line",
		"line with new line\n",
		"", // empty line
	}
	var lines []string
	w := &writer{writeFn: func(msg string) {
		lines = append(lines, msg)
	}}
	for _, s := range in {
		_, _ = w.Write([]byte(s))
	}
	expected := []string{
		"line with no new line",
		"line with new line",
		"",
	}
	if len(expected) != len(lines) {
		t.Fatalf("different number of entries (%d) than wanted (%d)", len(lines), len(expected))
	}
	for i, want := range expected {
		if want != lines[i] {
			t.Errorf("element %d different than the expected one.\n\texpected: %s\n\tgot:%s\n", i, want, lines[i])
		}
	}
}
