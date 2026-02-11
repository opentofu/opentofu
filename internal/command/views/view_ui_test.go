// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/mitchellh/cli"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/terminal"
)

func TestViewUiHuman_OutputStreams(t *testing.T) {
	testCases := []struct {
		name         string
		fn           func(ui cli.Ui)
		expectStdout string
		expectStderr string
	}{
		{
			name: "Output goes to stdout",
			fn: func(ui cli.Ui) {
				ui.Output("test output message")
			},
			expectStdout: withNewline("test output message"),
			expectStderr: "",
		},
		{
			name: "Info goes to stdout",
			fn: func(ui cli.Ui) {
				ui.Info("test info message")
			},
			expectStdout: withNewline("test info message"),
			expectStderr: "",
		},
		{
			name: "Warn goes to stdout",
			fn: func(ui cli.Ui) {
				ui.Warn("test warning message")
			},
			expectStdout: withNewline("test warning message"),
			expectStderr: "",
		},
		{
			name: "Error goes to stderr",
			fn: func(ui cli.Ui) {
				ui.Error("test error message")
			},
			expectStdout: "",
			expectStderr: withNewline("test error message"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			streams, done := terminal.StreamsForTesting(t)
			view := NewView(streams)

			ui := NewViewUI(arguments.ViewOptions{ViewType: arguments.ViewHuman}, view, nil) // testing output only, no need for Ui

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
		fn           func(ui cli.Ui)
		expectStdout []map[string]any
	}{
		{
			name: "Output goes to stdout",
			fn: func(ui cli.Ui) {
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
		{
			name: "Info goes to stdout",
			fn: func(ui cli.Ui) {
				ui.Info("test info")
			},
			expectStdout: []map[string]any{
				{
					"@level":   "info",
					"@message": "test info",
					"@module":  "tofu.ui",
				},
			},
		},
		{
			name: "Warn goes to stdout",
			fn: func(ui cli.Ui) {
				ui.Warn("test warning")
			},
			expectStdout: []map[string]any{
				{
					"@level":   "warn",
					"@message": "test warning",
					"@module":  "tofu.ui",
				},
			},
		},
		{
			name: "Error goes to stderr (via JSON view)",
			fn: func(ui cli.Ui) {
				ui.Error("test error")
			},
			expectStdout: []map[string]any{
				{
					"@level":   "error",
					"@message": "test error",
					"@module":  "tofu.ui",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			streams, done := terminal.StreamsForTesting(t)
			view := NewView(streams)

			ui := NewViewUI(arguments.ViewOptions{ViewType: arguments.ViewJSON}, view, nil)

			tc.fn(ui)
			output := done(t)
			testJSONViewOutputEquals(t, output.Stdout(), tc.expectStdout)
		})
	}
}
