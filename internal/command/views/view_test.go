// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/terminal"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu"
)

// TestNewView it's just a sanity check to be sure that we have it initialized as expected.
func TestNewView(t *testing.T) {
	streams, done := terminal.StreamsForTesting(t)
	view := NewView(streams)

	if view.streams != streams {
		t.Error("expected streams to be set")
	}
	if view.colorize == nil {
		t.Error("expected colorize to be initialized")
	}
	if !view.colorize.Disable {
		t.Error("expected colorize to be disabled by default")
	}
	if view.errorColor != "[red]" {
		t.Errorf("expected errorColor to be [red], got %s", view.errorColor)
	}
	if view.warnColor != "[yellow]" {
		t.Errorf("expected warnColor to be [yellow], got %s", view.warnColor)
	}
	if view.configSources == nil {
		t.Error("expected configSources to be initialized")
	}
	if view.diagsPrinter == nil {
		t.Error("expected diagsPrinter to be initialized")
	}
	// Test that configSources returns nil by default
	if sources := view.configSources(); sources != nil {
		t.Errorf("expected configSources to return nil by default, got %v", sources)
	}

	_ = done(t)
}

func TestView_Configure(t *testing.T) {
	testCases := map[string]struct {
		viewArgs *arguments.View
		validate func(*testing.T, *View)
	}{
		"no color": {

			viewArgs: &arguments.View{
				NoColor: true,
			},
			validate: func(t *testing.T, v *View) {
				if !v.colorize.Disable {
					t.Error("expected colorize to be disabled")
				}
			},
		},
		"with color": {

			viewArgs: &arguments.View{
				NoColor: false,
			},
			validate: func(t *testing.T, v *View) {
				if v.colorize.Disable {
					t.Error("expected colorize to be enabled")
				}
			},
		},
		"compact warnings": {

			viewArgs: &arguments.View{
				CompactWarnings: true,
			},
			validate: func(t *testing.T, v *View) {
				if !v.compactWarnings {
					t.Error("expected compactWarnings to be true")
				}
			},
		},
		"consolidate warnings": {

			viewArgs: &arguments.View{
				ConsolidateWarnings: true,
			},
			validate: func(t *testing.T, v *View) {
				if !v.consolidateWarnings {
					t.Error("expected consolidateWarnings to be true")
				}
			},
		},
		"consolidate errors": {

			viewArgs: &arguments.View{
				ConsolidateErrors: true,
			},
			validate: func(t *testing.T, v *View) {
				if !v.consolidateErrors {
					t.Error("expected consolidateErrors to be true")
				}
			},
		},
		"concise": {

			viewArgs: &arguments.View{
				Concise: true,
			},
			validate: func(t *testing.T, v *View) {
				if !v.concise {
					t.Error("expected concise to be true")
				}
			},
		},
		"module deprecation warning level": {

			viewArgs: &arguments.View{
				ModuleDeprecationWarnLvl: tofu.DeprecationWarningLevelLocal,
			},
			validate: func(t *testing.T, v *View) {
				if v.ModuleDeprecationWarnLvl != tofu.DeprecationWarningLevelLocal {
					t.Errorf("expected ModuleDeprecationWarnLvl to be Local, got %v", v.ModuleDeprecationWarnLvl)
				}
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			streams, done := terminal.StreamsForTesting(t)
			view := NewView(streams)
			view.Configure(tc.viewArgs)
			tc.validate(t, view)
			purpleVal, ok := view.colorize.Colors["purple"]
			const expectedPurpleVal = "38;5;57"
			if !ok {
				t.Error("expected purple color to be added to colorize colors")
			}
			if purpleVal != expectedPurpleVal {
				t.Errorf("expected purple color code to be %s, got %s", expectedPurpleVal, purpleVal)
			}

			_ = done(t)
		})
	}
}

