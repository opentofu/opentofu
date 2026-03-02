// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/command/workdir"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/terminal"
	"github.com/zclconf/go-cty/cty"
)

func TestConsole_multiline_interactive(t *testing.T) {
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
			defer testStdinPipe(t, strings.NewReader(tc.input))()

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
			streamsOut := done(t)
			if code != 0 {
				t.Fatalf("bad: %d\n\n%s", code, streamsOut.Stderr())
			}

			got := streamsOut.Stdout()
			if diff := cmp.Diff(got, tc.expected); diff != "" {
				t.Fatalf("unexpected output. For input: %s\n%s", tc.input, diff)
			}

			// TODO meta-refactor: remove this assertion once the stateLock from Meta is removed
			if !c.Meta.stateLock {
				t.Errorf("stateLock should always be nil for this command")
			}
		})
	}
}
