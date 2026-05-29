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

			encoded, err := change.Encode(nil)
			if err != nil {
				t.Fatal(err)
			}

			decoded, err := encoded.Decode(nil)
			if err != nil {
				t.Fatal(err)
			}

			if !v.RawEquals(decoded.After) {
				t.Fatalf("%#v != %#v\n", decoded.After, v)
			}
		})
	}
}

// TestOutputChangeSensitivityRoundTrip verifies that SensitiveBefore and
// SensitiveAfter fields survive a round-trip through Encode() → Decode().
//
// Regression test for https://github.com/opentofu/opentofu/issues/2680
// Before the fix, sensitivity changes would be lost during encoding/decoding.
func TestOutputChangeSensitivityRoundTrip(t *testing.T) {
	tests := []struct {
		name             string
		beforeSensitive bool
		afterSensitive  bool
	}{
		{"no change: both not sensitive", false, false},
		{"add sensitivity", false, true},
		{"remove sensitivity", true, false},
		{"no change: both sensitive", true, true},
	}

	root := addrs.RootModuleInstance
	addr := root.OutputValue("test_output")

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create an OutputChange with the sensitivity values
			oc := &OutputChange{
				Addr:            addr,
				Sensitive:       tc.beforeSensitive || tc.afterSensitive, // legacy field
				SensitiveBefore: tc.beforeSensitive,
				SensitiveAfter:  tc.afterSensitive,
				Change: Change{
					Action: NoOp,
					Before: cty.StringVal("test_value"),
					After:  cty.StringVal("test_value"),
				},
			}

			// Encode to OutputChangeSrc
			encoded, err := oc.Encode()
			if err != nil {
				t.Fatalf("unexpected error during encode: %s", err)
			}

			// Verify the encoded values
			if encoded.SensitiveBefore != tc.beforeSensitive {
				t.Errorf("encoded.SensitiveBefore = %v, want %v", encoded.SensitiveBefore, tc.beforeSensitive)
			}
			if encoded.SensitiveAfter != tc.afterSensitive {
				t.Errorf("encoded.SensitiveAfter = %v, want %v", encoded.SensitiveAfter, tc.afterSensitive)
			}

			// Decode back to OutputChange
			decoded, err := encoded.Decode()
			if err != nil {
				t.Fatalf("unexpected error during decode: %s", err)
			}

			// Verify the decoded values
			if decoded.SensitiveBefore != tc.beforeSensitive {
				t.Errorf("decoded.SensitiveBefore = %v, want %v", decoded.SensitiveBefore, tc.beforeSensitive)
			}
			if decoded.SensitiveAfter != tc.afterSensitive {
				t.Errorf("decoded.SensitiveAfter = %v, want %v", decoded.SensitiveAfter, tc.afterSensitive)
			}

			// Verify the legacy Sensitive field is still populated correctly
			expectedSensitive := tc.beforeSensitive || tc.afterSensitive
			if decoded.Sensitive != expectedSensitive {
				t.Errorf("decoded.Sensitive = %v, want %v", decoded.Sensitive, expectedSensitive)
			}
		})
	}
}