func TestView_Diagnostics(t *testing.T) {
	testCases := map[string]struct {
		diags    tfdiags.Diagnostics
		setup    func(*View)
		validate func(*testing.T, *terminal.TestOutput)
	}{
		"empty diagnostics": {

			diags: tfdiags.Diagnostics{},
			validate: func(t *testing.T, output *terminal.TestOutput) {
				if output.Stdout() != "" {
					t.Errorf("expected empty stdout, got %q", output.Stdout())
				}
				if output.Stderr() != "" {
					t.Errorf("expected empty stderr, got %q", output.Stderr())
				}
			},
		},
		"warning diagnostic": {

			diags: tfdiags.Diagnostics{
				tfdiags.Sourceless(
					tfdiags.Warning,
					"Test warning",
					"This is a test warning",
				),
			},
			validate: func(t *testing.T, output *terminal.TestOutput) {
				stdout := output.Stdout()
				if !strings.Contains(stdout, "Warning") {
					t.Errorf("expected stdout to contain 'Warning', got %q", stdout)
				}
				if !strings.Contains(stdout, "Test warning") {
					t.Errorf("expected stdout to contain 'Test warning', got %q", stdout)
				}
			},
		},
		"error diagnostic": {

			diags: tfdiags.Diagnostics{
				tfdiags.Sourceless(
					tfdiags.Error,
					"Test error",
					"This is a test error",
				),
			},
			validate: func(t *testing.T, output *terminal.TestOutput) {
				stderr := output.Stderr()
				if !strings.Contains(stderr, "Error") {
					t.Errorf("expected stderr to contain 'Error', got %q", stderr)
				}
				if !strings.Contains(stderr, "Test error") {
					t.Errorf("expected stderr to contain 'Test error', got %q", stderr)
				}
			},
		},
		"multiple diagnostics": {

			diags: tfdiags.Diagnostics{
				tfdiags.Sourceless(
					tfdiags.Warning,
					"Warning 1",
					"First warning",
				),
				tfdiags.Sourceless(
					tfdiags.Error,
					"Error 1",
					"First error",
				),
			},
			validate: func(t *testing.T, output *terminal.TestOutput) {
				stdout := output.Stdout()
				stderr := output.Stderr()

				if !strings.Contains(stdout, "Warning 1") {
					t.Errorf("expected stdout to contain warning, got %q", stdout)
				}
				if !strings.Contains(stderr, "Error 1") {
					t.Errorf("expected stderr to contain error, got %q", stderr)
				}
			},
		},
		"multiple diagnostics with newline": {

			setup: func(view *View) {
				view.DiagsWithNewline()
			},
			diags: tfdiags.Diagnostics{
				tfdiags.Sourceless(
					tfdiags.Warning,
					"Warning 1",
					"First warning",
				),
				tfdiags.Sourceless(
					tfdiags.Error,
					"Error 1",
					"First error",
				),
			},
			validate: func(t *testing.T, output *terminal.TestOutput) {
				stdout := output.Stdout()
				stderr := output.Stderr()

				if !strings.Contains(stdout, "Warning 1") {
					t.Errorf("expected stdout to contain warning, got %q", stdout)
				}
				if !strings.HasSuffix(stdout, "\n") {
					t.Fatalf("expected stdout to end in new line after the warning diagnostic")
				}
				if !strings.Contains(stderr, "Error 1") {
					t.Errorf("expected stderr to contain error, got %q", stderr)
				}
				if !strings.HasSuffix(stderr, "\n") {
					t.Fatalf("expected sstderr to end in new line after the error diagnostic")
				}
			},
		},
		"compact warnings - warnings only": {

			diags: tfdiags.Diagnostics{
				tfdiags.Sourceless(
					tfdiags.Warning,
					"Warning 1",
					"First warning detail",
				),
				tfdiags.Sourceless(
					tfdiags.Warning,
					"Warning 2",
					"Second warning detail",
				),
			},
			setup: func(v *View) {
				v.compactWarnings = true
			},
			validate: func(t *testing.T, output *terminal.TestOutput) {
				stdout := output.Stdout()
				if !strings.Contains(stdout, "Warnings:") {
					t.Errorf("expected compact warnings format, got %q", stdout)
				}
			},
		},
		"consolidate warnings": {

			diags: tfdiags.Diagnostics{}.
				Append(&hcl.Diagnostic{
					Severity: hcl.DiagWarning,
					Summary:  "Warning with source",
					Detail:   "foo bar warning",
					Subject:  hcl.RangeBetween(hcl.Range{Filename: "test.tf"}, hcl.Range{Filename: "test.tf"}).Ptr(),
				}).
				Append(&hcl.Diagnostic{
					Severity: hcl.DiagWarning,
					Summary:  "Warning with source",
					Detail:   "foo bar other warning",
					Subject:  hcl.RangeBetween(hcl.Range{Filename: "test.tf"}, hcl.Range{Filename: "test.tf"}).Ptr(),
				}),
			setup: func(v *View) {
				v.consolidateWarnings = true
			},
			validate: func(t *testing.T, output *terminal.TestOutput) {
				stdout := output.Stdout()
				if want, got := 1, strings.Count(stdout, "Warning with source"); want != got {
					t.Errorf("expected %d warnings in stdout, got %d\nstdout:\n%s", want, got, stdout)
				}
				if !strings.Contains(stdout, "and one more similar warning elsewhere") {
					t.Errorf("stdout should contain also the indication that the diagnostics have been consolidated")
				}
			},
		},
		"consolidate errors": {

			diags: tfdiags.Diagnostics{}.
				Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Error with source",
					Detail:   "foo bar error",
					Subject:  hcl.RangeBetween(hcl.Range{Filename: "test.tf"}, hcl.Range{Filename: "test.tf"}).Ptr(),
				}).
				Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Error with source",
					Detail:   "foo bar other error",
					Subject:  hcl.RangeBetween(hcl.Range{Filename: "test.tf"}, hcl.Range{Filename: "test.tf"}).Ptr(),
				}),
			setup: func(v *View) {
				v.consolidateErrors = true
			},
			validate: func(t *testing.T, output *terminal.TestOutput) {
				stderr := output.Stderr()
				if want, got := 1, strings.Count(stderr, "Error with source"); want != got {
					t.Errorf("expected %d errors in stderr, got %d\nstderr:\n%s", want, got, stderr)
				}
				if !strings.Contains(stderr, "and one more similar error elsewhere") {
					t.Errorf("stderr should contain also the indication that the diagnostics have been consolidated")
				}
			},
		},
		"diagnostics with sources": {

			diags: tfdiags.Diagnostics{}.
				Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Error with source",
					Detail:   "foo bar error",
					Subject:  hcl.RangeBetween(hcl.Range{Filename: "test.tf"}, hcl.Range{Filename: "test.tf"}).Ptr(),
				}).
				Append(&hcl.Diagnostic{
					Severity: hcl.DiagWarning,
					Summary:  "Warning with source",
					Detail:   "foo bar warning",
					Subject:  hcl.RangeBetween(hcl.Range{Filename: "test.tf"}, hcl.Range{Filename: "test.tf"}).Ptr(),
				}),
			setup: func(view *View) {
				testFile := &hcl.File{
					Bytes: []byte(`resource "null_resource" "test" {}`),
					Body:  hcl.EmptyBody(),
				}
				testSources := map[string]*hcl.File{
					"test.tf": testFile,
				}
				view.SetConfigSources(func() map[string]*hcl.File {
					return testSources
				})
			},
			validate: func(t *testing.T, output *terminal.TestOutput) {
				stderr := output.Stderr()
				expectedStderr := `
Error: Error with source

  on test.tf line 0:
   0: resource "null_resource" "test" {}

foo bar error
`
				if diff := cmp.Diff(expectedStderr, stderr); diff != "" {
					t.Fatalf("unexpected stderr:\n%s", diff)
				}
				stdout := output.Stdout()
				expectedStdout := `
Warning: Warning with source

  on test.tf line 0:
   0: resource "null_resource" "test" {}

foo bar warning
`
				if diff := cmp.Diff(expectedStdout, stdout); diff != "" {
					t.Fatalf("unexpected stdout:\n%s", diff)
				}
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			streams, done := terminal.StreamsForTesting(t)
			view := NewView(streams)

			if tc.setup != nil {
				tc.setup(view)
			}

			view.Diagnostics(tc.diags)
			output := done(t)
			tc.validate(t, output)
		})
	}
}

