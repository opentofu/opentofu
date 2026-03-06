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

func TestParseStatePush_basicValidation(t *testing.T) {
	testCases := map[string]struct {
		args        []string
		want        *StatePush
		wantErrText string
	}{
		"no arguments": {
			args:        nil,
			want:        statePushArgsWithDefaults(nil),
			wantErrText: "Exactly one argument expected",
		},
		"too many arguments": {
			args:        []string{"state1.tfstate", "state2.tfstate"},
			want:        statePushArgsWithDefaults(nil),
			wantErrText: "Exactly one argument expected",
		},
		"valid state file path": {
			args: []string{"terraform.tfstate"},
			want: statePushArgsWithDefaults(func(v *StatePush) {
				v.StateSrc = "terraform.tfstate"
			}),
		},
		"stdin as source": {
			args: []string{"-"},
			want: statePushArgsWithDefaults(func(v *StatePush) {
				v.StateSrc = "-"
			}),
		},
		"force flag": {
			args: []string{"-force", "terraform.tfstate"},
			want: statePushArgsWithDefaults(func(v *StatePush) {
				v.Force = true
				v.StateSrc = "terraform.tfstate"
			}),
		},
		"lock flag": {
			args: []string{"-lock=false", "terraform.tfstate"},
			want: statePushArgsWithDefaults(func(v *StatePush) {
				v.Backend.StateLock = false
				v.StateSrc = "terraform.tfstate"
			}),
		},
		"lock-timeout flag": {
			args: []string{"-lock-timeout=30s", "terraform.tfstate"},
			want: statePushArgsWithDefaults(func(v *StatePush) {
				v.Backend.StateLockTimeout = 30000000000 // 30s in nanoseconds
				v.StateSrc = "terraform.tfstate"
			}),
		},
		"ignore-remote-version flag": {
			args: []string{"-ignore-remote-version", "terraform.tfstate"},
			want: statePushArgsWithDefaults(func(v *StatePush) {
				v.Backend.IgnoreRemoteVersion = true
				v.StateSrc = "terraform.tfstate"
			}),
		},
		"multiple flags combined": {
			args: []string{"-force", "-lock=false", "-ignore-remote-version", "terraform.tfstate"},
			want: statePushArgsWithDefaults(func(v *StatePush) {
				v.Force = true
				v.Backend.StateLock = false
				v.Backend.IgnoreRemoteVersion = true
				v.StateSrc = "terraform.tfstate"
			}),
		},
		"unknown flag": {
			args: []string{"-unknown-flag", "terraform.tfstate"},
			want: statePushArgsWithDefaults(func(v *StatePush) {
				v.StateSrc = "terraform.tfstate"
			}),
			wantErrText: "Failed to parse command-line flags: flag provided but not defined: -unknown-flag",
		},
	}

	cmpOpts := cmpopts.IgnoreUnexported(Vars{}, ViewOptions{}, Backend{})

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseStatePush(tc.args)
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

func TestParseStatePush_vars(t *testing.T) {
	testCases := map[string]struct {
		args      []string
		wantCount int
		wantEmpty bool
	}{
		"no vars": {
			args:      []string{"terraform.tfstate"},
			wantCount: 0,
			wantEmpty: true,
		},
		"single var": {
			args:      []string{"-var", "foo=bar", "terraform.tfstate"},
			wantCount: 1,
			wantEmpty: false,
		},
		"single var-file": {
			args:      []string{"-var-file", "terraform.tfvars", "terraform.tfstate"},
			wantCount: 1,
			wantEmpty: false,
		},
		"multiple vars mixed": {
			args:      []string{"-var", "a=1", "-var-file", "f.tfvars", "-var", "b=2", "terraform.tfstate"},
			wantCount: 3,
			wantEmpty: false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseStatePush(tc.args)
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

func statePushArgsWithDefaults(mutate func(v *StatePush)) *StatePush {
	ret := &StatePush{
		StateSrc: "",
		Force:    false,
		ViewOptions: ViewOptions{
			ViewType:     ViewHuman,
			InputEnabled: false,
		},
		Vars: &Vars{},
		Backend: Backend{
			StateLock:           true,
			StateLockTimeout:    0,
			IgnoreRemoteVersion: false,
			Reconfigure:         false,
			MigrateState:        false,
			ForceInitCopy:       false,
		},
	}
	if mutate != nil {
		mutate(ret)
	}
	return ret
}
