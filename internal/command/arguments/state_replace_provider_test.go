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

func TestParseReplaceProvider_basicValidation(t *testing.T) {
	testCases := map[string]struct {
		args        []string
		want        *StateReplaceProvider
		wantErrText string
	}{
		"defaults": {
			args: []string{"source", "dest"},
			want: stateReplaceProviderArgsWithDefaults(func(srp *StateReplaceProvider) {
				srp.RawSrcAddr = "source"
				srp.RawDestAddr = "dest"
			}),
		},
		"auto-approve enabled": {
			args: []string{"-auto-approve", "source", "dest"},
			want: stateReplaceProviderArgsWithDefaults(func(srp *StateReplaceProvider) {
				srp.AutoApprove = true
				srp.RawSrcAddr = "source"
				srp.RawDestAddr = "dest"
			}),
		},
		"custom backup path": {
			args: []string{"-backup=/path/to/backup.tfstate", "source", "dest"},
			want: stateReplaceProviderArgsWithDefaults(func(srp *StateReplaceProvider) {
				srp.BackupPath = "/path/to/backup.tfstate"
				srp.RawSrcAddr = "source"
				srp.RawDestAddr = "dest"
			}),
		},
		"custom state path": {
			args: []string{"-state=/path/to/state.tfstate", "source", "dest"},
			want: stateReplaceProviderArgsWithDefaults(func(srp *StateReplaceProvider) {
				srp.StatePath = "/path/to/state.tfstate"
				srp.RawSrcAddr = "source"
				srp.RawDestAddr = "dest"
			}),
		},
		"only lock-timeout": {
			args: []string{"-lock-timeout=10s", "source", "dest"},
			want: stateReplaceProviderArgsWithDefaults(func(srp *StateReplaceProvider) {
				srp.Backend.StateLock = true
				srp.Backend.StateLockTimeout = 10 * time.Second
				srp.RawSrcAddr = "source"
				srp.RawDestAddr = "dest"
			}),
		},
		"disable locking": {
			args: []string{"-lock=false", "source", "dest"},
			want: stateReplaceProviderArgsWithDefaults(func(srp *StateReplaceProvider) {
				srp.Backend.StateLock = false
				srp.RawSrcAddr = "source"
				srp.RawDestAddr = "dest"
			}),
		},
		"all flags combined": {
			args: []string{
				"-auto-approve",
				"-backup=/path/to/backup.tfstate",
				"-state=/path/to/state.tfstate",
				"-lock-timeout=15s",
				"-lock=true",
				"-var=key=value",
				"source",
				"dest",
			},
			want: stateReplaceProviderArgsWithDefaults(func(srp *StateReplaceProvider) {
				srp.AutoApprove = true
				srp.BackupPath = "/path/to/backup.tfstate"
				srp.StatePath = "/path/to/state.tfstate"
				srp.Backend.StateLockTimeout = 15 * time.Second
				srp.Backend.StateLock = true
				srp.RawSrcAddr = "source"
				srp.RawDestAddr = "dest"
				// Vars would be updated, but we ignore it in cmp
			}),
		},
		"no arguments": {
			args:        []string{},
			want:        stateReplaceProviderArgsWithDefaults(nil),
			wantErrText: "Invalid number of arguments",
		},
		"only one argument": {
			args:        []string{"source"},
			want:        stateReplaceProviderArgsWithDefaults(nil),
			wantErrText: "Invalid number of arguments",
		},
		"too many arguments": {
			args:        []string{"source", "dest", "extra"},
			want:        stateReplaceProviderArgsWithDefaults(nil),
			wantErrText: "Invalid number of arguments",
		},
		"json without auto-approve": {
			args: []string{"-json", "source", "dest"},
			want: stateReplaceProviderArgsWithDefaults(func(srp *StateReplaceProvider) {
				srp.ViewOptions.ViewType = ViewJSON
				srp.RawSrcAddr = "source"
				srp.RawDestAddr = "dest"
			}),
			wantErrText: "Invalid usage: OpenTofu cannot ask user input when `-json` flag is used. Therefore, `-auto-approve` is required too",
		},
		"json with auto-approve": {
			args: []string{"-json", "-auto-approve", "source", "dest"},
			want: stateReplaceProviderArgsWithDefaults(func(srp *StateReplaceProvider) {
				srp.ViewOptions.ViewType = ViewJSON
				srp.AutoApprove = true
				srp.RawSrcAddr = "source"
				srp.RawDestAddr = "dest"
			}),
		},
	}

	cmpOpts := cmp.Options{
		cmpopts.IgnoreUnexported(Vars{}, ViewOptions{}),
		cmpopts.IgnoreFields(ViewOptions{}, "JSONInto"), // We ignore JSONInto because it contains a file which is not really diffable
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseReplaceProvider(tc.args)
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

func stateReplaceProviderArgsWithDefaults(mutate func(srp *StateReplaceProvider)) *StateReplaceProvider {
	ret := &StateReplaceProvider{
		AutoApprove: false,
		BackupPath:  "-",
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
