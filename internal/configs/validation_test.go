// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
)

func TestComplainRngAndMsg(t *testing.T) {
	tests := map[string]struct {
		countRng   hcl.Range
		enabledRng hcl.Range
		forEachRng hcl.Range
		wantRng    *hcl.Range
		wantMsg    string
	}{
		"count and enabled": {
			countRng: hcl.Range{
				Start: hcl.Pos{Byte: 0},
				End:   hcl.Pos{Byte: 1},
			},
			enabledRng: hcl.Range{
				Start: hcl.Pos{Byte: 2},
				End:   hcl.Pos{Byte: 3},
			},
			// The last item (by Byte) is the one that will be used
			wantRng: &hcl.Range{
				Start: hcl.Pos{Byte: 0},
				End:   hcl.Pos{Byte: 3},
			},
			wantMsg: "\"count\" and \"enabled\"",
		},
		"count and for_each": {
			countRng: hcl.Range{
				Start: hcl.Pos{Byte: 1},
				End:   hcl.Pos{Byte: 2},
			},
			forEachRng: hcl.Range{
				Start: hcl.Pos{Byte: 3},
				End:   hcl.Pos{Byte: 4},
			},
			wantRng: &hcl.Range{
				Start: hcl.Pos{Byte: 1},
				End:   hcl.Pos{Byte: 4},
			},
			wantMsg: "\"count\" and \"for_each\"",
		},
		"enabled and for_each": {
			enabledRng: hcl.Range{
				Start: hcl.Pos{Byte: 4},
				End:   hcl.Pos{Byte: 5},
			},
			forEachRng: hcl.Range{
				Start: hcl.Pos{Byte: 0},
				End:   hcl.Pos{Byte: 1},
			},
			wantRng: &hcl.Range{
				Start: hcl.Pos{Byte: 0},
				End:   hcl.Pos{Byte: 5},
			},
			wantMsg: "\"enabled\" and \"for_each\"",
		},
		"count and enabled and for_each": {
			countRng: hcl.Range{
				Start: hcl.Pos{Byte: 10},
				End:   hcl.Pos{Byte: 11},
			},
			enabledRng: hcl.Range{
				Start: hcl.Pos{Byte: 0},
				End:   hcl.Pos{Byte: 1},
			},
			forEachRng: hcl.Range{
				Start: hcl.Pos{Byte: 2},
				End:   hcl.Pos{Byte: 3},
			},
			wantRng: &hcl.Range{
				Start: hcl.Pos{Byte: 0},
				End:   hcl.Pos{Byte: 11},
			},
			wantMsg: "\"count\", \"enabled\", and \"for_each\"",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			gotRng, gotMsg := complainRngAndMsg(test.countRng, test.enabledRng, test.forEachRng)
			if diff := cmp.Diff(test.wantRng, gotRng); diff != "" {
				t.Errorf("gotRng diff: %s, want: %s", diff, test.wantRng)
			}

			if diff := cmp.Diff(test.wantMsg, gotMsg); diff != "" {
				t.Errorf("gotRng diff: %s, want: %s", diff, test.wantMsg)
			}
		})
	}
}
