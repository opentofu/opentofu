// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/tofu"
)

func TestRefreshFlagValue_Set(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    tofu.RefreshMode
		wantErr bool
	}{
		{"true", "true", tofu.RefreshAll, false},
		{"TRUE", "TRUE", tofu.RefreshAll, false},
		{"false", "false", tofu.RefreshNone, false},
		{"FALSE", "FALSE", tofu.RefreshNone, false},
		{"config", "config", tofu.RefreshConfig, false},
		{"CONFIG", "CONFIG", tofu.RefreshConfig, false},
		{"Config", "Config", tofu.RefreshConfig, false},
		{"empty string", "", tofu.RefreshAll, false},
		{"invalid", "invalid", 0, true},
		{"maybe", "maybe", 0, true},
		{"1", "1", 0, true},
		{"0", "0", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r RefreshFlagValue
			err := r.Set(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Set(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && r.Mode != tt.want {
				t.Errorf("Set(%q) mode = %v, want %v", tt.input, r.Mode, tt.want)
			}
		})
	}
}

func TestRefreshFlagValue_String(t *testing.T) {
	tests := []struct {
		mode tofu.RefreshMode
		want string
	}{
		{tofu.RefreshAll, "true"},
		{tofu.RefreshNone, "false"},
		{tofu.RefreshConfig, "config"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			r := &RefreshFlagValue{Mode: tt.mode}
			got := r.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRefreshFlagValue_RefreshEnabled(t *testing.T) {
	tests := []struct {
		mode tofu.RefreshMode
		want bool
	}{
		{tofu.RefreshAll, true},
		{tofu.RefreshNone, false},
		{tofu.RefreshConfig, true},
	}

	for _, tt := range tests {
		t.Run(tt.mode.String(), func(t *testing.T) {
			r := &RefreshFlagValue{Mode: tt.mode}
			got := r.RefreshEnabled()
			if got != tt.want {
				t.Errorf("RefreshEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRefreshFlagValue_IsBoolFlag(t *testing.T) {
	r := &RefreshFlagValue{}
	if !r.IsBoolFlag() {
		t.Error("IsBoolFlag() should return true for backward compatibility")
	}
}

func TestRefreshFlagValue_Get(t *testing.T) {
	r := &RefreshFlagValue{Mode: tofu.RefreshConfig}
	got := r.Get()
	if got != tofu.RefreshConfig {
		t.Errorf("Get() = %v, want %v", got, tofu.RefreshConfig)
	}
}

func TestParsePlan_refreshConfig(t *testing.T) {
	testCases := map[string]struct {
		args    []string
		wantErr string
	}{
		"refresh=config": {
			args: []string{"-refresh=config"},
		},
		"refresh=config with refresh-only": {
			args:    []string{"-refresh=config", "-refresh-only"},
			wantErr: "Incompatible refresh options",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, _, diags := ParsePlan(tc.args)
			if tc.wantErr != "" {
				if !diags.HasErrors() {
					t.Fatalf("expected error containing %q, got none", tc.wantErr)
				}
				errStr := diags.Err().Error()
				if !strings.Contains(errStr, tc.wantErr) {
					t.Errorf("expected error containing %q, got: %s", tc.wantErr, errStr)
				}
			} else {
				if diags.HasErrors() {
					t.Fatalf("unexpected errors: %s", diags.Err())
				}
				if got.Operation.Refresh.Mode != tofu.RefreshConfig {
					t.Errorf("expected refresh mode RefreshConfig, got %v", got.Operation.Refresh.Mode)
				}
			}
		})
	}
}
