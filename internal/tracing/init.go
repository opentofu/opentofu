// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tracing

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	// ---------------------------------------------------------------------
	// DO NOT IMPORT ANY "go.opentelemetry.io/otel/semconv/*" PACKAGES HERE!
	// ---------------------------------------------------------------------
	// Instead, use semconv indirectly through wrappers and reexports in
	// ./traceattrs/semconv.go, because we need to coordinate our chosen
	// semconv version with the OTel SDK's "resource" package.

	// The version number at the end of this package math MUST match the
	// semconv version imported by the "go.opentelemetry.io/otel/sdk/resource",
	// so we will typically need to update this each time we upgrade
	// the module "go.opentelemetry.io/otel/sdk".

	"github.com/opentofu/opentofu/internal/tracing/traceattrs"
)

/*
BEWARE! This is not a committed external interface.

Everything about this is experimental and subject to change in future
releases. Do not depend on anything about the structure of this output.
This mechanism might be removed altogether if a different strategy seems
better based on experience with this experiment.

*/

// OTELExporterEnvVar is the env var that should be used to instruct opentofu which
// exporter to use
// If this environment variable is set to "otlp" when running OpenTofu CLI
// then we'll enable an experimental OTLP trace exporter.
const OTELExporterEnvVar = "OTEL_TRACES_EXPORTER"

// traceParentEnvVar is the env var that should be used to instruct opentofu which
// trace parent to use.
// If this environment variable is set when running OpenTofu CLI
// then we'll extract the traceparent from the environment and add it to the context.
// This ensures that all opentofu traces are linked to the trace that invoked
// this command.
const traceParentEnvVar = "TRACEPARENT"

// traceStateEnvVar is the env var that should be used to instruct opentofu which
// trace state to use.
const traceStateEnvVar = "TRACESTATE"

// ServiceNameEnvVar is the standard OpenTelemetry environment variable for specifying the service name
const ServiceNameEnvVar = "OTEL_SERVICE_NAME"

// DefaultServiceName is the default service name to use if not specified in the environment
const DefaultServiceName = "OpenTofu CLI"

// isTracingEnabled is true if OpenTelemetry is enabled.
var isTracingEnabled bool

// OpenTelemetryInit initializes the optional OpenTelemetry exporter.
//
// By default, we don't export telemetry information at all, since OpenTofu is
// a CLI tool, and so we don't assume we're running in an environment with
// a telemetry collector available.
//
// However, for those running OpenTofu in automation we allow setting
// the standard OpenTelemetry environment variable OTEL_TRACES_EXPORTER=otlp
// to enable an OTLP exporter, which is in turn configured by all the
// standard OTLP exporter environment variables:
//
//	https://opentelemetry.io/docs/specs/otel/protocol/exporter/#configuration-options
//
// We don't currently support any other telemetry export protocols, because
// OTLP has emerged as a de-facto standard and each other exporter we support
// means another relatively-heavy external dependency. OTLP happens to use
// protocol buffers and gRPC, which OpenTofu would depend on for other reasons
// anyway.
//
// Returns the context with trace context extracted from environment variables
// if TRACEPARENT is set.
func OpenTelemetryInit(ctx context.Context) (context.Context, error) {
	isTracingEnabled = false

	// We'll check the environment variable ourselves first, because the
	// "autoexport" helper we're about to use is built under the assumption
	// that exporting should always be enabled and so will expect to find
	// an OTLP server on localhost if no environment variables are set at all.
	if os.Getenv(OTELExporterEnvVar) != "otlp" {
		return ctx, nil // By default, we just discard all telemetry calls
	}

	isTracingEnabled = true

	// Wire OTel SDK internal logging through OpenTofu's global logger
	// before initializing the tracer provider, so any SDK startup
	// diagnostics are captured.
	otel.SetLogger(logr.New(&otelLogSink{}))

	log.Printf("[DEBUG] OpenTelemetry: tracing enabled via %s=otlp", OTELExporterEnvVar)

	// Get service name from environment variable or use default
	serviceName := DefaultServiceName
	if envServiceName := os.Getenv(ServiceNameEnvVar); envServiceName != "" {
		log.Printf("[TRACE] OpenTelemetry: using service name from %s: %s", ServiceNameEnvVar, envServiceName)
		serviceName = envServiceName
	}

	otelResource, err := traceattrs.NewResource(ctx, serviceName)
	if err != nil {
		return ctx, fmt.Errorf("failed to create resource: %w", err)
	}

	// Check if the trace parent/state environment variable is set and extract it into our context
	if traceparent := os.Getenv(traceParentEnvVar); traceparent != "" {
		log.Printf("[TRACE] OpenTelemetry: found trace parent in environment: %s", traceparent)
		// Create a carrier that contains the traceparent from environment variables
		// The key is lowercase because the TraceContext propagator expects lowercase keys
		propCarrier := make(propagation.MapCarrier)
		propCarrier.Set("traceparent", traceparent)

		if tracestate := os.Getenv(traceStateEnvVar); tracestate != "" {
			log.Printf("[TRACE] OpenTelemetry: found trace state in environment: %s", tracestate)
			propCarrier.Set("tracestate", tracestate)
		}

		// Extract the trace context into the context
		tc := propagation.TraceContext{}
		ctx = tc.Extract(ctx, propCarrier)
	}

	exporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return ctx, err
	}

	// Set the global tracer provider, this allows us to use this global TracerProvider
	// to create tracers around the project
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBlocking(),
		),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(otelResource),
	)
	otel.SetTracerProvider(provider)

	// Create a composite propagator that includes both TraceContext and Baggage
	prop := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	otel.SetTextMapPropagator(prop)

	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		log.Printf("[ERROR] OpenTelemetry error: %s", err)
	}))

	return ctx, nil
}

