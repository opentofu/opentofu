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
	"github.com/opentofu/opentofu/internal/command/flags"
)

func TestParseInit_basicValidation(t *testing.T) {
	testCases := map[string]struct {
		args []string
		want *Init
	}{
		"defaults": {
			nil,
			initArgsWithDefaults(nil),
		},
		"upgrade flag": {
			[]string{"-upgrade"},
			initArgsWithDefaults(func(init *Init) {
				init.FlagUpgrade = true
			}),
		},
		"get flag disabled": {
			[]string{"-get=false"},
			initArgsWithDefaults(func(init *Init) {
				init.FlagGet = false
			}),
		},
		"from-module flag with value": {
			[]string{"-from-module=/path/to/module"},
			initArgsWithDefaults(func(init *Init) {
				init.FlagFromModule = "/path/to/module"
			}),
		},
		"lockfile readonly": {
			[]string{"-lockfile=readonly"},
			initArgsWithDefaults(func(init *Init) {
				init.FlagLockfile = "readonly"
			}),
		},
		"custom test-directory": {
			[]string{"-test-directory=integration"},
			initArgsWithDefaults(func(init *Init) {
				init.TestsDirectory = "integration"
			}),
		},
		"backend disabled": {
			[]string{"-backend=false"},
			initArgsWithDefaults(func(init *Init) {
				init.FlagBackend = false
				init.FlagCloud = false
				init.BackendFlagSet = true
			}),
		},
		"cloud disabled": {
			[]string{"-cloud=false"},
			initArgsWithDefaults(func(init *Init) {
				init.FlagBackend = false
				init.FlagCloud = false
				init.CloudFlagSet = true
			}),
		},
		"multiple flags combined": {
			[]string{"-upgrade", "-lockfile=readonly", "-get=false", "-from-module=/tmp/mod"},
			initArgsWithDefaults(func(init *Init) {
				init.FlagFromModule = "/tmp/mod"
				init.FlagLockfile = "readonly"
				init.FlagGet = false
				init.FlagUpgrade = true
			}),
		},
		"one plugin dir configured": {
			[]string{"-plugin-dir=/test1"},
			initArgsWithDefaults(func(init *Init) {
				_ = init.FlagPluginPath.Set("/test1")
			}),
		},
		"multiple plugin dirs configured": {
			[]string{"-plugin-dir=/test1", "-plugin-dir=/test2"},
			initArgsWithDefaults(func(init *Init) {
				_ = init.FlagPluginPath.Set("/test1")
				_ = init.FlagPluginPath.Set("/test2")
			}),
		},
	}

	cmpOpts := cmpopts.IgnoreUnexported(Vars{}, ViewOptions{})

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseInit(tc.args)
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

func TestParseInit_backendCloudErrors(t *testing.T) {
	testCases := map[string]struct {
		args        []string
		wantBackend bool
		wantCloud   bool
	}{
		"both explicitly set to true": {
			args:        []string{"-backend=true", "-cloud=true"},
			wantBackend: true,
			wantCloud:   true,
		},
		"both explicitly set to false": {
			args:        []string{"-backend=false", "-cloud=false"},
			wantBackend: false,
			wantCloud:   false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseInit(tc.args)
			defer closer()

			if len(diags) == 0 {
				t.Fatal("expected diagnostics but got none")
			}
			if got, want := diags.Err().Error(), "Wrong combination of options"; !strings.Contains(got, want) {
				t.Fatalf("wrong diags\n got: %s\nwant: %s", got, want)
			}
			if got, want := diags.Err().Error(), "mutually-exclusive"; !strings.Contains(got, want) {
				t.Fatalf("wrong diags\n got: %s\nwant: %s", got, want)
			}
			if got.FlagBackend != tc.wantBackend {
				t.Errorf("wrong FlagBackend. wanted %t but got %t", tc.wantBackend, got.FlagBackend)
			}
			if got.FlagCloud != tc.wantCloud {
				t.Errorf("wrong FlagCloud. wanted %t, want %t", tc.wantCloud, got.FlagCloud)
			}
		})
	}
}

