// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tracing

import (
	"bytes"
	"errors"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/go-logr/logr"
)

func TestOTelLogSink_ImplementsLogSink(t *testing.T) {
	// Compile-time check: otelLogSink satisfies logr.LogSink
	var sink logr.LogSink = &otelLogSink{}
	sink.Init(logr.RuntimeInfo{})
	_ = logr.New(sink)
}

func TestOTelLogSink_Info_LevelMapping(t *testing.T) {
	tests := []struct {
		name     string
		level    int
		msg      string
		wantPref string // expected prefix like [INFO], [DEBUG], [TRACE]
	}{
		{"verbosity 0 → INFO", 0, "test info", "[INFO]"},
		{"verbosity 1 → DEBUG", 1, "test debug1", "[DEBUG]"},
		{"verbosity 2 → DEBUG", 2, "test debug2", "[DEBUG]"},
		{"verbosity 3 → DEBUG", 3, "test debug3", "[DEBUG]"},
		{"verbosity 4 → TRACE", 4, "test trace", "[TRACE]"},
		{"verbosity 10 → TRACE", 10, "deep trace", "[TRACE]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			saved := log.Writer()
			log.SetOutput(&buf)
			defer log.SetOutput(saved)

			sink := &otelLogSink{}
			sink.Info(tt.level, tt.msg)

			output := buf.String()
			if !strings.Contains(output, tt.wantPref) {
				t.Errorf("Info(%d, %q) output = %q; want prefix %s", tt.level, tt.msg, output, tt.wantPref)
			}
			if !strings.Contains(output, tt.msg) {
				t.Errorf("Info(%d, %q) output = %q; want message %q", tt.level, tt.msg, output, tt.msg)
			}
		})
	}
}

func TestOTelLogSink_Error_WithErr(t *testing.T) {
	var buf bytes.Buffer
	saved := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(saved)

	sink := &otelLogSink{}
	sink.Error(errors.New("connection refused"), "export failed")

	output := buf.String()
	if !strings.Contains(output, "[ERROR]") {
		t.Errorf("Error output = %q; want [ERROR] prefix", output)
	}
	if !strings.Contains(output, "export failed") {
		t.Errorf("Error output = %q; want message 'export failed'", output)
	}
	if !strings.Contains(output, "connection refused") {
		t.Errorf("Error output = %q; want error details 'connection refused'", output)
	}
}

func TestOTelLogSink_Error_NilErr(t *testing.T) {
	var buf bytes.Buffer
	saved := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(saved)

	sink := &otelLogSink{}
	sink.Error(nil, "cleanup")

	output := buf.String()
	if !strings.Contains(output, "[ERROR]") {
		t.Errorf("Error output = %q; want [ERROR] prefix", output)
	}
	if !strings.Contains(output, "cleanup") {
		t.Errorf("Error output = %q; want message 'cleanup'", output)
	}
	// Should NOT contain "<nil>" when error is nil
	if strings.Contains(output, "<nil>") {
		t.Errorf("Error output = %q; should not contain '<nil>' when err is nil", output)
	}
}

func TestOTelLogSink_Enabled(t *testing.T) {
	sink := &otelLogSink{}
	if !sink.Enabled(0) {
		t.Errorf("Enabled(0) = false; want true")
	}
	if !sink.Enabled(100) {
		t.Errorf("Enabled(100) = false; want true")
	}
}

func TestOTelLogSink_WithValues(t *testing.T) {
	sink := &otelLogSink{}
	// WithValues should return the same sink (no-op)
	result := sink.WithValues("key", "value")
	if result != sink {
		t.Errorf("WithValues returned different sink")
	}
}

func TestOTelLogSink_WithName(t *testing.T) {
	sink := &otelLogSink{}
	result := sink.WithName("test-component")
	if result != sink {
		t.Errorf("WithName returned different sink")
	}
}

func TestOTelLogSink_OpenTelemetryInit_EnabledPath(t *testing.T) {
	// This test verifies the startup log line is emitted
	// when OTEL_TRACES_EXPORTER=otlp is set.
	var buf bytes.Buffer
	saved := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(saved)

	// Set env and restore after test
	t.Setenv("OTEL_TRACES_EXPORTER", "otlp")

	_, err := OpenTelemetryInit(t.Context())
	if err != nil {
		t.Fatalf("OpenTelemetryInit() unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "[DEBUG]") {
		t.Errorf("startup log = %q; want [DEBUG] prefix", output)
	}
	if !strings.Contains(output, "OTEL_TRACES_EXPORTER=otlp") {
		t.Errorf("startup log = %q; want OTEL_TRACES_EXPORTER=otlp", output)
	}
}

func TestOTelLogSink_OpenTelemetryInit_DisabledPath(t *testing.T) {
	// When OTEL_TRACES_EXPORTER is not set to "otlp", the function should
	// return silently without emitting any log lines.
	var buf bytes.Buffer
	saved := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(saved)

	// Ensure the env var is NOT set
	if _, ok := os.LookupEnv("OTEL_TRACES_EXPORTER"); ok {
		t.Setenv("OTEL_TRACES_EXPORTER", "")
	}

	_, err := OpenTelemetryInit(t.Context())
	if err != nil {
		t.Fatalf("OpenTelemetryInit() unexpected error: %v", err)
	}

	output := buf.String()
	if output != "" {
		t.Errorf("expected no log output when tracing is disabled, got: %q", output)
	}
}