// otelLogSink implements logr.LogSink to route OTel SDK internal logs
// through OpenTofu's standard library log package (redirected to hclog)
// with prefix-based log levels for compatibility with TF_LOG filtering.
type otelLogSink struct {
	name   string
	values []any
}

var _ logr.LogSink = (*otelLogSink)(nil)

func (s *otelLogSink) Init(info logr.RuntimeInfo) {}

func (s *otelLogSink) Enabled(level int) bool {
	return true
}

func (s *otelLogSink) Info(level int, msg string, keysAndValues ...interface{}) {
	var prefix string
	switch {
	case level == 0:
		prefix = "[INFO]"
	case level <= 3:
		prefix = "[DEBUG]"
	default:
		prefix = "[TRACE]"
	}

	var b strings.Builder
	b.WriteString(prefix)
	b.WriteString(" OpenTelemetry:")
	if s.name != "" {
		b.WriteString(" [")
		b.WriteString(s.name)
		b.WriteString("]")
	}
	b.WriteString(" ")
	b.WriteString(msg)

	writeLogValues(&b, s.values)
	writeLogValues(&b, keysAndValues)

	log.Print(b.String())
}

func (s *otelLogSink) Error(err error, msg string, keysAndValues ...interface{}) {
	var b strings.Builder
	b.WriteString("[ERROR] OpenTelemetry:")
	if s.name != "" {
		b.WriteString(" [")
		b.WriteString(s.name)
		b.WriteString("]")
	}
	b.WriteString(" ")
	b.WriteString(msg)
	if err != nil {
		b.WriteString(": ")
		b.WriteString(err.Error())
	}
	writeLogValues(&b, s.values)
	writeLogValues(&b, keysAndValues)

	log.Print(b.String())
}

// writeLogValues formats interleaved key-value pairs into the builder.
// Odd-length slices print the trailing key without a value.
func writeLogValues(b *strings.Builder, kv []any) {
	if len(kv) == 0 {
		return
	}
	b.WriteString(" ")
	for i := 0; i < len(kv); i += 2 {
		if i > 0 {
			b.WriteString(" ")
		}
		fmt.Fprint(b, kv[i])
		if i+1 < len(kv) {
			b.WriteString("=")
			fmt.Fprint(b, kv[i+1])
		}
	}
}

func (s *otelLogSink) WithValues(keysAndValues ...interface{}) logr.LogSink {
	return &otelLogSink{
		name:   s.name,
		values: append(s.values, keysAndValues...),
	}
}

func (s *otelLogSink) WithName(name string) logr.LogSink {
	newName := name
	if s.name != "" {
		newName = s.name + "." + name
	}
	return &otelLogSink{
		name:   newName,
		values: s.values,
	}
}
