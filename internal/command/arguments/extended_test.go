// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Legacy helpers
func BindStateBackupFlag(cli *CommandLine, def string) *State {
	s := BindState(cli, stateFlagBackup)
	cli.PreHook(func() tfdiags.Diagnostics {
		if s.BackupPath == "" {
			s.BackupPath = def
		}
		return nil
	})
	return s
}
func BindStateInFlag(cli *CommandLine, def string) *State {
	s := BindState(cli, stateFlagStateIn)
	cli.PreHook(func() tfdiags.Diagnostics {
		if s.StatePath == "" {
			s.StatePath = def
		}
		return nil
	})
	return s
}

func TestStateFlagsParsing(t *testing.T) {

	testCases := map[string]struct {
		args     []string
		register func(cli *CommandLine) *State
		want     *State
		wantErr  error
	}{
		"defaults": {
			args: nil,
			register: func(cli *CommandLine) *State {
				return BindState(cli, stateFlagAll)
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.Lock = true
			}),
		},
		"lock": {
			args: []string{"-lock=false"},
			register: func(cli *CommandLine) *State {
				return BindState(cli, stateFlagAll)
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.Lock = false
			}),
		},
		"lockTimeout": {
			args: []string{"-lock-timeout=2s"},
			register: func(cli *CommandLine) *State {
				return BindState(cli, stateFlagAll)
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.LockTimeout = 2 * time.Second
				v.Lock = true
			}),
		},
		"state": {
			args: []string{"-state=/path/to/state"},
			register: func(cli *CommandLine) *State {
				return BindState(cli, stateFlagAll)
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.StatePath = "/path/to/state"
				v.Lock = true
			}),
		},
		"stateOut": {
			args: []string{"-state-out=/path/to/output/state"},
			register: func(cli *CommandLine) *State {
				return BindState(cli, stateFlagAll)
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.StateOutPath = "/path/to/output/state"
				v.Lock = true
			}),
		},
		"backup": {
			args: []string{"-backup=/path/to/state/backup"},
			register: func(cli *CommandLine) *State {
				return BindState(cli, stateFlagAll)
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.BackupPath = "/path/to/state/backup"
				v.Lock = true
			}),
		},
		"all flags": {
			args: []string{
				"-backup=/path/to/state/backup",
				"-state-out=/path/to/output/state",
				"-state=/path/to/state",
				"-lock-timeout=2s",
				"-lock=false",
			},
			register: func(cli *CommandLine) *State {
				return BindState(cli, stateFlagAll)
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.BackupPath = "/path/to/state/backup"
				v.StateOutPath = "/path/to/output/state"
				v.StatePath = "/path/to/state"
				v.LockTimeout = 2 * time.Second
				v.Lock = false
			}),
		},
		"unknown flags provided": {
			args: []string{
				"-backup=/path/to/state/backup",
				"-unknown=foo",
			},
			register: func(cli *CommandLine) *State {
				return BindState(cli, stateFlagAll)
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.BackupPath = "/path/to/state/backup"
				v.Lock = true
			}),
			wantErr: fmt.Errorf("flag provided but not defined: -unknown"),
		},
		"register only backup flag - no flags provided": {
			args: []string{},
			register: func(cli *CommandLine) *State {
				return BindStateBackupFlag(cli, "-")
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.BackupPath = "-" // the provided different default
			}),
		},
		"register only backup flag - with backup flag": {
			args: []string{"-backup=/path/to/backup"},
			register: func(cli *CommandLine) *State {
				return BindStateBackupFlag(cli, "-")
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.BackupPath = "/path/to/backup"
			}),
		},
		"register only backup flag - unregistered flag": {
			args: []string{"-backup=/path/to/backup", "-lock=false"},
			register: func(cli *CommandLine) *State {
				return BindStateBackupFlag(cli, "-")
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.BackupPath = "/path/to/backup"
			}),
			wantErr: fmt.Errorf("flag provided but not defined: -lock"),
		},
		"register only stateIn flag - no flags provided": {
			args: []string{},
			register: func(cli *CommandLine) *State {
				return BindStateInFlag(cli, "default.tfstate")
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.StatePath = "default.tfstate" // the provided different default
			}),
		},
		"register only stateIn flag - with state flag": {
			args: []string{"-state=/path/to/state"},
			register: func(cli *CommandLine) *State {
				return BindStateInFlag(cli, "default.tfstate")
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.StatePath = "/path/to/state"
			}),
		},
		"register only stateIn flag - unregistered flag": {
			args: []string{"-state=/path/to/state", "-lock=false"},
			register: func(cli *CommandLine) *State {
				return BindStateInFlag(cli, "-")
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.StatePath = "/path/to/state"
			}),
			wantErr: fmt.Errorf("flag provided but not defined: -lock"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			var cli CommandLine
			s := tc.register(&cli)

			_, diags := cli.parseWithHooks("test", tc.args)

			if want, got := fmt.Sprintf("%s", tc.wantErr), fmt.Sprintf("%s", diags.Err()); !strings.Contains(got, want) {
				t.Errorf("wanted error: %q, got error %q", want, got)
			}
			if diff := cmp.Diff(tc.want, s); diff != "" {
				t.Errorf("unexpected result (-want,+got)\n%s", diff)
			}
		})
	}
}

