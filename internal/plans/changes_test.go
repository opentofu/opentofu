// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plans

import (
	"fmt"
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/zclconf/go-cty/cty"
)

func TestChangesEmpty(t *testing.T) {
	testCases := map[string]struct {
		changes *Changes
		want    bool
	}{
		"no changes": {
			&Changes{},
			true,
		},
		"resource change": {
			&Changes{
				Resources: []*ResourceInstanceChangeSrc{
					{
						Addr: addrs.Resource{
							Mode: addrs.ManagedResourceMode,
							Type: "test_thing",
							Name: "woot",
						}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
						PrevRunAddr: addrs.Resource{
							Mode: addrs.ManagedResourceMode,
							Type: "test_thing",
							Name: "woot",
						}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
						ChangeSrc: ChangeSrc{
							Action: Update,
						},
					},
					// but an ephemeral resources will not impact the "emptiness" of the plan
					{
						Addr: addrs.Resource{
							Mode: addrs.EphemeralResourceMode,
							Type: "test_thing",
							Name: "woot",
						}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
						PrevRunAddr: addrs.Resource{
							Mode: addrs.EphemeralResourceMode,
							Type: "test_thing",
							Name: "woot",
						}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
						ChangeSrc: ChangeSrc{
							Action: Open,
						},
					},
				},
			},
			false,
		},
		"resource change with no-op action": {
			&Changes{
				Resources: []*ResourceInstanceChangeSrc{
					{
						Addr: addrs.Resource{
							Mode: addrs.ManagedResourceMode,
							Type: "test_thing",
							Name: "woot",
						}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
						PrevRunAddr: addrs.Resource{
							Mode: addrs.ManagedResourceMode,
							Type: "test_thing",
							Name: "woot",
						}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
						ChangeSrc: ChangeSrc{
							Action: NoOp,
						},
					},
				},
			},
			true,
		},
		"resource moved with no-op change": {
			&Changes{
				Resources: []*ResourceInstanceChangeSrc{
					{
						Addr: addrs.Resource{
							Mode: addrs.ManagedResourceMode,
							Type: "test_thing",
							Name: "woot",
						}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
						PrevRunAddr: addrs.Resource{
							Mode: addrs.ManagedResourceMode,
							Type: "test_thing",
							Name: "toot",
						}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
						ChangeSrc: ChangeSrc{
							Action: NoOp,
						},
					},
				},
			},
			false,
		},
		"output change": {
			&Changes{
				Outputs: []*OutputChangeSrc{
					{
						Addr: addrs.OutputValue{
							Name: "result",
						}.Absolute(addrs.RootModuleInstance),
						ChangeSrc: ChangeSrc{
							Action: Update,
						},
					},
				},
			},
			false,
		},
		"output change no-op": {
			&Changes{
				Outputs: []*OutputChangeSrc{
					{
						Addr: addrs.OutputValue{
							Name: "result",
						}.Absolute(addrs.RootModuleInstance),
						ChangeSrc: ChangeSrc{
							Action: NoOp,
						},
					},
				},
			},
			true,
		},
		"ephemeral resource change": {
			&Changes{
				Resources: []*ResourceInstanceChangeSrc{
					{
						Addr: addrs.Resource{
							Mode: addrs.EphemeralResourceMode,
							Type: "test_thing",
							Name: "woot",
						}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
						PrevRunAddr: addrs.Resource{
							Mode: addrs.EphemeralResourceMode,
							Type: "test_thing",
							Name: "woot",
						}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
						ChangeSrc: ChangeSrc{
							Action: Open,
						},
					},
				},
			},
			true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			if got, want := tc.changes.Empty(), tc.want; got != want {
				t.Fatalf("unexpected result: got %v, want %v", got, want)
			}
		})
	}
}

func TestChangeEncodeSensitive(t *testing.T) {
	testVals := []cty.Value{
		cty.ObjectVal(map[string]cty.Value{
			"ding": cty.StringVal("dong").Mark(marks.Sensitive),
		}),
		cty.StringVal("bleep").Mark("bloop"),
		cty.ListVal([]cty.Value{cty.UnknownVal(cty.String).Mark("sup?")}),
	}

	for _, v := range testVals {
		t.Run(fmt.Sprintf("%#v", v), func(t *testing.T) {
			change := Change{
				Before: cty.NullVal(v.Type()),
				After:  v,
			}

			encoded, err := change.Encode(v.Type())
			if err != nil {
				t.Fatal(err)
			}

			decoded, err := encoded.Decode(v.Type())
			if err != nil {
				t.Fatal(err)
			}

			if !v.RawEquals(decoded.After) {
				t.Fatalf("%#v != %#v\n", decoded.After, v)
			}
		})
	}
}
