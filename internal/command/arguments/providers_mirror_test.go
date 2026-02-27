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

func TestParseProvidersMirror_basicValidation(t *testing.T) {
	testCases := map[string]struct {
		args        []string
		want        *ProvidersMirror
		wantErrText string
	}{
		"no directory": {
			args:        nil,
			want:        providersMirrorArgsWithDefaults(nil),
			wantErrText: "Wrong number of arguments: The providers mirror command requires an output directory as a command-line argument.",
		},
		"too many arguments": {
			args:        []string{"/path/to/mirror", "/another/path"},
			want:        providersMirrorArgsWithDefaults(func(v *ProvidersMirror) {}),
			wantErrText: "Wrong number of arguments: The providers mirror command requires an output directory as a command-line argument.",
		},
		"single directory": {
			args: []string{"/path/to/mirror"},
			want: providersMirrorArgsWithDefaults(func(v *ProvidersMirror) {
				v.Directory = "/path/to/mirror"
			}),
		},
		"single platform": {
			args: []string{"-platform=linux_amd64", "/path/to/mirror"},
			want: providersMirrorArgsWithDefaults(func(v *ProvidersMirror) {
				v.OptPlatforms = []string{"linux_amd64"}
				v.Directory = "/path/to/mirror"
			}),
		},
		"multiple platforms": {
			args: []string{"-platform=linux_amd64", "-platform=darwin_arm64", "/path/to/mirror"},
			want: providersMirrorArgsWithDefaults(func(v *ProvidersMirror) {
				v.OptPlatforms = []string{"linux_amd64", "darwin_arm64"}
				v.Directory = "/path/to/mirror"
			}),
		},
		"unknown flag": {
			args: []string{"-unknown-flag", "/path/to/mirror"},
			want: providersMirrorArgsWithDefaults(func(v *ProvidersMirror) {
				v.Directory = "/path/to/mirror"
			}),
			wantErrText: "Failed to parse command-line flags: flag provided but not defined: -unknown-flag",
		},
	}

	cmpOpts := cmpopts.IgnoreUnexported(Vars{}, ViewOptions{})

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseProvidersMirror(tc.args)
			defer closer()

			if tc.wantErrText != "" && len(diags) == 0 {
				t.Errorf("test wanted error but got nothing")
			} else if tc.wantErrText == "" && len(diags) > 0 {
				t.Errorf("test didn't expect errors but got some: %s", diags.ErrWithWarnings())
			} else if tc.wantErrText != "" && len(diags) > 0 {
				errStr := diags.ErrWithWarnings().Error()
				if !strings.Contains(errStr, tc.wantErrText) {
					t.Errorf("the returned diagnostics does not contain the expected error message.\ndiags:\n\t%s\nwanted:\n\t%s\n", errStr, tc.wantErrText)
				}
			}
			if diff := cmp.Diff(tc.want, got, cmpOpts); diff != "" {
				t.Errorf("unexpected result\n%s", diff)
			}
		})
	}
}

func TestParseProvidersMirror_vars(t *testing.T) {
	testCases := map[string]struct {
		args      []string
		wantCount int
		wantEmpty bool
	}{
		"no vars": {
			args:      []string{"/path/to/mirror"},
			wantCount: 0,
			wantEmpty: true,
		},
		"single var": {
			args:      []string{"-var", "foo=bar", "/path/to/mirror"},
			wantCount: 1,
			wantEmpty: false,
		},
		"single var-file": {
			args:      []string{"-var-file", "terraform.tfvars", "/path/to/mirror"},
			wantCount: 1,
			wantEmpty: false,
		},
		"multiple vars mixed": {
			args:      []string{"-var", "a=1", "-var-file", "f.tfvars", "-var", "b=2", "/path/to/mirror"},
			wantCount: 3,
			wantEmpty: false,
		},
		"vars with platforms": {
			args:      []string{"-var", "foo=bar", "-var-file", "test.tfvars", "-platform=linux_amd64", "/path/to/mirror"},
			wantCount: 2,
			wantEmpty: false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseProvidersMirror(tc.args)
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

func providersMirrorArgsWithDefaults(mutate func(v *ProvidersMirror)) *ProvidersMirror {
	ret := &ProvidersMirror{
		Directory:    "",
		OptPlatforms: nil,
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