func TestStateFlagsRegistering(t *testing.T) {
	testCases := map[string]struct {
		register func(cli *CommandLine) *State
		args     []string
		want     *State
		wantErr  error
	}{
		"no flag registered": {
			register: func(cli *CommandLine) *State {
				return new(State)
			},
			args:    []string{"-lock=false"},
			want:    stateArgsWithDefaults(func(v *State) {}),
			wantErr: fmt.Errorf("flag provided but not defined: -lock"),
		},
		"only lock flags registered": {
			register: func(cli *CommandLine) *State {
				return BindState(cli, stateFlagLock)
			},
			args: []string{
				"-lock=false",
				"-lock-timeout=42ns",
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.Lock = false
				v.LockTimeout = 42 * time.Nanosecond
			}),
		},
		"lock and state in": {
			register: func(cli *CommandLine) *State {
				return BindState(cli, stateFlagLock|stateFlagStateIn)
			},
			args: []string{
				"-lock=false",
				"-lock-timeout=42µs",
				"-state=/path/to/state",
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.Lock = false
				v.LockTimeout = 42 * time.Microsecond
				v.StatePath = "/path/to/state"
			}),
		},
		"lock, state in and state out": {
			register: func(cli *CommandLine) *State {
				return BindState(cli, stateFlagLock|stateFlagStateIn|stateFlagStateOut)
			},
			args: []string{
				"-lock=false",
				"-lock-timeout=42ms",
				"-state=/path/to/state",
				"-state-out=/path/to/output/state",
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.Lock = false
				v.LockTimeout = 42 * time.Millisecond
				v.StatePath = "/path/to/state"
				v.StateOutPath = "/path/to/output/state"
			}),
		},
		"lock, state in, state out and backup": {
			register: func(cli *CommandLine) *State {
				return BindState(cli, stateFlagAll)
			},
			args: []string{
				"-lock=false",
				"-lock-timeout=42s",
				"-state=/path/to/state",
				"-state-out=/path/to/output/state",
				"-backup=/path/to/state/backup",
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.Lock = false
				v.LockTimeout = 42 * time.Second
				v.StatePath = "/path/to/state"
				v.StateOutPath = "/path/to/output/state"
				v.BackupPath = "/path/to/state/backup"
			}),
		},
		"lock, state in, state out and backup with a different default": {
			register: func(cli *CommandLine) *State {
				s := BindState(cli, stateFlagLock|stateFlagStateIn|stateFlagStateOut|stateFlagBackup)
				cli.PreHook(func() tfdiags.Diagnostics {
					if s.BackupPath == "" {
						s.BackupPath = "-"
					}
					return nil
				})
				return s
			},
			args: []string{
				"-lock=false",
				"-lock-timeout=42m",
				"-state=/path/to/state",
				"-state-out=/path/to/output/state",
				"-backup=/path/to/state/backup",
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.Lock = false
				v.LockTimeout = 42 * time.Minute
				v.StatePath = "/path/to/state"
				v.StateOutPath = "/path/to/output/state"
				v.BackupPath = "/path/to/state/backup"
			}),
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			var cli CommandLine
			s := tc.register(&cli)
			_, diags := cli.parseWithHooks("test", tc.args)
			if want, got := fmt.Sprintf("%s", tc.wantErr), fmt.Sprintf("%s", diags.Err()); !strings.Contains(got, want) {
				t.Errorf("wanted error: %q, got error %q", want, got)
			}
			if diff := cmp.Diff(tc.want, s); diff != "" {
				t.Errorf("unexpected result (-want,+got)\n%s", diff)
			}
		})
	}
}

func stateArgsWithDefaults(mutate func(v *State)) *State {
	ret := &State{}
	if mutate != nil {
		mutate(ret)
	}
	return ret
}
