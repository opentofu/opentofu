// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"flag"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestBackend_AddIgnoreRemoteVersionFlag(t *testing.T) {
	testCases := map[string]struct {
		args []string
		want bool
	}{
		"default value": {
			args: nil,
			want: false,
		},
		"flag not provided": {
			args: []string{},
			want: false,
		},
		"flag set to true": {
			args: []string{"-ignore-remote-version"},
			want: true,
		},
		"flag explicitly set to false": {
			args: []string{"-ignore-remote-version=false"},
			want: false,
		},
		"flag explicitly set to true": {
			args: []string{"-ignore-remote-version=true"},
			want: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			backend := &Backend{}
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			backend.AddIgnoreRemoteVersionFlag(fs)

			if err := fs.Parse(tc.args); err != nil {
				t.Fatalf("unexpected error parsing flags: %v", err)
			}

			if got := backend.IgnoreRemoteVersion; got != tc.want {
				t.Errorf("IgnoreRemoteVersion = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBackend_AddStateFlags(t *testing.T) {
	testCases := map[string]struct {
		args            []string
		wantLock        bool
		wantLockTimeout time.Duration
	}{
		"default values": {
			args:            nil,
			wantLock:        true,
			wantLockTimeout: 0,
		},
		"lock set to false": {
			args:            []string{"-lock=false"},
			wantLock:        false,
			wantLockTimeout: 0,
		},
		"lock set to true explicitly": {
			args:            []string{"-lock=true"},
			wantLock:        true,
			wantLockTimeout: 0,
		},
		"lock-timeout set": {
			args:            []string{"-lock-timeout=10s"},
			wantLock:        true,
			wantLockTimeout: 10 * time.Second,
		},
		"lock-timeout set in minutes": {
			args:            []string{"-lock-timeout=5m"},
			wantLock:        true,
			wantLockTimeout: 5 * time.Minute,
		},
		"both flags set": {
			args:            []string{"-lock=false", "-lock-timeout=30s"},
			wantLock:        false,
			wantLockTimeout: 30 * time.Second,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			backend := &Backend{}
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			backend.AddStateFlags(fs)

			if err := fs.Parse(tc.args); err != nil {
				t.Fatalf("unexpected error parsing flags: %v", err)
			}

			if got := backend.StateLock; got != tc.wantLock {
				t.Errorf("StateLock = %v, want %v", got, tc.wantLock)
			}

			if got := backend.StateLockTimeout; got != tc.wantLockTimeout {
				t.Errorf("StateLockTimeout = %v, want %v", got, tc.wantLockTimeout)
			}
		})
	}
}

func TestBackend_AddMigrationFlags(t *testing.T) {
	testCases := map[string]struct {
		args              []string
		wantForceInitCopy bool
		wantReconfigure   bool
		wantMigrateState  bool
	}{
		"default values": {
			args:              nil,
			wantForceInitCopy: false,
			wantReconfigure:   false,
			wantMigrateState:  false,
		},
		"force-copy set": {
			args:              []string{"-force-copy"},
			wantForceInitCopy: true,
			wantReconfigure:   false,
			wantMigrateState:  false,
		},
		"force-copy explicitly true": {
			args:              []string{"-force-copy=true"},
			wantForceInitCopy: true,
			wantReconfigure:   false,
			wantMigrateState:  false,
		},
		"force-copy explicitly false": {
			args:              []string{"-force-copy=false"},
			wantForceInitCopy: false,
			wantReconfigure:   false,
			wantMigrateState:  false,
		},
		"reconfigure set": {
			args:              []string{"-reconfigure"},
			wantForceInitCopy: false,
			wantReconfigure:   true,
			wantMigrateState:  false,
		},
		"reconfigure explicitly true": {
			args:              []string{"-reconfigure=true"},
			wantForceInitCopy: false,
			wantReconfigure:   true,
			wantMigrateState:  false,
		},
		"reconfigure explicitly false": {
			args:              []string{"-reconfigure=false"},
			wantForceInitCopy: false,
			wantReconfigure:   false,
			wantMigrateState:  false,
		},
		"migrate-state set": {
			args:              []string{"-migrate-state"},
			wantForceInitCopy: false,
			wantReconfigure:   false,
			wantMigrateState:  true,
		},
		"migrate-state explicitly true": {
			args:              []string{"-migrate-state=true"},
			wantForceInitCopy: false,
			wantReconfigure:   false,
			wantMigrateState:  true,
		},
		"migrate-state explicitly false": {
			args:              []string{"-migrate-state=false"},
			wantForceInitCopy: false,
			wantReconfigure:   false,
			wantMigrateState:  false,
		},
		"force-copy and migrate-state set": {
			args:              []string{"-force-copy", "-migrate-state"},
			wantForceInitCopy: true,
			wantReconfigure:   false,
			wantMigrateState:  true,
		},
		"all flags set": {
			args:              []string{"-force-copy", "-reconfigure", "-migrate-state"},
			wantForceInitCopy: true,
			wantReconfigure:   true,
			wantMigrateState:  true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			backend := &Backend{}
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			backend.AddMigrationFlags(fs)

			if err := fs.Parse(tc.args); err != nil {
				t.Fatalf("unexpected error parsing flags: %v", err)
			}

			if got := backend.ForceInitCopy; got != tc.wantForceInitCopy {
				t.Errorf("ForceInitCopy = %v, want %v", got, tc.wantForceInitCopy)
			}

			if got := backend.Reconfigure; got != tc.wantReconfigure {
				t.Errorf("Reconfigure = %v, want %v", got, tc.wantReconfigure)
			}

			if got := backend.MigrateState; got != tc.wantMigrateState {
				t.Errorf("MigrateState = %v, want %v", got, tc.wantMigrateState)
			}
		})
	}
}

func TestBackend_migrationFlagsCheck(t *testing.T) {
	testCases := map[string]struct {
		backend          Backend
		wantDiags        bool
		wantMigrateState bool
		diagsSummary     string
	}{
		"no flags set": {
			backend: Backend{
				ForceInitCopy: false,
				Reconfigure:   false,
				MigrateState:  false,
			},
			wantDiags:        false,
			wantMigrateState: false,
		},
		"only migrate-state set": {
			backend: Backend{
				ForceInitCopy: false,
				Reconfigure:   false,
				MigrateState:  true,
			},
			wantDiags:        false,
			wantMigrateState: true,
		},
		"only reconfigure set": {
			backend: Backend{
				ForceInitCopy: false,
				Reconfigure:   true,
				MigrateState:  false,
			},
			wantDiags:        false,
			wantMigrateState: false,
		},
		"only force-copy set": {
			backend: Backend{
				ForceInitCopy: true,
				Reconfigure:   false,
				MigrateState:  false,
			},
			wantDiags:        false,
			wantMigrateState: true, // force-copy implies migrate-state
		},
		"force-copy and migrate-state set": {
			backend: Backend{
				ForceInitCopy: true,
				Reconfigure:   false,
				MigrateState:  true,
			},
			wantDiags:        false,
			wantMigrateState: true,
		},
		"migrate-state and reconfigure set (mutually exclusive)": {
			backend: Backend{
				ForceInitCopy: false,
				Reconfigure:   true,
				MigrateState:  true,
			},
			wantDiags:        true,
			wantMigrateState: true,
			diagsSummary:     "Wrong combination of options",
		},
		"all flags set (error due to reconfigure + migrate-state)": {
			backend: Backend{
				ForceInitCopy: true,
				Reconfigure:   true,
				MigrateState:  true,
			},
			wantDiags:        true,
			wantMigrateState: true,
			diagsSummary:     "Wrong combination of options",
		},
		"force-copy and reconfigure set (no error - check happens before force-copy sets migrate-state)": {
			backend: Backend{
				ForceInitCopy: true,
				Reconfigure:   true,
				MigrateState:  false,
			},
			wantDiags:        false, // No error because MigrateState is false when check happens
			wantMigrateState: true,  // force-copy sets this to true after the check
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			backend := tc.backend
			diags := backend.migrationFlagsCheck()

			if tc.wantDiags && len(diags) == 0 {
				t.Fatal("expected diagnostics but got none")
			}

			if !tc.wantDiags && len(diags) != 0 {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}

			if tc.wantDiags && len(diags) == 1 {
				diag := diags[0]
				if diag.Description().Summary != tc.diagsSummary {
					t.Errorf("diagnostic summary = %q, want %q",
						diag.Description().Summary, tc.diagsSummary)
				}

				// Verify it's an error
				if diag.Severity() != tfdiags.Error {
					t.Errorf("diagnostic severity = %v, want %v",
						diag.Severity(), tfdiags.Error)
				}
			}

			// Verify that MigrateState is set correctly
			if got := backend.MigrateState; got != tc.wantMigrateState {
				t.Errorf("MigrateState after check = %v, want %v", got, tc.wantMigrateState)
			}
		})
	}
}

