// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/terminal"
)

func TestViewUiHuman_OutputStreams(t *testing.T) {
	testCases := []struct {
		name         string
		fn           func(ui Ui)
		expectStdout string
		expectStderr string
	}{
		{
			name: "Output goes to stdout",
			fn: func(ui Ui) {
				ui.Output("test output message")
			},
			expectStdout: withNewline("test output message"),
			expectStderr: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			streams, done := terminal.StreamsForTesting(t)
			view := NewView(streams)

			ui := NewViewUI(&arguments.View{ViewType: arguments.ViewHuman}, view, nil) // testing output only, no need for Ui

			tc.fn(ui)
			output := done(t)
			if diff := cmp.Diff(tc.expectStderr, output.Stderr()); diff != "" {
				t.Errorf("invalid stderr (-want, +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.expectStdout, output.Stdout()); diff != "" {
				t.Errorf("invalid stdout (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestViewUiJSON_OutputStreams(t *testing.T) {
	testCases := []struct {
		name         string
		fn           func(ui Ui)
		expectStdout []map[string]any
	}{
		{
			name: "Output goes to stdout",
			fn: func(ui Ui) {
				ui.Output("test output")
			},
			expectStdout: []map[string]any{
				{
					"@level":   "info",
					"@message": "test output",
					"@module":  "tofu.ui",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			streams, done := terminal.StreamsForTesting(t)
			view := NewView(streams)

			ui := NewViewUI(&arguments.View{ViewType: arguments.ViewJSON}, view, nil)

			tc.fn(ui)
			output := done(t)
			testJSONViewOutputEquals(t, output.Stdout(), tc.expectStdout)
		})
	}
}
