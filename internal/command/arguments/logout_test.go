// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLogout_arguments(t *testing.T) {
	t.Run("happy case", func(t *testing.T) {
		args, closer, diags := ParseLogout([]string{"example.com"})
		defer closer()
		if len(diags) > 0 {
			t.Errorf("unexpected diagnostics: %s", diags.ErrWithWarnings())
		}
		if want, got := "example.com", args.Host; want != got {
			t.Fatalf("expected to have host %q but got %q", want, got)
		}
	})
	t.Run("error cases", func(t *testing.T) {
		testCases := map[string]struct {
			args            []string
			wantDiagSummary string
			wantDiagDetail  string
		}{
			"no hostname": {
				args:            []string{},
				wantDiagSummary: "Unexpected argument",
				wantDiagDetail:  "The logout command expects exactly one argument: the host to log out of.",
			},
			"too many hostnames": {
				args:            []string{"example.com", "registry.example.com"},
				wantDiagSummary: "Unexpected argument",
				wantDiagDetail:  "The logout command expects exactly one argument: the host to log out of.",
			},
		}

		for name, tc := range testCases {
			t.Run(name, func(t *testing.T) {
				_, closer, diags := ParseLogout(tc.args)
				defer closer()

				if len(diags) == 0 {
					t.Fatal("expected diagnostics but got none")
				}
				if got, want := diags.Err().Error(), tc.wantDiagSummary; !strings.Contains(got, want) {
					t.Fatalf("wrong diags summary\n got: %s\nwant: %s", got, want)
				}
				if got, want := diags.Err().Error(), tc.wantDiagDetail; !strings.Contains(got, want) {
					t.Fatalf("wrong diags detail\n got: %s\nwant: %s", got, want)
				}
			})
		}
	})
}

func TestParseLogout_viewOptions(t *testing.T) {
	tempDir := t.TempDir()
	testCases := map[string]struct {
		args         []string
		wantViewType ViewType
	}{
		"default view type": {
			args:         []string{"example.com"},
			wantViewType: ViewHuman,
		},
		"json view type": {
			args:         []string{"-json", "example.com"},
			wantViewType: ViewJSON,
		},
		"json-into with input enabled": {
			args:         []string{fmt.Sprintf("-json-into=%s", filepath.Join(tempDir, "json-into")), "example.com"},
			wantViewType: ViewHuman,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseLogout(tc.args)
			defer closer()

			if len(diags) > 0 {
				t.Fatalf("unexpected diags: %v", diags)
			}
			if got.ViewOptions.ViewType != tc.wantViewType {
				t.Errorf("ViewOptions.ViewType = %v, want %v", got.ViewOptions.ViewType, tc.wantViewType)
			}
			if got.ViewOptions.InputEnabled {
				t.Errorf("ViewOptions.InputEnabled always needs to be false but got %t", got.ViewOptions.InputEnabled)
			}
		})
	}
}