func TestParseInit_backendCloudSynchronization(t *testing.T) {
	testCases := map[string]struct {
		args           []string
		wantBackend    bool
		wantCloud      bool
		wantBackendSet bool
		wantCloudSet   bool
	}{
		"backend=false only": {
			args:           []string{"-backend=false"},
			wantBackend:    false,
			wantCloud:      false,
			wantBackendSet: true,
			wantCloudSet:   false,
		},
		"backend=true only": {
			args:           []string{"-backend=true"},
			wantBackend:    true,
			wantCloud:      true,
			wantBackendSet: true,
			wantCloudSet:   false,
		},
		"cloud=false only": {
			args:           []string{"-cloud=false"},
			wantBackend:    false,
			wantCloud:      false,
			wantBackendSet: false,
			wantCloudSet:   true,
		},
		"cloud=true only": {
			args:           []string{"-cloud=true"},
			wantBackend:    true,
			wantCloud:      true,
			wantBackendSet: false,
			wantCloudSet:   true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseInit(tc.args)
			defer closer()

			if len(diags) > 0 {
				t.Fatalf("unexpected diags: %v", diags)
			}
			if got.FlagBackend != tc.wantBackend {
				t.Errorf("wrong FlagBackend. wanted %t but got %t", tc.wantBackend, got.FlagBackend)
			}
			if got.FlagCloud != tc.wantCloud {
				t.Errorf("wrong FlagCloud. wanted %t but got want %t", tc.wantCloud, got.FlagCloud)
			}
			if got.BackendFlagSet != tc.wantBackendSet {
				t.Errorf("wrong BackendFlagSet. wanted %t but got %t", tc.wantBackendSet, got.BackendFlagSet)
			}
			if got.CloudFlagSet != tc.wantCloudSet {
				t.Errorf("wrong CloudFlagSet. wanted %t but got %t", tc.wantCloudSet, got.CloudFlagSet)
			}
			if got.FlagBackend != got.FlagCloud {
				t.Errorf("wrong FlagBackend. expected to be in sync with FlagCloud, instead got FlagBackend=%t and FlagCloud=%t", got.FlagCloud, got.FlagBackend)
			}
		})
	}
}

