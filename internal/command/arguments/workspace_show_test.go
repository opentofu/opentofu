// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"testing"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestParseWorkspaceShow_viewOptions(t *testing.T) {
	testCases := map[string]struct {
		args              []string
		wantViewType      ViewType
		expectDiagnostics bool
	}{
		"default view type": {
			args:              []string{},
			wantViewType:      ViewHuman,
			expectDiagnostics: false,
		},
		// workspace show doesn't register view flags, so -json is not supported
		"json view type not supported": {
			args:              []string{"-json"},
			wantViewType:      ViewHuman,
			expectDiagnostics: true, // -json flag is not defined for workspace show
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseWorkspaceShow(tc.args)
			defer closer()

			if tc.expectDiagnostics && len(diags) == 0 {
				t.Fatal("expected diagnostics but got none")
			}
			if !tc.expectDiagnostics && len(diags) > 0 {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}

			if got.ViewOptions.ViewType != tc.wantViewType {
				t.Errorf("ViewOptions.ViewType = %v, want %v", got.ViewOptions.ViewType, tc.wantViewType)
			}
		})
	}
}

func TestParseWorkspaceShow_tooManyArgs(t *testing.T) {
	_, closer, diags := ParseWorkspaceShow([]string{"unexpected-arg"})
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

func TestParseWorkspaceShow_vars(t *testing.T) {
	got, closer, diags := ParseWorkspaceShow([]string{})
	defer closer()

	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if got.Vars == nil {
		t.Fatal("Vars should never be nil")
	}

	if !got.Vars.Empty() {
		t.Errorf("expected Vars to be empty but got vars")
	}
}
