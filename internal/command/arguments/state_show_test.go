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
				stateShow.ShowSensitive = true
				stateShow.TargetRawAddr = "resource_address"
			}),
		},
		"custom state path": {
			args: []string{"-state=/path/to/state.tfstate", "resource_address"},
			want: stateShowArgsWithDefaults(func(stateShow *StateShow) {
				stateShow.StatePath = "/path/to/state.tfstate"
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
				stateShow.ShowSensitive = true
				stateShow.StatePath = "/path/to/state.tfstate"
				stateShow.TargetRawAddr = "resource_address"
				// Vars would be updated, but we ignore it in cmp
			}),
		},
		"no arguments": {
			args:        []string{},
			want:        stateShowArgsWithDefaults(nil),
			wantErrText: "Invalid number of arguments",
		},
		"too many arguments": {
			args:        []string{"resource_address", "extra"},
			want:        stateShowArgsWithDefaults(nil),
			wantErrText: "Invalid number of arguments",
		},
	}

	cmpOpts := cmp.Options{
		cmpopts.IgnoreUnexported(Vars{}, ViewOptions{}),
		cmpopts.IgnoreFields(ViewOptions{}, "JSONInto"), // We ignore JSONInto because it contains a file which is not really diffable
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
		ShowSensitive: false,
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
