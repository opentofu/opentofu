// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestStateFlagsParsing(t *testing.T) {
	testCases := map[string]struct {
		args     []string
		register func(s *State, f *flag.FlagSet)
		want     *State
		wantErr  error
	}{
		"defaults": {
			args: nil,
			register: func(s *State, f *flag.FlagSet) {
				s.AddFlags(f, true, true, true, true)
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.Lock = true
			}),
		},
		"lock": {
			args: []string{"-lock=false"},
			register: func(s *State, f *flag.FlagSet) {
				s.AddFlags(f, true, true, true, true)
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.Lock = false
			}),
		},
		"lockTimeout": {
			args: []string{"-lock-timeout=2s"},
			register: func(s *State, f *flag.FlagSet) {
				s.AddFlags(f, true, true, true, true)
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.LockTimeout = 2 * time.Second
				v.Lock = true
			}),
		},
		"state": {
			args: []string{"-state=/path/to/state"},
			register: func(s *State, f *flag.FlagSet) {
				s.AddFlags(f, true, true, true, true)
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.StatePath = "/path/to/state"
				v.Lock = true
			}),
		},
		"stateOut": {
			args: []string{"-state-out=/path/to/output/state"},
			register: func(s *State, f *flag.FlagSet) {
				s.AddFlags(f, true, true, true, true)
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.StateOutPath = "/path/to/output/state"
				v.Lock = true
			}),
		},
		"backup": {
			args: []string{"-backup=/path/to/state/backup"},
			register: func(s *State, f *flag.FlagSet) {
				s.AddFlags(f, true, true, true, true)
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
			register: func(s *State, f *flag.FlagSet) {
				s.AddFlags(f, true, true, true, true)
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
			register: func(s *State, f *flag.FlagSet) {
				s.AddFlags(f, true, true, true, true)
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.BackupPath = "/path/to/state/backup"
				v.Lock = true
			}),
			wantErr: fmt.Errorf("flag provided but not defined: -unknown"),
		},
		"register only backup flag - no flags provided": {
			args: []string{},
			register: func(s *State, f *flag.FlagSet) {
				s.AddBackupFlag(f, "-")
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.BackupPath = "-" // the provided different default
			}),
		},
		"register only backup flag - with backup flag": {
			args: []string{"-backup=/path/to/backup"},
			register: func(s *State, f *flag.FlagSet) {
				s.AddBackupFlag(f, "-")
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.BackupPath = "/path/to/backup"
			}),
		},
		"register only backup flag - unregistered flag": {
			args: []string{"-backup=/path/to/backup", "-lock=false"},
			register: func(s *State, f *flag.FlagSet) {
				s.AddBackupFlag(f, "-")
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.BackupPath = "/path/to/backup"
			}),
			wantErr: fmt.Errorf("flag provided but not defined: -lock"),
		},
		"register only stateIn flag - no flags provided": {
			args: []string{},
			register: func(s *State, f *flag.FlagSet) {
				s.AddStateInFlag(f, "default.tfstate")
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.StatePath = "default.tfstate" // the provided different default
			}),
		},
		"register only stateIn flag - with state flag": {
			args: []string{"-state=/path/to/state"},
			register: func(s *State, f *flag.FlagSet) {
				s.AddStateInFlag(f, "default.tfstate")
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.StatePath = "/path/to/state"
			}),
		},
		"register only stateIn flag - unregistered flag": {
			args: []string{"-state=/path/to/state", "-lock=false"},
			register: func(s *State, f *flag.FlagSet) {
				s.AddStateInFlag(f, "-")
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.StatePath = "/path/to/state"
			}),
			wantErr: fmt.Errorf("flag provided but not defined: -lock"),
		},
	}

	cmpOpts := cmpopts.IgnoreUnexported()

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			s := &State{}
			f := defaultFlagSet("test")
			tc.register(s, f)
			err := f.Parse(tc.args)
			if diff := cmp.Diff(fmt.Sprintf("%s", tc.wantErr), fmt.Sprintf("%s", err)); diff != "" {
				t.Errorf("unexpected error (-want,+got)\n%s", diff)
			}
			if diff := cmp.Diff(tc.want, s, cmpOpts); diff != "" {
				t.Errorf("unexpected result (-want,+got)\n%s", diff)
			}
		})
	}
}

func TestStateFlagsRegistering(t *testing.T) {
	testCases := map[string]struct {
		register func(s *State, f *flag.FlagSet)
		want     *State
		wantErr  error
	}{
		"no flag registered": {
			register: func(s *State, f *flag.FlagSet) {
			},
			want:    stateArgsWithDefaults(func(v *State) {}),
			wantErr: fmt.Errorf("flag provided but not defined: -lock"),
		},
		"only lock flags registered": {
			register: func(s *State, f *flag.FlagSet) {
				s.AddFlags(f, true, false, false, false)
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.Lock = false
				v.LockTimeout = 2 * time.Second
			}),
			wantErr: fmt.Errorf("flag provided but not defined: -state"),
		},
		"lock and state in": {
			register: func(s *State, f *flag.FlagSet) {
				s.AddFlags(f, true, true, false, false)
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.Lock = false
				v.LockTimeout = 2 * time.Second
				v.StatePath = "/path/to/state"
			}),
			wantErr: fmt.Errorf("flag provided but not defined: -state-out"),
		},
		"lock, state in and state out": {
			register: func(s *State, f *flag.FlagSet) {
				s.AddFlags(f, true, true, true, false)
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.Lock = false
				v.LockTimeout = 2 * time.Second
				v.StatePath = "/path/to/state"
				v.StateOutPath = "/path/to/output/state"
			}),
			wantErr: fmt.Errorf("flag provided but not defined: -backup"),
		},
		"lock, state in, state out and backup": {
			register: func(s *State, f *flag.FlagSet) {
				s.AddFlags(f, true, true, true, true)
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.Lock = false
				v.LockTimeout = 2 * time.Second
				v.StatePath = "/path/to/state"
				v.StateOutPath = "/path/to/output/state"
				v.BackupPath = "/path/to/state/backup"
			}),
		},
		"lock, state in, state out and backup with a different default": {
			register: func(s *State, f *flag.FlagSet) {
				s.AddFlags(f, true, true, true, false)
				s.AddBackupFlag(f, "-")
			},
			want: stateArgsWithDefaults(func(v *State) {
				v.Lock = false
				v.LockTimeout = 2 * time.Second
				v.StatePath = "/path/to/state"
				v.StateOutPath = "/path/to/output/state"
				v.BackupPath = "/path/to/state/backup"
			}),
		},
	}
	cmpOpts := cmpopts.IgnoreUnexported()

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			s := &State{}
			f := defaultFlagSet("test")
			tc.register(s, f)
			err := f.Parse([]string{
				"-lock=false",
				"-lock-timeout=2s",
				"-state=/path/to/state",
				"-state-out=/path/to/output/state",
				"-backup=/path/to/state/backup",
			})
			if diff := cmp.Diff(fmt.Sprintf("%s", tc.wantErr), fmt.Sprintf("%s", err)); diff != "" {
				t.Errorf("unexpected error (-want,+got)\n%s", diff)
			}
			if diff := cmp.Diff(tc.want, s, cmpOpts); diff != "" {
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
