// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestSemaphore(t *testing.T) {
	s := NewSemaphore(2)
	timer := time.AfterFunc(time.Second, func() {
		panic("deadlock")
	})
	defer timer.Stop()

	s.Acquire()
	if !s.TryAcquire() {
		t.Fatalf("should acquire")
	}
	if s.TryAcquire() {
		t.Fatalf("should not acquire")
	}
	s.Release()
	s.Release()

	// This release should panic
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("should panic")
		}
	}()
	s.Release()
}

func TestUniqueStrings(t *testing.T) {
	cases := []struct {
		Input    []string
		Expected []string
	}{
		{
			[]string{},
			[]string{},
		},
		{
			[]string{"x"},
			[]string{"x"},
		},
		{
			[]string{"a", "b", "c"},
			[]string{"a", "b", "c"},
		},
		{
			[]string{"a", "a", "a"},
			[]string{"a"},
		},
		{
			[]string{"a", "b", "a", "b", "a", "a"},
			[]string{"a", "b"},
		},
		{
			[]string{"c", "b", "a", "c", "b"},
			[]string{"a", "b", "c"},
		},
	}

	for i, tc := range cases {
		t.Run(fmt.Sprintf("unique-%d", i), func(t *testing.T) {
			actual := uniqueStrings(tc.Input)
			if !reflect.DeepEqual(tc.Expected, actual) {
				t.Fatalf("Expected: %q\nGot: %q", tc.Expected, actual)
			}
		})
	}
}
