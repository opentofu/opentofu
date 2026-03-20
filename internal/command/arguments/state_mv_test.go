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

func TestParseStateMv_basicValidation(t *testing.T) {
	testCases := map[string]struct {
		args        []string
		want        *StateMv
		wantErrText string
	}{
		"defaults": {
			args: []string{"source", "dest"},
			want: stateMvArgsWithDefaults(func(stateMv *StateMv) {
				stateMv.RawSrcAddr = "source"
				stateMv.RawDestAddr = "dest"
			}),
		},
		"dry-run enabled": {
			args: []string{"-dry-run", "source", "dest"},
			want: stateMvArgsWithDefaults(func(stateMv *StateMv) {
				stateMv.DryRun = true
				stateMv.RawSrcAddr = "source"
				stateMv.RawDestAddr = "dest"
			}),
		},
		"custom backup-out path": {
			args: []string{"-backup-out=/path/to/backup.tfstate", "source", "dest"},
			want: stateMvArgsWithDefaults(func(stateMv *StateMv) {
				stateMv.BackupPathOut = "/path/to/backup.tfstate"
				stateMv.RawSrcAddr = "source"
				stateMv.RawDestAddr = "dest"
			}),
		},
		"custom state path": {
			args: []string{"-state=/path/to/state.tfstate", "source", "dest"},
			want: stateMvArgsWithDefaults(func(stateMv *StateMv) {
				stateMv.StatePath = "/path/to/state.tfstate"
				stateMv.RawSrcAddr = "source"
				stateMv.RawDestAddr = "dest"
			}),
		},
		"custom state-out path": {
			args: []string{"-state-out=/path/to/state-out.tfstate", "source", "dest"},
			want: stateMvArgsWithDefaults(func(stateMv *StateMv) {
				stateMv.StateOutPath = "/path/to/state-out.tfstate"
				stateMv.RawSrcAddr = "source"
				stateMv.RawDestAddr = "dest"
			}),
		},
		"custom backup path": {
			args: []string{"-backup=/path/to/backup.tfstate", "source", "dest"},
			want: stateMvArgsWithDefaults(func(stateMv *StateMv) {
				stateMv.BackupPath = "/path/to/backup.tfstate"
				stateMv.RawSrcAddr = "source"
				stateMv.RawDestAddr = "dest"
			}),
		},
		"only lock-timeout": {
			args: []string{"-lock-timeout=10s", "source", "dest"},
			want: stateMvArgsWithDefaults(func(stateMv *StateMv) {
				// do not set `stateMv.State.Lock = true` since it's meant to be true already
				stateMv.Backend.StateLockTimeout = 10 * time.Second
				stateMv.RawSrcAddr = "source"
				stateMv.RawDestAddr = "dest"
			}),
		},
		"disable locking": {
			args: []string{"-lock=false", "source", "dest"},
			want: stateMvArgsWithDefaults(func(stateMv *StateMv) {
				stateMv.Backend.StateLock = false
				stateMv.RawSrcAddr = "source"
				stateMv.RawDestAddr = "dest"
			}),
		},
		"all flags combined": {
			args: []string{
				"-dry-run",
				"-backup-out=/path/to/backup-out.tfstate",
				"-state=/path/to/state.tfstate",
				"-state-out=/path/to/state-out.tfstate",
				"-backup=/path/to/backup.tfstate",
				"-lock-timeout=15s",
				"-lock=true",
				"-var=key=value",
				"source",
				"dest",
			},
			want: stateMvArgsWithDefaults(func(stateMv *StateMv) {
				stateMv.DryRun = true
				stateMv.BackupPathOut = "/path/to/backup-out.tfstate"
				stateMv.StatePath = "/path/to/state.tfstate"
				stateMv.StateOutPath = "/path/to/state-out.tfstate"
				stateMv.BackupPath = "/path/to/backup.tfstate"
				stateMv.Backend.StateLockTimeout = 15 * time.Second
				stateMv.Backend.StateLock = true
				stateMv.RawSrcAddr = "source"
				stateMv.RawDestAddr = "dest"
				// Vars would be updated, but we ignore it in cmp
			}),
		},
		"no arguments": {
			args:        []string{},
			want:        stateMvArgsWithDefaults(nil),
			wantErrText: "Invalid number of arguments",
		},
		"only one argument": {
			args:        []string{"source"},
			want:        stateMvArgsWithDefaults(nil),
			wantErrText: "Invalid number of arguments",
		},
		"too many arguments": {
			args:        []string{"source", "dest", "extra"},
			want:        stateMvArgsWithDefaults(nil),
			wantErrText: "Invalid number of arguments",
		},
	}

	cmpOpts := cmp.Options{
		cmpopts.IgnoreUnexported(Vars{}, ViewOptions{}, State{}),
		cmpopts.IgnoreFields(ViewOptions{}, "JSONInto"), // We ignore JSONInto because it contains a file which is not really diffable
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseStateMv(tc.args)
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

func stateMvArgsWithDefaults(mutate func(stateMv *StateMv)) *StateMv {
	ret := &StateMv{
		DryRun:        false,
		BackupPath:    "-",
		BackupPathOut: "-",
		ViewOptions: ViewOptions{
			ViewType:     ViewHuman,
			InputEnabled: false,
		},
		Backend: Backend{
			IgnoreRemoteVersion: false,
			StateLock:           true,
			StateLockTimeout:    0,
		},
		Vars: &Vars{},
	}
	if mutate != nil {
		mutate(ret)
	}
	return ret
}
