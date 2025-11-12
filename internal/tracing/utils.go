// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tracing

import (
	"context"
	"errors"
	"log"
	"runtime"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

func Tracer() trace.Tracer {
	if !isTracingEnabled {
		return otel.Tracer("")
	}

	pc, _, _, ok := runtime.Caller(1)
	if !ok || runtime.FuncForPC(pc) == nil {
		return otel.Tracer("")
	}

	// We use the import path of the caller function as the tracer name.
	return otel.GetTracerProvider().Tracer(extractImportPath(runtime.FuncForPC(pc).Name()))
}

// SetSpanError sets the error or diagnostic information on the span.
// It accepts an error, a string, or a diagnostics object.
// It also sets the span status to Error and records the error or message.
func SetSpanError(span trace.Span, input any) {
	if span == nil || input == nil {
		return
	}

	switch v := input.(type) {
	case error:
		if v != nil {
			span.SetStatus(codes.Error, v.Error())
			span.RecordError(v)
		}
	case string:
		if v != "" {
			span.SetStatus(codes.Error, v)
			span.RecordError(errors.New(v))
		}
	case tfdiags.Diagnostics: // Assuming Diagnostics is a custom type you have defined elsewhere
		if v.HasErrors() { // Assuming IsEmpty() checks if the diagnostics object has content
			span.SetStatus(codes.Error, v.Err().Error())
			span.RecordError(v.Err())
		}
	default:
		// Handle unsupported types gracefully
		// TODO: Discuss if this should panic?
		span.SetStatus(codes.Error, "ERROR: unsupported input type for SetSpanError.")
		span.AddEvent("ERROR: unsupported input type for SetSpanError")
	}
}

// ForceFlush ensures that all spans are exported to the collector before
// the application terminates. This is particularly important for CLI
// applications where the process exits immediately after the operation.
//
// This should be called before the application terminates to ensure
// all spans are exported properly.
func ForceFlush(timeout time.Duration) {
	if !isTracingEnabled {
		return
	}

	provider, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider)
	if !ok {
		log.Printf("[TRACE] OpenTelemetry: tracer provider is not an SDK provider, can't force flush")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	log.Printf("[TRACE] OpenTelemetry: flushing spans")
	if err := provider.ForceFlush(ctx); err != nil {
		log.Printf("[WARN] OpenTelemetry: error flushing spans: %v", err)
	}
}

// extractImportPath extracts the import path from a full function name.
// the function names returned by runtime.FuncForPC(pc).Name() can be in the following formats
//
//	main.(*MyType).MyMethod
//	github.com/you/pkg.(*SomeType).Method-fm
//	github.com/you/pkg.functionName
func extractImportPath(fullName string) string {
	lastSlash := strings.LastIndex(fullName, "/")
	if lastSlash == -1 {
		// When there is no slash, then use everything before the first dot
		if dot := strings.Index(fullName, "."); dot != -1 {
			return fullName[:dot]
		}
		log.Printf("[WARN] unable to extract import path from function name: %q. Tracing may be incomplete. This is a bug in OpenTofu, please report it.", fullName)
		return "unknown"
	}

	dotAfterSlash := strings.Index(fullName[lastSlash:], ".")
	if dotAfterSlash == -1 {
		log.Printf("[WARN] unable to extract import path from function name: %q. Tracing may be incomplete. This is a bug in OpenTofu, please report it.", fullName)
		return "unknown"
	}

	return fullName[:lastSlash+dotAfterSlash]
}

// Span is an alias for [trace.Span] just to centralize all of our direct
// imports of OpenTelemetry packages into our tracing packages, to help
// avoid dependency hell.
type Span = trace.Span

// SpanFromContext returns the trace span asssociated with the given context,
// or nil if there is no associated span.
//
// This is a wrapper around [trace.SpanFromContext] just to centralize all of
// our imports of OpenTelemetry packages into our tracing packages, to help
// avoid dependency hell.
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// SpanAttributes wraps [trace.WithAttributes] just so that we can minimize
// how many different OpenTofu packages directly import the OpenTelemetry
// packages, because we tend to need to control which versions we're using
// quite closely to avoid dependency hell.
func SpanAttributes(attrs ...attribute.KeyValue) trace.SpanStartEventOption {
	return trace.WithAttributes(attrs...)
}
