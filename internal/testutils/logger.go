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

type testContainersLogger struct {
	t *testing.T
}

func (t testContainersLogger) Printf(format string, v ...interface{}) {
	t.t.Helper()
	t.t.Logf(format, v...)
}

type testHCLogAdapter struct {
	t *testing.T
}

func (t testHCLogAdapter) Accept(name string, level hclog.Level, msg string, args ...interface{}) {
	t.t.Helper()
	msg = fmt.Sprintf(msg, args...)
	t.t.Logf("%s\t%s\t%s", name, level.String(), msg)
}

// SetupTestLogger configures the global HCLogAdapter as well as the default log.Logger.
func SetupTestLogger(t *testing.T) {
	hcLogAdapter := &testHCLogAdapter{t}
	logging.RegisterSinkAdapter(hcLogAdapter)
	t.Cleanup(func() {
		logging.DeregisterSinkAdapter(hcLogAdapter)
	})

	defaultLogger := log.Default()
	originalWriter := defaultLogger.Writer()
	adapter := &goLoggerAdapter{t: t}
	defaultLogger.SetOutput(adapter)
	t.Cleanup(func() {
		_ = adapter.Close()
		defaultLogger.SetOutput(originalWriter)
	})
}

// NewGoTestLogger returns a log.Logger implementation that writes to testing.T.
func NewGoTestLogger(t TestLogAdapter) *log.Logger {
	adapter := &goLoggerAdapter{t: t}
	t.Cleanup(func() {
		_ = adapter.Close()
	})
	return log.New(adapter, "", 0)
}

// TestLogAdapter is a simplified interface to *testing.T.
type TestLogAdapter interface {
	Logf(format string, args ...interface{})
	Cleanup(func())
	Helper()
}

type goLoggerAdapter struct {
	t   TestLogAdapter
	buf []byte
}

func (g *goLoggerAdapter) Write(p []byte) (int, error) {
	g.t.Helper()
	g.buf = append(g.buf, p...)
	i := 0
	for i < len(g.buf) {
		if g.buf[i] == '\n' {
			g.t.Logf("%s", strings.TrimRight(string(g.buf[:i]), "\r"))
			g.buf = g.buf[i+1:]
			i = 0
		} else {
			i++
		}
	}
	return len(p), nil
}

func (g *goLoggerAdapter) Close() error {
	g.t.Helper()
	if len(g.buf) > 0 {
		g.t.Logf("%s", g.buf)
	}
	return nil
}
