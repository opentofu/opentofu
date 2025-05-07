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

	"github.com/go-logr/stdr"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/opentofu/opentofu/version"
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
		log.Printf("[TRACE] OpenTelemetry: %s not set, OTel tracing is not enabled", OTELExporterEnvVar)
		return ctx, nil // By default, we just discard all telemetry calls
	}

	isTracingEnabled = true

	log.Printf("[TRACE] OpenTelemetry: enabled")

	otelResource, err := resource.New(context.Background(),
		// Use built-in detectors to simplify the collation of the racing information
		resource.WithOS(),
		resource.WithHost(),
		resource.WithProcess(),
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithAttributes(),

		// Add custom service attributes
		resource.WithAttributes(
			semconv.ServiceName("OpenTofu CLI"),
			semconv.ServiceVersion(version.Version),

			// We add in the telemetry SDK information so that we don't end up with
			// duplicate schema urls that clash
			semconv.TelemetrySDKName("opentelemetry"),
			semconv.TelemetrySDKLanguageGo,
			semconv.TelemetrySDKVersion(sdk.Version()),
		),
	)
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
			log.Printf("[TRACE] OpenTelemetry: found trace state in environment: %s", traceparent)
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

	logger := stdr.New(log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile))
	otel.SetLogger(logger)

	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		panic(fmt.Sprintf("OpenTelemetry error: %v", err))
	}))

	return ctx, nil
}
