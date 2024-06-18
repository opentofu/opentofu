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
	"github.com/opentofu/opentofu/internal/logging"
)

// SetupGlobalTestLogger configures the global HCLogAdapter as well as the default log.Logger. Note that this may not work
// correctly for parallel tests because they are globally scoped, consider using NewGoTestLogger or NewHCLogAdapter
// instead.
func SetupGlobalTestLogger(t *testing.T) {
	setupTestLogger(t)
}

// NewHCLogAdapter returns a hclog.SinkAdapter-compatible logger that logs into a test facility.
func NewHCLogAdapter(t *testing.T) hclog.SinkAdapter {
	return newAdapter(t)
}

// NewGoTestLogger returns a log.Logger implementation that writes to testing.T.
func NewGoTestLogger(t *testing.T) *log.Logger {
	return newGoTestLogger(t)
}

func setupTestLogger(t testingT) {
	logAdapter := newAdapter(t)
	logging.RegisterSinkAdapter(logAdapter)
	t.Cleanup(func() {
		logging.DeregisterSinkAdapter(logAdapter)
	})

	defaultLogger := log.Default()
	originalWriter := defaultLogger.Writer()
	defaultLogger.SetOutput(logAdapter)
	t.Cleanup(func() {
		_ = logAdapter.Close()
		defaultLogger.SetOutput(originalWriter)
	})
}

func newGoTestLogger(t testingT) *log.Logger {
	return log.New(newAdapter(t), "", 0)
}

func newAdapter(t testingT) *testLogAdapter {
	adapter := &testLogAdapter{t: t}
	t.Cleanup(func() {
		_ = adapter.Close()
	})
	return adapter
}

// testingT is a simplified interface to *testing.T. This interface is mainly used for internal testing purposes.
type testingT interface {
	Logf(format string, args ...interface{})
	Cleanup(func())
	Helper()
}

type testLogAdapter struct {
	t   testingT
	buf []byte
}

// Accept implements a hclog SinkAdapter.
func (t *testLogAdapter) Accept(name string, level hclog.Level, msg string, args ...interface{}) {
	t.t.Helper()
	msg = fmt.Sprintf(msg, args...)
	t.t.Logf("%s\t%s\t%s", name, level.String(), msg)
}

// Printf implements a standardized way to write logs, e.g. for the testcontainers package.
func (t *testLogAdapter) Printf(format string, v ...interface{}) {
	t.t.Helper()
	t.t.Logf(format, v...)
}

// Write provides a Go log-compatible writer.
func (t *testLogAdapter) Write(p []byte) (int, error) {
	t.t.Helper()
	t.buf = append(t.buf, p...)
	i := 0
	for i < len(t.buf) {
		if t.buf[i] == '\n' {
			t.t.Logf("%s", strings.TrimRight(string(t.buf[:i]), "\r"))
			t.buf = t.buf[i+1:]
			i = 0
		} else {
			i++
		}
	}
	return len(p), nil
}

func (t *testLogAdapter) Close() error {
	t.t.Helper()
	if len(t.buf) > 0 {
		t.t.Logf("%s", t.buf)
	}
	return nil
}
