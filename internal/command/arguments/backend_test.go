// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"testing"

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
			var cli CommandLine
			backend := BindBackend(&cli)
			if _, diags := cli.parseWithHooks("test", tc.args); diags.HasErrors() {
				t.Fatalf("unexpected error parsing flags: %v", diags.Err().Error())
			}

			if got := backend.IgnoreRemoteVersion; got != tc.want {
				t.Errorf("IgnoreRemoteVersion = %v, want %v", got, tc.want)
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
		wantDiags         bool
		diagsSummary      string
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
			wantMigrateState:  true,
		},
		"force-copy explicitly true": {
			args:              []string{"-force-copy=true"},
			wantForceInitCopy: true,
			wantReconfigure:   false,
			wantMigrateState:  true,
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
			wantDiags:         true,
			diagsSummary:      "Wrong combination of options",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			var cli CommandLine
			backend := BindBackendWithMigration(&cli)
			_, diags := cli.parseWithHooks("test", tc.args)

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

func TestBackend_AllFlags(t *testing.T) {
	testCases := map[string]struct {
		args      []string
		want      Backend
		wantDiags bool
	}{
		"all defaults": {
			args: nil,
			want: Backend{
				IgnoreRemoteVersion: false,
				ForceInitCopy:       false,
				Reconfigure:         false,
				MigrateState:        false,
			},
		},
		"all flags set": {
			args: []string{
				"-ignore-remote-version",
				"-force-copy",
				"-reconfigure",
				"-migrate-state",
			},
			want: Backend{
				IgnoreRemoteVersion: true,
				ForceInitCopy:       true,
				Reconfigure:         true,
				MigrateState:        true,
			},
			wantDiags: true,
		},
		"mixed flags": {
			args: []string{
				"-ignore-remote-version=true",
				"-migrate-state",
			},
			want: Backend{
				IgnoreRemoteVersion: true,
				ForceInitCopy:       false,
				Reconfigure:         false,
				MigrateState:        true,
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			var cli CommandLine
			backend := BindBackendWithMigration(&cli)

			if _, diags := cli.parseWithHooks("test", tc.args); diags.HasErrors() != tc.wantDiags {
				t.Fatalf("unexpected error parsing flags: %v", diags.Err().Error())
			}

			if diff := cmp.Diff(tc.want, *backend); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}
