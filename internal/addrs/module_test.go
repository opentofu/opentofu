// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

func TestModuleEqual_true(t *testing.T) {
	modules := []Module{
		RootModule,
		{"a"},
		{"a", "b"},
		{"a", "b", "c"},
	}
	for _, m := range modules {
		t.Run(m.String(), func(t *testing.T) {
			if !m.Equal(m) {
				t.Fatalf("expected %#v to be equal to itself", m)
			}
		})
	}
}

func TestModuleEqual_false(t *testing.T) {
	testCases := []struct {
		left  Module
		right Module
	}{
		{
			RootModule,
			Module{"a"},
		},
		{
			Module{"a"},
			Module{"b"},
		},
		{
			Module{"a"},
			Module{"a", "a"},
		},
		{
			Module{"a", "b"},
			Module{"a", "B"},
		},
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s = %s", tc.left, tc.right), func(t *testing.T) {
			if tc.left.Equal(tc.right) {
				t.Fatalf("expected %#v not to be equal to %#v", tc.left, tc.right)
			}

			if tc.right.Equal(tc.left) {
				t.Fatalf("expected %#v not to be equal to %#v", tc.right, tc.left)
			}
		})
	}
}

func TestModuleString(t *testing.T) {
	testCases := map[string]Module{
		"": {},
		"module.alpha": {
			"alpha",
		},
		"module.alpha.module.beta": {
			"alpha",
			"beta",
		},
		"module.alpha.module.beta.module.charlie": {
			"alpha",
			"beta",
			"charlie",
		},
	}
	for str, module := range testCases {
		t.Run(str, func(t *testing.T) {
			if got, want := module.String(), str; got != want {
				t.Errorf("wrong result: got %q, want %q", got, want)
			}
		})
	}
}

func BenchmarkModuleStringShort(b *testing.B) {
	module := Module{"a", "b"}
	for n := 0; n < b.N; n++ {
		module.String()
	}
}

func BenchmarkModuleStringLong(b *testing.B) {
	module := Module{"southamerica-brazil-region", "user-regional-desktop", "user-name"}
	for n := 0; n < b.N; n++ {
		module.String()
	}
}

func TestParseModule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Input      string
		WantModule Module
		WantErr    string
	}{
		{
			Input:      "module.a",
			WantModule: []string{"a"},
		},
		{
			Input:      "module.a.module.b",
			WantModule: []string{"a", "b"},
		},
		{
			Input:   "module.a.module.b.c.d",
			WantErr: "Module address expected: It's not allowed to reference anything other than module here.",
		},
		{
			Input:   "a.b.c.d",
			WantErr: "Module address expected: It's not allowed to reference anything other than module here.",
		},
		{
			Input:   "module",
			WantErr: `Invalid address operator: Prefix "module." must be followed by a module name.`,
		},
		{
			Input:   "module.a[0]",
			WantErr: `Module instance address with keys is not allowed: Module address cannot be a module instance (e.g. "module.a[0]"), it must be a module instead (e.g. "module.a").`,
		},
		{
			Input:   `module.a["k"]`,
			WantErr: `Module instance address with keys is not allowed: Module address cannot be a module instance (e.g. "module.a[0]"), it must be a module instead (e.g. "module.a").`,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.Input, func(t *testing.T) {
			t.Parallel()

			traversal, hclDiags := hclsyntax.ParseTraversalAbs([]byte(test.Input), "", hcl.InitialPos)
			if hclDiags.HasErrors() {
				t.Fatalf("Bug in tests: %s", hclDiags.Error())
			}

			mod, diags := ParseModule(traversal)

			switch {
			case test.WantErr != "":
				if !diags.HasErrors() {
					t.Fatalf("Unexpected success, wanted error: %s", test.WantErr)
				}

				gotErr := diags.Err().Error()
				if gotErr != test.WantErr {
					t.Fatalf("Mismatched error\nGot:  %s\nWant: %s", gotErr, test.WantErr)
				}
			default:
				if diags.HasErrors() {
					t.Fatalf("Unexpected error: %s", diags.Err().Error())
				}
				if diff := cmp.Diff(test.WantModule, mod); diff != "" {
					t.Fatalf("Mismatched result:\n%s", diff)
				}
			}
		})
	}
}
