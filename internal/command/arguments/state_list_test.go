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

func TestParseStateList_basicValidation(t *testing.T) {
	testCases := map[string]struct {
		args        []string
		want        *StateList
		wantErrText string
	}{
		"defaults": {
			args: []string{},
			want: stateListArgsWithDefaults(nil),
		},
		"custom state path": {
			args: []string{"-state=/path/to/state.tfstate"},
			want: stateListArgsWithDefaults(func(stateList *StateList) {
				stateList.State.StatePath = "/path/to/state.tfstate"
			}),
		},
		"lookup by id": {
			args: []string{"-id=i-1234567890abcdef0"},
			want: stateListArgsWithDefaults(func(stateList *StateList) {
				stateList.LookupId = "i-1234567890abcdef0"
			}),
		},
		"single instance address": {
			args: []string{"aws_instance.example"},
			want: stateListArgsWithDefaults(func(stateList *StateList) {
				stateList.InstancesRawAddr = []string{"aws_instance.example"}
			}),
		},
		"multiple instance addresses": {
			args: []string{"aws_instance.example", "aws_s3_bucket.data"},
			want: stateListArgsWithDefaults(func(stateList *StateList) {
				stateList.InstancesRawAddr = []string{"aws_instance.example", "aws_s3_bucket.data"}
			}),
		},
		"combined flags and addresses": {
			args: []string{"-state=/path/to/state.tfstate", "-id=i-123", "aws_instance.example", "aws_instance.example2"},
			want: stateListArgsWithDefaults(func(stateList *StateList) {
				stateList.State.StatePath = "/path/to/state.tfstate"
				stateList.LookupId = "i-123"
				stateList.InstancesRawAddr = []string{"aws_instance.example", "aws_instance.example2"}
			}),
		},
		"invalid flags": {
			args:        []string{"-unknown"},
			want:        stateListArgsWithDefaults(nil),
			wantErrText: "Failed to parse command-line flags: flag provided but not defined: -unknown",
		},
	}

	cmpOpts := cmp.Options{
		cmpopts.IgnoreUnexported(Vars{}, ViewOptions{}),
		cmpopts.IgnoreFields(ViewOptions{}, "JSONInto"), // We ignore JSONInto because it contains a file which is not really diffable
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseStateList(tc.args)
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

func TestParseStateList_vars(t *testing.T) {
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
			got, closer, diags := ParseStateList(tc.args)
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

func stateListArgsWithDefaults(mutate func(stateList *StateList)) *StateList {
	ret := &StateList{
		State:            &State{},
		LookupId:         "",
		InstancesRawAddr: []string{},
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
