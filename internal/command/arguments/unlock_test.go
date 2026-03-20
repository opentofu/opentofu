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

func TestParseUnlock_basicValidation(t *testing.T) {
	testCases := map[string]struct {
		args        []string
		want        *Unlock
		wantErrText string
	}{
		"without arguments": {
			args:        nil,
			want:        unlockArgsWithDefaults(nil),
			wantErrText: "Wrong number of arguments: Expected a single argument: LOCK_ID",
		},
		"too many arguments": {
			args: []string{"lockid1", "lockid2"},
			want: unlockArgsWithDefaults(func(a *Unlock) {
			}),
			wantErrText: "Wrong number of arguments: Expected a single argument: LOCK_ID",
		},
		"with force": {
			args: []string{"-force", "lockid"},
			want: unlockArgsWithDefaults(func(a *Unlock) {
				a.LockID = "lockid"
				a.Force = true
			}),
		},
		"invalid flag": {
			args:        []string{"-invalid", "lockid"},
			want:        unlockArgsWithDefaults(func(a *Unlock) {}),
			wantErrText: "Failed to parse command-line flags: flag provided but not defined: -invalid",
		},
	}

	cmpOpts := cmpopts.IgnoreUnexported(Vars{}, ViewOptions{})

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseUnlock(tc.args)
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

func TestParseUnlock_vars(t *testing.T) {
	testCases := map[string]struct {
		args      []string
		wantCount int
		wantEmpty bool
	}{
		"no vars": {
			args:      []string{"lockid"},
			wantCount: 0,
			wantEmpty: true,
		},
		"single var": {
			args:      []string{"-var", "foo=bar", "lockid"},
			wantCount: 1,
			wantEmpty: false,
		},
		"single var-file": {
			args:      []string{"-var-file", "terraform.tfvars", "lockid"},
			wantCount: 1,
			wantEmpty: false,
		},
		"multiple vars mixed": {
			args:      []string{"-var", "a=1", "-var-file", "f.tfvars", "-var", "b=2", "lockid"},
			wantCount: 3,
			wantEmpty: false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseUnlock(tc.args)
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

func unlockArgsWithDefaults(mutate func(a *Unlock)) *Unlock {
	ret := &Unlock{
		LockID: "",
		Force:  false,
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
