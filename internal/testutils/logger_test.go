// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils_test

import (
	"fmt"
	"testing"

	"github.com/opentofu/opentofu/internal/testutils"
)

func TestGoTestLogger(t *testing.T) {
	t2 := &fakeT{}
	logger := testutils.NewGoTestLogger(t2)
	logger.Print("Hello world!")

	if len(t2.lines) != 1 {
		t.Fatalf("Expected 1 line, got %d", len(t2.lines))
	}
	t2.RunCleanup()
	if len(t2.lines) != 1 {
		t.Fatalf("Expected 1 line, got %d", len(t2.lines))
	}
	if t2.lines[0] != "Hello world!" {
		t.Fatalf("Expected 'Hello world!', got '%s'", t2.lines[0])
	}
}

func TestGoTestLoggerMultiline(t *testing.T) {
	t2 := &fakeT{}
	logger := testutils.NewGoTestLogger(t2)
	logger.Print("Hello\nworld!")

	if len(t2.lines) != 2 {
		t.Fatalf("Expected 2 lines, got %d", len(t2.lines))
	}
	t2.RunCleanup()
	if len(t2.lines) != 2 {
		t.Fatalf("Expected 2 lines, got %d", len(t2.lines))
	}
	if t2.lines[0] != "Hello" {
		t.Fatalf("Expected 'Hello', got '%s'", t2.lines[0])
	}
	if t2.lines[1] != "world!" {
		t.Fatalf("Expected 'world!', got '%s'", t2.lines[0])
	}
}

type fakeT struct {
	lines        []string
	cleanupFuncs []func()
}

func (f *fakeT) Logf(format string, args ...interface{}) {
	f.lines = append(f.lines, fmt.Sprintf(format, args...))
}

func (f *fakeT) Cleanup(cleanupFunc func()) {
	f.cleanupFuncs = append(f.cleanupFuncs, cleanupFunc)
}

func (f *fakeT) RunCleanup() {
	for _, cleanupFunc := range f.cleanupFuncs {
		cleanupFunc()
	}
}
