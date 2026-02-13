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

func TestParseLogin_vars(t *testing.T) {
	testCases := map[string]struct {
		args      []string
		wantCount int
		wantEmpty bool
	}{
		"no vars": {
			args:      []string{"example.com"},
			wantCount: 0,
			wantEmpty: true,
		},
		"single var": {
			args:      []string{"-var", "foo=bar", "example.com"},
			wantCount: 1,
			wantEmpty: false,
		},
		"single var-file": {
			args:      []string{"-var-file", "terraform.tfvars", "example.com"},
			wantCount: 1,
			wantEmpty: false,
		},
		"multiple vars mixed": {
			args:      []string{"-var", "a=1", "-var-file", "f.tfvars", "-var", "b=2", "example.com"},
			wantCount: 3,
			wantEmpty: false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseLogin(tc.args)
			defer closer()

			if len(diags) > 0 {
				t.Fatalf("unexpected diags: %v", diags)
			}
			if got.Vars.Empty() != tc.wantEmpty {
				t.Errorf("Vars.Empty() = %v, want %v", got.Vars.Empty(), tc.wantEmpty)
			}
			if len(got.Vars.All()) != tc.wantCount {
				t.Errorf("len(Vars.All()) = %d, want %d", len(got.Vars.All()), tc.wantCount)
			}
		})
	}
}

func TestParseLogin_arguments(t *testing.T) {
	t.Run("happy case", func(t *testing.T) {
		args, closer, diags := ParseLogin([]string{"example.com"})
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
				wantDiagDetail:  "The login command expects exactly one argument: the host to log in to.",
			},
			"too many hostnames": {
				args:            []string{"example.com", "registry.example.com"},
				wantDiagSummary: "Unexpected argument",
				wantDiagDetail:  "The login command expects exactly one argument: the host to log in to.",
			},
		}

		for name, tc := range testCases {
			t.Run(name, func(t *testing.T) {
				_, closer, diags := ParseLogin(tc.args)
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

func TestParseLogin_viewOptions(t *testing.T) {
	tempDir := t.TempDir()
	testCases := map[string]struct {
		args             []string
		wantViewType     ViewType
		wantInputEnabled bool
	}{
		"default view type": {
			args:             []string{"example.com"},
			wantViewType:     ViewHuman,
			wantInputEnabled: true,
		},
		"json view type": {
			args:             []string{"-json", "example.com"},
			wantViewType:     ViewJSON,
			wantInputEnabled: false,
		},
		"input disabled": {
			args:             []string{"-input=false", "example.com"},
			wantViewType:     ViewHuman,
			wantInputEnabled: false,
		},
		"json with input disabled": {
			args:             []string{"-json", "-input=false", "example.com"},
			wantViewType:     ViewJSON,
			wantInputEnabled: false,
		},
		"json-into with input enabled": {
			args:             []string{fmt.Sprintf("-json-into=%s", filepath.Join(tempDir, "json-into")), "example.com"},
			wantViewType:     ViewHuman,
			wantInputEnabled: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseLogin(tc.args)
			defer closer()

			if len(diags) > 0 {
				t.Fatalf("unexpected diags: %v", diags)
			}
			if got.ViewOptions.ViewType != tc.wantViewType {
				t.Errorf("ViewOptions.ViewType = %v, want %v", got.ViewOptions.ViewType, tc.wantViewType)
			}
			if got.ViewOptions.InputEnabled != tc.wantInputEnabled {
				t.Errorf("ViewOptions.InputEnabled = %v, want %v", got.ViewOptions.InputEnabled, tc.wantInputEnabled)
			}
		})
	}
}
