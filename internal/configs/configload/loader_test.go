// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configload

import (
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/zclconf/go-cty/cty"
)

func assertNoDiagnostics(t *testing.T, diags hcl.Diagnostics) bool {
	t.Helper()
	return assertDiagnosticCount(t, diags, 0)
}

func assertDiagnosticCount(t *testing.T, diags hcl.Diagnostics, want int) bool {
	t.Helper()
	if len(diags) != want {
		t.Errorf("wrong number of diagnostics %d; want %d", len(diags), want)
		for _, diag := range diags {
			t.Logf("- %s", diag)
		}
		return true
	}
	return false
}
func assertResultCtyEqual(t *testing.T, got, want cty.Value) bool {
	t.Helper()
	if !got.RawEquals(want) {
		t.Errorf("wrong result\ngot:  %#v\nwant: %#v", got, want)
		return true
	}
	return false
}

func TestIsRemoteModuleSourceAddr(t *testing.T) {
	fixtureDir := filepath.Clean("testdata/already-installed")
	loader, err := NewLoader(&Config{
		ModulesDir: filepath.Join(fixtureDir, ".terraform/modules"),
	})
	if err != nil {
		t.Fatalf("unexpected error from NewLoader: %s", err)
	}

	_, diags := loader.LoadConfig(t.Context(), fixtureDir, configs.RootModuleCallForTesting())
	assertNoDiagnostics(t, diags)

	cases := []struct {
		path     addrs.Module
		expected bool
	}{
		{addrs.Module{}, false},
		{addrs.Module{"child_a"}, true},            // remote
		{addrs.Module{"child_b"}, true},            // remote
		{addrs.Module{"child_a", "child_c"}, true}, // remote -> local
		{addrs.Module{"child_b", "child_d"}, true}, // remote -> remote
	}

	for _, tc := range cases {
		t.Run(tc.path.String(), func(t *testing.T) {
			actual := loader.IsRemoteModuleSource(tc.path)
			if actual != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, actual)
			}
		})
	}

}

func TestModuleSourceAddrs(t *testing.T) {
	fixtureDir := filepath.Clean("testdata/already-installed")
	loader, err := NewLoader(&Config{
		ModulesDir: filepath.Join(fixtureDir, ".terraform/modules"),
	})
	if err != nil {
		t.Fatalf("unexpected error from NewLoader: %s", err)
	}

	_, diags := loader.LoadConfig(t.Context(), fixtureDir, configs.RootModuleCallForTesting())
	assertNoDiagnostics(t, diags)

	parseModuleSource := func(t *testing.T, source string) addrs.ModuleSource {
		s, err := addrs.ParseModuleSource(source)
		if err != nil {
			t.Fatalf("failed to parse module source %q: %s", source, err)
		}
		return s
	}

	cases := []struct {
		path     addrs.Module
		expected addrs.ModuleSource
	}{
		{addrs.Module{}, nil},
		{addrs.Module{"child_a"}, parseModuleSource(t, "example.com/foo/bar_a/baz")},
		{addrs.Module{"child_b"}, parseModuleSource(t, "example.com/foo/bar_b/baz")},
		{addrs.Module{"child_a", "child_c"}, parseModuleSource(t, "./child_c")},
		{addrs.Module{"child_b", "child_d"}, parseModuleSource(t, "example.com/foo/bar_d/baz")},
	}

	for _, tc := range cases {
		t.Run(tc.path.String(), func(t *testing.T) {
			actual := loader.ModuleSourceAddrs(tc.path)
			if actual != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, actual)
			}
		})
	}

}