func TestBackend_AllFlags(t *testing.T) {
	testCases := map[string]struct {
		args []string
		want Backend
	}{
		"all defaults": {
			args: nil,
			want: Backend{
				IgnoreRemoteVersion: false,
				StateLock:           true,
				StateLockTimeout:    0,
				ForceInitCopy:       false,
				Reconfigure:         false,
				MigrateState:        false,
			},
		},
		"all flags set": {
			args: []string{
				"-ignore-remote-version",
				"-lock=false",
				"-lock-timeout=1m",
				"-force-copy",
				"-reconfigure",
				"-migrate-state",
			},
			want: Backend{
				IgnoreRemoteVersion: true,
				StateLock:           false,
				StateLockTimeout:    time.Minute,
				ForceInitCopy:       true,
				Reconfigure:         true,
				MigrateState:        true,
			},
		},
		"mixed flags": {
			args: []string{
				"-ignore-remote-version=true",
				"-lock-timeout=30s",
				"-migrate-state",
			},
			want: Backend{
				IgnoreRemoteVersion: true,
				StateLock:           true,
				StateLockTimeout:    30 * time.Second,
				ForceInitCopy:       false,
				Reconfigure:         false,
				MigrateState:        true,
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			backend := &Backend{}
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			backend.AddIgnoreRemoteVersionFlag(fs)
			backend.AddStateFlags(fs)
			backend.AddMigrationFlags(fs)

			if err := fs.Parse(tc.args); err != nil {
				t.Fatalf("unexpected error parsing flags: %v", err)
			}

			if diff := cmp.Diff(tc.want, *backend); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}
