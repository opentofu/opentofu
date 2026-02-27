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

func TestParseProvidersLock_basicValidation(t *testing.T) {
	testCases := map[string]struct {
		args        []string
		want        *ProvidersLock
		wantErrText string
	}{
		"defaults": {
			args: nil,
			want: providersLockArgsWithDefaults(nil),
		},
		"single provider": {
			args: []string{"test_ns/test_provider"},
			want: providersLockArgsWithDefaults(func(v *ProvidersLock) {
				v.Providers = []string{"test_ns/test_provider"}
			}),
		},
		"multiple providers": {
			args: []string{"test_ns/test_provider", "test_ns2/test_provider2"},
			want: providersLockArgsWithDefaults(func(v *ProvidersLock) {
				v.Providers = []string{"test_ns/test_provider", "test_ns2/test_provider2"}
			}),
		},
		"single platform": {
			args: []string{"-platform=linux_amd64"},
			want: providersLockArgsWithDefaults(func(v *ProvidersLock) {
				v.Providers = []string{}
				v.OptPlatforms = []string{"linux_amd64"}
			}),
		},
		"multiple platforms": {
			args: []string{"-platform=linux_amd64", "-platform=darwin_arm64"},
			want: providersLockArgsWithDefaults(func(v *ProvidersLock) {
				v.Providers = []string{}
				v.OptPlatforms = []string{"linux_amd64", "darwin_arm64"}
			}),
		},
		"fs-mirror flag": {
			args: []string{"-fs-mirror=/path/to/mirror"},
			want: providersLockArgsWithDefaults(func(v *ProvidersLock) {
				v.Providers = []string{}
				v.FsMirrorDir = "/path/to/mirror"
			}),
		},
		"net-mirror flag": {
			args: []string{"-net-mirror=https://example.com/mirror"},
			want: providersLockArgsWithDefaults(func(v *ProvidersLock) {
				v.Providers = []string{}
				v.NetMirrorURL = "https://example.com/mirror"
			}),
		},
		"both mirrors error": {
			args: []string{"-fs-mirror=/path", "-net-mirror=https://example.com"},
			want: providersLockArgsWithDefaults(func(v *ProvidersLock) {
				v.FsMirrorDir = "/path"
				v.NetMirrorURL = "https://example.com"
			}),
			wantErrText: "The -fs-mirror and -net-mirror command line options are mutually-exclusive.",
		},
		"mixed flags and providers": {
			args: []string{"-platform=linux_amd64", "-platform=darwin_arm64", "test_ns/test_provider", "test_ns2/test_provider2"},
			want: providersLockArgsWithDefaults(func(v *ProvidersLock) {
				v.OptPlatforms = []string{"linux_amd64", "darwin_arm64"}
				v.Providers = []string{"test_ns/test_provider", "test_ns2/test_provider2"}
			}),
		},
		"unknown flag": {
			args:        []string{"-unknown-flag"},
			want:        providersLockArgsWithDefaults(func(v *ProvidersLock) {}),
			wantErrText: "Failed to parse command-line flags: flag provided but not defined: -unknown-flag",
		},
	}

	cmpOpts := cmpopts.IgnoreUnexported(Vars{}, ViewOptions{})

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseProvidersLock(tc.args)
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

func TestParseProvidersLock_vars(t *testing.T) {
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
		"vars with providers": {
			args:      []string{"-var", "foo=bar", "-var-file", "test.tfvars", "test_ns/test_provider"},
			wantCount: 2,
			wantEmpty: false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseProvidersLock(tc.args)
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

func providersLockArgsWithDefaults(mutate func(v *ProvidersLock)) *ProvidersLock {
	ret := &ProvidersLock{
		Providers:    nil,
		OptPlatforms: nil,
		FsMirrorDir:  "",
		NetMirrorURL: "",
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
