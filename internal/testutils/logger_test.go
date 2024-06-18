// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils

import (
	"fmt"
	"log"
	"strings"
	"testing"

	"github.com/hashicorp/go-hclog"
)

func TestGoTestLogger(t *testing.T) {
	t2 := &fakeT{}
	const testString = "Hello world!"

	logger := newGoTestLogger(t2)
	logger.Print(testString)

	if len(t2.lines) != 1 {
		t.Fatalf("❌ Expected 1 line, got %d", len(t2.lines))
	}
	t2.RunCleanup()
	if len(t2.lines) != 1 {
		t.Fatalf("❌ Expected 1 line, got %d", len(t2.lines))
	}
	if t2.lines[0] != testString {
		t.Fatalf("❌ Expected 'Hello world!', got '%s'", t2.lines[0])
	}
	t.Logf("✅ Correctly logged text.")
}

func TestGoTestLoggerMultiline(t *testing.T) {
	t2 := &fakeT{}
	const testString1 = "Hello"
	const testString2 = "world!"
	const testString = testString1 + "\n" + testString2
	logger := newGoTestLogger(t2)
	logger.Print(testString)

	if len(t2.lines) != 2 {
		t.Fatalf("❌ Expected 2 lines, got %d", len(t2.lines))
	}
	t2.RunCleanup()
	if len(t2.lines) != 2 {
		t.Fatalf("❌ Expected 2 lines, got %d", len(t2.lines))
	}
	if t2.lines[0] != testString1 {
		t.Fatalf("❌ Expected '%s', got '%s'", testString1, t2.lines[0])
	}
	if t2.lines[1] != testString2 {
		t.Fatalf("❌ Expected '%s', got '%s'", testString2, t2.lines[0])
	}
	t.Logf("✅ Correctly logged multiline text.")
}

func TestHCLogAdapter(t *testing.T) {
	t2 := &fakeT{}
	logger := newAdapter(t2)
	const testString = "Hello world!"

	interceptLogger := hclog.NewInterceptLogger(nil)
	interceptLogger.RegisterSink(logger)
	interceptLogger.Log(hclog.Error, testString)
	for _, line := range t2.lines {
		if strings.Contains(line, testString) {
			t.Logf("✅ Found the test string in the log output.")
			return
		}
	}
	t.Fatalf("❌ Failed to find test string in the log output.")
}

func TestGlobalTestLogger(t *testing.T) {
	t2 := &fakeT{}
	const testString = "Hello world!"
	setupTestLogger(t2)

	// Intentionally write without t.Logf:
	log.Print(testString)

	if len(t2.lines) != 1 {
		t.Fatalf("❌ Expected 1 line, got %d", len(t2.lines))
	}
	if t2.lines[0] != testString {
		t.Fatalf("❌ Expected '%s', got '%s'", testString, t2.lines[0])
	}
	t.Logf("✅ Found the test string in the log output.")
}

type fakeT struct {
	lines        []string
	cleanupFuncs []func()
}

func (f *fakeT) Helper() {
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
