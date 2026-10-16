// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseEvalScopeStr(t *testing.T) {
	tests := []struct {
		Input   string
		Want    EvalScope
		WantErr string
	}{
		{
			``,
			RootModuleInstance,
			``,
		},
		{
			`module.foo`,
			ModuleInstance{
				{Name: "foo"},
			},
			``,
		},
		{
			`module.foo[0]`,
			ModuleInstance{
				{Name: "foo", InstanceKey: IntKey(0)},
			},
			``,
		},
		{
			`module.foo["a"]`,
			ModuleInstance{
				{Name: "foo", InstanceKey: StringKey("a")},
			},
			``,
		},
		{
			`module.foo.module.bar`,
			ModuleInstance{
				{Name: "foo"},
				{Name: "bar"},
			},
			``,
		},
		{
			`aws_instance.foo`,
			nil,
			`Invalid evaluation scope address: Only an entire module instance can be used as an evaluation scope.`,
		},
		{
			`module.foo.aws_instance.foo`,
			nil,
			`Invalid evaluation scope address: Only an entire module instance can be used as an evaluation scope.`,
		},
		{
			`module.foo[*]`,
			nil,
			`Splat expressions ([*]) may not be used here.`,
		},
		{
			`module..foo`,
			nil,
			`Dot must be followed by attribute name.`,
		},
		{
			`$$$`,
			nil,
			`Invalid character: This character is not used within the language.`,
		},
	}

	for _, test := range tests {
		t.Run(test.Input, func(t *testing.T) {
			got, gotDiags := ParseEvalScopeStr(test.Input)

			if diff := cmp.Diff(test.Want, got); diff != "" {
				t.Error("wrong result\n" + diff)
			}
			if wantErr := test.WantErr; wantErr != "" {
				if !gotDiags.HasErrors() {
					t.Fatalf("unexpected success\nwant error: %s", wantErr)
				}
				if gotErr := gotDiags.Err().Error(); !strings.Contains(gotErr, wantErr) {
					t.Errorf("wrong error\ngot:\n%s\n\nwant substring: %s", gotErr, wantErr)
				}
			} else if gotDiags.HasErrors() {
				t.Fatalf("unexpected error\ngot:\n%s\n\nwant: %#v", gotDiags.Err().Error(), test.Want)
			}
		})
	}
}
