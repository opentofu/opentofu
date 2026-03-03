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

func TestParseWorkspaceList_viewOptions(t *testing.T) {
	tempDir := t.TempDir()
	testCases := map[string]struct {
		args         []string
		wantViewType ViewType
	}{
		"default view type": {
			args:         []string{},
			wantViewType: ViewHuman,
		},
		"json view type": {
			args:         []string{"-json"},
			wantViewType: ViewJSON,
		},
		"json-into": {
			args:         []string{fmt.Sprintf("-json-into=%s", filepath.Join(tempDir, "json-into"))},
			wantViewType: ViewHuman,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseWorkspaceList(tc.args)
			defer closer()

			if len(diags) > 0 {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}

			if got.ViewOptions.ViewType != tc.wantViewType {
				t.Errorf("ViewOptions.ViewType = %v, want %v", got.ViewOptions.ViewType, tc.wantViewType)
			}
		})
	}
}

func TestParseWorkspaceList_tooManyArgs(t *testing.T) {
	_, closer, diags := ParseWorkspaceList([]string{"unexpected-arg"})
	defer closer()

	if len(diags) == 0 {
		t.Fatal("expected diagnostics but got none")
	}

	// Check that we got the expected error
	found := false
	for _, diag := range diags {
		if diag.Severity() == tfdiags.Error && diag.Description().Summary == "Unexpected argument" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'Unexpected argument' error but got: %v", diags)
	}
}

func TestParseWorkspaceList_vars(t *testing.T) {
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
			got, closer, diags := ParseWorkspaceList(tc.args)
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
