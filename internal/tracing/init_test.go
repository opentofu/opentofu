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
)

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

func TestOTelLogSink_Error(t *testing.T) {
	t.Run("with error", func(t *testing.T) {
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
	})
	t.Run("nil error", func(t *testing.T) {
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
	})
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
	result := sink.WithValues("key1", "val1", "key2", "val2")

	if result == sink {
		t.Errorf("WithValues should return a new sink, got same pointer")
	}

	rs := result.(*otelLogSink)
	if len(rs.values) != 4 {
		t.Errorf("WithValues: got %d values, want 4", len(rs.values))
	}
	if rs.values[0] != "key1" || rs.values[1] != "val1" {
		t.Errorf("WithValues: values[0:2] = %v, %v; want key1, val1", rs.values[0], rs.values[1])
	}
	// Original sink should be unchanged
	if len(sink.values) != 0 {
		t.Errorf("WithValues: original sink was mutated, got %d values", len(sink.values))
	}
}

func TestOTelLogSink_WithValues_Chaining(t *testing.T) {
	sink := &otelLogSink{}
	s1 := sink.WithValues("k1", "v1")
	s2 := s1.WithValues("k2", "v2")

	rs2 := s2.(*otelLogSink)
	if len(rs2.values) != 4 {
		t.Errorf("chained WithValues: got %d values, want 4", len(rs2.values))
	}
	if rs2.values[2] != "k2" || rs2.values[3] != "v2" {
		t.Errorf("chained WithValues: expected k2=v2 at end, got %v, %v", rs2.values[2], rs2.values[3])
	}
}

func TestOTelLogSink_WithName(t *testing.T) {
	sink := &otelLogSink{}
	result := sink.WithName("test-component")

	if result == sink {
		t.Errorf("WithName should return a new sink, got same pointer")
	}

	rs := result.(*otelLogSink)
	if rs.name != "test-component" {
		t.Errorf("WithName: got name %q, want %q", rs.name, "test-component")
	}
}

func TestOTelLogSink_WithName_Chaining(t *testing.T) {
	sink := &otelLogSink{}
	s1 := sink.WithName("parent")
	s2 := s1.WithName("child")

	rs2 := s2.(*otelLogSink)
	if rs2.name != "parent.child" {
		t.Errorf("chained WithName: got name %q, want %q", rs2.name, "parent.child")
	}
}

func TestOTelLogSink_WithName_PreservesValues(t *testing.T) {
	sink := &otelLogSink{}
	sv := sink.WithValues("key", "val")
	rs := sv.WithName("component").(*otelLogSink)

	if rs.name != "component" {
		t.Errorf("WithName after WithValues: got name %q, want %q", rs.name, "component")
	}
	if len(rs.values) != 2 {
		t.Errorf("WithName after WithValues: got %d values, want 2", len(rs.values))
	}
	if rs.values[0] != "key" || rs.values[1] != "val" {
		t.Errorf("WithName after WithValues: values = %v, want [key val]", rs.values)
	}
}

func TestOTelLogSink_Info_WithContext(t *testing.T) {
	var buf bytes.Buffer
	saved := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(saved)

	sink := &otelLogSink{}
	sv := sink.WithValues("processor", "batch")
	si := sv.WithName("batch-processor")
	si.Info(0, "exporting", "count", 42)

	output := buf.String()
	if !strings.Contains(output, "processor=batch") {
		t.Errorf("Info with values: output %q, want processor=batch", output)
	}
	if !strings.Contains(output, "count=42") {
		t.Errorf("Info with values: output %q, want count=42", output)
	}
	if !strings.Contains(output, "[batch-processor]") {
		t.Errorf("Info with name: output %q, want [batch-processor]", output)
	}
}

func TestOTelLogSink_Error_WithContext(t *testing.T) {
	var buf bytes.Buffer
	saved := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(saved)

	err := errors.New("timeout")
	sink := &otelLogSink{}
	sv := sink.WithValues("processor", "batch")
	sv.Error(err, "export failed", "retry", 3)

	output := buf.String()
	if !strings.Contains(output, "processor=batch") {
		t.Errorf("Error with values: output %q, want processor=batch", output)
	}
	if !strings.Contains(output, "retry=3") {
		t.Errorf("Error with values: output %q, want retry=3", output)
	}
	if !strings.Contains(output, "timeout") {
		t.Errorf("Error with values: output %q, want timeout", output)
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
