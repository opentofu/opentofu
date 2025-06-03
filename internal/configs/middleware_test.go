// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
)

func TestDecodeMiddlewareBlock(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name: "basic middleware",
			input: `
middleware "cost_tracker" {
  command = "tofu-middleware-cost"
  args    = ["--format", "json"]
  env = {
    COST_API_KEY = "test-key"
    DEBUG        = "true"
  }
}
`,
			wantErr: false,
		},
		{
			name: "minimal middleware",
			input: `
middleware "simple" {
  command = "/usr/bin/middleware"
}
`,
			wantErr: false,
		},
		{
			name: "missing command",
			input: `
middleware "broken" {
  args = ["--help"]
}
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := hclparse.NewParser()
			file, diags := parser.ParseHCL([]byte(tt.input), "test.tf")
			if diags.HasErrors() {
				t.Fatalf("failed to parse HCL: %s", diags.Error())
			}

			content, _, diags := file.Body.PartialContent(&hcl.BodySchema{
				Blocks: []hcl.BlockHeaderSchema{
					{
						Type:       "middleware",
						LabelNames: []string{"name"},
					},
				},
			})

			if diags.HasErrors() {
				t.Fatalf("failed to get content: %s", diags.Error())
			}

			if len(content.Blocks) != 1 {
				t.Fatalf("expected 1 block, got %d", len(content.Blocks))
			}

			mw, diags := decodeMiddlewareBlock(content.Blocks[0])

			if tt.wantErr {
				if !diags.HasErrors() {
					t.Errorf("expected error but got none")
				}
			} else {
				if diags.HasErrors() {
					t.Errorf("unexpected error: %s", diags.Error())
				}

				if mw == nil {
					t.Fatal("middleware is nil")
				}

				// Basic validation
				if mw.Name == "" {
					t.Error("middleware name is empty")
				}

				if mw.Command == nil {
					t.Error("middleware command is nil")
				}

				t.Logf("Parsed middleware: name=%s, has args=%v, has env=%v",
					mw.Name, len(mw.Args) > 0, len(mw.Env) > 0)
			}
		})
	}
}

func TestDecodeMiddlewareEnv(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]string
	}{
		{
			name: "basic env",
			input: `middleware "env_test" {
  command = "test-command"
  env = {
	"TEST_VAR" = "value1"
	"ANOTHER_VAR" = "value2"
  }
}`,
			expected: map[string]string{
				"TEST_VAR":    "value1",
				"ANOTHER_VAR": "value2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := hclparse.NewParser()
			file, diags := parser.ParseHCL([]byte(tt.input), "test.tf")
			if diags.HasErrors() {
				t.Fatalf("failed to parse HCL: %s", diags.Error())
			}

			content, _, diags := file.Body.PartialContent(&hcl.BodySchema{
				Blocks: []hcl.BlockHeaderSchema{
					{
						Type:       "middleware",
						LabelNames: []string{"name"},
					},
				},
			})

			if diags.HasErrors() {
				t.Fatalf("failed to get content: %s", diags.Error())
			}

			if len(content.Blocks) != 1 {
				t.Fatalf("expected 1 block, got %d", len(content.Blocks))
			}

			mw, diags := decodeMiddlewareBlock(content.Blocks[0])

			if diags.HasErrors() {
				t.Errorf("unexpected error: %s", diags.Error())
				return
			}

			if mw == nil {
				t.Fatal("middleware is nil")
			}

			if len(mw.Env) != len(tt.expected) {
				t.Errorf("expected %d env vars, got %d", len(tt.expected), len(mw.Env))
				return
			}

			for k, v := range tt.expected {
				if mw.Env[k] != v {
					t.Errorf("expected env[%s] = %s, got %s", k, v, mw.Env[k])
				}
			}
		})
	}
}