func TestView_HelpPrompt(t *testing.T) {
	for _, cmd := range []string{"apply", "init", "dummy"} {
		t.Run(cmd, func(t *testing.T) {
			streams, done := terminal.StreamsForTesting(t)
			view := NewView(streams)
			view.HelpPrompt(cmd)
			output := done(t)

			stderr := output.Stderr()
			if !strings.Contains(stderr, fmt.Sprintf("tofu %s -help", cmd)) {
				t.Errorf("expected help prompt to contain 'tofu apply -help', got %q", stderr)
			}
		})
	}
}

func TestView_errorln(t *testing.T) {
	testCases := map[string]struct {
		message  string
		noColor  bool
		validate func(*testing.T, string)
	}{
		"simple error message": {

			message: "This is an error",
			noColor: true,
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "This is an error") {
					t.Errorf("expected error message in output, got %q", output)
				}
				if !strings.HasSuffix(output, "\n") {
					t.Error("expected output to end with newline")
				}
			},
		},
		"error with color disabled": {

			message: "Colored error",
			noColor: true,
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "Colored error") {
					t.Errorf("expected error message in output, got %q", output)
				}
			},
		},
		"error with color enabled": {

			message: "Colored error",
			noColor: false,
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "Colored error") {
					t.Errorf("expected error message in output, got %q", output)
				}
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			streams, done := terminal.StreamsForTesting(t)
			view := NewView(streams)
			view.colorize.Disable = tc.noColor

			view.errorln(tc.message)
			output := done(t)

			stderr := output.Stderr()
			tc.validate(t, stderr)
		})
	}
}

func TestView_warnln(t *testing.T) {
	testCases := map[string]struct {
		message  string
		noColor  bool
		validate func(*testing.T, string)
	}{
		"simple warning message": {

			message: "This is a warning",
			noColor: true,
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "This is a warning") {
					t.Errorf("expected warning message in output, got %q", output)
				}
				if !strings.HasSuffix(output, "\n") {
					t.Error("expected output to end with newline")
				}
			},
		},
		"warning with color disabled": {

			message: "Colored warning",
			noColor: true,
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "Colored warning") {
					t.Errorf("expected warning message in output, got %q", output)
				}
			},
		},
		"warning with color enabled": {

			message: "Colored warning",
			noColor: false,
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "Colored warning") {
					t.Errorf("expected warning message in output, got %q", output)
				}
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			streams, done := terminal.StreamsForTesting(t)
			view := NewView(streams)
			view.colorize.Disable = tc.noColor

			view.warnln(tc.message)
			output := done(t)

			// Warnings go to stdout
			stdout := output.Stdout()
			tc.validate(t, stdout)
		})
	}
}
