// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestParseConsole_viewOptions(t *testing.T) {
	tempDir := t.TempDir()
	testCases := map[string]struct {
		args           []string
		wantViewType   ViewType
		wantInput      bool
		wantDiagnostic tfdiags.Diagnostics
	}{
		"default view type": {
			args:         []string{},
			wantViewType: ViewHuman,
			wantInput:    true,
		},
		"json view type": {
			args:         []string{"-json"},
			wantViewType: ViewHuman, // This is because we are reverting to be able to print the diagnostic in human form
			wantInput:    false,
			wantDiagnostic: tfdiags.Diagnostics{
				tfdiags.Sourceless(
					tfdiags.Error,
					"Output only in json is not allowed",
					"In case you want to stream the output of the console into json, use the \"-json-into\" instead.",
				),
			},
		},
		"json-into with input enabled": {
			args:         []string{fmt.Sprintf("-json-into=%s", filepath.Join(tempDir, "json-into"))},
			wantViewType: ViewHuman,
			wantInput:    true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseConsole(tc.args)
			defer closer()

			wantDiagsNo, gotDiagsNo := len(tc.wantDiagnostic), len(diags)
			if gotDiagsNo > 0 && wantDiagsNo == 0 {
				t.Errorf("expected no diagnostics but got %d: %s", gotDiagsNo, diags)
			} else if gotDiagsNo == 0 && wantDiagsNo > 0 {
				t.Errorf("expected %d diagnostics but got none", wantDiagsNo)
			} else if wantDiagsNo != gotDiagsNo {
				t.Errorf("expected to have %d diags but got %d", wantDiagsNo, gotDiagsNo)
			} else if wantDiagsNo == gotDiagsNo {
				for i, wantDiag := range tc.wantDiagnostic {
					sameDiagnostic(t, diags[i], wantDiag)
				}
			}
			if got.ViewOptions.ViewType != tc.wantViewType {
				t.Errorf("ViewOptions.ViewType = %v, want %v", got.ViewOptions.ViewType, tc.wantViewType)
			}
			if got.ViewOptions.InputEnabled != tc.wantInput {
				t.Errorf("ViewOptions.InputEnabled = %v want %t", got.ViewOptions.InputEnabled, tc.wantInput)
			}
		})
	}
}

func TestParseConsole_statePath(t *testing.T) {
	testCases := map[string]struct {
		args          []string
		wantStatePath string
	}{
		"default state path": {
			args:          []string{},
			wantStatePath: DefaultStateFilename,
		},
		"custom state path": {
			args:          []string{"-state=/path/to/state.tfstate"},
			wantStatePath: "/path/to/state.tfstate",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseConsole(tc.args)
			defer closer()

			if len(diags) > 0 {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}

			if got.StatePath != tc.wantStatePath {
				t.Errorf("StatePath = %q, want %q", got.StatePath, tc.wantStatePath)
			}
		})
	}
}

func TestParseConsole_vars(t *testing.T) {
	testCases := map[string]struct {
		args              []string
		expectVarsEmpty   bool
		expectDiagnostics bool
	}{
		"no vars": {
			args:              []string{},
			expectVarsEmpty:   true,
			expectDiagnostics: false,
		},
		"single var": {
			args:              []string{"-var=key=value"},
			expectVarsEmpty:   false,
			expectDiagnostics: false,
		},
		"multiple vars": {
			args:              []string{"-var=key1=value1", "-var=key2=value2"},
			expectVarsEmpty:   false,
			expectDiagnostics: false,
		},
		"var-file": {
			args:              []string{"-var-file=test.tfvars"},
			expectVarsEmpty:   false,
			expectDiagnostics: false,
		},
		"mixed vars and var-files": {
			args:              []string{"-var=key=value", "-var-file=test.tfvars", "-var=another=val"},
			expectVarsEmpty:   false,
			expectDiagnostics: false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseConsole(tc.args)
			defer closer()

			if tc.expectDiagnostics && len(diags) == 0 {
				t.Fatal("expected diagnostics but got none")
			}
			if !tc.expectDiagnostics && len(diags) > 0 {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}

			if got.Vars == nil {
				t.Fatal("Vars should never be nil")
			}

			if tc.expectVarsEmpty && !got.Vars.Empty() {
				t.Errorf("expected Vars to be empty but got vars")
			}
			if !tc.expectVarsEmpty && got.Vars.Empty() {
				t.Errorf("expected Vars to be populated but got empty")
			}
		})
	}
}

func sameDiagnostic(t *testing.T, gotD, wantD tfdiags.Diagnostic) {
	if got, want := gotD.Severity(), wantD.Severity(); got != want {
		t.Errorf("wrong severity. got %q; want %q", got, want)
	}
	if got, want := gotD.Description().Address, wantD.Description().Address; got != want {
		t.Errorf("wrong description. got %q; want %q", got, want)
	}
	if got, want := gotD.Description().Detail, wantD.Description().Detail; got != want {
		t.Errorf("wrong detail. got %q; want %q", got, want)
	}
	if got, want := gotD.Description().Summary, wantD.Description().Summary; got != want {
		t.Errorf("wrong summary. got %q; want %q", got, want)
	}
	if got, want := gotD.ExtraInfo(), wantD.ExtraInfo(); got != want {
		t.Errorf("wrong extra info. got %q; want %q", got, want)
	}
}
