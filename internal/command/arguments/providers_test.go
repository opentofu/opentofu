// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestParseProviders_basicValidation(t *testing.T) {
	testCases := map[string]struct {
		args        []string
		want        *Providers
		wantErrText string
	}{
		"defaults": {
			args: nil,
			want: providersArgsWithDefaults(nil),
		},
		"custom test-directory": {
			args: []string{"-test-directory=integration"},
			want: providersArgsWithDefaults(func(v *Providers) {
				v.TestsDirectory = "integration"
			}),
		},
		"unknown flag": {
			args:        []string{"-json"},
			want:        providersArgsWithDefaults(func(v *Providers) {}),
			wantErrText: "Failed to parse command-line flags: flag provided but not defined: -json",
		},
		"unknown flag2": {
			args:        []string{"-json-into"},
			want:        providersArgsWithDefaults(func(v *Providers) {}),
			wantErrText: "Failed to parse command-line flags: flag provided but not defined: -json-into",
		},
		"too many args": {
			args:        []string{"foo"},
			want:        providersArgsWithDefaults(func(v *Providers) {}),
			wantErrText: "Too many command line arguments. Did you mean to use -chdir?",
		},
	}

	cmpOpts := cmpopts.IgnoreUnexported(Vars{}, ViewOptions{})

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseProviders(tc.args)
			defer closer()

			if tc.wantErrText != "" && len(diags) == 0 {
				t.Errorf("test wanted error but got nothing")
			} else if tc.wantErrText == "" && len(diags) > 0 {
				t.Errorf("test didn't expect errors but got some: %s", diags.ErrWithWarnings())
			} else if tc.wantErrText != "" && len(diags) > 0 {
				errStr := diags.ErrWithWarnings().Error()
				if !strings.Contains(errStr, tc.wantErrText) {
					t.Errorf("the returned diagnostics does not contain the expected error message.\ndiags:\n%s\nwanted: %s\n", errStr, tc.wantErrText)
				}
			}
			if diff := cmp.Diff(tc.want, got, cmpOpts); diff != "" {
				t.Errorf("unexpected result\n%s", diff)
			}
		})
	}
}

func TestParseProviders_vars(t *testing.T) {
	testCases := map[string]struct {
		args      []string
		wantCount int
		wantEmpty bool
	}{
		"no vars": {
			args:      nil,
			wantCount: 0,
			wantEmpty: true,
		},
		"single var": {
			args:      []string{"-var", "foo=bar"},
			wantCount: 1,
			wantEmpty: false,
		},
		"single var-file": {
			args:      []string{"-var-file", "terraform.tfvars"},
			wantCount: 1,
			wantEmpty: false,
		},
		"multiple vars mixed": {
			args:      []string{"-var", "a=1", "-var-file", "f.tfvars", "-var", "b=2"},
			wantCount: 3,
			wantEmpty: false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseProviders(tc.args)
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

func providersArgsWithDefaults(mutate func(v *Providers)) *Providers {
	ret := &Providers{
		TestsDirectory: "tests",
		ViewOptions: ViewOptions{
			ViewType:     ViewHuman,
			InputEnabled: false,
		},
		Vars: &Vars{},
	}
	if mutate != nil {
		mutate(ret)
	}
	return ret
}
