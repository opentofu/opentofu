// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"context"
	"testing"

	version "github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/configs/symlib"
	"github.com/zclconf/go-cty/cty"
)

func TestVariableFinalize_missingSymbolLibrary(t *testing.T) {
	tests := []struct {
		name       string
		src        string
		checkDiags func(*testing.T, hcl.Diagnostics)
	}{
		{
			name: "optional attribute with default (issue #4733)",
			src: `
variable "test" {
  type = object({
    bar = optional(symbols::missing::some_type(), "default")
  })
}
`,
			checkDiags: func(t *testing.T, diags hcl.Diagnostics) {
				if !diags.HasErrors() {
					t.Fatalf("expected errors for missing symbol library, got none")
				}
				found := false
				for _, diag := range diags {
					if diag.Summary == "Missing symbol library" {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected 'Missing symbol library' diagnostic, got: %s", diags.Error())
				}
			},
		},
		{
			name: "direct symbol type",
			src: `
variable "test" {
  type = symbols::missing::some_type()
}
`,
			checkDiags: func(t *testing.T, diags hcl.Diagnostics) {
				if !diags.HasErrors() {
					t.Fatalf("expected errors for missing symbol library, got none")
				}
				found := false
				for _, diag := range diags {
					if diag.Summary == "Missing symbol library" {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected 'Missing symbol library' diagnostic, got: %s", diags.Error())
				}
			},
		},
		{
			name: "list of symbol type",
			src: `
variable "test" {
  type = list(symbols::missing::some_type())
}
`,
			checkDiags: func(t *testing.T, diags hcl.Diagnostics) {
				if !diags.HasErrors() {
					t.Fatalf("expected errors for missing symbol library, got none")
				}
				found := false
				for _, diag := range diags {
					if diag.Summary == "Missing symbol library" {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected 'Missing symbol library' diagnostic, got: %s", diags.Error())
				}
			},
		},
		{
			name: "invalid symbol call format",
			src: `
variable "test" {
  type = symbols::invalid_call()
}
`,
			checkDiags: func(t *testing.T, diags hcl.Diagnostics) {
				if !diags.HasErrors() {
					t.Fatalf("expected errors for invalid call structure, got none")
				}
				found := false
				for _, diag := range diags {
					if diag.Summary == "Invalid type call structure" {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected 'Invalid type call structure' diagnostic, got: %s", diags.Error())
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := testParser(map[string]string{
				"test.tf": tt.src,
			})
			file, diags := parser.LoadConfigFile("test.tf")
			if diags.HasErrors() {
				t.Fatalf("unexpected load errors: %s", diags.Error())
			}
			if len(file.Variables) != 1 {
				t.Fatalf("expected 1 variable, got %d", len(file.Variables))
			}

			v := file.Variables[0]
			finalizeDiags := v.finalize(symlib.EmptyTable)
			tt.checkDiags(t, finalizeDiags)

			// Ensure Type is valid and WithoutOptionalAttributesDeep doesn't panic
			if v.Type == cty.NilType {
				t.Errorf("variable Type should not be NilType")
			}
		})
	}
}

func TestOutputFinalize_missingSymbolLibrary(t *testing.T) {
	src := `
output "test" {
  value = "example"
  type  = object({
    bar = optional(symbols::missing::some_type(), "default")
  })
}
`
	parser := testParser(map[string]string{
		"test.tf": src,
	})
	file, diags := parser.LoadConfigFile("test.tf")
	if diags.HasErrors() {
		t.Fatalf("unexpected load errors: %s", diags.Error())
	}
	if len(file.Outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(file.Outputs))
	}

	o := file.Outputs[0]
	finalizeDiags := o.finalize(symlib.EmptyTable)
	if !finalizeDiags.HasErrors() {
		t.Fatalf("expected errors for missing symbol library, got none")
	}

	found := false
	for _, diag := range finalizeDiags {
		if diag.Summary == "Missing symbol library" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'Missing symbol library' diagnostic, got: %s", finalizeDiags.Error())
	}
}

func TestBuildConfig_uninitializedSymbolLibrary(t *testing.T) {
	// End-to-end test simulating issue #4733:
	// A configuration declares a symbol library module and a variable using a type
	// from that symbol library. When the symbol library is not installed/initialized,
	// BuildConfig must not crash, and should emit proper diagnostics.
	src := `
language {
  experiments = [symbol_libraries]
}

symbols "ext" {
  source = "./missing_symlib"
}

variable "config" {
  type = object({
    entry = optional(symbols::ext::my_type(), "default_val")
  })
}
`
	parser := testParser(map[string]string{
		"main.tf": src,
	})
	mod, diags := parser.LoadConfigDir(".")
	if diags.HasErrors() {
		t.Fatalf("unexpected load errors: %s", diags.Error())
	}

	walker := ModuleWalkerFunc(
		func(_ context.Context, req *ModuleRequest) (*Module, *version.Version, hcl.Diagnostics) {
			// Simulate uninitialized module: loader fails to load missing module
			return nil, nil, hcl.Diagnostics{{
				Severity: hcl.DiagError,
				Summary:  "Module not installed",
				Detail:   "This module is not yet installed. Run \"tofu init\" to install it.",
				Subject:  &req.CallRange,
			}}
		},
		parser.LoadSymbolFilesInDir,
	)

	cfg, buildDiags := BuildConfig(t.Context(), mod, RootModuleCallForTesting(), walker)
	if !buildDiags.HasErrors() {
		t.Fatalf("expected build errors for uninitialized symbol library, got none")
	}

	if cfg == nil {
		t.Fatal("expected non-nil config even with build errors")
	}

	// Verify that the variable in cfg.Module has been safely finalized to DynamicPseudoType
	v := cfg.Module.Variables["config"]
	if v == nil {
		t.Fatal("expected variable 'config' to exist in module")
	}
	if v.Type == cty.NilType {
		t.Errorf("expected variable Type to not be NilType")
	}

	foundModuleNotInstalled := false
	foundMissingSymbolLib := false
	for _, diag := range buildDiags {
		if diag.Summary == "Module not installed" {
			foundModuleNotInstalled = true
		}
		if diag.Summary == "Missing symbol library" {
			foundMissingSymbolLib = true
		}
	}
	if !foundModuleNotInstalled {
		t.Errorf("expected 'Module not installed' diagnostic in buildDiags: %s", buildDiags.Error())
	}
	if !foundMissingSymbolLib {
		t.Errorf("expected 'Missing symbol library' diagnostic in buildDiags: %s", buildDiags.Error())
	}
}
