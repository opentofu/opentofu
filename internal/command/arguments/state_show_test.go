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

func TestParseStateShow_basicValidation(t *testing.T) {
	testCases := map[string]struct {
		args        []string
		want        *StateShow
		wantErrText string
	}{
		"defaults": {
			args: []string{"resource_address"},
			want: stateShowArgsWithDefaults(func(stateShow *StateShow) {
				stateShow.TargetRawAddr = "resource_address"
			}),
		},
		"show-sensitive enabled": {
			args: []string{"-show-sensitive", "resource_address"},
			want: stateShowArgsWithDefaults(func(stateShow *StateShow) {
				stateShow.View.ShowSensitive = true
				stateShow.TargetRawAddr = "resource_address"
			}),
		},
		"custom state path": {
			args: []string{"-state=/path/to/state.tfstate", "resource_address"},
			want: stateShowArgsWithDefaults(func(stateShow *StateShow) {
				stateShow.State.StatePath = "/path/to/state.tfstate"
				stateShow.TargetRawAddr = "resource_address"
			}),
		},
		"all flags combined": {
			args: []string{
				"-show-sensitive",
				"-state=/path/to/state.tfstate",
				"-var=key=value",
				"resource_address",
			},
			want: stateShowArgsWithDefaults(func(stateShow *StateShow) {
				stateShow.View.ShowSensitive = true
				stateShow.State.StatePath = "/path/to/state.tfstate"
				stateShow.TargetRawAddr = "resource_address"
				stateShow.Vars = &Vars{{Name: "-var", Value: "key=value"}}
			}),
		},
		"no arguments": {
			args:        []string{},
			want:        stateShowArgsWithDefaults(nil),
			wantErrText: "Expected exactly one positional argument",
		},
		"too many arguments": {
			args: []string{"resource_address", "extra"},
			want: stateShowArgsWithDefaults(func(stateShow *StateShow) {
				stateShow.TargetRawAddr = "resource_address"
			}),
			wantErrText: "Expected exactly one positional argument",
		},
		"unknown flag": {
			args:        []string{"-unknown-flag", "resource_address"},
			want:        stateShowArgsWithDefaults(nil),
			wantErrText: "flag provided but not defined: -unknown-flag",
		},
	}

	cmpOpts := cmp.Options{
		cmpopts.IgnoreFields(View{}, "JSONInto"), // We ignore JSONInto because it contains a file which is not really diffable
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseStateShow(tc.args)
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

func stateShowArgsWithDefaults(mutate func(stateShow *StateShow)) *StateShow {
	ret := &StateShow{
		View: &View{
			ShowSensitive:       false,
			ConsolidateWarnings: true,
			ViewType:            ViewHuman,
			InputEnabled:        false,
		},
		Vars:  &Vars{},
		State: &State{},
	}
	if mutate != nil {
		mutate(ret)
	}
	return ret
}
