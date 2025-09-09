// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cliconfig

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestLoadConfig_registryProtocols(t *testing.T) {
	tests := map[string]struct {
		// The fixture names correspond to files under the "testdata" directory.
		fixture string
		env     map[string]string
		want    *RegistryProtocolsConfig
		wantErr string
	}{
		"none": {
			"registry-protocols-none",
			nil,
			&RegistryProtocolsConfig{
				// These are the default settings used when nothing overrides them.
				RetryCount:        1,
				RetryCountSet:     true,
				RequestTimeout:    10 * time.Second,
				RequestTimeoutSet: true,
			},
			``,
		},
		"both": {
			"registry-protocols-both",
			nil,
			&RegistryProtocolsConfig{
				RetryCount:        256,
				RetryCountSet:     true,
				RequestTimeout:    50 * time.Second,
				RequestTimeoutSet: true,
			},
			``,
		},
		"only-count": {
			"registry-protocols-only-count",
			nil,
			&RegistryProtocolsConfig{
				RetryCount:        256,
				RetryCountSet:     true,
				RequestTimeout:    10 * time.Second,
				RequestTimeoutSet: true,
			},
			``,
		},
		"only-timeout": {
			"registry-protocols-only-timeout",
			nil,
			&RegistryProtocolsConfig{
				RetryCount:        1,
				RetryCountSet:     true,
				RequestTimeout:    50 * time.Second,
				RequestTimeoutSet: true,
			},
			``,
		},
		"future": {
			"registry-protocols-future",
			nil,
			&RegistryProtocolsConfig{
				// These just the defaults again because this fixture only
				// includes a hypothetical future setting that our current
				// code doesn't recognize at all.
				RetryCount:        1,
				RetryCountSet:     true,
				RequestTimeout:    10 * time.Second,
				RequestTimeoutSet: true,
			},
			``,
		},
		"none-env-both": {
			"registry-protocols-none",
			map[string]string{
				"TF_REGISTRY_DISCOVERY_RETRY": "123",
				"TF_REGISTRY_CLIENT_TIMEOUT":  "456",
			},
			&RegistryProtocolsConfig{
				RetryCount:        123,
				RetryCountSet:     true,
				RequestTimeout:    456 * time.Second,
				RequestTimeoutSet: true,
			},
			``,
		},
		"none-env-retry": {
			"registry-protocols-none",
			map[string]string{
				"TF_REGISTRY_DISCOVERY_RETRY": "123",
			},
			&RegistryProtocolsConfig{
				RetryCount:        123,
				RetryCountSet:     true,
				RequestTimeout:    10 * time.Second,
				RequestTimeoutSet: true,
			},
			``,
		},
		"none-env-timeout": {
			"registry-protocols-none",
			map[string]string{
				"TF_REGISTRY_CLIENT_TIMEOUT": "456",
			},
			&RegistryProtocolsConfig{
				RetryCount:        1,
				RetryCountSet:     true,
				RequestTimeout:    456 * time.Second,
				RequestTimeoutSet: true,
			},
			``,
		},
		"both-env-both": {
			"registry-protocols-both",
			map[string]string{
				"TF_REGISTRY_DISCOVERY_RETRY": "123",
				"TF_REGISTRY_CLIENT_TIMEOUT":  "456",
			},
			&RegistryProtocolsConfig{
				RetryCount:        123,
				RetryCountSet:     true,
				RequestTimeout:    456 * time.Second,
				RequestTimeoutSet: true,
			},
			``,
		},
		"both-env-retry": {
			"registry-protocols-both",
			map[string]string{
				"TF_REGISTRY_DISCOVERY_RETRY": "123",
			},
			&RegistryProtocolsConfig{
				RetryCount:        123,
				RetryCountSet:     true,
				RequestTimeout:    50 * time.Second,
				RequestTimeoutSet: true,
			},
			``,
		},
		"both-env-timeout": {
			"registry-protocols-both",
			map[string]string{
				"TF_REGISTRY_CLIENT_TIMEOUT": "456",
			},
			&RegistryProtocolsConfig{
				RetryCount:        256,
				RetryCountSet:     true,
				RequestTimeout:    456 * time.Second,
				RequestTimeoutSet: true,
			},
			``,
		},
		"invalid-syntax": {
			"registry-protocols-invalid-syntax",
			nil,
			&RegistryProtocolsConfig{
				RetryCount:        1,
				RetryCountSet:     true,
				RequestTimeout:    10 * time.Second,
				RequestTimeoutSet: true,
			},
			`The registry_protocols item at 3:1 must have an open brace after the block type`,
		},
		"invalid-count": {
			"registry-protocols-invalid-count",
			nil,
			&RegistryProtocolsConfig{
				RetryCount:        1,
				RetryCountSet:     true,
				RequestTimeout:    10 * time.Second,
				RequestTimeoutSet: true,
			},
			`parsing "not a number": invalid syntax`,
		},
		"invalid-timeout": {
			"registry-protocols-invalid-timeout",
			nil,
			&RegistryProtocolsConfig{
				RetryCount:        1,
				RetryCountSet:     true,
				RequestTimeout:    10 * time.Second,
				RequestTimeoutSet: true,
			},
			`parsing "not a number": invalid syntax`,
		},
		"invalid-multi": {
			"registry-protocols-invalid-multi",
			nil,
			&RegistryProtocolsConfig{
				RetryCount:        1,
				RetryCountSet:     true,
				RequestTimeout:    10 * time.Second,
				RequestTimeoutSet: true,
			},
			`The registry protocol settings were already defined at 4:1`,
		},
		"invalid-syntax-env-both": {
			// The environment variable settings still work even when the
			// config file is invalid, because OpenTofu still makes a best
			// effort to proceed even when the CLI configuration has errors.
			"registry-protocols-invalid-syntax",
			map[string]string{
				"TF_REGISTRY_DISCOVERY_RETRY": "123",
				"TF_REGISTRY_CLIENT_TIMEOUT":  "456",
			},
			&RegistryProtocolsConfig{
				RetryCount:        123,
				RetryCountSet:     true,
				RequestTimeout:    456 * time.Second,
				RequestTimeoutSet: true,
			},
			`The registry_protocols item at 3:1 must have an open brace after the block type`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fixtureFile := filepath.Join("testdata", test.fixture)

			// We set the file to load using this environment variable because
			// otherwise we can't use the LoadConfig entrypoint, which we're
			// relying on to test inheritance of the default settings and
			// overriding settings from the environment variables. :(
			t.Setenv("TF_CLI_CONFIG_FILE", fixtureFile)
			for name, val := range test.env {
				t.Setenv(name, val)
			}
			// If either of the environment variables we're sensitive to are
			// not in the map then we'll explicitly set them to empty so
			// that we won't pick up stray values that might be in the real
			// environment whereever these tests are being run.
			if _, ok := test.env["TF_REGISTRY_DISCOVERY_RETRY"]; !ok {
				t.Setenv("TF_REGISTRY_DISCOVERY_RETRY", "")
			}
			if _, ok := test.env["TF_REGISTRY_CLIENT_TIMEOUT"]; !ok {
				t.Setenv("TF_REGISTRY_CLIENT_TIMEOUT", "")
			}

			gotConfig, diags := LoadConfig(t.Context())
			if diags.HasErrors() {
				errStr := diags.Err().Error()
				if test.wantErr == "" {
					t.Errorf("unexpected errors: %s", errStr)
				}
				if !strings.Contains(errStr, test.wantErr) {
					t.Errorf("missing expected error\nwant substring: %s\ngot: %s", test.wantErr, errStr)
				}
			} else if test.wantErr != "" {
				t.Errorf("unexpected success\nwant error with substring: %s", test.wantErr)
			}

			got := gotConfig.RegistryProtocols
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Error("unexpected result\n" + diff)
			}
		})
	}
}
