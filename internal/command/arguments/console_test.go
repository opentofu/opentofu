// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestParseConsole_basicValidation(t *testing.T) {
	tempDir := t.TempDir()
	testCases := map[string]struct {
		args []string
		want *Console
	}{
		"defaults": {
			args: nil,
			want: consoleArgsWithDefaults(nil),
		},
		"custom state path": {
			args: []string{"-state=/path/to/state.tfstate"},
			want: consoleArgsWithDefaults(func(console *Console) {
				console.StatePath = "/path/to/state.tfstate"
			}),
		},
		"json-into with input enabled": {
			args: []string{fmt.Sprintf("-json-into=%s", filepath.Join(tempDir, "json-into"))},
			want: consoleArgsWithDefaults(func(console *Console) {
				// ViewOptions would be updated, but we ignore it in cmp
			}),
		},
		"single var": {
			args: []string{"-var=key=value"},
			want: consoleArgsWithDefaults(func(console *Console) {
				// Vars would be updated, but we ignore it in cmp
			}),
		},
		"multiple vars": {
			args: []string{"-var=key1=value1", "-var=key2=value2"},
			want: consoleArgsWithDefaults(func(console *Console) {
				// Vars would be updated, but we ignore it in cmp
			}),
		},
		"var-file": {
			args: []string{"-var-file=test.tfvars"},
			want: consoleArgsWithDefaults(func(console *Console) {
				// Vars would be updated, but we ignore it in cmp
			}),
		},
		"mixed vars and var-files": {
			args: []string{"-var=key=value", "-var-file=test.tfvars", "-var=another=val"},
			want: consoleArgsWithDefaults(func(console *Console) {
				// Vars would be updated, but we ignore it in cmp
			}),
		},
		"only lock-timeout": {
			args: []string{"-lock-timeout=10s"},
			want: consoleArgsWithDefaults(func(console *Console) {
				// do not set `console.Backend.StateLock = true` since it's meant to be true already
				console.Backend.StateLockTimeout = 10 * time.Second
			}),
		},
		"disable locking": {
			args: []string{"-lock=false"},
			want: consoleArgsWithDefaults(func(console *Console) {
				console.Backend.StateLock = false
			}),
		},
	}

	cmpOpts := cmp.Options{
		cmpopts.IgnoreUnexported(Vars{}, ViewOptions{}),
		cmpopts.IgnoreFields(ViewOptions{}, "JSONInto"), // We ignore JSONInto because it contains a file which is not really diffable
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseConsole(tc.args)
			defer closer()

			if len(diags) > 0 {
				t.Fatalf("unexpected diags: %v", diags)
			}
			if diff := cmp.Diff(tc.want, got, cmpOpts); diff != "" {
				t.Errorf("unexpected result\n%s", diff)
			}
		})
	}
}

func consoleArgsWithDefaults(mutate func(console *Console)) *Console {
	ret := &Console{
		StatePath: DefaultStateFilename,
		ViewOptions: ViewOptions{
			ViewType:     ViewHuman,
			InputEnabled: true,
		},
		Vars: &Vars{},
		Backend: Backend{
			StateLock: true,
		},
	}
	if mutate != nil {
		mutate(ret)
	}
	return ret
}
