// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package collections

import (
	"fmt"
	"testing"
)

func TestProcessSliceCostCap(t *testing.T) {
	// Two slices with nothing in common exceed the search limits and come
	// back with an empty subsequence. The walk then pairs the elements the
	// same way it does when nothing matches, so with shouldDiffElement
	// returning true the overlapping elements are updates, not deletes and
	// creates.
	before := make([]string, 3000)
	after := make([]string, 2500)
	for i := range before {
		before[i] = fmt.Sprintf("before-%04d", i)
	}
	for i := range after {
		after[i] = fmt.Sprintf("after-%04d", i)
	}

	type call struct {
		action   string
		beforeIx int
		afterIx  int
	}
	var calls []call
	ProcessSlice(before, after, func(beforeIx, afterIx int) {
		// Nothing is held in common, so any call with both indices in range
		// is an update pairing.
		action := "update"
		switch {
		case afterIx >= len(after):
			action = "delete"
		case beforeIx >= len(before):
			action = "create"
		}
		calls = append(calls, call{action, beforeIx, afterIx})
	}, func(_, _ string) bool { return true }, func(x, y string) bool { return x == y })

	if len(calls) != len(before) {
		t.Fatalf("expected %d calls, got %d", len(before), len(calls))
	}
	for i, c := range calls {
		wantAction, wantBefore, wantAfter := "update", i, i
		if i >= len(after) {
			wantAction, wantAfter = "delete", len(after)
		}
		if c.action != wantAction || c.beforeIx != wantBefore || c.afterIx != wantAfter {
			t.Fatalf("call %d: got %s(%d, %d), want %s(%d, %d)", i, c.action, c.beforeIx, c.afterIx, wantAction, wantBefore, wantAfter)
		}
	}
}