func TestParseInit_backendFlags(t *testing.T) {
	testCases := map[string]struct {
		args        []string
		wantBackend Backend
	}{
		"ignore-remote-version": {
			args: []string{"-ignore-remote-version"},
			wantBackend: backendWithDefaults(func(backend *Backend) {
				backend.IgnoreRemoteVersion = true
			}),
		},
		"lock disabled": {
			args: []string{"-lock=false"},
			wantBackend: backendWithDefaults(func(backend *Backend) {
				backend.StateLock = false
			}),
		},
		"lock-timeout": {
			args: []string{"-lock-timeout=30s"},
			wantBackend: backendWithDefaults(func(backend *Backend) {
				backend.StateLockTimeout = 30 * time.Second
			}),
		},
		"migrate-state": {
			args: []string{"-migrate-state"},
			wantBackend: backendWithDefaults(func(backend *Backend) {
				backend.MigrateState = true
			}),
		},
		"reconfigure": {
			args: []string{"-reconfigure"},
			wantBackend: backendWithDefaults(func(backend *Backend) {
				backend.Reconfigure = true
			}),
		},
		"force-copy": {
			args: []string{"-force-copy"},
			wantBackend: backendWithDefaults(func(backend *Backend) {
				backend.ForceInitCopy = true
				backend.MigrateState = true
			}),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseInit(tc.args)
			defer closer()

			if len(diags) > 0 {
				t.Fatalf("unexpected diags: %v", diags)
			}
			if diff := cmp.Diff(tc.wantBackend, *got.Backend); diff != "" {
				t.Errorf("unexpected Backend result (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseInit_migrationFlagsValidation(t *testing.T) {
	testCases := map[string]struct {
		args             []string
		wantMigrateState bool
		wantDiagDetails  string
	}{
		"force-copy implies migrate-state": {
			args:             []string{"-force-copy"},
			wantMigrateState: true,
		},
		"migrate-state set": {
			args:             []string{"-migrate-state"},
			wantMigrateState: true,
		},
		"reconfigure set": {
			args:             []string{"-reconfigure"},
			wantMigrateState: false,
		},
		"migration-state and reconfigure set": {
			args:             []string{"-reconfigure", "-migrate-state"},
			wantMigrateState: true,
			wantDiagDetails:  "The -migrate-state and -reconfigure options are mutually-exclusive",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseInit(tc.args)
			defer closer()

			switch {
			case len(diags) == 0 && len(tc.wantDiagDetails) > 0:
				t.Fatal("expected to have a diagnostic but got none")
			case len(diags) > 0 && len(tc.wantDiagDetails) == 0:
				t.Fatalf("expected no diagnostic but got: %s", diags)
			case len(diags) > 0 && len(tc.wantDiagDetails) > 0:
				diag := diags[0]
				if diag.Description().Detail != tc.wantDiagDetails {
					t.Fatalf("Diagnostic Detail = %q; want %q", diag.Description().Detail, tc.wantDiagDetails)
				}
			}

			if got.Backend.MigrateState != tc.wantMigrateState {
				t.Errorf("Backend.MigrateState = %v, want %v", got.Backend.MigrateState, tc.wantMigrateState)
			}
		})
	}
}

func TestParseInit_backendConfig(t *testing.T) {
	testCases := map[string]struct {
		args       []string
		wantCount  int
		wantValues []string
	}{
		"no backend config": {
			args:       nil,
			wantCount:  0,
			wantValues: nil,
		},
		"single backend config kv": {
			args:       []string{"-backend-config=key=value"},
			wantCount:  1,
			wantValues: []string{"key=value"},
		},
		"backend config file": {
			args:       []string{"-backend-config=/path/to/config.hcl"},
			wantCount:  1,
			wantValues: []string{"/path/to/config.hcl"},
		},
		"multiple backend configs": {
			args:       []string{"-backend-config=k1=v1", "-backend-config=k2=v2", "-backend-config=/path/config.hcl"},
			wantCount:  3,
			wantValues: []string{"k1=v1", "k2=v2", "/path/config.hcl"},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseInit(tc.args)
			defer closer()

			if len(diags) > 0 {
				t.Fatalf("unexpected diags: %v", diags)
			}
			if tc.wantCount == 0 {
				if !got.FlagConfigExtra.Empty() {
					t.Error("FlagConfigExtra should be empty")
				}
				return
			}
			if got.FlagConfigExtra.Empty() {
				t.Error("FlagConfigExtra should not be empty")
			}
			items := got.FlagConfigExtra.AllItems()
			if len(items) != tc.wantCount {
				t.Errorf("len(FlagConfigExtra.AllItems()) = %d, want %d", len(items), tc.wantCount)
			}
			for i, want := range tc.wantValues {
				if items[i].Value != want {
					t.Errorf("FlagConfigExtra.AllItems()[%d].Value = %q, want %q", i, items[i].Value, want)
				}
				if items[i].Name != "-backend-config" {
					t.Errorf("FlagConfigExtra.AllItems()[%d].Name = %q, want %q", i, items[i].Name, "-backend-config")
				}
			}
		})
	}
}

func TestParseInit_vars(t *testing.T) {
	testCases := map[string]struct {
		args      []string
		wantCount int
		wantEmpty bool
	}{
		"no vars": {
			args:      nil,
			wantCount: 0,
			wantEmpty: true,
		},
		"single var": {
			args:      []string{"-var", "foo=bar"},
			wantCount: 1,
			wantEmpty: false,
		},
		"single var-file": {
			args:      []string{"-var-file", "terraform.tfvars"},
			wantCount: 1,
			wantEmpty: false,
		},
		"multiple vars mixed": {
			args:      []string{"-var", "a=1", "-var-file", "f.tfvars", "-var", "b=2"},
			wantCount: 3,
			wantEmpty: false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseInit(tc.args)
			defer closer()

			if len(diags) > 0 {
				t.Fatalf("unexpected diags: %v", diags)
			}
			if got.Vars.Empty() != tc.wantEmpty {
				t.Errorf("Vars.Empty() = %v, want %v", got.Vars.Empty(), tc.wantEmpty)
			}
			if len(got.Vars.All()) != tc.wantCount {
				t.Errorf("len(Vars.All()) = %d, want %d", len(got.Vars.All()), tc.wantCount)
			}
		})
	}
}

func TestParseInit_tooManyArguments(t *testing.T) {
	testCases := map[string]struct {
		args []string
	}{
		"one positional argument": {
			args: []string{"mydir"},
		},
		"multiple positional arguments": {
			args: []string{"dir1", "dir2"},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			_, closer, diags := ParseInit(tc.args)
			defer closer()

			if len(diags) == 0 {
				t.Fatal("expected diagnostics but got none")
			}
			if got, want := diags.Err().Error(), "Unexpected argument"; !strings.Contains(got, want) {
				t.Fatalf("wrong diags\n got: %s\nwant: %s", got, want)
			}
			if got, want := diags.Err().Error(), "Too many command line arguments"; !strings.Contains(got, want) {
				t.Fatalf("wrong diags\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

func initArgsWithDefaults(mutate func(init *Init)) *Init {
	ret := &Init{
		FlagFromModule:  "",
		FlagLockfile:    "",
		TestsDirectory:  "tests",
		FlagGet:         true,
		FlagUpgrade:     false,
		FlagPluginPath:  nil,
		FlagConfigExtra: flags.NewRawFlags("-backend-config"),
		FlagBackend:     true,
		FlagCloud:       true,
		BackendFlagSet:  false,
		CloudFlagSet:    false,
		ViewOptions: ViewOptions{
			ViewType:     ViewHuman,
			InputEnabled: true,
		},
		Vars:    &Vars{},
		Backend: &Backend{StateLock: true},
	}
	if mutate != nil {
		mutate(ret)
	}
	return ret
}

func backendWithDefaults(mutate func(backend *Backend)) Backend {
	ret := Backend{
		IgnoreRemoteVersion: false,
		StateLock:           true,
		StateLockTimeout:    0,
		ForceInitCopy:       false,
		Reconfigure:         false,
		MigrateState:        false,
	}
	if mutate != nil {
		mutate(&ret)
	}
	return ret
}
