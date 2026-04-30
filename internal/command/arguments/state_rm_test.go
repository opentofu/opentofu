// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestParseStateRm_basicValidation(t *testing.T) {
	testCases := map[string]struct {
		args        []string
		want        *StateRm
		wantErrText string
	}{
		"defaults": {
			args: []string{"resource.foo"},
			want: stateRmArgsWithDefaults(func(stateRm *StateRm) {
				stateRm.TargetAddrs = []string{"resource.foo"}
			}),
		},
		"multiple addresses": {
			args: []string{"resource.foo", "resource.bar", "module.baz"},
			want: stateRmArgsWithDefaults(func(stateRm *StateRm) {
				stateRm.TargetAddrs = []string{"resource.foo", "resource.bar", "module.baz"}
			}),
		},
		"dry-run enabled": {
			args: []string{"-dry-run", "resource.foo"},
			want: stateRmArgsWithDefaults(func(stateRm *StateRm) {
				stateRm.DryRun = true
				stateRm.TargetAddrs = []string{"resource.foo"}
			}),
		},
		"custom backup path": {
			args: []string{"-backup=/path/to/backup.tfstate", "resource.foo"},
			want: stateRmArgsWithDefaults(func(stateRm *StateRm) {
				stateRm.State.BackupPath = "/path/to/backup.tfstate"
				stateRm.TargetAddrs = []string{"resource.foo"}
			}),
		},
		"custom state path": {
			args: []string{"-state=/path/to/state.tfstate", "resource.foo"},
			want: stateRmArgsWithDefaults(func(stateRm *StateRm) {
				stateRm.State.StatePath = "/path/to/state.tfstate"
				stateRm.TargetAddrs = []string{"resource.foo"}
			}),
		},
		"only lock-timeout": {
			args: []string{"-lock-timeout=10s", "resource.foo"},
			want: stateRmArgsWithDefaults(func(stateRm *StateRm) {
				stateRm.State.LockTimeout = 10 * time.Second
				stateRm.TargetAddrs = []string{"resource.foo"}
			}),
		},
		"disable locking": {
			args: []string{"-lock=false", "resource.foo"},
			want: stateRmArgsWithDefaults(func(stateRm *StateRm) {
				stateRm.State.Lock = false
				stateRm.TargetAddrs = []string{"resource.foo"}
			}),
		},
		"all flags combined": {
			args: []string{
				"-dry-run",
				"-backup=/path/to/backup.tfstate",
				"-state=/path/to/state.tfstate",
				"-lock-timeout=15s",
				"-lock=true",
				"-var=key=value",
				"resource.foo",
				"resource.bar",
			},
			want: stateRmArgsWithDefaults(func(stateRm *StateRm) {
				stateRm.DryRun = true
				stateRm.State.BackupPath = "/path/to/backup.tfstate"
				stateRm.State.StatePath = "/path/to/state.tfstate"
				stateRm.State.LockTimeout = 15 * time.Second
				stateRm.State.Lock = true
				stateRm.TargetAddrs = []string{"resource.foo", "resource.bar"}
				// Vars would be updated, but we ignore it in cmp
			}),
		},
		"no arguments": {
			args:        []string{},
			want:        stateRmArgsWithDefaults(nil),
			wantErrText: "Invalid number of arguments: At least one address is required",
		},
	}

	cmpOpts := cmp.Options{
		cmpopts.IgnoreUnexported(Vars{}, ViewOptions{}, State{}),
		cmpopts.IgnoreFields(ViewOptions{}, "JSONInto"), // We ignore JSONInto because it contains a file which is not really diffable
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseStateRm(tc.args)
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

func stateRmArgsWithDefaults(mutate func(stateRm *StateRm)) *StateRm {
	ret := &StateRm{
		DryRun: false,
		ViewOptions: ViewOptions{
			ViewType:     ViewHuman,
			InputEnabled: false,
		},
		Backend: &Backend{
			IgnoreRemoteVersion: false,
		},
		Vars: &Vars{},
		State: &State{
			Lock: true,
			// Because the default value is different on this command
			BackupPath: "-",
		},
	}
	if mutate != nil {
		mutate(ret)
	}
	return ret
}
