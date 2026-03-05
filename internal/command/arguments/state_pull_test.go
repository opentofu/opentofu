// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestParseStatePull_basicValidation(t *testing.T) {
	testCases := map[string]struct {
		args        []string
		want        *StatePull
		wantErrText string
	}{
		"no arguments": {
			args:        []string{},
			want:        statePullArgsWithDefaults(nil),
			wantErrText: "",
		},
		"too many arguments": {
			args:        []string{"foo"},
			want:        statePullArgsWithDefaults(nil),
			wantErrText: "Unexpected argument",
		},
		"unknown flag": {
			args:        []string{"-unknown"},
			want:        statePullArgsWithDefaults(nil),
			wantErrText: "Failed to parse command-line flags",
		},
	}

	cmpOpts := cmp.Options{
		cmpopts.IgnoreUnexported(Vars{}, ViewOptions{}, State{}),
		cmpopts.IgnoreFields(ViewOptions{}, "JSONInto"), // We ignore JSONInto because it contains a file which is not really diffable
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseStatePull(tc.args)
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

func TestParseStatePull_vars(t *testing.T) {
	testCases := map[string]struct {
		args      []string
		wantCount int
		wantEmpty bool
	}{
		"no vars": {
			args:      []string{},
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
		for _, isTaint := range []bool{true, false} {
			tcName := fmt.Sprintf("%s_%t", name, isTaint)
			t.Run(tcName, func(t *testing.T) {
				got, closer, diags := ParseStatePull(tc.args)
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
}

func statePullArgsWithDefaults(mutate func(args *StatePull)) *StatePull {
	ret := &StatePull{
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
