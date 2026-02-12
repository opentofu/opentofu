// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/command/workdir"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/terminal"
	"github.com/zclconf/go-cty/cty"
)

// ConsoleCommand is tested primarily with tests in the "repl" package.
// It is not tested here because the Console uses a readline-like library
// that takes over stdin/stdout. It is difficult to test directly. The
// core logic is tested in "repl"
//
// This file still contains some tests using the stdin-based input.

func TestConsole_basic(t *testing.T) {
	testCwdTemp(t)

	p := testProvider()
	streams, done := terminal.StreamsForTesting(t)
	view := views.NewView(streams)
	c := &ConsoleCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
			Streams:          streams,
		},
	}
	defer testStdinPipe(t, strings.NewReader("1+5\n"))()

	code := c.Run(nil)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	actual := output.Stdout()
	if actual != "6\n" {
		t.Fatalf("bad: %q", actual)
	}
}

func TestConsole_tfvars(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath("apply-vars"), td)
	t.Chdir(td)

	// Write a terraform.tfvars
	varFilePath := filepath.Join(td, "terraform.tfvars")
	if err := os.WriteFile(varFilePath, []byte(applyVarFile), 0644); err != nil {
		t.Fatalf("err: %s", err)
	}

	p := testProvider()
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		ResourceTypes: map[string]providers.Schema{
			"test_instance": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"value": {Type: cty.String, Optional: true},
					},
				},
			},
		},
	}
	streams, done := terminal.StreamsForTesting(t)
	view := views.NewView(streams)
	c := &ConsoleCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
			Streams:          streams,
		},
	}

	defer testStdinPipe(t, strings.NewReader("var.foo\n"))()

	code := c.Run(nil)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	actual := output.Stdout()
	if actual != "\"bar\"\n" {
		t.Fatalf("bad: %q", actual)
	}
}

func TestConsole_unsetRequiredVars(t *testing.T) {
	// This test is verifying that it's possible to run "tofu console"
	// without providing values for all required variables, without
	// "tofu console" producing an interactive prompt for those variables
	// or producing errors. Instead, it should allow evaluation in that
	// partial context but see the unset variables values as being unknown.
	//
	// This test fixture includes variable "foo" {}, which we are
	// intentionally not setting here.
	td := t.TempDir()
	testCopyDir(t, testFixturePath("apply-vars"), td)
	t.Chdir(td)

	p := testProvider()
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		ResourceTypes: map[string]providers.Schema{
			"test_instance": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"value": {Type: cty.String, Optional: true},
					},
				},
			},
		},
	}
	streams, done := terminal.StreamsForTesting(t)
	view := views.NewView(streams)
	c := &ConsoleCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
			Streams:          streams,
		},
	}

	defer testStdinPipe(t, strings.NewReader("var.foo\n"))()

	code := c.Run(nil)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	if got, want := output.Stdout(), "(known after apply)\n"; got != want {
		t.Fatalf("unexpected output\n got: %q\nwant: %q", got, want)
	}
}

func TestConsole_variables(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath("variables"), td)
	t.Chdir(td)

	p := testProvider()

	commands := map[string]string{
		"var.foo\n":          "\"bar\"\n",
		"var.snack\n":        "\"popcorn\"\n",
		"var.secret_snack\n": "(sensitive value)\n",
		"local.snack_bar\n":  "[\n  \"popcorn\",\n  (sensitive value),\n]\n",
	}

	args := []string{}

	for cmd, val := range commands {
		t.Run(strings.TrimSpace(cmd), func(t *testing.T) {
			defer testStdinPipe(t, strings.NewReader(cmd))()
			streams, done := terminal.StreamsForTesting(t)
			view := views.NewView(streams)
			c := &ConsoleCommand{
				Meta: Meta{
					WorkingDir:       workdir.NewDir("."),
					testingOverrides: metaOverridesForProvider(p),
					View:             view,
					Streams:          streams,
				},
			}

			code := c.Run(args)
			output := done(t)
			if code != 0 {
				t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
			}

			actual := output.Stdout()
			if output.Stdout() != val {
				t.Fatalf("bad: %q, expected %q", actual, val)
			}
		})
	}
}

func TestConsole_modules(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath("modules"), td)
	t.Chdir(td)

	p := applyFixtureProvider()

	commands := map[string]string{
		"module.child.myoutput\n":          "\"bar\"\n",
		"module.count_child[0].myoutput\n": "\"bar\"\n",
		"local.foo\n":                      "3\n",
	}

	for cmd, val := range commands {
		t.Run(strings.TrimSpace(cmd), func(t *testing.T) {
			defer testStdinPipe(t, strings.NewReader(cmd))()
			streams, done := terminal.StreamsForTesting(t)
			view := views.NewView(streams)
			c := &ConsoleCommand{
				Meta: Meta{
					WorkingDir:       workdir.NewDir("."),
					testingOverrides: metaOverridesForProvider(p),
					View:             view,
					Streams:          streams,
				},
			}
			code := c.Run(nil)
			output := done(t)
			if code != 0 {
				t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
			}

			actual := output.Stdout()
			if output.Stdout() != val {
				t.Fatalf("bad: %q, expected %q", actual, val)
			}
		})

	}
}

func TestConsole_multiline_pipe(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath("console-multiline-vars"), td)
	t.Chdir(td)

	p := testProvider()
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		ResourceTypes: map[string]providers.Schema{
			"test_instance": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"value": {Type: cty.String, Optional: true},
					},
				},
			},
		},
	}

	type testCase struct {
		input    string
		expected string
	}

	tests := map[string]testCase{
		"single_line": {
			input:    `var.counts.lalala`,
			expected: "1\n",
		},
		"basic_multi_line": {
			input: `
			var.counts.lalala
			var.counts.lololo`,
			expected: "\n1\n2\n",
		},
		"brackets_multi_line": {
			input: `
			var.counts.lalala
			split(
			"_",
			"lalala_lolol_lelelele"
			)`,
			expected: "\n1\ntolist([\n  \"lalala\",\n  \"lolol\",\n  \"lelelele\",\n])\n",
		},
		"braces_multi_line": {
			input: `
			{ 
			for key, value in var.counts : key => value 
			if value == 1
			}`,
			expected: "\n{\n  \"lalala\" = 1\n}\n",
		},
		"escaped_new_line": {
			input: `
			5 + 4 \
			
			`,
			expected: "\n9\n\n",
		},
		"heredoc": {
			input: `
			{
				default = <<-EOT
				lulululu
				EOT
			}`,
			expected: "\n{\n  \"default\" = <<-EOT\n  lulululu\n  \n  EOT\n}\n",
		},
		"quoted_braces": {
			input:    "{\ndefault = format(\"%s%s%s\",\"{\",var.counts.lalala,\"}\")\n}",
			expected: "{\n  \"default\" = \"{1}\"\n}\n",
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			streams, done := terminal.StreamsForTesting(t)
			view := views.NewView(streams)
			c := &ConsoleCommand{
				Meta: Meta{
					WorkingDir:       workdir.NewDir("."),
					testingOverrides: metaOverridesForProvider(p),
					View:             view,
					Streams:          streams,
				},
			}

			defer testStdinPipe(t, strings.NewReader(tc.input))()

			code := c.Run(nil)
			output := done(t)
			if code != 0 {
				t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
			}

			got := output.Stdout()
			if got != tc.expected {
				t.Fatalf("unexpected output\ngot: %q\nexpected: %q", got, tc.expected)
			}
		})
	}
}
