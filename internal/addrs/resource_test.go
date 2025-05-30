// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"fmt"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

func TestResourceEqual_true(t *testing.T) {
	resources := []Resource{
		{
			Mode: ManagedResourceMode,
			Type: "a",
			Name: "b",
		},
		{
			Mode: DataResourceMode,
			Type: "a",
			Name: "b",
		},
		{
			Mode: EphemeralResourceMode,
			Type: "a",
			Name: "b",
		},
	}
	for _, r := range resources {
		t.Run(r.String(), func(t *testing.T) {
			if !r.Equal(r) {
				t.Fatalf("expected %#v to be equal to itself", r)
			}
		})
	}
}

func TestResourceEqual_false(t *testing.T) {
	testCases := []struct {
		left  Resource
		right Resource
	}{
		{
			Resource{Mode: DataResourceMode, Type: "a", Name: "b"},
			Resource{Mode: ManagedResourceMode, Type: "a", Name: "b"},
		},
		{
			Resource{Mode: ManagedResourceMode, Type: "a", Name: "b"},
			Resource{Mode: ManagedResourceMode, Type: "b", Name: "b"},
		},
		{
			Resource{Mode: ManagedResourceMode, Type: "a", Name: "b"},
			Resource{Mode: ManagedResourceMode, Type: "a", Name: "c"},
		},
		{
			Resource{Mode: EphemeralResourceMode, Type: "a", Name: "b"},
			Resource{Mode: EphemeralResourceMode, Type: "a", Name: "c"},
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

func TestResourceInstanceEqual_true(t *testing.T) {
	resources := []ResourceInstance{
		{
			Resource: Resource{
				Mode: ManagedResourceMode,
				Type: "a",
				Name: "b",
			},
			Key: IntKey(0),
		},
		{
			Resource: Resource{
				Mode: DataResourceMode,
				Type: "a",
				Name: "b",
			},
			Key: StringKey("x"),
		},
		{
			Resource: Resource{
				Mode: EphemeralResourceMode,
				Type: "a",
				Name: "b",
			},
			Key: StringKey("x"),
		},
	}
	for _, r := range resources {
		t.Run(r.String(), func(t *testing.T) {
			if !r.Equal(r) {
				t.Fatalf("expected %#v to be equal to itself", r)
			}
		})
	}
}

func TestResourceInstanceEqual_false(t *testing.T) {
	testCases := []struct {
		left  ResourceInstance
		right ResourceInstance
	}{
		{
			ResourceInstance{
				Resource: Resource{Mode: DataResourceMode, Type: "a", Name: "b"},
				Key:      IntKey(0),
			},
			ResourceInstance{
				Resource: Resource{Mode: ManagedResourceMode, Type: "a", Name: "b"},
				Key:      IntKey(0),
			},
		},
		{
			ResourceInstance{
				Resource: Resource{Mode: EphemeralResourceMode, Type: "a", Name: "b"},
				Key:      IntKey(0),
			},
			ResourceInstance{
				Resource: Resource{Mode: ManagedResourceMode, Type: "a", Name: "b"},
				Key:      IntKey(0),
			},
		},
		{
			ResourceInstance{
				Resource: Resource{Mode: ManagedResourceMode, Type: "a", Name: "b"},
				Key:      IntKey(0),
			},
			ResourceInstance{
				Resource: Resource{Mode: ManagedResourceMode, Type: "b", Name: "b"},
				Key:      IntKey(0),
			},
		},
		{
			ResourceInstance{
				Resource: Resource{Mode: ManagedResourceMode, Type: "a", Name: "b"},
				Key:      IntKey(0),
			},
			ResourceInstance{
				Resource: Resource{Mode: ManagedResourceMode, Type: "a", Name: "c"},
				Key:      IntKey(0),
			},
		},
		{
			ResourceInstance{
				Resource: Resource{Mode: DataResourceMode, Type: "a", Name: "b"},
				Key:      IntKey(0),
			},
			ResourceInstance{
				Resource: Resource{Mode: DataResourceMode, Type: "a", Name: "b"},
				Key:      StringKey("0"),
			},
		},
		{
			ResourceInstance{
				Resource: Resource{Mode: EphemeralResourceMode, Type: "a", Name: "b"},
				Key:      IntKey(0),
			},
			ResourceInstance{
				Resource: Resource{Mode: EphemeralResourceMode, Type: "a", Name: "b"},
				Key:      StringKey("0"),
			},
		},
		{
			ResourceInstance{
				Resource: Resource{Mode: DataResourceMode, Type: "a", Name: "b"},
				Key:      IntKey(0),
			},
			ResourceInstance{
				Resource: Resource{Mode: EphemeralResourceMode, Type: "a", Name: "b"},
				Key:      IntKey(0),
			},
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

func TestAbsResourceInstanceEqual_true(t *testing.T) {
	managed := Resource{Mode: ManagedResourceMode, Type: "a", Name: "b"}
	data := Resource{Mode: DataResourceMode, Type: "a", Name: "b"}
	ephemeral := Resource{Mode: EphemeralResourceMode, Type: "a", Name: "b"}

	foo, diags := ParseModuleInstanceStr("module.foo")
	if len(diags) > 0 {
		t.Fatalf("unexpected diags: %s", diags.Err())
	}
	foobar, diags := ParseModuleInstanceStr("module.foo[1].module.bar")
	if len(diags) > 0 {
		t.Fatalf("unexpected diags: %s", diags.Err())
	}

	instances := []AbsResourceInstance{
		managed.Instance(IntKey(0)).Absolute(foo),
		data.Instance(IntKey(0)).Absolute(foo),
		ephemeral.Instance(IntKey(0)).Absolute(foo),
		managed.Instance(StringKey("a")).Absolute(foobar),
		ephemeral.Instance(IntKey(0)).Absolute(foobar),
	}
	for _, r := range instances {
		t.Run(r.String(), func(t *testing.T) {
			if !r.Equal(r) {
				t.Fatalf("expected %#v to be equal to itself", r)
			}
		})
	}
}

func TestAbsResourceInstanceEqual_false(t *testing.T) {
	managed := Resource{Mode: ManagedResourceMode, Type: "a", Name: "b"}
	data := Resource{Mode: DataResourceMode, Type: "a", Name: "b"}
	ephemeral := Resource{Mode: EphemeralResourceMode, Type: "a", Name: "b"}

	foo, diags := ParseModuleInstanceStr("module.foo")
	if len(diags) > 0 {
		t.Fatalf("unexpected diags: %s", diags.Err())
	}
	foobar, diags := ParseModuleInstanceStr("module.foo[1].module.bar")
	if len(diags) > 0 {
		t.Fatalf("unexpected diags: %s", diags.Err())
	}

	testCases := []struct {
		left  AbsResourceInstance
		right AbsResourceInstance
	}{
		{
			managed.Instance(IntKey(0)).Absolute(foo),
			data.Instance(IntKey(0)).Absolute(foo),
		},
		{
			managed.Instance(IntKey(0)).Absolute(foo),
			managed.Instance(IntKey(0)).Absolute(foobar),
		},
		{
			managed.Instance(IntKey(0)).Absolute(foo),
			managed.Instance(StringKey("0")).Absolute(foo),
		},
		{
			ephemeral.Instance(IntKey(0)).Absolute(foo),
			ephemeral.Instance(IntKey(0)).Absolute(foobar),
		},
		{
			ephemeral.Instance(StringKey("0")).Absolute(foo),
			ephemeral.Instance(IntKey(0)).Absolute(foo),
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

func TestAbsResourceUniqueKey(t *testing.T) {
	resourceAddr1 := Resource{
		Mode: ManagedResourceMode,
		Type: "a",
		Name: "b1",
	}.Absolute(RootModuleInstance)
	resourceAddr2 := Resource{
		Mode: ManagedResourceMode,
		Type: "a",
		Name: "b2",
	}.Absolute(RootModuleInstance)
	resourceAddr3 := Resource{
		Mode: ManagedResourceMode,
		Type: "a",
		Name: "in_module",
	}.Absolute(RootModuleInstance.Child("boop", NoKey))

	tests := []struct {
		Receiver  AbsResource
		Other     UniqueKeyer
		WantEqual bool
	}{
		{
			resourceAddr1,
			resourceAddr1,
			true,
		},
		{
			resourceAddr1,
			resourceAddr2,
			false,
		},
		{
			resourceAddr1,
			resourceAddr3,
			false,
		},
		{
			resourceAddr3,
			resourceAddr3,
			true,
		},
		{
			resourceAddr1,
			resourceAddr1.Instance(NoKey),
			false, // no-key instance key is distinct from its resource even though they have the same String result
		},
		{
			resourceAddr1,
			resourceAddr1.Instance(IntKey(1)),
			false,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s matches %T %s?", test.Receiver, test.Other, test.Other), func(t *testing.T) {
			rKey := test.Receiver.UniqueKey()
			oKey := test.Other.UniqueKey()

			gotEqual := rKey == oKey
			if gotEqual != test.WantEqual {
				t.Errorf(
					"wrong result\nreceiver: %s\nother:    %s (%T)\ngot:  %t\nwant: %t",
					test.Receiver, test.Other, test.Other,
					gotEqual, test.WantEqual,
				)
			}
		})
	}
}

func TestConfigResourceEqual_true(t *testing.T) {
	resources := []ConfigResource{
		{
			Resource: Resource{Mode: ManagedResourceMode, Type: "a", Name: "b"},
			Module:   RootModule,
		},
		{
			Resource: Resource{Mode: DataResourceMode, Type: "a", Name: "b"},
			Module:   RootModule,
		},
		{
			Resource: Resource{Mode: EphemeralResourceMode, Type: "a", Name: "b"},
			Module:   RootModule,
		},
		{
			Resource: Resource{Mode: ManagedResourceMode, Type: "a", Name: "b"},
			Module:   Module{"foo"},
		},
		{
			Resource: Resource{Mode: DataResourceMode, Type: "a", Name: "b"},
			Module:   Module{"foo"},
		},
		{
			Resource: Resource{Mode: EphemeralResourceMode, Type: "a", Name: "b"},
			Module:   Module{"foo"},
		},
	}
	for _, r := range resources {
		t.Run(r.String(), func(t *testing.T) {
			if !r.Equal(r) {
				t.Fatalf("expected %#v to be equal to itself", r)
			}
		})
	}
}

func TestConfigResourceEqual_false(t *testing.T) {
	managed := Resource{Mode: ManagedResourceMode, Type: "a", Name: "b"}
	data := Resource{Mode: DataResourceMode, Type: "a", Name: "b"}
	ephemeral := Resource{Mode: EphemeralResourceMode, Type: "a", Name: "b"}

	foo := Module{"foo"}
	foobar := Module{"foobar"}
	testCases := []struct {
		left  ConfigResource
		right ConfigResource
	}{
		{
			ConfigResource{Resource: managed, Module: foo},
			ConfigResource{Resource: data, Module: foo},
		},
		{
			ConfigResource{Resource: managed, Module: foo},
			ConfigResource{Resource: ephemeral, Module: foo},
		},
		{
			ConfigResource{Resource: data, Module: foo},
			ConfigResource{Resource: ephemeral, Module: foo},
		},
		{
			ConfigResource{Resource: managed, Module: foo},
			ConfigResource{Resource: managed, Module: foobar},
		},
		{
			ConfigResource{Resource: ephemeral, Module: foo},
			ConfigResource{Resource: ephemeral, Module: foobar},
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

func TestParseConfigResource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Input              string
		WantConfigResource ConfigResource
		WantErr            string
	}{
		{
			Input: "a.b",
			WantConfigResource: ConfigResource{
				Module: RootModule,
				Resource: Resource{
					Mode: ManagedResourceMode,
					Type: "a",
					Name: "b",
				},
			},
		},
		{
			Input: "data.a.b",
			WantConfigResource: ConfigResource{
				Module: RootModule,
				Resource: Resource{
					Mode: DataResourceMode,
					Type: "a",
					Name: "b",
				},
			},
		},
		{
			Input: "ephemeral.a.b",
			WantConfigResource: ConfigResource{
				Module: RootModule,
				Resource: Resource{
					Mode: EphemeralResourceMode,
					Type: "a",
					Name: "b",
				},
			},
		},
		{
			Input: "module.a.b.c",
			WantConfigResource: ConfigResource{
				Module: []string{"a"},
				Resource: Resource{
					Mode: ManagedResourceMode,
					Type: "b",
					Name: "c",
				},
			},
		},
		{
			Input: "module.a.data.b.c",
			WantConfigResource: ConfigResource{
				Module: []string{"a"},
				Resource: Resource{
					Mode: DataResourceMode,
					Type: "b",
					Name: "c",
				},
			},
		},
		{
			Input: "module.a.ephemeral.b.c",
			WantConfigResource: ConfigResource{
				Module: []string{"a"},
				Resource: Resource{
					Mode: EphemeralResourceMode,
					Type: "b",
					Name: "c",
				},
			},
		},
		{
			Input: "module.a.module.b.c.d",
			WantConfigResource: ConfigResource{
				Module: []string{"a", "b"},
				Resource: Resource{
					Mode: ManagedResourceMode,
					Type: "c",
					Name: "d",
				},
			},
		},
		{
			Input: "module.a.module.b.data.c.d",
			WantConfigResource: ConfigResource{
				Module: []string{"a", "b"},
				Resource: Resource{
					Mode: DataResourceMode,
					Type: "c",
					Name: "d",
				},
			},
		},
		{
			Input: "module.a.module.b.ephemeral.c.d",
			WantConfigResource: ConfigResource{
				Module: []string{"a", "b"},
				Resource: Resource{
					Mode: EphemeralResourceMode,
					Type: "c",
					Name: "d",
				},
			},
		},
		{
			Input:   "module.a.module.b",
			WantErr: "Module address is not allowed: Expected reference to either resource or data block. Provided reference appears to be a module.",
		},
		{
			Input:   "module",
			WantErr: `Invalid address operator: Prefix "module." must be followed by a module name.`,
		},
		{
			Input:   "module.a.module.b.c",
			WantErr: "Invalid address: Resource specification must include a resource type and name.",
		},
		{
			Input:   "module.a.module.b.c.d[0]",
			WantErr: `Resource instance address with keys is not allowed: Resource address cannot be a resource instance (e.g. "null_resource.a[0]"), it must be a resource instead (e.g. "null_resource.a").`,
		},
		{
			Input:   "module.a.module.b.data.c.d[0]",
			WantErr: `Resource instance address with keys is not allowed: Resource address cannot be a resource instance (e.g. "null_resource.a[0]"), it must be a resource instead (e.g. "null_resource.a").`,
		},
		{
			Input:   "module.a.module.b.ephemeral.c.d[0]",
			WantErr: `Resource instance address with keys is not allowed: Resource address cannot be a resource instance (e.g. "null_resource.a[0]"), it must be a resource instead (e.g. "null_resource.a").`,
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

			configRes, diags := ParseConfigResource(traversal)

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
				if diff := cmp.Diff(test.WantConfigResource, configRes); diff != "" {
					t.Fatalf("Mismatched result:\n%s", diff)
				}
			}
		})
	}
}

func TestResourceLess(t *testing.T) {
	tests := []struct {
		left, right Resource
		want        bool
	}{
		{
			Resource{Mode: ManagedResourceMode, Type: "a", Name: "b"},
			Resource{Mode: DataResourceMode, Type: "a", Name: "b"},
			false,
		},
		{
			Resource{Mode: DataResourceMode, Type: "a", Name: "b"},
			Resource{Mode: ManagedResourceMode, Type: "a", Name: "b"},
			true,
		},
		{
			Resource{Mode: ManagedResourceMode, Type: "a", Name: "b"},
			Resource{Mode: ManagedResourceMode, Type: "a", Name: "b"},
			false,
		},
		{
			Resource{Mode: ManagedResourceMode, Type: "a", Name: "b"},
			Resource{Mode: ManagedResourceMode, Type: "a", Name: "c"},
			true,
		},
		{
			Resource{Mode: ManagedResourceMode, Type: "a", Name: "b"},
			Resource{Mode: ManagedResourceMode, Type: "b", Name: "b"},
			true,
		},
		{
			Resource{Mode: EphemeralResourceMode, Type: "a", Name: "b"},
			Resource{Mode: ManagedResourceMode, Type: "a", Name: "b"},
			true,
		},
		{
			Resource{Mode: ManagedResourceMode, Type: "a", Name: "b"},
			Resource{Mode: EphemeralResourceMode, Type: "a", Name: "b"},
			false,
		},
		{
			Resource{Mode: EphemeralResourceMode, Type: "a", Name: "b"},
			Resource{Mode: DataResourceMode, Type: "a", Name: "b"},
			true,
		},
		{
			Resource{Mode: DataResourceMode, Type: "a", Name: "b"},
			Resource{Mode: EphemeralResourceMode, Type: "a", Name: "b"},
			false,
		},
	}

	for _, tt := range tests {
		wantComparison := ">"
		if tt.want {
			wantComparison = "<"
		}
		t.Run(fmt.Sprintf("%s %s %s", tt.left, wantComparison, tt.right), func(t *testing.T) {
			if got, want := tt.left.Less(tt.right), tt.want; got != want {
				t.Fatalf("wrong expectation between %q and %q. want: %t; got: %t", tt.left, tt.right, want, got)
			}
		})
	}
}

func TestResourceSort(t *testing.T) {
	managed := Resource{Mode: ManagedResourceMode, Type: "a", Name: "b"}
	data := Resource{Mode: DataResourceMode, Type: "a", Name: "b"}
	ephemeral := Resource{Mode: EphemeralResourceMode, Type: "a", Name: "b"}

	got := []Resource{managed, data, ephemeral, managed, ephemeral, data}
	sort.SliceStable(got, func(i, j int) bool { return got[i].Less(got[j]) })

	want := []Resource{ephemeral, ephemeral, data, data, managed, managed}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("expected no diff meaning that sorting is not working properly.\ndiff: %s", diff)
	}
}
